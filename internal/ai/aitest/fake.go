// Package aitest provides a scripted ai.Provider for tests. It never makes
// network calls, and it validates structured output exactly like the real
// adapter, so callers' rejection paths are exercised.
package aitest

import (
	"context"
	"errors"
	"sync"

	"github.com/bot-bbio/legaj/internal/ai"
)

// Fake is an ai.Provider that replies from a queue, or from Respond.
type Fake struct {
	mu       sync.Mutex
	queue    []reply
	requests []ai.Request
	// Respond, when set, answers any request the queue doesn't.
	Respond func(req ai.Request) (string, error)
}

type reply struct {
	text string
	err  error
}

var _ ai.Provider = (*Fake)(nil)

// ErrNoReply is returned when the queue is empty and Respond is nil.
var ErrNoReply = errors.New("aitest: no scripted reply")

// Push queues a text reply.
func (f *Fake) Push(text string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.queue = append(f.queue, reply{text: text})
}

// PushErr queues an error reply.
func (f *Fake) PushErr(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.queue = append(f.queue, reply{err: err})
}

// Requests returns a copy of every request received, in order.
func (f *Fake) Requests() []ai.Request {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]ai.Request(nil), f.requests...)
}

// Generate records req and returns the next scripted reply.
func (f *Fake) Generate(ctx context.Context, req ai.Request) (ai.Response, error) {
	if err := ctx.Err(); err != nil {
		return ai.Response{}, err
	}
	f.mu.Lock()
	f.requests = append(f.requests, req)
	var r reply
	switch {
	case len(f.queue) > 0:
		r, f.queue = f.queue[0], f.queue[1:]
	case f.Respond != nil:
		respond := f.Respond
		f.mu.Unlock()
		text, err := respond(req)
		f.mu.Lock()
		r = reply{text: text, err: err}
	default:
		r = reply{err: ErrNoReply}
	}
	f.mu.Unlock()

	if r.err != nil {
		return ai.Response{}, r.err
	}
	if len(req.Schema) > 0 {
		if err := ai.ValidateJSON(req.Schema, r.text); err != nil {
			return ai.Response{}, err
		}
	}
	return ai.Response{Text: r.text, Model: "fake"}, nil
}
