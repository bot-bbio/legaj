package pipeline_test

import (
	"database/sql"
	"fmt"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/bot-bbio/legaj/internal/apperr"
	"github.com/bot-bbio/legaj/internal/events"
	"github.com/bot-bbio/legaj/internal/pipeline"
	"github.com/bot-bbio/legaj/internal/store"
)

// manualTimers lets a test fire the purge timer instead of waiting 10 s.
type manualTimers struct {
	mu    sync.Mutex
	fns   []func()
	delay []time.Duration
}

type stopper struct{ stopped *bool }

func (s stopper) Stop() bool { *s.stopped = true; return true }

func (m *manualTimers) after(d time.Duration, f func()) pipeline.Timer {
	m.mu.Lock()
	defer m.mu.Unlock()
	stopped := false
	m.fns = append(m.fns, func() {
		if !stopped {
			f()
		}
	})
	m.delay = append(m.delay, d)
	return stopper{&stopped}
}

func (m *manualTimers) fireAll() {
	m.mu.Lock()
	fns := m.fns
	m.fns = nil
	m.mu.Unlock()
	for _, f := range fns {
		f()
	}
}

type svcFixture struct {
	*fixture
	svc       *pipeline.Service
	timers    *manualTimers
	rec       *events.Recorder
	cancelled [][]int64
	cmu       sync.Mutex
}

func newSvc(t *testing.T) *svcFixture {
	t.Helper()
	f := newFixture(t)
	sf := &svcFixture{fixture: f, timers: &manualTimers{}, rec: &events.Recorder{}}
	sf.svc = pipeline.NewService(pipeline.Config{
		DB: f.db, Bus: events.NewBus(sf.rec), Machine: f.m, Now: func() time.Time { return f.t0 },
		AfterFunc: sf.timers.after,
	})
	sf.svc.OnDelete(func(ids []int64) {
		sf.cmu.Lock()
		defer sf.cmu.Unlock()
		sf.cancelled = append(sf.cancelled, append([]int64(nil), ids...))
	})
	return sf
}

func (sf *svcFixture) add(t *testing.T, company, role, url string) int64 {
	t.Helper()
	id, err := sf.svc.Create(ctx, pipeline.NewLead{Company: company, Role: role, URL: url, CanonicalURL: url, Description: "JD for " + company, Source: store.SourcePaste})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func (sf *svcFixture) liveIDs(t *testing.T) []int64 {
	t.Helper()
	apps, err := sf.svc.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var ids []int64
	for _, a := range apps {
		ids = append(ids, a.ID)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

func eq(a, b []int64) bool { return fmt.Sprint(a) == fmt.Sprint(b) }

func TestR13_BulkDeleteExactIDs(t *testing.T) {
	sf := newSvc(t)
	a := sf.add(t, "Acme", "Dev", "https://jobs.example/1")
	twin1 := sf.add(t, "Globex", "Analyst", "")
	twin2 := sf.add(t, "Globex", "Analyst", "") // same company + role (U4)
	b := sf.add(t, "Initech", "QA", "https://jobs.example/2")
	c := sf.add(t, "Umbrella", "Ops", "https://jobs.example/3")
	before := len(sf.rec.Events())

	res, err := sf.svc.Delete(ctx, []int64{c, twin1, a, a}) // unordered, with a duplicate
	if err != nil {
		t.Fatal(err)
	}
	if !eq(res.IDs, []int64{a, twin1, c}) || res.UndoID == "" {
		t.Fatalf("result = %+v", res)
	}
	if got := sf.liveIDs(t); !eq(got, []int64{twin2, b}) {
		t.Fatalf("live = %v, want only the twin and the untouched row", got)
	}
	if len(sf.cancelled) != 1 || !eq(sf.cancelled[0], []int64{a, twin1, c}) {
		t.Fatalf("cancel hook got %v (R13: queued jobs for deleted rows must be cancelled)", sf.cancelled)
	}
	evs := sf.rec.Events()[before:]
	if len(evs) != 1 || !eq(evs[0].Payload.(events.Changed).IDs, []int64{a, twin1, c}) {
		t.Fatalf("delete emitted %+v, want one change naming exactly the deleted IDs", evs)
	}
}

func TestR13_DeleteIgnoresMissingAndAlreadyDeleted(t *testing.T) {
	sf := newSvc(t)
	a := sf.add(t, "Acme", "Dev", "")
	if _, err := sf.svc.Delete(ctx, []int64{a}); err != nil {
		t.Fatal(err)
	}
	res, err := sf.svc.Delete(ctx, []int64{a, 9999})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.IDs) != 0 {
		t.Fatalf("deleted %v again", res.IDs)
	}
}

func count(t *testing.T, db *store.DB, q string, args ...any) int {
	t.Helper()
	var n int
	if err := db.Reader().QueryRowContext(ctx, q, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func addChildren(t *testing.T, sf *svcFixture, id int64) {
	t.Helper()
	now := store.FormatTime(sf.t0)
	err := sf.db.Tx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `INSERT INTO documents (application_id, kind, status, body, created_at, updated_at) VALUES (?, 'letter', 'draft', '{}', ?, ?)`, id, now, now); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO rounds (application_id, starts_at, zone, created_at, updated_at) VALUES (?, ?, 'UTC', ?, ?)`, id, now, now, now)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestR13_UndoRestoresWithChildren(t *testing.T) {
	sf := newSvc(t)
	ids := []int64{sf.add(t, "A", "r", ""), sf.add(t, "B", "r", ""), sf.add(t, "C", "r", "")}
	for _, id := range ids {
		addChildren(t, sf, id)
	}
	eventCount := func(id int64) int {
		return count(t, sf.db, `SELECT count(*) FROM stage_events WHERE application_id = ?`, id)
	}

	res, err := sf.svc.Delete(ctx, ids)
	if err != nil {
		t.Fatal(err)
	}
	if len(sf.liveIDs(t)) != 0 {
		t.Fatal("rows still listed after delete")
	}
	restored, err := sf.svc.Undo(ctx, res.UndoID)
	if err != nil {
		t.Fatal(err)
	}
	if !eq(restored.IDs, ids) || len(restored.Skipped) != 0 {
		t.Fatalf("undo = %+v", restored)
	}
	if got := sf.liveIDs(t); !eq(got, ids) {
		t.Fatalf("live after undo = %v", got)
	}
	for _, id := range ids {
		if count(t, sf.db, `SELECT count(*) FROM documents WHERE application_id = ?`, id) != 1 ||
			count(t, sf.db, `SELECT count(*) FROM rounds WHERE application_id = ?`, id) != 1 || eventCount(id) != 1 {
			t.Fatalf("row %d lost children", id)
		}
	}

	// The purge timer from the undone batch must not delete the restored rows.
	sf.timers.fireAll()
	if got := sf.liveIDs(t); !eq(got, ids) {
		t.Fatalf("purge removed restored rows: %v", got)
	}
}

func TestR13_PurgeAfterWindowRemovesChildrenAndUndoFails(t *testing.T) {
	sf := newSvc(t)
	id := sf.add(t, "A", "r", "")
	addChildren(t, sf, id)
	res, err := sf.svc.Delete(ctx, []int64{id})
	if err != nil {
		t.Fatal(err)
	}
	if len(sf.timers.delay) != 1 || sf.timers.delay[0] != pipeline.UndoWindow || pipeline.UndoWindow != 10*time.Second {
		t.Fatalf("purge scheduled after %v, want the 10 s undo window", sf.timers.delay)
	}
	sf.timers.fireAll()

	if n := count(t, sf.db, `SELECT count(*) FROM applications WHERE id = ?`, id); n != 0 {
		t.Fatal("row not purged")
	}
	if n := count(t, sf.db, `SELECT count(*) FROM documents WHERE application_id = ?`, id); n != 0 {
		t.Fatal("children not purged")
	}
	if _, err := sf.svc.Undo(ctx, res.UndoID); apperr.CodeOf(err) != apperr.CodeConflict {
		t.Fatalf("late undo: err = %v, want conflict", err)
	}
}

func TestR13_SoftDeletedIgnoredByDedupeAndLists(t *testing.T) {
	sf := newSvc(t)
	const u = "https://jobs.example/123"
	first := sf.add(t, "Acme", "Dev", u)
	res, err := sf.svc.Delete(ctx, []int64{first})
	if err != nil {
		t.Fatal(err)
	}
	// Re-clip within the 10 s window: a new row, no unique-index error.
	second := sf.add(t, "Acme", "Dev", u)
	if second == first {
		t.Fatal("re-adding reused the deleted row")
	}
	if got := sf.liveIDs(t); !eq(got, []int64{second}) {
		t.Fatalf("live = %v", got)
	}
	if _, err := sf.svc.Get(ctx, first); apperr.CodeOf(err) != apperr.CodeNotFound {
		t.Fatalf("Get of a deleted row: %v", err)
	}

	// Undo can't bring back a duplicate of the live row; it reports it.
	undo, err := sf.svc.Undo(ctx, res.UndoID)
	if err != nil {
		t.Fatal(err)
	}
	if len(undo.IDs) != 0 || len(undo.Skipped) != 1 || undo.Skipped[0] != first {
		t.Fatalf("undo = %+v", undo)
	}
}

func TestR13_ConcurrentDeleteRaceClean(t *testing.T) {
	sf := newSvc(t)
	var ids []int64
	for i := range 40 {
		ids = append(ids, sf.add(t, fmt.Sprintf("Co%d", i), "Dev", ""))
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	deleted := map[int64]int{}
	for g := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Overlapping windows: every ID is targeted by two goroutines.
			batch := ids[g*5 : min(len(ids), g*5+10)]
			res, err := sf.svc.Delete(ctx, batch)
			if err != nil {
				t.Error(err)
				return
			}
			mu.Lock()
			for _, id := range res.IDs {
				deleted[id]++
			}
			mu.Unlock()
			if _, err := sf.svc.List(ctx); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	for _, id := range ids {
		if deleted[id] != 1 {
			t.Fatalf("id %d deleted by %d batches, want exactly 1", id, deleted[id])
		}
	}
	if len(sf.liveIDs(t)) != 0 {
		t.Fatal("rows survived")
	}
	sf.timers.fireAll() // concurrent purges
	if n := count(t, sf.db, `SELECT count(*) FROM applications`); n != 0 {
		t.Fatalf("%d rows left after purge", n)
	}
}

func TestR12_BulkStageStableOrder(t *testing.T) {
	sf := newSvc(t)
	a := sf.add(t, "A", "r", "")
	b := sf.add(t, "B", "r", "")
	c := sf.add(t, "C", "r", "")
	// c can't go back to Lead later; put it in Offer first.
	if _, err := sf.svc.SetStage(ctx, []int64{c}, store.StageOffer); err != nil {
		t.Fatal(err)
	}

	res, err := sf.svc.SetStage(ctx, []int64{c, b, a, b, 9999}, store.StageApplied)
	if err != nil {
		t.Fatal(err)
	}
	if !eq(res.Changed, []int64{a, b}) {
		t.Fatalf("changed = %v, want [a b] in id order", res.Changed)
	}
	if len(res.Skipped) != 2 || res.Skipped[0].ID != c || res.Skipped[1].ID != 9999 || res.Skipped[0].Reason == "" {
		t.Fatalf("skipped = %+v", res.Skipped)
	}

	// Events were written in id order, regardless of the input order.
	rows, err := sf.db.Reader().QueryContext(ctx, `SELECT application_id FROM stage_events WHERE to_stage = 'applied' ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var order []int64
	for rows.Next() {
		var id int64
		_ = rows.Scan(&id)
		order = append(order, id)
	}
	if !eq(order, []int64{a, b}) {
		t.Fatalf("event order = %v", order)
	}
}

func TestR16_ListCarriesDescription(t *testing.T) {
	sf := newSvc(t)
	id := sf.add(t, "Acme", "Dev", "")
	app, err := sf.svc.Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if app.Description != "JD for Acme" || app.Stage != store.StageLead || app.Source != store.SourcePaste {
		t.Fatalf("app = %+v", app)
	}
}

func TestR13_PurgeOnStartRemovesLeftovers(t *testing.T) {
	sf := newSvc(t)
	id := sf.add(t, "A", "r", "")
	if _, err := sf.svc.Delete(ctx, []int64{id}); err != nil {
		t.Fatal(err)
	}
	// The app quit before the timer fired; the next start cleans up.
	n, err := sf.svc.PurgeDeleted(ctx)
	if err != nil || n != 1 {
		t.Fatalf("purged %d, %v", n, err)
	}
}

func TestR9_UpdateDetailsFixesNeedsCompany(t *testing.T) {
	sf := newSvc(t)
	id := sf.add(t, "", "Dev", "")
	app, err := sf.svc.UpdateDetails(ctx, id, pipeline.Details{Company: "  Acme  ", Role: "Dev", Location: "Remote", Notes: "referral"})
	if err != nil {
		t.Fatal(err)
	}
	if app.Company != "Acme" || app.Description != "JD for " {
		t.Fatalf("app = %+v; the description must survive a details edit (R16)", app)
	}
}
