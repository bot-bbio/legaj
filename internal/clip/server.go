package clip

import (
	"context"
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"html/template"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bot-bbio/legaj/internal/apperr"
	"github.com/bot-bbio/legaj/internal/clip/extract"
)

// The clip server (R18) serves one popup page and takes one kind of
// request: a save from that popup. The bookmarklet opens the popup on the
// job page and hands it the page snapshot with postMessage (D15, D33); the
// user clicks Save, and the popup posts the snapshot here.
//
// Every request must name this server in its Host header, which defeats
// DNS rebinding (C38). A save must come from the popup's own origin, carry
// the single-use session the popup was served with, fit in 2 MB and stay
// within 60 a minute. Nothing in the bookmarklet is secret.
const (
	// MaxPayload caps a save's body (R18).
	MaxPayload = 2 << 20
	// RateLimit is how many saves are accepted in any 60 seconds (R18).
	RateLimit = 60
	// SessionTTL is how long a popup's session can be used.
	SessionTTL = 15 * time.Minute
	// maxSessions bounds the sessions kept; the oldest goes first.
	maxSessions = 64

	maxJSONLD   = 20
	maxMeta     = 100
	maxHeadings = 5
	maxURL      = 8192
)

//go:embed static
var static embed.FS

var popupPage = template.Must(template.ParseFS(static, "static/clip.html"))

// Saver stores a clipped page (Ingester implements it).
type Saver interface {
	Save(ctx context.Context, p extract.Page) (SaveResult, error)
}

// ServerConfig wires the server.
type ServerConfig struct {
	// Port is the port the server listens on; Host and Origin checks use it.
	Port  int
	Saver Saver
	Log   *slog.Logger
	// Now is the clock; default time.Now.
	Now func() time.Time
	// OnSaved, if set, runs after each save that worked, before the reply.
	OnSaved func(ctx context.Context)
}

// Server is the clip server.
type Server struct {
	cfg     ServerConfig
	hosts   map[string]bool
	origins map[string]bool
	mux     *http.ServeMux
	http    *http.Server

	mu       sync.Mutex
	sessions map[string]time.Time // session → issued at
	saves    []time.Time          // accepted saves in the last minute
}

// NewServer creates the server.
func NewServer(cfg ServerConfig) *Server {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.Log == nil {
		cfg.Log = slog.New(slog.DiscardHandler)
	}
	port := strconv.Itoa(cfg.Port)
	s := &Server{
		cfg:      cfg,
		hosts:    map[string]bool{"127.0.0.1:" + port: true, "localhost:" + port: true},
		origins:  map[string]bool{"http://127.0.0.1:" + port: true, "http://localhost:" + port: true},
		sessions: map[string]time.Time{},
	}
	s.mux = http.NewServeMux()
	s.mux.HandleFunc("GET /clip", s.popup)
	s.mux.HandleFunc("GET /clip.js", s.asset("static/clip.js", "text/javascript; charset=utf-8"))
	s.mux.HandleFunc("GET /clip.css", s.asset("static/clip.css", "text/css; charset=utf-8"))
	s.mux.HandleFunc("POST /api/clips", s.save)
	s.http = &http.Server{
		Handler:           s.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    16 << 10,
	}
	return s
}

// Handler is the full handler chain: host check, headers, routes.
func (s *Server) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.hosts[r.Host] {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Cache-Control", "no-store")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		s.mux.ServeHTTP(w, r)
	})
}

// Serve accepts connections on ln until Shutdown.
func (s *Server) Serve(ln net.Listener) error {
	if err := s.http.Serve(ln); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// Shutdown stops the server, letting saves in flight finish.
func (s *Server) Shutdown(ctx context.Context) error { return s.http.Shutdown(ctx) }

// popup serves the page the bookmarklet opens, with a fresh session. It
// sends no Cross-Origin-Opener-Policy: that would cut the popup's link to
// the job page, which the handoff needs.
func (s *Server) popup(w http.ResponseWriter, _ *http.Request) {
	session, err := s.issue()
	if err != nil {
		s.cfg.Log.Error("clip session", "err", err)
		http.Error(w, "LeGaJ couldn't open the clipper.", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self'; connect-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := popupPage.Execute(w, struct{ Session string }{session}); err != nil {
		s.cfg.Log.Error("clip popup", "err", err)
	}
}

func (s *Server) asset(name, contentType string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		data, err := static.ReadFile(name)
		if err != nil {
			http.NotFound(w, nil)
			return
		}
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
		_, _ = w.Write(data)
	}
}

// issue creates a single-use session for one popup.
func (s *Server) issue() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	id := hex.EncodeToString(b)
	now := s.cfg.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, at := range s.sessions {
		if now.Sub(at) > SessionTTL {
			delete(s.sessions, k)
		}
	}
	for len(s.sessions) >= maxSessions {
		oldest, at := "", now
		for k, t := range s.sessions {
			if !t.After(at) {
				oldest, at = k, t
			}
		}
		delete(s.sessions, oldest)
	}
	s.sessions[id] = now
	return id, nil
}

// take uses up a session; it returns when the session was issued and
// whether it was valid.
func (s *Server) take(id string) (time.Time, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	at, ok := s.sessions[id]
	delete(s.sessions, id)
	return at, ok && s.cfg.Now().Sub(at) <= SessionTTL
}

// giveBack restores a session after a save that failed, so the popup can
// try again. It keeps its issue time: a retry never extends a session.
func (s *Server) giveBack(id string, issued time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[id] = issued
}

// allow counts a save against the rate limit: RateLimit in any minute.
func (s *Server) allow() bool {
	now := s.cfg.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	kept := s.saves[:0]
	for _, t := range s.saves {
		if now.Sub(t) < time.Minute {
			kept = append(kept, t)
		}
	}
	s.saves = kept
	if len(s.saves) >= RateLimit {
		return false
	}
	s.saves = append(s.saves, now)
	return true
}

// payload is the page snapshot the bookmarklet sends (protocol v1). The
// collector's extra fields (method, timings) are ignored.
type payload struct {
	Type      string            `json:"type"`
	V         int               `json:"v"`
	URL       string            `json:"url"`
	Title     string            `json:"title"`
	JSONLD    []string          `json:"jsonld"`
	Meta      map[string]string `json:"meta"`
	H1        []string          `json:"h1"`
	Selection string            `json:"selection"`
	Text      string            `json:"text"`
}

func (p payload) valid() bool {
	return p.Type == "legaj-clip" && p.V == 1 && p.URL != "" && len(p.URL) <= maxURL &&
		len(p.JSONLD) <= maxJSONLD && len(p.Meta) <= maxMeta && len(p.H1) <= maxHeadings
}

type reply struct {
	OK      bool        `json:"ok"`
	Result  *SaveResult `json:"result,omitempty"`
	Message string      `json:"message,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, v reply) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func refuse(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, reply{Message: message})
}

// save takes a clip from the popup. The checks run cheapest first, and
// nothing about the page is logged.
func (s *Server) save(w http.ResponseWriter, r *http.Request) {
	if !s.origins[r.Header.Get("Origin")] {
		refuse(w, http.StatusForbidden, "Forbidden")
		return
	}
	if site := r.Header.Get("Sec-Fetch-Site"); site != "" && site != "same-origin" {
		refuse(w, http.StatusForbidden, "Forbidden")
		return
	}
	if ct := r.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		refuse(w, http.StatusUnsupportedMediaType, "LeGaJ expects JSON.")
		return
	}
	if !s.allow() {
		w.Header().Set("Retry-After", "60")
		refuse(w, http.StatusTooManyRequests, "That's a lot of clips in one minute. Wait a moment and try again.")
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, MaxPayload))
	var tooBig *http.MaxBytesError
	if errors.As(err, &tooBig) {
		refuse(w, http.StatusRequestEntityTooLarge, "This page is too large to clip. Select the job description, then click the bookmarklet again.")
		return
	}
	if err != nil {
		refuse(w, http.StatusBadRequest, "LeGaJ couldn't read the clip.")
		return
	}
	var req struct {
		Session string  `json:"session"`
		Page    payload `json:"page"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		refuse(w, http.StatusBadRequest, "The page sent something LeGaJ can't read.")
		return
	}
	issued, ok := s.take(req.Session)
	if !ok {
		refuse(w, http.StatusForbidden, "This clip window has expired. Click the bookmarklet again.")
		return
	}
	if !req.Page.valid() {
		refuse(w, http.StatusBadRequest, "The page sent something LeGaJ can't read.")
		return
	}
	p := req.Page
	res, err := s.cfg.Saver.Save(r.Context(), extract.Page{
		URL: p.URL, Title: p.Title, JSONLD: p.JSONLD, Meta: p.Meta, H1: p.H1, Selection: p.Selection, Text: p.Text,
	})
	if err != nil {
		s.giveBack(req.Session, issued)
		status := http.StatusInternalServerError
		switch apperr.CodeOf(err) {
		case apperr.CodeInvalid:
			status = http.StatusUnprocessableEntity
		case apperr.CodeConflict:
			status = http.StatusConflict
		}
		if status == http.StatusInternalServerError {
			s.cfg.Log.Error("clip save failed", "err", err)
		}
		refuse(w, status, apperr.UserMessage(err))
		return
	}
	if s.cfg.OnSaved != nil {
		s.cfg.OnSaved(r.Context())
	}
	writeJSON(w, http.StatusOK, reply{OK: true, Result: &res})
}
