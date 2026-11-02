package onboarding_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/bot-bbio/legaj/internal/apperr"
	"github.com/bot-bbio/legaj/internal/events"
	"github.com/bot-bbio/legaj/internal/logx"
	"github.com/bot-bbio/legaj/internal/onboarding"
	"github.com/bot-bbio/legaj/internal/pipeline"
	"github.com/bot-bbio/legaj/internal/secrets"
	"github.com/bot-bbio/legaj/internal/store"
	"github.com/bot-bbio/legaj/internal/testenv"
)

func TestMain(m *testing.M) { testenv.Main(m) }

var ctx = context.Background()

const goodKey = "AIzaSyD-SENTINEL-KEY-0123456789abcdWXYZ"

type fixture struct {
	svc    *onboarding.Service
	db     *store.DB
	keys   *secrets.Memory
	tested []string
	result error
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	db, st, err := store.Open(ctx, store.Options{Dir: testenv.Isolate(t)})
	if err != nil || st.Kind != store.StartupReady {
		t.Fatalf("open: %v %+v", err, st)
	}
	t.Cleanup(func() { _ = db.Close() })
	f := &fixture{db: db, keys: secrets.NewMemory()}
	f.svc = onboarding.NewService(onboarding.Config{
		DB: db, Bus: events.NewBus(&events.Recorder{}),
		Secrets: secrets.NewManager(f.keys, logx.NewRedactor()),
		TestKey: func(_ context.Context, key string) error {
			f.tested = append(f.tested, key)
			return f.result
		},
	})
	return f
}

func (f *fixture) exec(t *testing.T, q string, args ...any) {
	t.Helper()
	if err := f.db.Tx(ctx, func(tx *sql.Tx) error { _, err := tx.ExecContext(ctx, q, args...); return err }); err != nil {
		t.Fatal(err)
	}
}

func done(st onboarding.State) map[onboarding.StepID]bool {
	out := map[onboarding.StepID]bool{}
	for _, s := range st.Steps {
		out[s.ID] = s.Done
	}
	return out
}

func TestR7_StepsInOrderAndDerivedFromData(t *testing.T) {
	f := newFixture(t)
	st, err := f.svc.State(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := []onboarding.StepID{onboarding.StepKey, onboarding.StepResume, onboarding.StepProfile, onboarding.StepClipper}
	if len(st.Steps) != len(want) || st.Finished {
		t.Fatalf("fresh state = %+v", st)
	}
	for i, s := range st.Steps {
		if s.ID != want[i] || s.Done {
			t.Fatalf("step %d = %+v, want %s not done", i, s, want[i])
		}
	}

	if err := f.keys.Set(goodKey); err != nil {
		t.Fatal(err)
	}
	now := store.FormatTime(time.Now())
	f.exec(t, `INSERT INTO resume_files (file_name, stored_path, mime_type, sha256, size_bytes, uploaded_at) VALUES ('cv.pdf', 'resumes/a.pdf', 'application/pdf', 'a', 1, ?)`, now)
	f.exec(t, `INSERT INTO profile (id, data, updated_at) VALUES (1, '{"fullName":"Ada"}', ?)`, now)
	if err := f.db.Tx(ctx, func(tx *sql.Tx) error {
		_, err := pipeline.NewMachine(time.Now).CreateLead(ctx, tx, pipeline.NewLead{Company: "Acme", Source: store.SourceClip})
		return err
	}); err != nil {
		t.Fatal(err)
	}

	st, err = f.svc.State(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for id, ok := range done(st) {
		if !ok {
			t.Errorf("step %s not done after its data exists", id)
		}
	}
}

func TestR7_ProfileStepNeedsAName(t *testing.T) {
	f := newFixture(t)
	f.exec(t, `INSERT INTO profile (id, data, updated_at) VALUES (1, '{"fullName":""}', ?)`, store.FormatTime(time.Now()))
	st, _ := f.svc.State(ctx)
	if done(st)[onboarding.StepProfile] {
		t.Fatal("an empty profile counts as done")
	}
}

func TestR7_FinishPersistsAndSkippedStepsStayOnChecklist(t *testing.T) {
	f := newFixture(t)
	if err := f.svc.Finish(ctx); err != nil {
		t.Fatal(err)
	}
	st, err := f.svc.State(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !st.Finished {
		t.Fatal("Finish didn't persist")
	}
	for id, ok := range done(st) {
		if ok {
			t.Errorf("skipped step %s reported done", id)
		}
	}
}

func TestR7_KeyVerifiedThenSaved(t *testing.T) {
	f := newFixture(t)
	res, err := f.svc.SaveKey(ctx, "  "+goodKey+"  ")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Verified {
		t.Fatalf("result = %+v", res)
	}
	if len(f.tested) != 1 || f.tested[0] != goodKey {
		t.Fatalf("tested %q; the trimmed key must be tested before saving", f.tested)
	}
	if got, _ := f.keys.Get(); got != goodKey {
		t.Fatal("verified key not saved")
	}
}

func TestR7_RejectedKeyNotSaved(t *testing.T) {
	f := newFixture(t)
	f.result = apperr.New(apperr.CodeAIKeyRejected, "Gemini rejected the key. Check it in Settings.")
	_, err := f.svc.SaveKey(ctx, goodKey)
	if apperr.CodeOf(err) != apperr.CodeAIKeyRejected {
		t.Fatalf("err = %v", err)
	}
	if _, err := f.keys.Get(); !errors.Is(err, secrets.ErrNotFound) {
		t.Fatal("a rejected key was saved")
	}
}

func TestR7_UnverifiableKeySavedWithNote(t *testing.T) {
	for _, code := range []apperr.Code{apperr.CodeAIRateLimited, apperr.CodeAIUnavailable} {
		f := newFixture(t)
		f.result = apperr.New(code, "busy")
		res, err := f.svc.SaveKey(ctx, goodKey)
		if err != nil {
			t.Fatalf("%s: %v", code, err)
		}
		if res.Verified || res.Message == "" {
			t.Fatalf("%s: result = %+v, want saved but unverified with a note", code, res)
		}
		if got, _ := f.keys.Get(); got != goodKey {
			t.Fatalf("%s: key not saved", code)
		}
	}
}

func TestR7_MalformedKeyNeverTested(t *testing.T) {
	f := newFixture(t)
	if _, err := f.svc.SaveKey(ctx, "short"); apperr.CodeOf(err) != apperr.CodeInvalid {
		t.Fatalf("err = %v", err)
	}
	if len(f.tested) != 0 {
		t.Fatal("a malformed key was sent to Gemini")
	}
}
