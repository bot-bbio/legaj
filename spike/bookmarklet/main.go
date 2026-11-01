// Command bookmarklet is a THROWAWAY spike for plan item 2 (decision D15).
//
// It answers one question before the real clipper is built: can a
// bookmarklet on a job site hand a page snapshot to a LeGaJ-served popup
// window, despite the site's CSP, COOP and Chrome's Local Network Access
// rules? It serves:
//
//	/            install page: two test bookmarklets + live results table
//	/clip        the popup page (receives the payload, shows Save)
//	/api/result  the popup reports how the handoff went (no page content)
//	/api/save    a user-confirmed Save writes the payload to -out/captures
//
// Run from the repo root:  go run ./spike/bookmarklet
// Delete spike/ before feat/v2 is merged.
package main

import (
	"embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

//go:embed static
var staticFS embed.FS

// maxBody is deliberately above the planned 2 MB cap (R18) so the spike can
// measure how big real pages are.
const maxBody = 8 << 20

type server struct {
	port    int
	outDir  string
	mu      sync.Mutex
	install *template.Template
	snippet string // bookmarklet source with the port filled in
}

func main() {
	port := flag.Int("port", 47321, "loopback port to listen on")
	out := flag.String("out", filepath.Join("spike", "bookmarklet", "out"), "directory for results.jsonl and captures/")
	flag.Parse()

	if err := os.MkdirAll(filepath.Join(*out, "captures"), 0o700); err != nil {
		log.Fatal(err)
	}
	src, err := staticFS.ReadFile("static/bookmarklet.js")
	if err != nil {
		log.Fatal(err)
	}
	tmpl, err := template.ParseFS(staticFS, "static/install.html")
	if err != nil {
		log.Fatal(err)
	}
	s := &server{
		port:    *port,
		outDir:  *out,
		install: tmpl,
		snippet: strings.ReplaceAll(string(src), "__PORT__", fmt.Sprint(*port)),
	}

	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		log.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.handleInstall)
	mux.HandleFunc("GET /clip", s.handleClip)
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(sub)))
	mux.HandleFunc("POST /api/result", s.handleResult)
	mux.HandleFunc("POST /api/save", s.handleSave)
	mux.HandleFunc("GET /api/results", s.handleResults)

	addr := fmt.Sprintf("127.0.0.1:%d", *port)
	srv := &http.Server{
		Addr:              addr,
		Handler:           s.hostCheck(mux),
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Printf("spike: open http://%s/ in Edge and Chrome; results go to %s", addr, *out)
	log.Fatal(srv.ListenAndServe())
}

// hostCheck rejects any request whose Host isn't our loopback address
// (the DNS-rebinding defence the real server will use, R18).
func (s *server) hostCheck(next http.Handler) http.Handler {
	allowed := map[string]bool{
		fmt.Sprintf("127.0.0.1:%d", s.port): true,
		fmt.Sprintf("localhost:%d", s.port): true,
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !allowed[r.Host] {
			http.Error(w, "forbidden host", http.StatusForbidden)
			return
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}

// sameOrigin reports whether a POST came from our own pages.
func (s *server) sameOrigin(r *http.Request) bool {
	o := r.Header.Get("Origin")
	return o == fmt.Sprintf("http://127.0.0.1:%d", s.port) || o == fmt.Sprintf("http://localhost:%d", s.port)
}

func (s *server) handleInstall(w http.ResponseWriter, _ *http.Request) {
	link := func(method string) template.URL {
		code := strings.ReplaceAll(s.snippet, "__METHOD__", method)
		// Percent-encode so '#', '%' and spaces survive as a javascript: URL.
		// template.URL is safe here: code is our own embedded constant.
		return template.URL("javascript:" + url.PathEscape(code))
	}
	w.Header().Set("Content-Security-Policy", "default-src 'self'; frame-ancestors 'none'")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	data := struct{ PM, Frag template.URL }{PM: link("pm"), Frag: link("frag")}
	if err := s.install.Execute(w, data); err != nil {
		log.Printf("install template: %v", err)
	}
}

func (s *server) handleClip(w http.ResponseWriter, _ *http.Request) {
	page, err := staticFS.ReadFile("static/clip.html")
	if err != nil {
		http.Error(w, "missing page", http.StatusInternalServerError)
		return
	}
	// The popup must never be framed by a job site (R18).
	w.Header().Set("Content-Security-Policy", "default-src 'self'; frame-ancestors 'none'")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(page)
}

// result is what the popup reports. It carries sizes and flags only, never
// page content, so results.jsonl can be shared safely.
type result struct {
	Time         time.Time `json:"time"`
	Method       string    `json:"method"`
	Channel      string    `json:"channel,omitempty"`
	Status       string    `json:"status"`
	Ms           int       `json:"ms"`
	UA           string    `json:"ua"`
	Host         string    `json:"host,omitempty"`
	OpenerOrigin string    `json:"openerOrigin,omitempty"`
	SizeBytes    int       `json:"sizeBytes,omitempty"`
	FragChars    int       `json:"fragChars,omitempty"`
	JSONLDCount  int       `json:"jsonldCount"`
	JobPosting   bool      `json:"jobPosting"`
	TextChars    int       `json:"textChars,omitempty"`
	HasSelection bool      `json:"hasSelection"`
	CollectMs    int       `json:"collectMs,omitempty"`
}

func (s *server) handleResult(w http.ResponseWriter, r *http.Request) {
	if !s.sameOrigin(r) {
		http.Error(w, "forbidden origin", http.StatusForbidden)
		return
	}
	var res result
	if err := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&res); err != nil {
		http.Error(w, "bad result", http.StatusBadRequest)
		return
	}
	res.Time = time.Now().UTC()
	line, err := json.Marshal(res)
	if err != nil {
		http.Error(w, "encode", http.StatusInternalServerError)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := os.OpenFile(filepath.Join(s.outDir, "results.jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		http.Error(w, "write", http.StatusInternalServerError)
		return
	}
	defer f.Close()
	if _, err := f.Write(append(line, '\n')); err != nil {
		http.Error(w, "write", http.StatusInternalServerError)
		return
	}
	log.Printf("result: %s %s %s %s %dB", res.Method, res.Status, res.Host, browser(res.UA), res.SizeBytes)
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) handleResults(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	data, err := os.ReadFile(filepath.Join(s.outDir, "results.jsonl"))
	s.mu.Unlock()
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		http.Error(w, "read", http.StatusInternalServerError)
		return
	}
	var all []result
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var res result
		if line != "" && json.Unmarshal([]byte(line), &res) == nil {
			all = append(all, res)
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(all)
}

var unsafeName = regexp.MustCompile(`[^a-zA-Z0-9.-]+`)

// handleSave stores a user-confirmed capture as a golden-fixture candidate
// for plan item 23. Captures can hold personal data (e.g. a logged-in
// LinkedIn header), so they stay in the gitignored out/ folder and must be
// scrubbed before becoming fixtures.
func (s *server) handleSave(w http.ResponseWriter, r *http.Request) {
	if !s.sameOrigin(r) {
		http.Error(w, "forbidden origin", http.StatusForbidden)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBody+1))
	if err != nil || len(body) > maxBody {
		http.Error(w, "payload too large", http.StatusRequestEntityTooLarge)
		return
	}
	var probe struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(body, &probe); err != nil {
		http.Error(w, "bad payload", http.StatusBadRequest)
		return
	}
	host := "unknown"
	if u, err := url.Parse(probe.URL); err == nil && u.Host != "" {
		host = unsafeName.ReplaceAllString(u.Host, "_")
	}
	name := fmt.Sprintf("%s-%d.json", host, time.Now().Unix())
	if err := os.WriteFile(filepath.Join(s.outDir, "captures", name), body, 0o600); err != nil {
		http.Error(w, "write", http.StatusInternalServerError)
		return
	}
	log.Printf("saved capture %s (%d bytes)", name, len(body))
	w.WriteHeader(http.StatusNoContent)
}

func browser(ua string) string {
	switch {
	case strings.Contains(ua, "Edg/"):
		return "Edge"
	case strings.Contains(ua, "Chrome/"):
		return "Chrome"
	default:
		return "other"
	}
}
