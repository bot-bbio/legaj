//go:build live

// Package live is the live Gemini suite (R69): every production request
// type, sent through the production code to the real API with the tester's
// own key. It never runs in CI. Run it with
//
//	powershell -NoProfile -ExecutionPolicy Bypass -File scripts\live.ps1
//
// which asks for the key without echoing it (so it never reaches the shell's
// history), sets it for this run only, and uses a scratch LEGAJ_HOME. The key
// is never printed or logged. All test data here is synthetic, so a failure
// may print Gemini's own error message (APIError.UnsafeMessage).
package live_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/bot-bbio/legaj/internal/ai"
	"github.com/bot-bbio/legaj/internal/ai/gemini"
	"github.com/bot-bbio/legaj/internal/testenv"
)

func TestMain(m *testing.M) { testenv.Main(m) }

// liveTimeout bounds one test's calls, retries included.
const liveTimeout = 3 * time.Minute

func liveCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), liveTimeout)
	t.Cleanup(cancel)
	return ctx
}

func liveClient(t *testing.T) *gemini.Client {
	t.Helper()
	key := os.Getenv("LEGAJ_LIVE_GEMINI_KEY")
	if key == "" {
		t.Fatal("LEGAJ_LIVE_GEMINI_KEY is not set: nothing was sent to Gemini (run scripts\\live.ps1)")
	}
	return gemini.New(gemini.Config{Key: func() (string, error) {
		if key == "" {
			return "", errors.New("no key")
		}
		return key, nil
	}})
}

// liveModel is the model under test: LEGAJ_LIVE_GEMINI_MODEL, else the
// app's default.
func liveModel() string {
	if m := os.Getenv("LEGAJ_LIVE_GEMINI_MODEL"); m != "" {
		return m
	}
	return gemini.DefaultModel
}

// explain fails the test with everything known about a Gemini error. The
// data sent is synthetic, so Gemini's own message is safe to print here.
func explain(t *testing.T, what string, err error) {
	t.Helper()
	var apiErr *gemini.APIError
	if errors.As(err, &apiErr) {
		t.Fatalf("%s: %v\n  model: %s\n  Gemini says: %s", what, apiErr, liveModel(), apiErr.UnsafeMessage())
	}
	t.Fatalf("%s: %v (model %s)", what, err, liveModel())
}

// TestLive_ModelList (R67): the key's real model listing yields at least one
// model LeGaJ works with, and the built-in seed is still among them, so a
// retirement of the default shows up here before users hit it.
func TestLive_ModelList(t *testing.T) {
	models, err := liveClient(t).ListModels(liveCtx(t))
	if err != nil {
		explain(t, "listing models", err)
	}
	if len(models) == 0 {
		t.Fatal("no stable Flash or Flash-Lite model in the listing: check the eligibility pattern against Gemini's current names")
	}
	listed := map[string]bool{}
	for _, m := range models {
		listed[m.ID] = true
		t.Logf("eligible: %s (%s)", m.ID, m.DisplayName)
	}
	for _, seed := range gemini.SeedModels() {
		if !listed[seed] {
			t.Errorf("seed model %s is no longer listed: update gemini.DefaultModel", seed)
		}
	}
}

// TestLive_StructuredOutputFieldsAccepted confirms D24: the request fields
// LeGaJ uses for JSON output (responseMimeType + responseJsonSchema) are
// accepted by the live API and the answer matches the schema.
func TestLive_StructuredOutputFieldsAccepted(t *testing.T) {
	c := liveClient(t)
	temp := 0.0
	resp, err := c.Generate(liveCtx(t), ai.Request{
		Model:       liveModel(),
		System:      "Answer with JSON only.",
		Parts:       []ai.Part{ai.Text(`Give the word "ready" and the number 3.`)},
		Temperature: &temp,
		Schema: []byte(`{"type": "object", "properties": {"word": {"type": "string"}, "n": {"type": "integer"}},
			"required": ["word", "n"], "additionalProperties": false}`),
	})
	if err != nil {
		explain(t, "structured output (check D24's field names against the current API reference)", err)
	}
	var out struct {
		Word string `json:"word"`
		N    int    `json:"n"`
	}
	if err := json.Unmarshal([]byte(resp.Text), &out); err != nil || out.N != 3 {
		t.Fatalf("unexpected structured answer %+v (%v)", out, err)
	}
	t.Logf("model %s answered with schema-valid JSON", resp.Model)
}
