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

func TestR18_InstallWithoutServerIsRefused(t *testing.T) {
	app, _ := newTestApp(t)
	if err := app.MarkBookmarkletInstalled(); err == nil {
		t.Fatal("recorded a bookmarklet for a server that isn't running")
	}
	_, svc, err := app.services()
	if err != nil {
		t.Fatal(err)
	}
	if port, err := clip.InstalledPort(context.Background(), svc.DB); err != nil || port != 0 {
		t.Fatalf("stored %d, %v", port, err)
	}
}

// The port moved and the user took the new bookmarklet: the banner clears
// at once, without a restart, and every view hears about it (R3).
func TestR18_InstallClearsReinstallAtOnce(t *testing.T) {
	app, svc, emitted := newClipApp(t)
	ctx := context.Background()
	if err := svc.DB.SetMeta(ctx, clip.BookmarkletPortKey, strconv.Itoa(clip.PortLow-1)); err != nil {
		t.Fatal(err)
	}
	st, err := app.ClipStatus()
	if err != nil {
		t.Fatal(err)
	}
	if !st.Running || !st.NeedsReinstall {
		t.Fatalf("before reinstalling: %+v", st)
	}
	before := len(emitted.Events())

	if err := app.MarkBookmarkletInstalled(); err != nil {
		t.Fatal(err)
	}
	st, err = app.ClipStatus()
	if err != nil {
		t.Fatal(err)
	}
	if st.NeedsReinstall || st.Port != svc.ClipPort.Port {
		t.Fatalf("after reinstalling: %+v", st)
	}
	if port, err := clip.InstalledPort(ctx, svc.DB); err != nil || port != st.Port {
		t.Fatalf("stored %d, %v; server on %d", port, err, st.Port)
	}
	var settings int
	for _, e := range emitted.Events()[before:] {
		if c, ok := e.Payload.(events.Changed); ok && e.Name == events.ChangedEvent && c.Entity == events.EntitySettings {
			settings++
		}
	}
	if settings != 1 {
		t.Fatalf("%d settings changes emitted, want 1", settings)
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
	return saved.StatusCode
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

// A clip that reached this port came from a bookmark holding it, which proves
// more than a drag can: it clears the banner even if the browser never said
// the drop landed.
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
