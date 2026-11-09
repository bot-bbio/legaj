package pipeline_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/bot-bbio/legaj/internal/apperr"
	"github.com/bot-bbio/legaj/internal/pipeline"
	"github.com/bot-bbio/legaj/internal/store"
)

// withMaterials creates a row in stage s with the given materials choice.
func (f *fixture) withMaterials(t *testing.T, s store.Stage, m store.Materials) int64 {
	t.Helper()
	id := f.at(t, s)
	if err := f.tx(t, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE applications SET materials = ? WHERE id = ?`, string(m), id)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return id
}

func (f *fixture) approveLetter(t *testing.T, id int64) {
	t.Helper()
	if err := f.tx(t, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO documents (application_id, kind, status, version, body, created_at, updated_at)
			VALUES (?, 'letter', 'approved', 1, '{}', '', '')`, id)
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

func (f *fixture) answer(t *testing.T, id int64, status string) {
	t.Helper()
	if err := f.tx(t, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO answers (application_id, question, answer, status, created_at, updated_at)
			VALUES (?, 'Why us?', 'Because.', ?, '', '')`, id, status)
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

func (f *fixture) checkReady(t *testing.T, id int64) pipeline.Change {
	t.Helper()
	var c pipeline.Change
	if err := f.tx(t, func(tx *sql.Tx) error {
		var err error
		c, err = f.m.CheckReady(ctx, tx, id)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return c
}

func TestR26_LeadAdvancesToReadyWhenChosenMaterialsApproved(t *testing.T) {
	f := newFixture(t)

	letter := f.withMaterials(t, store.StageLead, store.MaterialsLetter)
	if c := f.checkReady(t, letter); c.To != store.StageLead {
		t.Fatalf("nothing approved yet, but moved to %s", c.To)
	}
	f.approveLetter(t, letter)
	if c := f.checkReady(t, letter); c.To != store.StageReady {
		t.Fatalf("letter approved: stage %s, want ready", c.To)
	}
	evs := f.events(t, letter)
	last := evs[len(evs)-1]
	if last.Cause != store.CauseAuto || last.Reason != "materials_approved" || last.To != store.StageReady {
		t.Fatalf("event = %+v, want auto materials_approved -> ready", last)
	}

	both := f.withMaterials(t, store.StageLead, store.MaterialsBoth)
	f.approveLetter(t, both)
	if c := f.checkReady(t, both); c.To != store.StageLead {
		t.Fatal("both chosen: the letter alone must not make the row Ready")
	}
	f.answer(t, both, "approved")
	f.answer(t, both, "draft")
	if c := f.checkReady(t, both); c.To != store.StageLead {
		t.Fatal("an answer still in draft must keep the row in Lead")
	}
	if err := f.tx(t, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE answers SET status = 'approved' WHERE application_id = ?`, both)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if c := f.checkReady(t, both); c.To != store.StageReady {
		t.Fatalf("letter and answers approved: stage %s, want ready", c.To)
	}
}

func TestR26_AppliedRowNeverMovesBackToReady(t *testing.T) {
	f := newFixture(t)
	for _, s := range []store.Stage{store.StageApplied, store.StageInterviewing, store.StageOffer, store.StageRejected, store.StageWithdrawn, store.StageReady} {
		id := f.withMaterials(t, s, store.MaterialsLetter)
		f.approveLetter(t, id)
		before := len(f.events(t, id))
		if c := f.checkReady(t, id); c.To != s {
			t.Errorf("%s row moved to %s on approval", s, c.To)
		}
		if f.stage(t, id) != s || len(f.events(t, id)) != before {
			t.Errorf("%s row changed or logged an event", s)
		}
	}
}

func TestR26_NoMaterialsChosenNeverAdvances(t *testing.T) {
	f := newFixture(t)
	id := f.withMaterials(t, store.StageLead, store.MaterialsNone)
	f.approveLetter(t, id)
	f.answer(t, id, "approved")
	if c := f.checkReady(t, id); c.To != store.StageLead {
		t.Fatalf("no materials chosen, but moved to %s", c.To)
	}
}

func TestR26_NoLetterRequired(t *testing.T) {
	f := newFixture(t)
	id := f.withMaterials(t, store.StageLead, store.MaterialsAnswers)
	f.answer(t, id, "approved")
	if c := f.checkReady(t, id); c.To != store.StageReady {
		t.Fatalf("answers-only row with approved answers: stage %s, want ready", c.To)
	}
}

func TestR26_NewLeadsTakeTheDefaultMaterials(t *testing.T) {
	f := newFixture(t)
	var id int64
	if err := f.tx(t, func(tx *sql.Tx) error {
		var err error
		id, err = f.m.CreateLead(ctx, tx, pipeline.NewLead{Company: "Acme", Source: store.SourcePaste, Materials: store.MaterialsBoth})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	var m string
	if err := f.db.Reader().QueryRowContext(ctx, `SELECT materials FROM applications WHERE id = ?`, id).Scan(&m); err != nil {
		t.Fatal(err)
	}
	if m != "both" {
		t.Fatalf("materials = %q, want both", m)
	}

	sf := newSvc(t)
	sf.svc = pipeline.NewService(pipeline.Config{
		DB: sf.db, Bus: nil, Machine: sf.m, Now: func() time.Time { return sf.t0 },
		DefaultMaterials: func(context.Context) store.Materials { return store.MaterialsAnswers },
	})
	res, err := sf.svc.PasteJob(ctx, pipeline.PasteInput{Company: "Initech", Role: "Dev"})
	if err != nil {
		t.Fatal(err)
	}
	a, err := sf.svc.Get(ctx, res.ID)
	if err != nil {
		t.Fatal(err)
	}
	if a.Materials != store.MaterialsAnswers {
		t.Fatalf("pasted row materials = %q, want the default (answers)", a.Materials)
	}
}

func TestR26_SetMaterialsValidatesAndRechecksReady(t *testing.T) {
	sf := newSvc(t)
	id := sf.withMaterials(t, store.StageLead, store.MaterialsBoth)
	sf.approveLetter(t, id)

	for _, bad := range []store.Materials{store.MaterialsNone, "resume", "LETTER"} {
		if _, err := sf.svc.SetMaterials(ctx, id, bad); apperr.CodeOf(err) != apperr.CodeInvalid {
			t.Errorf("SetMaterials(%q) err = %v, want invalid", bad, err)
		}
	}
	a, err := sf.svc.SetMaterials(ctx, id, store.MaterialsLetter)
	if err != nil {
		t.Fatal(err)
	}
	if a.Materials != store.MaterialsLetter || a.Stage != store.StageReady {
		t.Fatalf("after choosing letter only (already approved): %+v, want letter/ready", a)
	}
	if _, err := sf.svc.SetMaterials(ctx, 9999, store.MaterialsLetter); apperr.CodeOf(err) != apperr.CodeNotFound {
		t.Fatalf("missing row err = %v", err)
	}
}
