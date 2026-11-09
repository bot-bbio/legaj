// Package gemini is the Google Gemini adapter for ai.Provider (R5).
//
// Wire format, checked against https://ai.google.dev/api/generate-content
// on 2026-11-01:
//   - POST /v1beta/models/{model}:generateContent with the key in the
//     x-goog-api-key header. The key is never in the URL, where net/http
//     errors would print it, and never in the body (R35).
//   - Structured output uses generationConfig.responseMimeType plus
//     responseJsonSchema. The reference now marks the schema fields
//     deprecated in favour of generationConfig.responseFormat.text
//     {mimeType, schema}; switching is a change to buildBody only, pending
//     the live smoke test. Output is validated client-side either way.
package gemini

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/bot-bbio/legaj/internal/ai"
	"github.com/bot-bbio/legaj/internal/apperr"
	"github.com/bot-bbio/legaj/internal/logx"
)

const (
	// DefaultModel is carried over from V1, where it worked on the free tier (D21).
	DefaultModel = "gemini-3.1-flash-lite"
	// MaxAttempts bounds retries of 429 and 5xx responses.
	MaxAttempts = 4

	baseURL          = "https://generativelanguage.googleapis.com/v1beta/models/"
	baseBackoff      = 2 * time.Second
	maxRetryWait     = 60 * time.Second
	maxResponseBytes = 16 << 20
	requestTimeout   = 2 * time.Minute
)

// Config wires the client's dependencies.
type Config struct {
	// HTTPClient defaults to one with a 2-minute timeout.
	HTTPClient *http.Client
	// Key returns the API key for each call (secrets.Manager.Load).
	Key func() (string, error)
	// Logger receives one line per attempt via logx.LogAICall.
	Logger *slog.Logger
	// Sleep waits between retries; tests inject a recorder.
	Sleep func(ctx context.Context, d time.Duration) error
}

// Client calls the Gemini API.
type Client struct {
	http  *http.Client
	key   func() (string, error)
	log   *slog.Logger
	sleep func(ctx context.Context, d time.Duration) error
}

var _ ai.Provider = (*Client)(nil)

// New creates a client.
func New(cfg Config) *Client {
	c := &Client{http: cfg.HTTPClient, key: cfg.Key, log: cfg.Logger, sleep: cfg.Sleep}
	if c.http == nil {
		c.http = &http.Client{Timeout: requestTimeout}
	}
	if c.sleep == nil {
		c.sleep = sleepCtx
	}
	if c.log == nil {
		c.log = slog.New(slog.DiscardHandler)
	}
	return c
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

var (
	// modelName is checked before it is placed in the URL path.
	modelName = regexp.MustCompile(`^[a-z0-9][a-z0-9.\-]{0,63}$`)
	// enumValue matches Google enum strings (INVALID_ARGUMENT, SAFETY).
	enumValue = regexp.MustCompile(`^[A-Z_]{1,40}$`)
)

// ValidModel reports whether name is safe to use as a model ID.
func ValidModel(name string) bool {
	return modelName.MatchString(name)
}

// Generate sends req, retrying 429 and 5xx with exponential backoff.
func (c *Client) Generate(ctx context.Context, req ai.Request) (ai.Response, error) {
	model := req.Model
	if model == "" {
		model = DefaultModel
	}
	if !modelName.MatchString(model) {
		return ai.Response{}, apperr.New(apperr.CodeInvalid, "That Gemini model name isn't valid. Pick one in Settings.")
	}
	body, err := buildBody(req)
	if err != nil {
		return ai.Response{}, apperr.Wrap(err, apperr.CodeInternal, apperr.GenericMessage)
	}
	key, err := c.key()
	if err != nil {
		return ai.Response{}, apperr.Public(err)
	}

	for attempt := 1; ; attempt++ {
		start := time.Now()
		res, err := c.attempt(ctx, model, key, body)
		outcome := "ok"
		switch {
		case err == nil:
		case res.retryable && attempt < MaxAttempts:
			outcome = "retry"
		default:
			outcome = "error"
		}
		logx.LogAICall(c.log, logx.AICall{Provider: "gemini", Model: model, Status: res.status, Attempt: attempt, Duration: time.Since(start), Outcome: outcome})
		if err == nil {
			return c.finish(res.body, model, req.Schema)
		}
		if outcome != "retry" {
			return ai.Response{}, err
		}
		if err := c.sleep(ctx, backoff(attempt, res.retryAfter)); err != nil {
			return ai.Response{}, cancelled(err)
		}
	}
}

func cancelled(err error) error {
	return apperr.Wrap(err, apperr.CodeAIUnavailable, "The Gemini request was cancelled.")
}

// backoff is the wait before attempt+1: the server's hint when given,
// otherwise 2s, 4s, 8s… with up to 25% jitter. Capped at a minute.
func backoff(attempt int, hint time.Duration) time.Duration {
	if hint > 0 {
		return min(hint, maxRetryWait)
	}
	d := baseBackoff << (attempt - 1)
	d += time.Duration(rand.Int64N(int64(d / 4)))
	return min(d, maxRetryWait)
}

// result is one HTTP attempt.
type result struct {
	status     int
	body       []byte
	retryable  bool
	retryAfter time.Duration
}

func (c *Client) attempt(ctx context.Context, model, key string, body []byte) (result, error) {
	if err := ctx.Err(); err != nil {
		return result{}, cancelled(err)
	}
	hreq, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+model+":generateContent", bytes.NewReader(body))
	if err != nil {
		return result{}, apperr.Wrap(err, apperr.CodeInternal, apperr.GenericMessage)
	}
	hreq.Header.Set("Content-Type", "application/json")
	hreq.Header.Set("x-goog-api-key", key)

	resp, err := c.http.Do(hreq)
	if err != nil {
		if ctx.Err() != nil {
			return result{}, cancelled(ctx.Err())
		}
		// Network trouble: retry, then report Gemini as unreachable. The
		// URL in the error holds no key.
		return result{retryable: true}, apperr.Wrap(err, apperr.CodeAIUnavailable, "LeGaJ couldn't reach Gemini. Check your internet connection.")
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return result{status: resp.StatusCode, retryable: true}, apperr.Wrap(err, apperr.CodeAIUnavailable, "The connection to Gemini dropped. Try again.")
	}
	res := result{status: resp.StatusCode, body: data}
	if resp.StatusCode == http.StatusOK {
		return res, nil
	}
	gerr := parseError(data)
	res.retryAfter = retryAfter(resp.Header.Get("Retry-After"), gerr)
	// The cause renders only the HTTP status and Google's enums, never the
	// error message, which can echo request content (see APIError).
	cause := newAPIError(resp.StatusCode, gerr)
	// R63's order: a bad key first, since Gemini reports one as a 400 too.
	switch {
	case resp.StatusCode == http.StatusUnauthorized, resp.StatusCode == http.StatusForbidden, gerr.keyInvalid():
		return res, apperr.Wrap(cause, apperr.CodeAIKeyRejected, "Gemini rejected the key. Check it in Settings.")
	case resp.StatusCode == http.StatusTooManyRequests:
		res.retryable = true
		return res, apperr.Wrap(cause, apperr.CodeAIRateLimited, "Gemini's rate limit was reached. Wait a minute and try again.")
	case resp.StatusCode >= 500:
		res.retryable = true
		return res, apperr.Wrap(cause, apperr.CodeAIUnavailable, "Gemini is busy or unavailable right now. Try again shortly.")
	case resp.StatusCode == http.StatusNotFound:
		return res, apperr.Wrap(cause, apperr.CodeInvalid, "The selected Gemini model isn't available. Pick another model in Settings.")
	case gerr.Status == "FAILED_PRECONDITION":
		// Gemini's documented meaning: the free tier isn't offered in the
		// user's country until billing is enabled.
		return res, apperr.Wrap(cause, apperr.CodeAIBilling, "Gemini's free tier isn't available in your region. Enable billing in Google AI Studio.")
	default:
		// Gemini named no reason the user could act on, so the likeliest
		// cause is LeGaJ's own request (as with V2's resume schema). Callers
		// that checked the input themselves may say more (resume: an
		// encrypted PDF).
		return res, apperr.Wrap(cause, apperr.CodeAIRequestRejected, "Gemini couldn't accept LeGaJ's request. If it keeps happening, please report it: it's most likely a LeGaJ bug.")
	}
}

// APIError is a Gemini HTTP error reduced to what LeGaJ may log: the HTTP
// status, Google's status enum and the reason enums from the error details
// (R34, R63). Every rendering of it (Error, %#v, JSON) holds only those.
// Gemini's own message can echo request content, such as resume text or a
// job description, so it is kept apart and read only through UnsafeMessage.
type APIError struct {
	HTTPStatus int
	Status     string   // e.g. INVALID_ARGUMENT
	Reasons    []string // e.g. API_KEY_INVALID
	message    string
}

func newAPIError(httpStatus int, g googleError) *APIError {
	e := &APIError{HTTPStatus: httpStatus, Status: g.Status, message: g.Message}
	for _, d := range g.Details {
		if enumValue.MatchString(d.Reason) {
			e.Reasons = append(e.Reasons, d.Reason)
		}
	}
	return e
}

func (e *APIError) Error() string {
	s := fmt.Sprintf("gemini: HTTP %d %s", e.HTTPStatus, e.Status)
	if len(e.Reasons) > 0 {
		s += " (" + strings.Join(e.Reasons, ", ") + ")"
	}
	return s
}

// GoString keeps %#v from printing the unexported message.
func (e *APIError) GoString() string { return e.Error() }

// UnsafeMessage returns Gemini's error message, which can contain request
// content: never log, show or store it. Only the live suite (R69) reads it,
// with synthetic data; a static test keeps it there.
func (e *APIError) UnsafeMessage() string { return e.message }

// googleError is the error envelope of Google APIs.
type googleError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Status  string `json:"status"`
	Details []struct {
		Type       string `json:"@type"`
		Reason     string `json:"reason"`
		RetryDelay string `json:"retryDelay"`
	} `json:"details"`
}

func parseError(data []byte) googleError {
	var env struct {
		Error googleError `json:"error"`
	}
	_ = json.Unmarshal(data, &env)
	// Status is an enum like INVALID_ARGUMENT; drop anything else so the
	// error cause can't carry arbitrary text.
	if !enumValue.MatchString(env.Error.Status) {
		env.Error.Status = ""
	}
	return env.Error
}

// keyInvalid: Gemini answers a bad key with 400 INVALID_ARGUMENT and reason
// API_KEY_INVALID, not 401.
func (g googleError) keyInvalid() bool {
	for _, d := range g.Details {
		if d.Reason == "API_KEY_INVALID" {
			return true
		}
	}
	return strings.Contains(g.Message, "API key not valid")
}

// retryAfter reads the Retry-After header (seconds) or Google's RetryInfo.
func retryAfter(header string, g googleError) time.Duration {
	if s, err := strconv.Atoi(strings.TrimSpace(header)); err == nil && s > 0 {
		return time.Duration(s) * time.Second
	}
	for _, d := range g.Details {
		if strings.HasSuffix(d.Type, "RetryInfo") {
			if dur, err := time.ParseDuration(d.RetryDelay); err == nil && dur > 0 {
				return dur
			}
		}
	}
	return 0
}

// generateResponse is the subset of the response LeGaJ reads.
type generateResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text    string `json:"text"`
				Thought bool   `json:"thought"`
			} `json:"parts"`
		} `json:"content"`
		FinishReason string `json:"finishReason"`
	} `json:"candidates"`
	PromptFeedback *struct {
		BlockReason string `json:"blockReason"`
	} `json:"promptFeedback"`
	ModelVersion string `json:"modelVersion"`
}

func (c *Client) finish(data []byte, model string, schema json.RawMessage) (ai.Response, error) {
	var gr generateResponse
	if err := json.Unmarshal(data, &gr); err != nil {
		return ai.Response{}, apperr.Wrap(errors.New("gemini: 200 with a non-JSON body"), apperr.CodeAIUnavailable, "Gemini sent an unreadable reply. Try again.")
	}
	declined := apperr.CodeAIInvalidOutput
	if gr.PromptFeedback != nil && gr.PromptFeedback.BlockReason != "" {
		return ai.Response{}, apperr.Wrap(errors.New("gemini: prompt blocked"), declined, "Gemini declined to answer this request.")
	}
	if len(gr.Candidates) == 0 {
		return ai.Response{}, apperr.Wrap(errors.New("gemini: no candidates"), declined, "Gemini returned no answer. Try again.")
	}
	cand := gr.Candidates[0]
	switch cand.FinishReason {
	case "STOP", "":
	case "MAX_TOKENS":
		return ai.Response{}, apperr.Wrap(errors.New("gemini: finish MAX_TOKENS"), declined, "Gemini's answer was cut off. Try again.")
	default:
		return ai.Response{}, apperr.Wrap(errors.New("gemini: finish "+safeEnum(cand.FinishReason)), declined, "Gemini declined to answer this request.")
	}
	var sb strings.Builder
	for _, p := range cand.Content.Parts {
		if !p.Thought {
			sb.WriteString(p.Text)
		}
	}
	text := sb.String()
	if len(schema) > 0 {
		if err := ai.ValidateJSON(schema, text); err != nil {
			return ai.Response{}, err
		}
	}
	if gr.ModelVersion != "" {
		model = gr.ModelVersion
	}
	return ai.Response{Text: text, Model: model}, nil
}

func safeEnum(s string) string {
	if enumValue.MatchString(s) {
		return s
	}
	return "UNKNOWN"
}

// Request wire types.
type (
	inlineData struct {
		MimeType string `json:"mimeType"`
		Data     []byte `json:"data"` // base64 via encoding/json
	}
	part struct {
		Text       string      `json:"text,omitempty"`
		InlineData *inlineData `json:"inlineData,omitempty"`
	}
	content struct {
		Role  string `json:"role,omitempty"`
		Parts []part `json:"parts"`
	}
	generationConfig struct {
		Temperature        *float64        `json:"temperature,omitempty"`
		MaxOutputTokens    int             `json:"maxOutputTokens,omitempty"`
		ResponseMimeType   string          `json:"responseMimeType,omitempty"`
		ResponseJSONSchema json.RawMessage `json:"responseJsonSchema,omitempty"`
	}
	generateRequest struct {
		SystemInstruction *content          `json:"systemInstruction,omitempty"`
		Contents          []content         `json:"contents"`
		GenerationConfig  *generationConfig `json:"generationConfig,omitempty"`
	}
)

func buildBody(req ai.Request) ([]byte, error) {
	if len(req.Parts) == 0 {
		return nil, errors.New("gemini: request has no parts")
	}
	user := content{Role: "user"}
	for _, p := range req.Parts {
		switch {
		case p.Data != nil:
			user.Parts = append(user.Parts, part{InlineData: &inlineData{MimeType: p.MIMEType, Data: p.Data}})
		case p.Text != "":
			user.Parts = append(user.Parts, part{Text: p.Text})
		default:
			return nil, errors.New("gemini: empty part")
		}
	}
	gr := generateRequest{Contents: []content{user}}
	if req.System != "" {
		gr.SystemInstruction = &content{Parts: []part{{Text: req.System}}}
	}
	if req.Temperature != nil || req.MaxOutputTokens > 0 || len(req.Schema) > 0 {
		gc := &generationConfig{Temperature: req.Temperature, MaxOutputTokens: req.MaxOutputTokens}
		if len(req.Schema) > 0 {
			gc.ResponseMimeType = "application/json"
			gc.ResponseJSONSchema = req.Schema
		}
		gr.GenerationConfig = gc
	}
	return json.Marshal(gr)
}
