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
	"github.com/bot-bbio/legaj/internal/apperr"
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

// TestR63_ErrorMessagesInOrder: each Gemini failure tells the user whose
// problem it is, the key check first (R63). Only a request Gemini refuses for
// no reason it names is reported as LeGaJ's, and never as the user's file.
func TestR63_ErrorMessagesInOrder(t *testing.T) {
	const (
		keyMsg      = "Gemini rejected the key. Check it in Settings."
		billingMsg  = "Gemini's free tier isn't available in your region. Enable billing in Google AI Studio."
		rejectedMsg = "Gemini couldn't accept LeGaJ's request. If it keeps happening, please report it: it's most likely a LeGaJ bug."
		modelMsg    = "The selected Gemini model isn't available. Pick another model in Settings."
	)
	cases := []struct {
		name string
		rp   reply
		code apperr.Code
		msg  string
	}{
		{"400 API_KEY_INVALID", reply{status: 400, body: errBody(400, "INVALID_ARGUMENT", "API key not valid. Please pass a valid API key.", "API_KEY_INVALID")}, apperr.CodeAIKeyRejected, keyMsg},
		// The key check wins even over a precondition failure.
		{"400 precondition with a bad key", reply{status: 400, body: errBody(400, "FAILED_PRECONDITION", "x", "API_KEY_INVALID")}, apperr.CodeAIKeyRejected, keyMsg},
		{"401", reply{status: 401, body: errBody(401, "UNAUTHENTICATED", "x", "")}, apperr.CodeAIKeyRejected, keyMsg},
		{"403", reply{status: 403, body: errBody(403, "PERMISSION_DENIED", "x", "")}, apperr.CodeAIKeyRejected, keyMsg},
		{"400 region", reply{status: 400, body: errBody(400, "FAILED_PRECONDITION", "User location is not supported for the API use.", "")}, apperr.CodeAIBilling, billingMsg},
		{"400 other", reply{status: 400, body: errBody(400, "INVALID_ARGUMENT", "The specified schema produces a constraint that has too many states for serving.", "")}, apperr.CodeAIRequestRejected, rejectedMsg},
		{"404", reply{status: 404, body: errBody(404, "NOT_FOUND", "models/x is not found", "")}, apperr.CodeInvalid, modelMsg},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newHarness(t, c.rp)
			_, err := h.client.Generate(context.Background(), textRequest())
			if apperr.CodeOf(err) != c.code || err.Error() != c.msg {
				t.Fatalf("got %q %q, want %q %q", apperr.CodeOf(err), err.Error(), c.code, c.msg)
			}
			if len(h.tr.requests) != 1 {
				t.Fatalf("%d requests: a 4xx must not be retried", len(h.tr.requests))
			}
		})
	}
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
