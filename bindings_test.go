package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/bot-bbio/legaj/internal/ai/aitest"
	"github.com/bot-bbio/legaj/internal/core"
	"github.com/bot-bbio/legaj/internal/events"
	"github.com/bot-bbio/legaj/internal/secrets"
	"github.com/bot-bbio/legaj/internal/testenv"
)

func TestMain(m *testing.M) { testenv.Main(m) }

const sentinelKey = "AIzaSyD-SENTINEL-KEY-0123456789abcdWXYZ"

type noAutostart struct{}

func (noAutostart) Enable(string) error { return nil }
func (noAutostart) Disable() error      { return nil }

// newTestApp boots a real core in a temp folder, with the API key in an
// in-memory store, and marks the app as started.
func newTestApp(t *testing.T) (*App, *secrets.Memory) {
	t.Helper()
	keys := secrets.NewMemory()
	c, st, err := core.Boot(context.Background(), core.Options{
		Dir: testenv.Isolate(t), Now: time.Now, Emitter: &events.Recorder{},
		Secrets: keys, Autostart: noAutostart{}, AI: &aitest.Fake{}, KeyTester: func(context.Context, string) error { return nil },
		Exe: func() (string, error) { return `C:\LeGaJ\legaj.exe`, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	app := NewApp()
	app.ctx = context.Background()
	// Never open a real Windows dialog from a test: "user cancelled".
	app.pickFile = func(context.Context) (string, error) { return "", nil }
	app.setStartup(c, toStartupView(st))
	close(app.booted)
	return app, keys
}

func TestR8_UploadResumeStoresFileAndReportsParseFailure(t *testing.T) {
	app, _ := newTestApp(t)
	pdf := filepath.Join(t.TempDir(), "My CV.pdf")
	if err := os.WriteFile(pdf, []byte("%PDF-1.7\n%%EOF\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	app.pickFile = func(context.Context) (string, error) { return pdf, nil }

	// The test core's fake AI has no reply queued, so parsing fails.
	res, err := (&OnboardingAPI{app: app}).UploadResume()
	if err != nil {
		t.Fatal(err)
	}
	if res.Cancelled || res.File.ID == 0 || res.File.FileName != "My CV.pdf" {
		t.Fatalf("result = %+v", res)
	}
	if res.Draft != nil || res.ParseError == "" || strings.Contains(res.ParseError, "aitest") {
		t.Fatalf("parse failure must come back as a user-safe message: %+v", res)
	}
}

func TestR7_UploadCancelledIsNotAnError(t *testing.T) {
	app, _ := newTestApp(t)
	res, err := (&OnboardingAPI{app: app}).UploadResume()
	if err != nil || !res.Cancelled {
		t.Fatalf("res = %+v, err = %v", res, err)
	}
}

// keyLikeField matches field names that could carry a secret.
var keyLikeField = map[string]bool{"key": true, "apikey": true, "secret": true, "token": true, "password": true}

// checkType fails if t (recursively) has a string field named like a key.
func checkType(t *testing.T, where string, typ reflect.Type, seen map[reflect.Type]bool) {
	t.Helper()
	for typ.Kind() == reflect.Pointer || typ.Kind() == reflect.Slice || typ.Kind() == reflect.Array || typ.Kind() == reflect.Map {
		typ = typ.Elem()
	}
	if typ.Kind() != reflect.Struct || seen[typ] {
		return
	}
	seen[typ] = true
	for i := range typ.NumField() {
		f := typ.Field(i)
		if f.Type.Kind() == reflect.String && keyLikeField[strings.ToLower(f.Name)] {
			t.Errorf("%s returns %s.%s, a string field that could hold the API key", where, typ.Name(), f.Name)
		}
		checkType(t, where, f.Type, seen)
	}
}

// methodsNotCalled have effects outside the test process.
var methodsNotCalled = map[string]bool{"Quit": true}

func TestR4_NoBindingReturnsFullKey(t *testing.T) {
	app, keys := newTestApp(t)
	calls := 0
	for _, b := range bindings(app) {
		v := reflect.ValueOf(b)
		typ := v.Type()
		for i := range typ.NumMethod() {
			m := typ.Method(i)
			where := typ.Elem().Name() + "." + m.Name
			for o := range m.Type.NumOut() {
				checkType(t, where, m.Type.Out(o), map[reflect.Type]bool{})
			}
			if methodsNotCalled[m.Name] {
				continue
			}
			// Every call starts with the sentinel key saved.
			if err := keys.Set(sentinelKey); err != nil {
				t.Fatal(err)
			}
			args := make([]reflect.Value, m.Type.NumIn()-1)
			for a := range args {
				args[a] = reflect.Zero(m.Type.In(a + 1))
			}
			for _, out := range v.Method(i).Call(args) {
				var text string
				if err, ok := out.Interface().(error); ok && err != nil {
					text = err.Error()
				} else {
					data, err := json.Marshal(out.Interface())
					if err != nil {
						t.Fatalf("%s: result not serialisable: %v", where, err)
					}
					text = string(data)
				}
				if strings.Contains(text, sentinelKey) || strings.Contains(text, "SENTINEL-KEY") {
					t.Errorf("%s returned the API key: %s", where, text)
				}
			}
			calls++
		}
	}
	if calls < 3 {
		t.Fatalf("only %d bound methods exercised; is bindings() complete?", calls)
	}
}

func TestR4_SettingsShowMaskOnly(t *testing.T) {
	app, keys := newTestApp(t)
	if err := keys.Set(sentinelKey); err != nil {
		t.Fatal(err)
	}
	view, err := (&SettingsAPI{app: app}).GetSettings()
	if err != nil {
		t.Fatal(err)
	}
	if !view.Key.Saved || view.Key.Masked != "••••WXYZ" {
		t.Fatalf("key status = %+v", view.Key)
	}
}

func TestR4_ReplaceAndRemoveKey(t *testing.T) {
	app, keys := newTestApp(t)
	api := &SettingsAPI{app: app}
	if err := api.SaveAPIKey("  " + sentinelKey + "  "); err != nil {
		t.Fatal(err)
	}
	if got, _ := keys.Get(); got != sentinelKey {
		t.Fatalf("stored key = %q", got)
	}
	if err := api.RemoveAPIKey(); err != nil {
		t.Fatal(err)
	}
	if view, _ := api.GetSettings(); view.Key.Saved {
		t.Fatal("key still reported after Remove")
	}
}
