package clip_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bot-bbio/legaj/internal/apperr"
	"github.com/bot-bbio/legaj/internal/clip"
	"github.com/bot-bbio/legaj/internal/clip/extract"
)

// fakeSaver records saved pages and answers with a fixed result.
type fakeSaver struct {
	mu     sync.Mutex
	pages  []extract.Page
	result clip.SaveResult
	err    error
}

func (f *fakeSaver) Save(_ context.Context, p extract.Page) (clip.SaveResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pages = append(f.pages, p)
	return f.result, f.err
}

func (f *fakeSaver) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.pages)
}

type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *clock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

type server struct {
	base   string // http://127.0.0.1:<port>
	origin string
	host   string
	saver  *fakeSaver
	clock  *clock
}

// startServer runs the real handler on a real loopback listener, as the
// clip server will (R18, R20: through the real HTTP chain).
func startServer(t *testing.T, saver clip.Saver) *server {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	c := &clock{t: time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)}
	s := clip.NewServer(clip.ServerConfig{Port: port, Saver: saver, Now: c.now})
	hs := httptest.NewUnstartedServer(s.Handler())
	hs.Listener.Close()
	hs.Listener = ln
	hs.Start()
	t.Cleanup(hs.Close)
	host := fmt.Sprintf("127.0.0.1:%d", port)
	srv := &server{base: "http://" + host, origin: "http://" + host, host: host, clock: c}
	if f, ok := saver.(*fakeSaver); ok {
		srv.saver = f
	}
	return srv
}

func (s *server) do(t *testing.T, req *http.Request) (*http.Response, string) {
	t.Helper()
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	return res, string(body)
}

var sessionMeta = regexp.MustCompile(`<meta name="legaj-session" content="([0-9a-f]{32})">`)

// session opens the popup page and returns its single-use session code.
func (s *server) session(t *testing.T) string {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, s.base+"/clip", nil)
	res, body := s.do(t, req)
	m := sessionMeta.FindStringSubmatch(body)
	if res.StatusCode != http.StatusOK || m == nil {
		t.Fatalf("popup: %d, session %v", res.StatusCode, m)
	}
	return m[1]
}

func validPage(url string) map[string]any {
	return map[string]any{
		"type": "legaj-clip", "v": 1, "method": "pm", "url": url, "title": "Data Analyst | Contoso Careers",
		"jsonld": []string{}, "meta": map[string]string{}, "h1": []string{"Data Analyst"},
		"selection": "", "text": "Contoso is hiring a Data Analyst.", "collectMs": 3,
	}
}

// post sends a save from the popup: same origin, JSON, with the session.
func (s *server) post(t *testing.T, session string, page any, edit func(*http.Request)) (*http.Response, string) {
	t.Helper()
	b, err := json.Marshal(map[string]any{"session": session, "page": page})
	if err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest(http.MethodPost, s.base+"/api/clips", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", s.origin)
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	if edit != nil {
		edit(req)
	}
	return s.do(t, req)
}

func TestR18_RejectsForeignHost(t *testing.T) {
	// DNS rebinding (C38): an attacker's domain pointing at 127.0.0.1
	// reaches the server with its own name in the Host header.
	s := startServer(t, &fakeSaver{})
	routes := []struct{ method, path string }{
		{http.MethodGet, "/clip"}, {http.MethodGet, "/clip.js"}, {http.MethodPost, "/api/clips"}, {http.MethodGet, "/nope"},
	}
	for _, host := range []string{"evil.example", "evil.example:" + strings.Split(s.host, ":")[1], "127.0.0.1:1", "localhost"} {
		for _, r := range routes {
			req, _ := http.NewRequest(r.method, s.base+r.path, strings.NewReader("{}"))
			req.Host = host
			if res, _ := s.do(t, req); res.StatusCode != http.StatusForbidden {
				t.Errorf("Host %q %s %s: %d, want 403", host, r.method, r.path, res.StatusCode)
			}
		}
	}
	if s.saver.count() != 0 {
		t.Fatal("a foreign-host request reached the saver")
	}
	// localhost with the right port is the server itself.
	req, _ := http.NewRequest(http.MethodGet, s.base+"/clip", nil)
	req.Host = "localhost:" + strings.Split(s.host, ":")[1]
	if res, _ := s.do(t, req); res.StatusCode != http.StatusOK {
		t.Fatalf("localhost: %d", res.StatusCode)
	}
}

func TestR18_PopupHeadersFrameAncestorsNone(t *testing.T) {
	s := startServer(t, &fakeSaver{})
	req, _ := http.NewRequest(http.MethodGet, s.base+"/clip", nil)
	res, body := s.do(t, req)
	csp := res.Header.Get("Content-Security-Policy")
	for _, want := range []string{"frame-ancestors 'none'", "default-src 'none'", "script-src 'self'", "connect-src 'self'"} {
		if !strings.Contains(csp, want) {
			t.Errorf("CSP %q lacks %q", csp, want)
		}
	}
	if strings.Contains(csp, "unsafe-inline") {
		t.Errorf("CSP %q allows inline code", csp)
	}
	for h, want := range map[string]string{"X-Frame-Options": "DENY", "X-Content-Type-Options": "nosniff", "Cache-Control": "no-store", "Referrer-Policy": "no-referrer"} {
		if got := res.Header.Get(h); got != want {
			t.Errorf("%s = %q, want %q", h, got, want)
		}
	}
	// COOP on the popup would cut its own link to the job page, which the
	// postMessage handoff needs (D15, D33).
	if coop := res.Header.Get("Cross-Origin-Opener-Policy"); coop != "" && coop != "unsafe-none" {
		t.Errorf("Cross-Origin-Opener-Policy %q breaks the handoff", coop)
	}
	if !strings.Contains(body, `<script src="/clip.js"></script>`) || strings.Contains(body, "<script>") {
		t.Error("the popup must load its script from /clip.js, with no inline script")
	}
	// Each popup gets its own session.
	if a, b := s.session(t), s.session(t); a == b {
		t.Fatal("two popups got the same session")
	}
}

func TestR18_SaveRejectsForeignOrigin(t *testing.T) {
	s := startServer(t, &fakeSaver{})
	cases := map[string]func(*http.Request){
		"another site":     func(r *http.Request) { r.Header.Set("Origin", "https://evil.example") },
		"no origin":        func(r *http.Request) { r.Header.Del("Origin") },
		"null origin":      func(r *http.Request) { r.Header.Set("Origin", "null") },
		"other port":       func(r *http.Request) { r.Header.Set("Origin", "http://127.0.0.1:1") },
		"cross-site fetch": func(r *http.Request) { r.Header.Set("Sec-Fetch-Site", "cross-site") },
	}
	for name, edit := range cases {
		t.Run(name, func(t *testing.T) {
			if res, _ := s.post(t, s.session(t), validPage("https://careers.example.com/1"), edit); res.StatusCode != http.StatusForbidden {
				t.Fatalf("%d, want 403", res.StatusCode)
			}
		})
	}
	if s.saver.count() != 0 {
		t.Fatal("a foreign-origin save reached the saver")
	}
}

func TestR18_SaveRequiresPopupSession(t *testing.T) {
	s := startServer(t, &fakeSaver{})
	page := validPage("https://careers.example.com/1")
	for name, session := range map[string]string{"none": "", "made up": strings.Repeat("ab", 16)} {
		if res, _ := s.post(t, session, page, nil); res.StatusCode != http.StatusForbidden {
			t.Errorf("%s session: %d, want 403", name, res.StatusCode)
		}
	}
	// A session works once.
	sess := s.session(t)
	if res, body := s.post(t, sess, page, nil); res.StatusCode != http.StatusOK {
		t.Fatalf("first use: %d %s", res.StatusCode, body)
	}
	if res, _ := s.post(t, sess, page, nil); res.StatusCode != http.StatusForbidden {
		t.Fatalf("reused session: %d, want 403", res.StatusCode)
	}
	// And expires.
	old := s.session(t)
	s.clock.advance(clip.SessionTTL + time.Second)
	if res, _ := s.post(t, old, page, nil); res.StatusCode != http.StatusForbidden {
		t.Fatalf("expired session: %d, want 403", res.StatusCode)
	}
	if s.saver.count() != 1 {
		t.Fatalf("%d saves, want 1", s.saver.count())
	}
}

func TestR18_PayloadOver2MBRejected(t *testing.T) {
	s := startServer(t, &fakeSaver{})
	page := validPage("https://careers.example.com/1")
	page["text"] = strings.Repeat("x", clip.MaxPayload)
	if res, _ := s.post(t, s.session(t), page, nil); res.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("%d, want 413", res.StatusCode)
	}
	// Just under the cap is fine.
	page["text"] = strings.Repeat("x", clip.MaxPayload-1024)
	if res, body := s.post(t, s.session(t), page, nil); res.StatusCode != http.StatusOK {
		t.Fatalf("under the cap: %d %s", res.StatusCode, body)
	}
	if s.saver.count() != 1 {
		t.Fatalf("%d saves, want 1", s.saver.count())
	}
}

func TestR18_RateLimit60PerMinute(t *testing.T) {
	s := startServer(t, &fakeSaver{})
	save := func() int {
		res, _ := s.post(t, s.session(t), validPage("https://careers.example.com/1"), nil)
		return res.StatusCode
	}
	// 30 clips in 60 seconds all pass.
	for i := 0; i < 30; i++ {
		if code := save(); code != http.StatusOK {
			t.Fatalf("clip %d: %d", i+1, code)
		}
		s.clock.advance(2 * time.Second)
	}
	// Then a burst: 60 in a minute pass, the 61st is refused.
	s.clock.advance(time.Minute)
	for i := 0; i < clip.RateLimit; i++ {
		if code := save(); code != http.StatusOK {
			t.Fatalf("burst clip %d: %d", i+1, code)
		}
	}
	if code := save(); code != http.StatusTooManyRequests {
		t.Fatalf("61st clip: %d, want 429", code)
	}
	// A minute later clips pass again.
	s.clock.advance(time.Minute + time.Second)
	if code := save(); code != http.StatusOK {
		t.Fatalf("after the window: %d", code)
	}
}

func TestR18_InvalidPayloadRejected(t *testing.T) {
	s := startServer(t, &fakeSaver{})
	bad := map[string]map[string]any{
		"wrong type":    {"type": "other"},
		"wrong version": {"v": 2},
		"no url":        {"url": ""},
		"url as number": {"url": 7},
		"too many h1":   {"h1": []string{"a", "b", "c", "d", "e", "f"}},
	}
	for name, change := range bad {
		t.Run(name, func(t *testing.T) {
			page := validPage("https://careers.example.com/1")
			for k, v := range change {
				page[k] = v
			}
			if res, _ := s.post(t, s.session(t), page, nil); res.StatusCode != http.StatusBadRequest {
				t.Fatalf("%d, want 400", res.StatusCode)
			}
		})
	}
	if res, _ := s.post(t, s.session(t), validPage("https://careers.example.com/1"), func(r *http.Request) { r.Header.Set("Content-Type", "text/plain") }); res.StatusCode != http.StatusUnsupportedMediaType {
		t.Fatalf("text/plain: %d, want 415", res.StatusCode)
	}
	if s.saver.count() != 0 {
		t.Fatal("an invalid payload reached the saver")
	}
}

func TestR18_SaveReturnsResult(t *testing.T) {
	f := &fakeSaver{result: clip.SaveResult{Created: true, ID: 7, Company: "Contoso", Role: "Data Analyst"}}
	s := startServer(t, f)
	res, body := s.post(t, s.session(t), validPage("https://careers.example.com/1"), nil)
	if res.StatusCode != http.StatusOK || res.Header.Get("Content-Type") != "application/json" {
		t.Fatalf("%d %s", res.StatusCode, res.Header.Get("Content-Type"))
	}
	var out struct {
		OK     bool            `json:"ok"`
		Result clip.SaveResult `json:"result"`
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil || !out.OK || out.Result.ID != 7 || out.Result.Company != "Contoso" {
		t.Fatalf("body %s: %v", body, err)
	}
	if got := f.pages[0]; got.URL != "https://careers.example.com/1" || got.H1[0] != "Data Analyst" || got.Text == "" {
		t.Fatalf("saver got %+v", got)
	}

	// Errors carry the user-safe message only.
	f.err = apperr.Wrap(fmt.Errorf("disk on fire at C:\\secret"), apperr.CodeStorage, "LeGaJ couldn't save the clip.")
	res, body = s.post(t, s.session(t), validPage("https://careers.example.com/2"), nil)
	if res.StatusCode != http.StatusInternalServerError || !strings.Contains(body, "LeGaJ couldn't save the clip.") || strings.Contains(body, "secret") {
		t.Fatalf("storage error: %d %s", res.StatusCode, body)
	}
	f.err = apperr.New(apperr.CodeInvalid, "LeGaJ couldn't find a job on this page.")
	if res, _ := s.post(t, s.session(t), validPage("https://careers.example.com/3"), nil); res.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("invalid page: %d, want 422", res.StatusCode)
	}
}

func TestR18_PopupScriptTreatsPageAsText(t *testing.T) {
	// A static guard on the popup's script: the page snapshot is hostile, so
	// it's only ever shown with textContent, and only the window that opened
	// the popup may send it.
	s := startServer(t, &fakeSaver{})
	req, _ := http.NewRequest(http.MethodGet, s.base+"/clip.js", nil)
	res, js := s.do(t, req)
	if res.StatusCode != http.StatusOK || !strings.HasPrefix(res.Header.Get("Content-Type"), "text/javascript") {
		t.Fatalf("clip.js: %d %s", res.StatusCode, res.Header.Get("Content-Type"))
	}
	for _, banned := range []string{"innerHTML", "outerHTML", "insertAdjacentHTML", "document.write", "eval(", "new Function"} {
		if strings.Contains(js, banned) {
			t.Errorf("clip.js uses %s", banned)
		}
	}
	for _, needed := range []string{"e.source !== window.opener", ".textContent =", "'/api/clips'"} {
		if !strings.Contains(js, needed) {
			t.Errorf("clip.js lacks %q", needed)
		}
	}
}

func TestR18_OnlyKnownRoutes(t *testing.T) {
	s := startServer(t, &fakeSaver{})
	for _, r := range []struct {
		method, path string
		want         int
	}{
		{http.MethodGet, "/", http.StatusNotFound},
		{http.MethodGet, "/api/clips", http.StatusMethodNotAllowed},
		{http.MethodPost, "/clip", http.StatusMethodNotAllowed},
		{http.MethodGet, "/clip.js", http.StatusOK},
		{http.MethodGet, "/clip.css", http.StatusOK},
		{http.MethodGet, "/../etc/passwd", http.StatusNotFound},
	} {
		req, _ := http.NewRequest(r.method, s.base+r.path, nil)
		if res, _ := s.do(t, req); res.StatusCode != r.want {
			t.Errorf("%s %s: %d, want %d", r.method, r.path, res.StatusCode, r.want)
		}
	}
}
