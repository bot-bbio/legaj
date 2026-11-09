package gemini_test

import (
	"context"
	"encoding/json"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/bot-bbio/legaj/internal/ai"
	"github.com/bot-bbio/legaj/internal/ai/gemini"
	"github.com/bot-bbio/legaj/internal/apperr"
)

// listing builds a models.list page. Each name maps to its supported
// generation methods; nil means generateContent and countTokens.
func listing(t *testing.T, next string, models map[string][]string) string {
	t.Helper()
	var out []map[string]any
	for name, methods := range models {
		if methods == nil {
			methods = []string{"generateContent", "countTokens"}
		}
		out = append(out, map[string]any{"name": "models/" + name, "displayName": "Display " + name, "supportedGenerationMethods": methods})
	}
	body := map[string]any{"models": out}
	if next != "" {
		body["nextPageToken"] = next
	}
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func ids(models []ai.ModelInfo) []string {
	var out []string
	for _, m := range models {
		out = append(out, m.ID)
	}
	return out
}

// TestR67_ListModelsKeepsStableFlashModels: only stable Flash and Flash-Lite
// text models survive, Flash-Lite first, newest version first. Everything a
// listing can also hold (previews, experiments, image, speech, live and audio
// models, aliases that move, dated snapshots, other families, models without
// generateContent, malformed names) is dropped by one strict name pattern.
func TestR67_ListModelsKeepsStableFlashModels(t *testing.T) {
	h := newHarness(t, reply{status: 200, body: listing(t, "", map[string][]string{
		"gemini-2.5-flash":                     nil,
		"gemini-3.1-flash-lite":                nil,
		"gemini-2.5-flash-lite":                nil,
		"gemini-3.1-flash":                     nil,
		"gemini-10-flash-lite":                 nil, // versions compare as numbers, not text
		"gemini-2.5-pro":                       nil,
		"gemini-3.1-flash-preview-09-2026":     nil,
		"gemini-2.0-flash-exp":                 nil,
		"gemini-2.5-flash-image":               nil,
		"gemini-2.5-flash-preview-tts":         nil,
		"gemini-live-2.5-flash":                nil,
		"gemini-2.5-flash-native-audio-dialog": nil,
		"gemini-flash-latest":                  nil,
		"gemini-2.0-flash-001":                 nil,
		"text-embedding-004":                   {"embedContent"},
		"gemini-1.5-flash":                     {"countTokens"},
		"gemini-9-flash-lite; DROP":            nil,
	})})
	models, err := h.client.ListModels(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"gemini-10-flash-lite", "gemini-3.1-flash-lite", "gemini-2.5-flash-lite", "gemini-3.1-flash", "gemini-2.5-flash"}
	if got := ids(models); !slices.Equal(got, want) {
		t.Fatalf("models = %q\nwant      %q", got, want)
	}
	if models[1].DisplayName != "Display gemini-3.1-flash-lite" {
		t.Fatalf("display name = %q", models[1].DisplayName)
	}
}

// TestR67_ListModelsRequest: a GET to models.list, the key in the header
// only, every page followed, and a repeated page token can't loop forever.
func TestR67_ListModelsRequest(t *testing.T) {
	h := newHarness(t,
		reply{status: 200, body: listing(t, "p2", map[string][]string{"gemini-3.1-flash-lite": nil})},
		reply{status: 200, body: listing(t, "p2", map[string][]string{"gemini-3.1-flash": nil})},
	)
	models, err := h.client.ListModels(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got := ids(models); !slices.Equal(got, []string{"gemini-3.1-flash-lite", "gemini-3.1-flash"}) {
		t.Fatalf("models = %q", got)
	}
	if len(h.tr.requests) != 2 {
		t.Fatalf("%d requests, want 2 (the repeated token stops the walk)", len(h.tr.requests))
	}
	first, second := h.tr.requests[0], h.tr.requests[1]
	if first.Method != "GET" || first.URL.Path != "/v1beta/models" || first.URL.Query().Get("pageToken") != "" {
		t.Fatalf("first request = %s %s", first.Method, first.URL)
	}
	if second.URL.Query().Get("pageToken") != "p2" {
		t.Fatalf("second request = %s", second.URL)
	}
	for _, r := range h.tr.requests {
		if r.Header.Get("x-goog-api-key") != sentinelKey {
			t.Fatal("key not sent in the header")
		}
		if u, _ := url.QueryUnescape(r.URL.String()); strings.Contains(u, sentinelKey) {
			t.Fatal("key leaked into the URL")
		}
	}
	leaks(t, "AI call log", h.log.String())
}

// TestR67_ListModelsErrorsMapped: listing errors carry the same honest
// messages as any Gemini call (R63).
func TestR67_ListModelsErrorsMapped(t *testing.T) {
	h := newHarness(t, reply{status: 400, body: errBody(400, "INVALID_ARGUMENT", "API key not valid.", "API_KEY_INVALID")})
	if _, err := h.client.ListModels(context.Background()); apperr.CodeOf(err) != apperr.CodeAIKeyRejected {
		t.Fatalf("err = %v, want key rejected", err)
	}
}

// TestR67_ListedDisplayNamesCleaned: display names come from the network and
// are shown in Settings, so control characters go, long ones are cut, and an
// empty one falls back to the ID.
func TestR67_ListedDisplayNamesCleaned(t *testing.T) {
	body, _ := json.Marshal(map[string]any{"models": []any{
		map[string]any{"name": "models/gemini-3.1-flash-lite", "displayName": "Gemini\n3.1\x07 Flash-Lite", "supportedGenerationMethods": []string{"generateContent"}},
		map[string]any{"name": "models/gemini-3.1-flash", "displayName": strings.Repeat("Ł", 200), "supportedGenerationMethods": []string{"generateContent"}},
		map[string]any{"name": "models/gemini-2.5-flash", "displayName": "  ", "supportedGenerationMethods": []string{"generateContent"}},
	}})
	h := newHarness(t, reply{status: 200, body: string(body)})
	models, err := h.client.ListModels(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, m := range models {
		got[m.ID] = m.DisplayName
	}
	if got["gemini-3.1-flash-lite"] != "Gemini3.1 Flash-Lite" {
		t.Errorf("control characters kept: %q", got["gemini-3.1-flash-lite"])
	}
	if n := len([]rune(got["gemini-3.1-flash"])); n != 80 {
		t.Errorf("long name is %d runes, want 80", n)
	}
	if got["gemini-2.5-flash"] != "gemini-2.5-flash" {
		t.Errorf("empty name = %q, want the ID", got["gemini-2.5-flash"])
	}
}

// TestR67_SeedIsTheDefault: the built-in fallback, used when listing fails,
// is the one model ID LeGaJ ships with.
func TestR67_SeedIsTheDefault(t *testing.T) {
	if !slices.Equal(gemini.SeedModels(), []string{gemini.DefaultModel}) {
		t.Fatalf("seed = %q", gemini.SeedModels())
	}
}
