// Package events tells the frontend that backend data changed (R3).
//
// The UI keeps no copy of its own (U1): each frontend store listens for
// entity:changed and refetches. Writes go through Mutate, which emits only
// after the transaction commits, so listeners never refetch data that was
// rolled back, and always see the new state when they do.
package events

import (
	"context"
	"database/sql"
	"sync"

	"github.com/bot-bbio/legaj/internal/store"
)

// ChangedEvent is the event name the frontend subscribes to.
const ChangedEvent = "entity:changed"

// Entity names a kind of data the frontend caches in a store.
type Entity string

// Entities.
const (
	EntityApplication Entity = "application"
	EntityProfile     Entity = "profile"
	EntitySettings    Entity = "settings"
	EntityResume      Entity = "resume"
	EntityDocument    Entity = "document"
	EntityAnswer      Entity = "answer"
	EntityRound       Entity = "round"
	EntityFollowup    Entity = "followup"
	EntityJob         Entity = "job"
)

// Changed is the payload of ChangedEvent. IDs is empty when the change
// isn't about specific rows (e.g. settings).
type Changed struct {
	Entity Entity  `json:"entity"`
	IDs    []int64 `json:"ids"`
}

// Change builds a payload.
func Change(e Entity, ids ...int64) Changed {
	if ids == nil {
		ids = []int64{}
	}
	return Changed{Entity: e, IDs: ids}
}

// Emitter delivers an event to the frontend (Wails runtime.EventsEmit).
type Emitter interface {
	Emit(name string, payload any)
}

// Bus emits change notifications.
type Bus struct {
	emitter Emitter
}

// NewBus wraps an emitter.
func NewBus(e Emitter) *Bus {
	return &Bus{emitter: e}
}

// Emit sends a change notification. A nil Bus or emitter drops it.
func (b *Bus) Emit(c Changed) {
	if b == nil || b.emitter == nil {
		return
	}
	b.emitter.Emit(ChangedEvent, c)
}

// Mutate runs fn in a write transaction and, only if it commits, emits c.
func Mutate(ctx context.Context, db *store.DB, b *Bus, c Changed, fn func(tx *sql.Tx) error) error {
	if err := db.Tx(ctx, fn); err != nil {
		return err
	}
	b.Emit(c)
	return nil
}

// Recorded is one emitted event.
type Recorded struct {
	Name    string
	Payload any
}

// Recorder is an Emitter for tests. OnEmit, if set, runs on each event.
type Recorder struct {
	mu     sync.Mutex
	events []Recorded
	OnEmit func(name string, payload any)
}

// Emit records the event.
func (r *Recorder) Emit(name string, payload any) {
	r.mu.Lock()
	r.events = append(r.events, Recorded{Name: name, Payload: payload})
	hook := r.OnEmit
	r.mu.Unlock()
	if hook != nil {
		hook(name, payload)
	}
}

// Events returns a copy of everything emitted so far.
func (r *Recorder) Events() []Recorded {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]Recorded(nil), r.events...)
}
