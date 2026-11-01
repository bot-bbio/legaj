// Package logx is LeGaJ's only logger (R34). It writes JSON lines and
// guarantees, in two layers, that no secret or PII reaches the log:
//
//  1. The slog handler drops over-long values and redacts every message and
//     attribute (strings, errors, groups, structs) before formatting.
//  2. The output writer redacts each finished line again, which also
//     catches attributes pre-formatted by With() before a secret was
//     registered.
//
// AI calls are logged only through LogAICall, whose AICall type has no
// field that could hold a prompt or a response.
package logx

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

// New returns a JSON logger writing to w through the redactor.
func New(w io.Writer, r *Redactor, level slog.Level) *slog.Logger {
	inner := slog.NewJSONHandler(&redactWriter{w: w, r: r}, &slog.HandlerOptions{Level: level})
	return slog.New(&handler{inner: inner, r: r})
}

// Log file location and rotation.
const (
	LogDir      = "logs"
	logFileName = "legaj.log"
	maxLogBytes = 5 << 20
)

// NewFile opens <dataDir>/logs/legaj.log for appending. When the file is
// over 5 MB at open, it is rotated to legaj.log.1 (one generation kept).
func NewFile(dataDir string, r *Redactor, level slog.Level) (*slog.Logger, io.Closer, error) {
	dir := filepath.Join(dataDir, LogDir)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, nil, err
	}
	path := filepath.Join(dir, logFileName)
	if fi, err := os.Stat(path); err == nil && fi.Size() > maxLogBytes {
		if err := os.Rename(path, path+".1"); err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, nil, err
		}
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, nil, err
	}
	return New(f, r, level), f, nil
}

// AICall is everything ever logged about an AI request: who, what model,
// the HTTP status, and timing. There is deliberately no body field.
type AICall struct {
	Provider string
	Model    string
	Status   int // HTTP status; 0 when no response arrived
	Attempt  int
	Duration time.Duration
	Outcome  string // "ok", "retry" or "error"
}

// LogAICall records one AI request attempt.
func LogAICall(l *slog.Logger, c AICall) {
	level := slog.LevelInfo
	if c.Outcome != "ok" {
		level = slog.LevelWarn
	}
	l.LogAttrs(context.Background(), level, "ai call",
		slog.String("provider", c.Provider),
		slog.String("model", c.Model),
		slog.Int("status", c.Status),
		slog.Int("attempt", c.Attempt),
		slog.Int64("duration_ms", c.Duration.Milliseconds()),
		slog.String("outcome", c.Outcome),
	)
}

// handler scrubs records before the inner JSON handler formats them.
type handler struct {
	inner slog.Handler
	r     *Redactor
}

func (h *handler) Enabled(ctx context.Context, l slog.Level) bool {
	return h.inner.Enabled(ctx, l)
}

func (h *handler) Handle(ctx context.Context, rec slog.Record) error {
	out := slog.NewRecord(rec.Time, rec.Level, h.r.scrub(rec.Message), rec.PC)
	rec.Attrs(func(a slog.Attr) bool {
		out.AddAttrs(h.clean(a))
		return true
	})
	return h.inner.Handle(ctx, out)
}

func (h *handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	cleaned := make([]slog.Attr, len(attrs))
	for i, a := range attrs {
		cleaned[i] = h.clean(a)
	}
	return &handler{inner: h.inner.WithAttrs(cleaned), r: h.r}
}

func (h *handler) WithGroup(name string) slog.Handler {
	return &handler{inner: h.inner.WithGroup(name), r: h.r}
}

// clean resolves LogValuers (e.g. apperr.Error) and scrubs every string
// inside the attribute, recursing into groups.
func (h *handler) clean(a slog.Attr) slog.Attr {
	v := a.Value.Resolve()
	switch v.Kind() {
	case slog.KindString:
		return slog.String(a.Key, h.r.scrub(v.String()))
	case slog.KindGroup:
		group := v.Group()
		out := make([]slog.Attr, len(group))
		for i, g := range group {
			out[i] = h.clean(g)
		}
		return slog.Attr{Key: a.Key, Value: slog.GroupValue(out...)}
	case slog.KindAny:
		if err, ok := v.Any().(error); ok {
			return slog.String(a.Key, h.r.scrub(err.Error()))
		}
		return slog.String(a.Key, h.r.scrub(fmt.Sprintf("%+v", v.Any())))
	default:
		return slog.Attr{Key: a.Key, Value: v}
	}
}

// redactWriter redacts each formatted line before it reaches w.
type redactWriter struct {
	w io.Writer
	r *Redactor
}

func (rw *redactWriter) Write(p []byte) (int, error) {
	if _, err := rw.w.Write([]byte(rw.r.Redact(string(p)))); err != nil {
		return 0, err
	}
	return len(p), nil
}
