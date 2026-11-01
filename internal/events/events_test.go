package events_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/bot-bbio/legaj/internal/events"
	"github.com/bot-bbio/legaj/internal/store"
	"github.com/bot-bbio/legaj/internal/testenv"
)

func TestMain(m *testing.M) { testenv.Main(m) }

var ctx = context.Background()

func openDB(t *testing.T) *store.DB {
	t.Helper()
	db, st, err := store.Open(ctx, store.Options{Dir: testenv.Isolate(t)})
	if err != nil || st.Kind != store.StartupReady {
		t.Fatalf("open: %v %+v", err, st)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func insertSetting(tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO settings(key, value) VALUES ('model', '"x"')`)
	return err
}

func TestR3_MutationEmitsChangedEvent(t *testing.T) {
	db := openDB(t)
	var seenAtEmit int
	rec := &events.Recorder{OnEmit: func(string, any) {
		// The change must already be committed when listeners hear about it.
		_ = db.Reader().QueryRowContext(ctx, `SELECT count(*) FROM settings`).Scan(&seenAtEmit)
	}}
	bus := events.NewBus(rec)

	err := events.Mutate(ctx, db, bus, events.Change(events.EntitySettings), insertSetting)
	if err != nil {
		t.Fatal(err)
	}
	got := rec.Events()
	if len(got) != 1 || got[0].Name != events.ChangedEvent {
		t.Fatalf("events = %+v, want one %s", got, events.ChangedEvent)
	}
	payload, ok := got[0].Payload.(events.Changed)
	if !ok || payload.Entity != events.EntitySettings {
		t.Fatalf("payload = %#v", got[0].Payload)
	}
	if seenAtEmit != 1 {
		t.Fatal("event emitted before the transaction committed")
	}
}

func TestR3_FailedMutationEmitsNothing(t *testing.T) {
	db := openDB(t)
	rec := &events.Recorder{}
	bus := events.NewBus(rec)
	boom := errors.New("boom")
	err := events.Mutate(ctx, db, bus, events.Change(events.EntityApplication, 7), func(tx *sql.Tx) error {
		if err := insertSetting(tx); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
	if n := len(rec.Events()); n != 0 {
		t.Fatalf("%d events for a rolled-back change", n)
	}
}

func TestR3_ChangedCarriesIDs(t *testing.T) {
	rec := &events.Recorder{}
	events.NewBus(rec).Emit(events.Change(events.EntityApplication, 3, 1, 2))
	p := rec.Events()[0].Payload.(events.Changed)
	if len(p.IDs) != 3 || p.IDs[0] != 3 {
		t.Fatalf("ids = %v", p.IDs)
	}
}

func TestR3_NilBusIsSafe(t *testing.T) {
	var bus *events.Bus
	bus.Emit(events.Change(events.EntityProfile)) // must not panic
}
