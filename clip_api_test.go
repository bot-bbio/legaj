package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/bot-bbio/legaj/internal/ai/aitest"
	"github.com/bot-bbio/legaj/internal/clip"
	"github.com/bot-bbio/legaj/internal/core"
	"github.com/bot-bbio/legaj/internal/events"
	"github.com/bot-bbio/legaj/internal/secrets"
	"github.com/bot-bbio/legaj/internal/testenv"
)

// newClipApp is a test app with its clip server running on loopback.
func newClipApp(t *testing.T) (*App, *core.Services, *events.Recorder) {
	t.Helper()
	emitted := &events.Recorder{}
	c, st, err := core.Boot(context.Background(), core.Options{
		Dir: testenv.Isolate(t), Now: time.Now, Emitter: emitted,
		Secrets: secrets.NewMemory(), Autostart: noAutostart{}, AI: &aitest.Fake{},
		KeyTester:       func(context.Context, string) error { return nil },
		Exe:             func() (string, error) { return `C:\LeGaJ\legaj.exe`, nil },
		Launcher:        &launches{},
		StartClipServer: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	app := NewApp()
	app.ctx = context.Background()
	app.setStartup(c, toStartupView(st))
	close(app.booted)
	svc, err := c.Ready()
	if err != nil {
		t.Fatal(err)
	}
	return app, svc, emitted
}

func TestR18_ClipStatusWithoutServer(t *testing.T) {
	// Test apps run no clip server (core.Options.StartClipServer is off).
	app, _ := newTestApp(t)
	st, err := app.ClipStatus()
	if err != nil {
		t.Fatal(err)
	}
	if st.Running || st.Port != 0 || st.NeedsReinstall {
		t.Fatalf("status %+v", st)
	}
}

// The port moved while LeGaJ was closed, so the status read at startup and
// the stored value agree until the user reinstalls.
func TestR18_ClipStatusReadsTheStoredBookmarklet(t *testing.T) {
	app, svc, _ := newClipApp(t)
	if st, err := app.ClipStatus(); err != nil || st.NeedsReinstall {
		t.Fatalf("fresh: %+v %v", st, err)
	}
	if err := svc.DB.SetMeta(context.Background(), clip.BookmarkletPortKey, strconv.Itoa(clip.PortLow-1)); err != nil {
		t.Fatal(err)
	}
	if st, err := app.ClipStatus(); err != nil || !st.NeedsReinstall {
		t.Fatalf("stale bookmarklet: %+v %v", st, err)
	}
}

var sessionMeta = regexp.MustCompile(`name="legaj-session" content="([0-9a-f]{32})"`)

// clipOnPort saves a page through the real server, as the popup does, and
// returns the status.
func clipOnPort(t *testing.T, port int, page map[string]any) int {
	t.Helper()
	code, _ := clipReply(t, port, page)
	return code
}

func clipReply(t *testing.T, port int, page map[string]any) (int, string) {
	t.Helper()
	base := fmt.Sprintf("http://127.0.0.1:%d", port)
	res, err := http.Get(base + "/clip")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	html, _ := io.ReadAll(res.Body)
	m := sessionMeta.FindSubmatch(html)
	if m == nil {
		t.Fatalf("no session in the popup page: %d", res.StatusCode)
	}
	body, err := json.Marshal(map[string]any{"session": string(m[1]), "page": page})
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, base+"/api/clips", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", base)
	saved, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer saved.Body.Close()
	reply, _ := io.ReadAll(saved.Body)
	return saved.StatusCode, string(reply)
}

func jobPage(url string) map[string]any {
	return map[string]any{
		"type": "legaj-clip", "v": 1, "url": url, "title": "Data Analyst", "meta": map[string]string{}, "h1": []string{"Data Analyst"},
		"jsonld": []string{`{"@type":"JobPosting","title":"Data Analyst","hiringOrganization":"Contoso","description":"Analyse data."}`},
	}
}

func settingsChanges(e *events.Recorder) int {
	n := 0
	for _, ev := range e.Events() {
		if c, ok := ev.Payload.(events.Changed); ok && ev.Name == events.ChangedEvent && c.Entity == events.EntitySettings {
			n++
		}
	}
	return n
}

// A clip that reached this port came from a bookmark holding it. Nothing else
// proves the reinstall: a drag can be dropped anywhere, and a copy never pasted.
func TestR18_ARealClipProvesTheBookmarklet(t *testing.T) {
	app, svc, emitted := newClipApp(t)
	ctx := context.Background()
	if err := svc.DB.SetMeta(ctx, clip.BookmarkletPortKey, strconv.Itoa(clip.PortLow-1)); err != nil {
		t.Fatal(err)
	}
	before := settingsChanges(emitted)

	port := svc.ClipPort.Port
	if code := clipOnPort(t, port, jobPage("https://example.com/jobs/1")); code != http.StatusOK {
		t.Fatalf("clip: %d", code)
	}
	st, err := app.ClipStatus()
	if err != nil || st.NeedsReinstall {
		t.Fatalf("after a real clip: %+v %v", st, err)
	}
	if got, err := clip.InstalledPort(ctx, svc.DB); err != nil || got != port {
		t.Fatalf("stored %d, %v; server on %d", got, err, port)
	}
	if n := settingsChanges(emitted) - before; n != 1 {
		t.Fatalf("%d settings changes after the first clip, want 1", n)
	}

	// Only the change is news: a run of clips doesn't reload every view.
	if code := clipOnPort(t, port, jobPage("https://example.com/jobs/2")); code != http.StatusOK {
		t.Fatalf("clip: %d", code)
	}
	if n := settingsChanges(emitted) - before; n != 1 {
		t.Fatalf("%d settings changes after the second clip, want 1", n)
	}
}

func TestR18_AFailedClipProvesNothing(t *testing.T) {
	app, svc, _ := newClipApp(t)
	if err := svc.DB.SetMeta(context.Background(), clip.BookmarkletPortKey, strconv.Itoa(clip.PortLow-1)); err != nil {
		t.Fatal(err)
	}
	empty := map[string]any{"type": "legaj-clip", "v": 1, "url": "https://example.com/jobs/1", "title": ""}
	if code := clipOnPort(t, svc.ClipPort.Port, empty); code == http.StatusOK {
		t.Fatal("a page with nothing on it was saved")
	}
	if st, err := app.ClipStatus(); err != nil || !st.NeedsReinstall {
		t.Fatalf("%+v %v", st, err)
	}
}

// Core tells the ingester about the key: the popup's promise of a lookup
// depends on it, through the real server.
func TestR18_PopupPromisesALookupOnlyWithAKey(t *testing.T) {
	app, svc, _ := newClipApp(t)
	noCompany := func(url string) map[string]any {
		return map[string]any{"type": "legaj-clip", "v": 1, "url": url, "title": "Data Analyst", "h1": []string{"Data Analyst"}, "text": "We need a data analyst."}
	}
	port := svc.ClipPort.Port
	code, body := clipReply(t, port, noCompany("https://example.com/jobs/1"))
	if code != http.StatusOK || !strings.Contains(body, `"enriching":false`) {
		t.Fatalf("no key: %d %s", code, body)
	}
	c, err := app.coreOnly()
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Secrets.Save(sentinelKey); err != nil {
		t.Fatal(err)
	}
	code, body = clipReply(t, port, noCompany("https://example.com/jobs/2"))
	if code != http.StatusOK || !strings.Contains(body, `"enriching":true`) {
		t.Fatalf("with a key: %d %s", code, body)
	}
}
