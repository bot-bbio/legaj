package gemini_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"testing"

	"github.com/bot-bbio/legaj/internal/ai/gemini"
	"github.com/bot-bbio/legaj/internal/logx"
)

// TestR63_APIErrorLogsStatusAndReasonOnly: a Gemini error reaches callers as
// an *APIError holding the HTTP status, Google's status enum and the reason
// enums, which is all the log may carry (R34, R63). Gemini's own message can
// echo request content, so it never appears in any rendering of the error;
// only UnsafeMessage returns it, for the live suite's synthetic data (R69).
func TestR63_APIErrorLogsStatusAndReasonOnly(t *testing.T) {
	body, _ := json.Marshal(map[string]any{"error": map[string]any{
		"code":    400,
		"status":  "INVALID_ARGUMENT",
		"message": "Invalid value at contents: " + sentinelPrompt,
		"details": []any{
			map[string]any{"@type": "type.googleapis.com/google.rpc.ErrorInfo", "reason": "SCHEMA_TOO_COMPLEX"},
			// Not an enum: dropped, so a reason can't smuggle text into logs.
			map[string]any{"@type": "type.googleapis.com/google.rpc.ErrorInfo", "reason": "x " + sentinelPrompt},
		},
	}})
	h := newHarness(t, reply{status: 400, body: string(body)})
	_, err := h.client.Generate(context.Background(), textRequest())
	if err == nil {
		t.Fatal("want an error")
	}

	var apiErr *gemini.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error chain has no *gemini.APIError: %v", err)
	}
	if apiErr.HTTPStatus != 400 || apiErr.Status != "INVALID_ARGUMENT" {
		t.Fatalf("status = %d %q", apiErr.HTTPStatus, apiErr.Status)
	}
	if !slices.Equal(apiErr.Reasons, []string{"SCHEMA_TOO_COMPLEX"}) {
		t.Fatalf("reasons = %q, want only the enum", apiErr.Reasons)
	}
	if got, want := apiErr.Error(), "gemini: HTTP 400 INVALID_ARGUMENT (SCHEMA_TOO_COMPLEX)"; got != want {
		t.Fatalf("Error() = %q, want %q", got, want)
	}
	if !strings.Contains(apiErr.UnsafeMessage(), "SENTINEL-RESUME-LINE") {
		t.Fatalf("UnsafeMessage lost Gemini's message: %q", apiErr.UnsafeMessage())
	}

	for _, verb := range []string{"%v", "%+v", "%#v", "%s"} {
		leaks(t, "apiErr "+verb, fmt.Sprintf(verb, apiErr))
		leaks(t, "err "+verb, fmt.Sprintf(verb, err))
	}
	if js, jerr := json.Marshal(apiErr); jerr == nil {
		leaks(t, "apiErr as JSON", string(js))
	}
	var logged bytes.Buffer
	logx.New(&logged, logx.NewRedactor(), slog.LevelDebug).Error("x", "err", err)
	leaks(t, "logged error", strings.ReplaceAll(logged.String(), `\"`, `"`))
	leaks(t, "AI call log", h.log.String())
}

// TestR63_APIErrorWithoutReasons keeps V2's cause text when Gemini sends no
// reason, so existing log readers see the same line.
func TestR63_APIErrorWithoutReasons(t *testing.T) {
	h := newHarness(t, reply{status: 400, body: errBody(400, "INVALID_ARGUMENT", "bad", "")})
	_, err := h.client.Generate(context.Background(), textRequest())
	var apiErr *gemini.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("no *gemini.APIError in %v", err)
	}
	if got := apiErr.Error(); got != "gemini: HTTP 400 INVALID_ARGUMENT" {
		t.Fatalf("Error() = %q", got)
	}
}
