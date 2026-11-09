package pipeline_test

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/bot-bbio/legaj/internal/apperr"
	"github.com/bot-bbio/legaj/internal/pipeline"
	"github.com/bot-bbio/legaj/internal/store"
	"github.com/bot-bbio/legaj/internal/testenv"
)

func TestMain(m *testing.M) { testenv.Main(m) }

var ctx = context.Background()

type fixture struct {
	db *store.DB
	m  *pipeline.Machine
	t0 time.Time
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	db, st, err := store.Open(ctx, store.Options{Dir: testenv.Isolate(t)})
	if err != nil || st.Kind != store.StartupReady {
		t.Fatalf("open: %v %+v", err, st)
	}
	t.Cleanup(func() { _ = db.Close() })
	t0 := time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)
	return &fixture{db: db, m: pipeline.NewMachine(func() time.Time { return t0 }), t0: t0}
}

func (f *fixture) tx(t *testing.T, fn func(tx *sql.Tx) error) error {
	t.Helper()
	return f.db.Tx(ctx, fn)
}

func (f *fixture) lead(t *testing.T) int64 {
	t.Helper()
	var id int64
	err := f.tx(t, func(tx *sql.Tx) error {
		var err error
		id, err = f.m.CreateLead(ctx, tx, pipeline.NewLead{Company: "Acme", Role: "Dev", Description: "JD", Source: store.SourceClip})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// at puts an application directly into a stage (test setup only).
func (f *fixture) at(t *testing.T, s store.Stage) int64 {
	t.Helper()
	id := f.lead(t)
	err := f.tx(t, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE applications SET stage = ? WHERE id = ?`, string(s), id)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func (f *fixture) stage(t *testing.T, id int64) store.Stage {
	t.Helper()
	var s string
	if err := f.db.Reader().QueryRowContext(ctx, `SELECT stage FROM applications WHERE id = ?`, id).Scan(&s); err != nil {
		t.Fatal(err)
	}
	return store.Stage(s)
}

func (f *fixture) events(t *testing.T, id int64) []store.StageEvent {
	t.Helper()
	rows, err := f.db.Reader().QueryContext(ctx, `SELECT from_stage, to_stage, cause, reason, at FROM stage_events WHERE application_id = ? ORDER BY id`, id)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []store.StageEvent
	for rows.Next() {
		var from sql.NullString
		var e store.StageEvent
		var at string
		if err := rows.Scan(&from, &e.To, &e.Cause, &e.Reason, &at); err != nil {
			t.Fatal(err)
		}
		if from.Valid {
			s := store.Stage(from.String)
			e.From = &s
		}
		e.At, _ = store.ParseTime(at)
		out = append(out, e)
	}
	return out
}

const (
	L = store.StageLead
	R = store.StageReady
	A = store.StageApplied
	I = store.StageInterviewing
	O = store.StageOffer
	X = store.StageRejected
	W = store.StageWithdrawn
)

// allowed is the user-transition table: from -> the targets allowed
// without Reopen. Same-stage moves are no-ops, tested separately.
var allowed = map[store.Stage][]store.Stage{
	L: {R, A, I, O, X, W},
	R: {L, A, I, O, X, W},
	A: {I, O, X, W},
	I: {O, X, W},
	O: {X, W},
	X: {},
	W: {},
}

// reopenable is the Reopen table: back to an earlier active stage, or out
// of a terminal stage.
var reopenable = map[store.Stage][]store.Stage{
	L: {},
	R: {L},
	A: {L, R},
	I: {L, R, A},
	O: {L, R, A, I},
	X: {L, R, A, I, O},
	W: {L, R, A, I, O},
}

func contains(list []store.Stage, s store.Stage) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func TestR11_AllowedTransitions(t *testing.T) {
	f := newFixture(t)
	for _, from := range store.Stages {
		for _, to := range store.Stages {
			if from == to {
				continue
			}
			t.Run(fmt.Sprintf("%s->%s", from, to), func(t *testing.T) {
				want := contains(allowed[from], to)
				if got := pipeline.CanTransition(from, to); got != want {
					t.Fatalf("CanTransition = %v, want %v", got, want)
				}
				id := f.at(t, from)
				err := f.tx(t, func(tx *sql.Tx) error {
					_, err := f.m.Transition(ctx, tx, id, to, "")
					return err
				})
				if want {
					if err != nil {
						t.Fatalf("allowed move failed: %v", err)
					}
					if s := f.stage(t, id); s != to {
						t.Fatalf("stage = %s, want %s", s, to)
					}
					return
				}
				if apperr.CodeOf(err) != apperr.CodeConflict {
					t.Fatalf("invalid move: err = %v, want a conflict error", err)
				}
				if err.Error() == "" || err.Error() == apperr.GenericMessage {
					t.Fatal("invalid move needs a specific message for the user")
				}
				if s := f.stage(t, id); s != from {
					t.Fatalf("stage changed to %s on a rejected move", s)
				}
			})
		}
	}
}

func TestR11_ReopenTable(t *testing.T) {
	f := newFixture(t)
	for _, from := range store.Stages {
		for _, to := range store.Stages {
			if from == to {
				continue
			}
			t.Run(fmt.Sprintf("%s->%s", from, to), func(t *testing.T) {
				want := contains(reopenable[from], to)
				id := f.at(t, from)
				err := f.tx(t, func(tx *sql.Tx) error {
					_, err := f.m.Reopen(ctx, tx, id, to)
					return err
				})
				if (err == nil) != want {
					t.Fatalf("Reopen err = %v, want allowed=%v", err, want)
				}
				if want {
					evs := f.events(t, id)
					last := evs[len(evs)-1]
					if last.Reason != "reopen" || last.Cause != store.CauseUser {
						t.Fatalf("reopen event = %+v", last)
					}
				}
			})
		}
	}
}

func TestR11_SameStageIsNoOp(t *testing.T) {
	f := newFixture(t)
	id := f.at(t, A)
	before := len(f.events(t, id))
	var ch pipeline.Change
	err := f.tx(t, func(tx *sql.Tx) error {
		var err error
		ch, err = f.m.Transition(ctx, tx, id, A, "")
		return err
	})
	if err != nil || ch.Changed {
		t.Fatalf("change = %+v, err = %v; want a silent no-op", ch, err)
	}
	if len(f.events(t, id)) != before {
		t.Fatal("a no-op logged an event")
	}
}

func TestR11_NewClipIsLead(t *testing.T) {
	f := newFixture(t)
	for _, src := range []store.Source{store.SourceClip, store.SourcePaste} {
		var id int64
		err := f.tx(t, func(tx *sql.Tx) error {
			var err error
			id, err = f.m.CreateLead(ctx, tx, pipeline.NewLead{Company: "Acme", Role: "Dev", Source: src})
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		if s := f.stage(t, id); s != L {
			t.Fatalf("%s: new row stage = %s, want lead (U9)", src, s)
		}
		evs := f.events(t, id)
		if len(evs) != 1 || evs[0].From != nil || evs[0].To != L || evs[0].Reason != string(src) {
			t.Fatalf("%s: creation event = %+v", src, evs)
		}
	}
}

func TestR11_TransitionLogsEvent(t *testing.T) {
	f := newFixture(t)
	id := f.lead(t)
	err := f.tx(t, func(tx *sql.Tx) error {
		if _, err := f.m.Transition(ctx, tx, id, A, "bulk"); err != nil {
			return err
		}
		_, err := f.m.AutoAdvance(ctx, tx, id, pipeline.RuleRoundAdded)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	evs := f.events(t, id)
	if len(evs) != 3 {
		t.Fatalf("events = %+v", evs)
	}
	user, auto := evs[1], evs[2]
	if *user.From != L || user.To != A || user.Cause != store.CauseUser || user.Reason != "bulk" || !user.At.Equal(f.t0) {
		t.Fatalf("user event = %+v", user)
	}
	if *auto.From != A || auto.To != I || auto.Cause != store.CauseAuto || auto.Reason != pipeline.RuleRoundAdded.Name {
		t.Fatalf("auto event = %+v", auto)
	}
}

func TestR11_AutoAdvanceOnlyFromAllowedSources(t *testing.T) {
	f := newFixture(t)
	cases := []struct {
		rule    pipeline.AutoRule
		from    store.Stage
		advance bool
	}{
		{pipeline.RuleMaterialsApproved, L, true},
		{pipeline.RuleMaterialsApproved, R, false}, // already there
		{pipeline.RuleMaterialsApproved, A, false}, // R26: never moves an Applied row back
		{pipeline.RuleMaterialsApproved, I, false},
		{pipeline.RuleMaterialsApproved, X, false},
		{pipeline.RuleRoundAdded, L, true},
		{pipeline.RuleRoundAdded, R, true},
		{pipeline.RuleRoundAdded, A, true},
		{pipeline.RuleRoundAdded, I, false},
		{pipeline.RuleRoundAdded, O, false}, // R30: a round on an Offer row keeps its stage
		{pipeline.RuleRoundAdded, X, false},
		{pipeline.RuleRoundAdded, W, false},
	}
	for _, c := range cases {
		t.Run(fmt.Sprintf("%s from %s", c.rule.Name, c.from), func(t *testing.T) {
			id := f.at(t, c.from)
			before := len(f.events(t, id))
			var ch pipeline.Change
			err := f.tx(t, func(tx *sql.Tx) error {
				var err error
				ch, err = f.m.AutoAdvance(ctx, tx, id, c.rule)
				return err
			})
			if err != nil {
				t.Fatalf("AutoAdvance must never error on a non-matching stage: %v", err)
			}
			want := c.from
			if c.advance {
				want = c.rule.Target
			}
			if s := f.stage(t, id); s != want || ch.Changed != c.advance {
				t.Fatalf("stage = %s (changed %v), want %s (changed %v)", s, ch.Changed, want, c.advance)
			}
			if got := len(f.events(t, id)) - before; got != map[bool]int{true: 1, false: 0}[c.advance] {
				t.Fatalf("%d events logged", got)
			}
		})
	}
}

func TestR10_AppliedRecordsCurrentResume(t *testing.T) {
	f := newFixture(t)
	var v1, v2 int64
	err := f.tx(t, func(tx *sql.Tx) error {
		for i, p := range []*int64{&v1, &v2} {
			res, err := tx.ExecContext(ctx, `INSERT INTO resume_files (file_name, stored_path, mime_type, sha256, size_bytes, uploaded_at) VALUES ('cv.pdf', ?, 'application/pdf', 'h', 1, ?)`,
				fmt.Sprintf("resumes/%d.pdf", i), store.FormatTime(f.t0.Add(time.Duration(i)*time.Hour)))
			if err != nil {
				return err
			}
			*p, _ = res.LastInsertId()
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	id := f.lead(t)
	if err := f.tx(t, func(tx *sql.Tx) error { _, err := f.m.Transition(ctx, tx, id, A, ""); return err }); err != nil {
		t.Fatal(err)
	}
	var resume sql.NullInt64
	var appliedAt sql.NullString
	if err := f.db.Reader().QueryRowContext(ctx, `SELECT resume_file_id, applied_at FROM applications WHERE id = ?`, id).Scan(&resume, &appliedAt); err != nil {
		t.Fatal(err)
	}
	if !resume.Valid || resume.Int64 != v2 || !appliedAt.Valid {
		t.Fatalf("resume_file_id = %v, applied_at = %v; want the newest resume (%d) and a timestamp", resume, appliedAt, v2)
	}

	// Reopening to before Applied clears both.
	if err := f.tx(t, func(tx *sql.Tx) error { _, err := f.m.Reopen(ctx, tx, id, R); return err }); err != nil {
		t.Fatal(err)
	}
	if err := f.db.Reader().QueryRowContext(ctx, `SELECT resume_file_id, applied_at FROM applications WHERE id = ?`, id).Scan(&resume, &appliedAt); err != nil {
		t.Fatal(err)
	}
	if resume.Valid || appliedAt.Valid {
		t.Fatalf("after reopen to Ready: resume %v, applied_at %v; want both cleared", resume, appliedAt)
	}
}

func TestR11_DeletedOrMissingRowNotFound(t *testing.T) {
	f := newFixture(t)
	id := f.lead(t)
	if err := f.tx(t, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE applications SET deleted_at = ? WHERE id = ?`, store.FormatTime(f.t0), id)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	for _, target := range []int64{id, 99999} {
		err := f.tx(t, func(tx *sql.Tx) error { _, err := f.m.Transition(ctx, tx, target, A, ""); return err })
		if apperr.CodeOf(err) != apperr.CodeNotFound {
			t.Fatalf("id %d: err = %v, want not found", target, err)
		}
		err = f.tx(t, func(tx *sql.Tx) error {
			_, err := f.m.AutoAdvance(ctx, tx, target, pipeline.RuleRoundAdded)
			return err
		})
		if apperr.CodeOf(err) != apperr.CodeNotFound {
			t.Fatalf("auto id %d: err = %v, want not found", target, err)
		}
	}
}
