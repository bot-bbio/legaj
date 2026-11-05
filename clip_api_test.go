package main

import (
	"context"
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
