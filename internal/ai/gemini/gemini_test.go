package gemini_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bot-bbio/legaj/internal/ai"
	"github.com/bot-bbio/legaj/internal/ai/gemini"
	"github.com/bot-bbio/legaj/internal/apperr"
	"github.com/bot-bbio/legaj/internal/logx"
	"github.com/bot-bbio/legaj/internal/testenv"
)

func TestMain(m *testing.M) { testenv.Main(m) }

const (
	sentinelKey    = "AIzaSyD-SENTINEL-KEY-0123456789abcdWXYZ"
	sentinelPrompt = "SENTINEL-RESUME-LINE Built a compiler at Żółć Corp"
)

// reply is one scripted HTTP response.
type reply struct {
	status  int
	body    string
	headers map[string]string
}

// transport records requests and plays back scripted replies.
type transport struct {
	mu       sync.Mutex
	replies  []reply
	requests []*http.Request
	bodies   [][]byte
}

func (tr *transport) RoundTrip(r *http.Request) (*http.Response, error) {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	var body []byte
	if r.Body != nil { // a GET (model listing) has none
		body, _ = io.ReadAll(r.Body)
	}
	tr.requests = append(tr.requests, r)
	tr.bodies = append(tr.bodies, body)
	if len(tr.replies) == 0 {
		return nil, errors.New("transport: no scripted reply left")
	}
	rp := tr.replies[0]
	tr.replies = tr.replies[1:]
	h := http.Header{"Content-Type": {"application/json"}}
	for k, v := range rp.headers {
		h.Set(k, v)
	}
	return &http.Response{StatusCode: rp.status, Header: h, Body: io.NopCloser(strings.NewReader(rp.body)), Request: r}, nil
}

func okBody(text string) string {
	b, _ := json.Marshal(map[string]any{
		"candidates": []any{map[string]any{
			"content":      map[string]any{"parts": []any{map[string]any{"text": text}}, "role": "model"},
			"finishReason": "STOP",
		}},
		"modelVersion": "gemini-3.1-flash-lite",
	})
	return string(b)
}

func errBody(code int, status, msg, reason string) string {
	details := []any{}
	if reason != "" {
		details = append(details, map[string]any{"@type": "type.googleapis.com/google.rpc.ErrorInfo", "reason": reason})
	}
	b, _ := json.Marshal(map[string]any{"error": map[string]any{"code": code, "message": msg, "status": status, "details": details}})
	return string(b)
}

type harness struct {
	client *gemini.Client
	tr     *transport
	sleeps []time.Duration
	log    *bytes.Buffer
}

func newHarness(t *testing.T, replies ...reply) *harness {
	t.Helper()
	h := &harness{tr: &transport{replies: replies}, log: &bytes.Buffer{}}
	r := logx.NewRedactor()
	h.client = gemini.New(gemini.Config{
		HTTPClient: &http.Client{Transport: h.tr},
		Key:        func() (string, error) { return sentinelKey, nil },
		Logger:     logx.New(h.log, r, slog.LevelDebug),
		Sleep: func(ctx context.Context, d time.Duration) error {
			h.sleeps = append(h.sleeps, d)
			return ctx.Err()
		},
	})
	return h
}

func textRequest() ai.Request {
	return ai.Request{Parts: []ai.Part{ai.Text(sentinelPrompt)}}
}

// leaks fails if s contains the key or the prompt.
func leaks(t *testing.T, where, s string) {
	t.Helper()
	for _, secret := range []string{sentinelKey, "SENTINEL-RESUME-LINE"} {
		if strings.Contains(s, secret) {
			t.Fatalf("%s leaks %q: %s", where, secret, s)
		}
	}
}

func TestR5_RetriesOn429And503ThenSucceeds(t *testing.T) {
	h := newHarness(t,
		reply{status: 429, body: errBody(429, "RESOURCE_EXHAUSTED", "quota", "")},
		reply{status: 503, body: errBody(503, "UNAVAILABLE", "overloaded", "")},
		reply{status: 200, body: okBody("hello")},
	)
	resp, err := h.client.Generate(context.Background(), textRequest())
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if resp.Text != "hello" {
		t.Fatalf("text = %q", resp.Text)
	}
	if len(h.tr.requests) != 3 {
		t.Fatalf("%d attempts, want 3", len(h.tr.requests))
	}
	if len(h.sleeps) != 2 || h.sleeps[1] <= h.sleeps[0] {
		t.Fatalf("backoff = %v, want two growing waits", h.sleeps)
	}
}

func TestR5_RetryAfterHonoured(t *testing.T) {
	h := newHarness(t,
		reply{status: 429, body: errBody(429, "RESOURCE_EXHAUSTED", "quota", ""), headers: map[string]string{"Retry-After": "7"}},
		reply{status: 200, body: okBody("ok")},
	)
	if _, err := h.client.Generate(context.Background(), textRequest()); err != nil {
		t.Fatal(err)
	}
	if len(h.sleeps) != 1 || h.sleeps[0] != 7*time.Second {
		t.Fatalf("sleeps = %v, want [7s]", h.sleeps)
	}
}

func TestR5_GivesUpOnPersistent429(t *testing.T) {
	var replies []reply
	for range 10 {
		replies = append(replies, reply{status: 429, body: errBody(429, "RESOURCE_EXHAUSTED", "quota", "")})
	}
	h := newHarness(t, replies...)
	_, err := h.client.Generate(context.Background(), textRequest())
	if apperr.CodeOf(err) != apperr.CodeAIRateLimited {
		t.Fatalf("code = %q (err %v)", apperr.CodeOf(err), err)
	}
	if n := len(h.tr.requests); n != gemini.MaxAttempts {
		t.Fatalf("%d attempts, want %d", n, gemini.MaxAttempts)
	}
}

func TestR5_401MapsToKeyRejected(t *testing.T) {
	cases := map[string]reply{
		"401":             {status: 401, body: errBody(401, "UNAUTHENTICATED", "Request had invalid authentication credentials: "+sentinelPrompt, "")},
		"403":             {status: 403, body: errBody(403, "PERMISSION_DENIED", "denied", "")},
		"400 invalid key": {status: 400, body: errBody(400, "INVALID_ARGUMENT", "API key not valid. Please pass a valid API key.", "API_KEY_INVALID")},
	}
	for name, rp := range cases {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t, rp)
			_, err := h.client.Generate(context.Background(), textRequest())
			if apperr.CodeOf(err) != apperr.CodeAIKeyRejected {
				t.Fatalf("code = %q", apperr.CodeOf(err))
			}
			if err.Error() != "Gemini rejected the key. Check it in Settings." {
				t.Fatalf("message = %q", err.Error())
			}
			if len(h.tr.requests) != 1 {
				t.Fatal("a rejected key must not be retried")
			}
			// Neither the message nor the logged cause may carry the key,
			// the request body or the response body.
			leaks(t, "error", err.Error())
			var cause bytes.Buffer
			logx.New(&cause, logx.NewRedactor(), slog.LevelDebug).Error("x", "err", err)
			leaks(t, "logged cause", strings.ReplaceAll(cause.String(), `\"`, `"`))
			leaks(t, "AI call log", h.log.String())
		})
	}
}

func TestR5_OtherErrorsMapped(t *testing.T) {
	cases := []struct {
		name string
		rp   reply
		code apperr.Code
	}{
		// R63 (V3) gives an unexplained 400 its own code; V2 used CodeInternal.
		{"bad request", reply{status: 400, body: errBody(400, "INVALID_ARGUMENT", "bad", "")}, apperr.CodeAIRequestRejected},
		{"model not found", reply{status: 404, body: errBody(404, "NOT_FOUND", "no model", "")}, apperr.CodeInvalid},
		{"blocked prompt", reply{status: 200, body: `{"promptFeedback": {"blockReason": "SAFETY"}}`}, apperr.CodeAIInvalidOutput},
		{"truncated", reply{status: 200, body: strings.Replace(okBody("{\"a\""), "STOP", "MAX_TOKENS", 1)}, apperr.CodeAIInvalidOutput},
		{"no candidates", reply{status: 200, body: `{"candidates": []}`}, apperr.CodeAIInvalidOutput},
		{"not json", reply{status: 200, body: `<html>proxy error</html>`}, apperr.CodeAIUnavailable},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newHarness(t, c.rp)
			_, err := h.client.Generate(context.Background(), textRequest())
			if apperr.CodeOf(err) != c.code {
				t.Fatalf("code = %q, want %q (err %v)", apperr.CodeOf(err), c.code, err)
			}
		})
	}
}

func TestR5_StructuredOutputValidated(t *testing.T) {
	schema := json.RawMessage(`{"type":"object","properties":{"name":{"type":"string"}},"required":["name"],"additionalProperties":false}`)

	h := newHarness(t, reply{status: 200, body: okBody(`{"nmae": "typo"}`)})
	_, err := h.client.Generate(context.Background(), ai.Request{Parts: []ai.Part{ai.Text("x")}, Schema: schema})
	if apperr.CodeOf(err) != apperr.CodeAIInvalidOutput {
		t.Fatalf("schema-invalid output: code = %q, want invalid output", apperr.CodeOf(err))
	}

	h = newHarness(t, reply{status: 200, body: okBody(`{"name": "Ada"}`)})
	resp, err := h.client.Generate(context.Background(), ai.Request{Parts: []ai.Part{ai.Text("x")}, Schema: schema})
	if err != nil || resp.Text != `{"name": "Ada"}` {
		t.Fatalf("valid output: %q, %v", resp.Text, err)
	}
}

func TestR35_KeyNeverInPromptBody(t *testing.T) {
	h := newHarness(t, reply{status: 200, body: okBody("ok")})
	if _, err := h.client.Generate(context.Background(), textRequest()); err != nil {
		t.Fatal(err)
	}
	req, body := h.tr.requests[0], string(h.tr.bodies[0])
	if strings.Contains(body, sentinelKey) {
		t.Fatal("the key is in the request body")
	}
	if strings.Contains(req.URL.String(), sentinelKey) {
		t.Fatal("the key is in the URL, where net/http errors would print it")
	}
	if got := req.Header.Get("x-goog-api-key"); got != sentinelKey {
		t.Fatalf("x-goog-api-key = %q", got)
	}
}

func TestR5_RequestShape(t *testing.T) {
	h := newHarness(t, reply{status: 200, body: okBody(`{"a":1}`)})
	temp := 0.9
	pdf := []byte("%PDF-1.7 fake")
	_, err := h.client.Generate(context.Background(), ai.Request{
		Model:       "gemini-2.5-pro",
		System:      "You extract resumes.",
		Parts:       []ai.Part{ai.Text("Parse this."), ai.Inline("application/pdf", pdf)},
		Temperature: &temp,
		Schema:      json.RawMessage(`{"type":"object","properties":{"a":{"type":"integer"}},"required":["a"]}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	req := h.tr.requests[0]
	if req.Method != http.MethodPost || req.URL.Host != "generativelanguage.googleapis.com" ||
		req.URL.Path != "/v1beta/models/gemini-2.5-pro:generateContent" {
		t.Fatalf("request = %s %s", req.Method, req.URL)
	}
	var body struct {
		SystemInstruction struct {
			Parts []struct{ Text string } `json:"parts"`
		} `json:"systemInstruction"`
		Contents []struct {
			Role  string `json:"role"`
			Parts []struct {
				Text       string `json:"text"`
				InlineData *struct {
					MimeType string `json:"mimeType"`
					Data     string `json:"data"`
				} `json:"inlineData"`
			} `json:"parts"`
		} `json:"contents"`
		GenerationConfig struct {
			Temperature        *float64        `json:"temperature"`
			ResponseMimeType   string          `json:"responseMimeType"`
			ResponseJSONSchema json.RawMessage `json:"responseJsonSchema"`
		} `json:"generationConfig"`
	}
	if err := json.Unmarshal(h.tr.bodies[0], &body); err != nil {
		t.Fatal(err)
	}
	if len(body.SystemInstruction.Parts) != 1 || body.SystemInstruction.Parts[0].Text != "You extract resumes." {
		t.Fatalf("systemInstruction = %+v", body.SystemInstruction)
	}
	if len(body.Contents) != 1 || body.Contents[0].Role != "user" || len(body.Contents[0].Parts) != 2 {
		t.Fatalf("contents = %+v", body.Contents)
	}
	inline := body.Contents[0].Parts[1].InlineData
	if inline == nil || inline.MimeType != "application/pdf" || inline.Data != base64.StdEncoding.EncodeToString(pdf) {
		t.Fatalf("inlineData = %+v", inline)
	}
	gc := body.GenerationConfig
	if gc.Temperature == nil || *gc.Temperature != 0.9 || gc.ResponseMimeType != "application/json" || len(gc.ResponseJSONSchema) == 0 {
		t.Fatalf("generationConfig = %+v", gc)
	}
}

func TestR5_DefaultModelAndValidation(t *testing.T) {
	h := newHarness(t, reply{status: 200, body: okBody("ok")})
	if _, err := h.client.Generate(context.Background(), textRequest()); err != nil {
		t.Fatal(err)
	}
	if want := "/v1beta/models/" + gemini.DefaultModel + ":generateContent"; h.tr.requests[0].URL.Path != want {
		t.Fatalf("path = %s, want %s", h.tr.requests[0].URL.Path, want)
	}
	if gemini.DefaultModel != "gemini-3.1-flash-lite" {
		t.Fatalf("DefaultModel = %s, want the V1 default (D21)", gemini.DefaultModel)
	}

	for _, bad := range []string{"../../x", "models/x", "gemini pro", "x?key=1", "UPPER"} {
		h := newHarness(t)
		_, err := h.client.Generate(context.Background(), ai.Request{Model: bad, Parts: []ai.Part{ai.Text("x")}})
		if apperr.CodeOf(err) != apperr.CodeInvalid || len(h.tr.requests) != 0 {
			t.Errorf("model %q: code %q, %d requests; want invalid and no request", bad, apperr.CodeOf(err), len(h.tr.requests))
		}
	}
}

func TestR5_MissingKeyMakesNoRequest(t *testing.T) {
	tr := &transport{}
	c := gemini.New(gemini.Config{
		HTTPClient: &http.Client{Transport: tr},
		Key:        func() (string, error) { return "", apperr.New(apperr.CodeAIKeyMissing, "Add your key.") },
		Logger:     logx.New(io.Discard, logx.NewRedactor(), slog.LevelInfo),
	})
	_, err := c.Generate(context.Background(), textRequest())
	if apperr.CodeOf(err) != apperr.CodeAIKeyMissing || len(tr.requests) != 0 {
		t.Fatalf("code %q, %d requests", apperr.CodeOf(err), len(tr.requests))
	}
}

func TestR5_CancelDuringBackoffStops(t *testing.T) {
	h := newHarness(t,
		reply{status: 503, body: errBody(503, "UNAVAILABLE", "x", "")},
		reply{status: 200, body: okBody("too late")},
	)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := h.client.Generate(ctx, textRequest())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if len(h.tr.requests) > 1 {
		t.Fatalf("%d requests after cancel", len(h.tr.requests))
	}
}

func TestR34_AICallLogHasNoBodies(t *testing.T) {
	h := newHarness(t,
		reply{status: 503, body: errBody(503, "UNAVAILABLE", sentinelPrompt, "")},
		reply{status: 200, body: okBody(sentinelPrompt)},
	)
	if _, err := h.client.Generate(context.Background(), textRequest()); err != nil {
		t.Fatal(err)
	}
	out := h.log.String()
	leaks(t, "log", out)
	if strings.Count(out, `"msg":"ai call"`) != 2 {
		t.Fatalf("want one log line per attempt:\n%s", out)
	}
	for _, want := range []string{`"status":503`, `"status":200`, `"outcome":"retry"`, `"outcome":"ok"`} {
		if !strings.Contains(out, want) {
			t.Fatalf("log missing %s:\n%s", want, out)
		}
	}
}
