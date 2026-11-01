// Package ai is the provider-neutral interface LeGaJ's features call (D2,
// C9 ports & adapters). Gemini is the only adapter (internal/ai/gemini);
// tests use internal/ai/aitest.
//
// Contract every adapter keeps:
//   - Errors are *apperr.Error with an ai.* code and a user-safe message.
//   - When Request.Schema is set, Response.Text has already passed
//     ValidateJSON. Callers still decode it into a typed struct and run their
//     own semantic checks before anything is stored (R35).
//   - Request and response bodies are never logged (R34).
package ai

import (
	"context"
	"encoding/json"
)

// Part is one piece of user content: text, or inline file data.
type Part struct {
	Text     string
	MIMEType string
	Data     []byte
}

// Text is a text part.
func Text(s string) Part { return Part{Text: s} }

// Inline is a file part sent inline (e.g. a PDF resume, R8).
func Inline(mimeType string, data []byte) Part {
	return Part{MIMEType: mimeType, Data: data}
}

// Request is one generation call.
type Request struct {
	// Model overrides the provider's default (from Settings, R5/R37).
	Model string
	// System holds the instructions. Untrusted data never goes here; it goes
	// in Parts, inside delimited blocks (R35).
	System string
	Parts  []Part
	// Temperature is nil for the provider default.
	Temperature     *float64
	MaxOutputTokens int
	// Schema is a JSON Schema the output must match. nil means plain text.
	Schema json.RawMessage
}

// Response is the model's answer.
type Response struct {
	Text  string
	Model string
}

// Provider generates text from a request.
type Provider interface {
	Generate(ctx context.Context, req Request) (Response, error)
}
