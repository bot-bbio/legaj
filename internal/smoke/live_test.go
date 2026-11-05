//go:build live

// Package smoke holds the live release checks (item 45). They call the real
// Gemini API with the release tester's own key and never run in CI:
//
//	$env:LEGAJ_HOME = "$env:TEMP\legaj-live"
//	$env:LEGAJ_LIVE_GEMINI_KEY = '<a key you can spend a few requests of>'
//	go test -tags live -count=1 ./internal/smoke/
//
// The key is read from the environment only for this run and is never
// printed or logged.
package smoke_test

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

func liveClient(t *testing.T) *gemini.Client {
	t.Helper()
	key := os.Getenv("LEGAJ_LIVE_GEMINI_KEY")
	if key == "" {
		t.Fatal("LEGAJ_LIVE_GEMINI_KEY is not set: nothing was sent to Gemini")
	}
	return gemini.New(gemini.Config{Key: func() (string, error) {
		if key == "" {
			return "", errors.New("no key")
		}
		return key, nil
	}})
}

// TestLive_StructuredOutputFieldsAccepted confirms D24: the request fields
// LeGaJ uses for JSON output (responseMimeType + responseJsonSchema) are
// accepted by the live API and the answer matches the schema.
func TestLive_StructuredOutputFieldsAccepted(t *testing.T) {
	c := liveClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	temp := 0.0
	resp, err := c.Generate(ctx, ai.Request{
		System:      "Answer with JSON only.",
		Parts:       []ai.Part{ai.Text(`Give the word "ready" and the number 3.`)},
		Temperature: &temp,
		Schema: []byte(`{"type": "object", "properties": {"word": {"type": "string"}, "n": {"type": "integer"}},
			"required": ["word", "n"], "additionalProperties": false}`),
	})
	if err != nil {
		t.Fatalf("live call failed (check D24's field names against the current API reference): %v", err)
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
