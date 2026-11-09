// Package apperr separates what a user may see from what developers need
// (R34). An *Error carries a stable code and a user-safe message; its cause
// stays reachable through errors.Is/As and the log (LogValue), but never
// appears in Error(), fmt verbs or JSON, which is what the frontend sees.
package apperr

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
)

// Code is a stable, machine-readable error category the frontend can switch on.
type Code string

// Error codes. Messages are written per call site; codes are shared.
const (
	CodeInternal        Code = "internal"
	CodeInvalid         Code = "invalid"
	CodeNotFound        Code = "not_found"
	CodeConflict        Code = "conflict"
	CodeStorage         Code = "storage"
	CodeAIKeyMissing    Code = "ai.key_missing"
	CodeAIKeyRejected   Code = "ai.key_rejected"
	CodeAIRateLimited   Code = "ai.rate_limited"
	CodeAIUnavailable   Code = "ai.unavailable"
	CodeAIInvalidOutput Code = "ai.invalid_output"
	// CodeAIBilling: the provider refuses the free tier here (Gemini's
	// FAILED_PRECONDITION); the key works once billing is on (R63).
	CodeAIBilling Code = "ai.billing_required"
	// CodeAIRequestRejected: the provider refused LeGaJ's request for a
	// reason it doesn't name: most likely LeGaJ's bug, never assumed to be
	// the user's file (R63).
	CodeAIRequestRejected Code = "ai.request_rejected"
)

// GenericMessage is shown for any error that isn't an *Error.
const GenericMessage = "Something went wrong. The details were saved to LeGaJ's log."

// Error is a user-facing error.
type Error struct {
	code    Code
	message string
	cause   error
}

// New returns an error with no underlying cause.
func New(code Code, message string) *Error {
	return &Error{code: code, message: message}
}

// Wrap attaches a user-safe code and message to an internal cause.
func Wrap(cause error, code Code, message string) *Error {
	return &Error{code: code, message: message, cause: cause}
}

// Error returns the user-safe message only. Wails sends this string to the
// frontend when a bound method returns the error.
func (e *Error) Error() string { return e.message }

// Unwrap exposes the cause to errors.Is/As (for handling and logging).
func (e *Error) Unwrap() error { return e.cause }

// Code returns the error's category.
func (e *Error) Code() Code { return e.code }

// GoString keeps %#v from printing the unexported cause field.
func (e *Error) GoString() string {
	return fmt.Sprintf("apperr.Error{Code:%q, Message:%q}", e.code, e.message)
}

// MarshalJSON emits only the code and the user-safe message.
func (e *Error) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Code    Code   `json:"code"`
		Message string `json:"message"`
	}{e.code, e.message})
}

// LogValue is what slog records: code, message and the cause's text. The
// log handler (internal/logx) redacts secrets and PII from it.
func (e *Error) LogValue() slog.Value {
	attrs := []slog.Attr{slog.String("code", string(e.code)), slog.String("message", e.message)}
	if e.cause != nil {
		attrs = append(attrs, slog.String("cause", e.cause.Error()))
	}
	return slog.GroupValue(attrs...)
}

// CodeOf returns the code of the first *Error in err's chain, or CodeInternal.
func CodeOf(err error) Code {
	var e *Error
	if errors.As(err, &e) {
		return e.code
	}
	return CodeInternal
}

// UserMessage returns the user-safe message for any error.
func UserMessage(err error) string {
	var e *Error
	if errors.As(err, &e) {
		return e.message
	}
	return GenericMessage
}

// Public converts any error into one safe to return to the frontend: the
// first *Error in the chain, or a generic internal error wrapping err.
func Public(err error) error {
	if err == nil {
		return nil
	}
	var e *Error
	if errors.As(err, &e) {
		return e
	}
	return &Error{code: CodeInternal, message: GenericMessage, cause: err}
}
