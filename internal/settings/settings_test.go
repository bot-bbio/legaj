package settings_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/bot-bbio/legaj/internal/apperr"
	"github.com/bot-bbio/legaj/internal/events"
	"github.com/bot-bbio/legaj/internal/settings"
	"github.com/bot-bbio/legaj/internal/store"
	"github.com/bot-bbio/legaj/internal/testenv"
)

func TestMain(m *testing.M) { testenv.Main(m) }

var ctx = context.Background()

// fakeAutostart records registry calls instead of touching HKCU.
type fakeAutostart struct {
	enabled  bool
	command  string
	failNext error
	calls    []string
}

func (f *fakeAutostart) Enable(command string) error {
	f.calls = append(f.calls, "enable")
	if err := f.failNext; err != nil {
		f.failNext = nil
		return err
	}
	f.enabled, f.command = true, command
	return nil
}

func (f *fakeAutostart) Disable() error {
	f.calls = append(f.calls, "disable")
	f.enabled, f.command = false, ""
	return nil
}

func (f *fakeAutostart) Current() (string, bool, error) {
	return f.command, f.enabled, nil
}

type fixture struct {
	svc  *settings.Service
	rec  *events.Recorder
	auto *fakeAutostart
	dir  string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	dir := testenv.Isolate(t)
	db, st, err := store.Open(ctx, store.Options{Dir: dir})
	if err != nil || st.Kind != store.StartupReady {
		t.Fatalf("open: %v %+v", err, st)
	}
	t.Cleanup(func() { _ = db.Close() })
	f := &fixture{rec: &events.Recorder{}, auto: &fakeAutostart{}, dir: dir}
	f.svc = settings.New(db, events.NewBus(f.rec), f.auto, func() (string, error) { return `C:\Users\Łukasz Ż\AppData\Local\Programs\LeGaJ\legaj.exe`, nil })
	return f
}

func TestR37_DefaultsBeforeAnythingIsSaved(t *testing.T) {
	f := newFixture(t)
	got, err := f.svc.Get(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := settings.Defaults()
	if got != want {
		t.Fatalf("Get = %+v, want defaults %+v", got, want)
	}
	if want.StartWithWindows {
		t.Fatal("Start with Windows must be off by default (R37)")
	}
	if want.Model != "gemini-3.1-flash-lite" || want.FollowUpDays != 7 || want.SkipReview {
		t.Fatalf("unexpected defaults: %+v", want)
	}
}

func TestR37_SettingsPersistAndEmit(t *testing.T) {
	f := newFixture(t)
	in := settings.Settings{
		Model:            "gemini-2.5-pro",
		SkipReview:       true,
		DefaultMaterials: settings.MaterialsBoth,
		FollowUpDays:     10,
		UpdateCheck:      false,
		StartWithWindows: false,
	}
	saved, err := f.svc.Save(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	if saved != in {
		t.Fatalf("Save returned %+v", saved)
	}
	got, err := f.svc.Get(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got != in {
		t.Fatalf("Get after Save = %+v, want %+v", got, in)
	}
	evs := f.rec.Events()
	if len(evs) != 1 || evs[0].Payload.(events.Changed).Entity != events.EntitySettings {
		t.Fatalf("events = %+v, want one settings change (R3)", evs)
	}
}

func TestR37_InvalidSettingsRejectedAndNothingSaved(t *testing.T) {
	f := newFixture(t)
	bad := map[string]func(*settings.Settings){
		"model with path":   func(s *settings.Settings) { s.Model = "../x" },
		"empty model":       func(s *settings.Settings) { s.Model = "" },
		"unknown materials": func(s *settings.Settings) { s.DefaultMaterials = "resume" },
		"zero days":         func(s *settings.Settings) { s.FollowUpDays = 0 },
		"too many days":     func(s *settings.Settings) { s.FollowUpDays = 400 },
	}
	for name, mutate := range bad {
		t.Run(name, func(t *testing.T) {
			s := settings.Defaults()
			s.SkipReview = true // a valid change riding along must not persist either
			mutate(&s)
			if _, err := f.svc.Save(ctx, s); apperr.CodeOf(err) != apperr.CodeInvalid {
				t.Fatalf("err = %v, want invalid", err)
			}
			if got, _ := f.svc.Get(ctx); got != settings.Defaults() {
				t.Fatalf("partial save: %+v", got)
			}
		})
	}
	if len(f.rec.Events()) != 0 {
		t.Fatal("rejected saves emitted change events")
	}
}

func TestR37_StartWithWindowsWritesHKCURun(t *testing.T) {
	f := newFixture(t)
	s := settings.Defaults()
	s.StartWithWindows = true
	if _, err := f.svc.Save(ctx, s); err != nil {
		t.Fatal(err)
	}
	if !f.auto.enabled || f.auto.command != `"C:\Users\Łukasz Ż\AppData\Local\Programs\LeGaJ\legaj.exe"` {
		t.Fatalf("Run entry = %v %q; want the quoted exe path", f.auto.enabled, f.auto.command)
	}

	// Saving again without a change doesn't rewrite the registry.
	if _, err := f.svc.Save(ctx, s); err != nil {
		t.Fatal(err)
	}
	if strings.Count(strings.Join(f.auto.calls, ","), "enable") != 1 {
		t.Fatalf("calls = %v", f.auto.calls)
	}

	s.StartWithWindows = false
	if _, err := f.svc.Save(ctx, s); err != nil {
		t.Fatal(err)
	}
	if f.auto.enabled {
		t.Fatal("turning the setting off left the Run entry")
	}
}

func TestR37_ReconcileAppliesStoredSetting(t *testing.T) {
	f := newFixture(t)
	f.auto.enabled = true // the Run entry exists, but the saved setting is off
	if err := f.svc.Reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	if f.auto.enabled {
		t.Fatal("Reconcile left a Run entry the setting says shouldn't exist")
	}
}

func TestR37_ReconcileOnlyRepairsAMismatch(t *testing.T) {
	f := newFixture(t)
	s := settings.Defaults()
	s.StartWithWindows = true
	if _, err := f.svc.Save(ctx, s); err != nil {
		t.Fatal(err)
	}

	// Another copy of LeGaJ (an installed build vs a dev build) owns the
	// entry: starting this one must not take autostart over.
	f.auto.command = `"D:\other\legaj.exe"`
	f.auto.calls = nil
	if err := f.svc.Reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	if len(f.auto.calls) != 0 || f.auto.command != `"D:\other\legaj.exe"` {
		t.Fatalf("Reconcile rewrote a matching entry: calls %v, command %q", f.auto.calls, f.auto.command)
	}

	// The entry vanished while the setting is on: put it back.
	f.auto.enabled, f.auto.command = false, ""
	if err := f.svc.Reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	if !f.auto.enabled {
		t.Fatal("Reconcile didn't restore a missing entry")
	}
}

func TestR37_RegistryFailureSavesNothing(t *testing.T) {
	f := newFixture(t)
	f.auto.failNext = errors.New("access denied")
	s := settings.Defaults()
	s.StartWithWindows = true
	s.FollowUpDays = 12
	if _, err := f.svc.Save(ctx, s); err == nil {
		t.Fatal("Save succeeded although the registry write failed")
	}
	if got, _ := f.svc.Get(ctx); got != settings.Defaults() {
		t.Fatalf("settings changed despite the failure: %+v", got)
	}
}
