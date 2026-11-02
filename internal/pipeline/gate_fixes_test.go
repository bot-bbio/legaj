package pipeline_test

import (
	"database/sql"
	"testing"
	"time"

	"github.com/bot-bbio/legaj/internal/apperr"
	"github.com/bot-bbio/legaj/internal/store"
)

func TestR11_BulkReopen(t *testing.T) {
	sf := newSvc(t)
	rejected := sf.add(t, "A", "r", "")
	offer := sf.add(t, "B", "r", "")
	lead := sf.add(t, "C", "r", "")
	if _, err := sf.svc.SetStage(ctx, []int64{rejected}, store.StageRejected); err != nil {
		t.Fatal(err)
	}
	if _, err := sf.svc.SetStage(ctx, []int64{offer}, store.StageOffer); err != nil {
		t.Fatal(err)
	}

	res, err := sf.svc.Reopen(ctx, []int64{offer, lead, rejected}, store.StageApplied)
	if err != nil {
		t.Fatal(err)
	}
	if !eq(res.Changed, []int64{rejected, offer}) || len(res.Skipped) != 1 || res.Skipped[0].ID != lead {
		t.Fatalf("reopen = %+v; Lead can't be reopened forward", res)
	}
	for _, id := range []int64{rejected, offer} {
		if a, _ := sf.svc.Get(ctx, id); a.Stage != store.StageApplied {
			t.Fatalf("row %d is %s", id, a.Stage)
		}
	}
}

func TestR10_ResumePinnedOnlyAtFirstApply(t *testing.T) {
	sf := newSvc(t)
	id := sf.add(t, "Acme", "Dev", "")
	if _, err := sf.svc.SetStage(ctx, []int64{id}, store.StageApplied); err != nil {
		t.Fatal(err)
	}
	// The user uploads a resume only after applying...
	sf.uploadResume(t, "resumes/later.pdf", sf.t0.Add(time.Hour))
	// ...and the row moves on.
	if _, err := sf.svc.SetStage(ctx, []int64{id}, store.StageInterviewing); err != nil {
		t.Fatal(err)
	}
	var pin sql.NullInt64
	if err := sf.db.Reader().QueryRowContext(ctx, `SELECT resume_file_id FROM applications WHERE id = ?`, id).Scan(&pin); err != nil {
		t.Fatal(err)
	}
	if pin.Valid {
		t.Fatal("a resume uploaded after applying was pinned as the one applied with")
	}
	_, err := sf.svc.ResumeFor(ctx, id)
	if apperr.CodeOf(err) != apperr.CodeNotFound {
		t.Fatalf("err = %v; LeGaJ must say it doesn't know which resume was sent", err)
	}
}

func TestR13_UndoMovesChildrenOfAReaddedRowInsteadOfLosingThem(t *testing.T) {
	sf := newSvc(t)
	const u = "https://jobs.example/9"
	first := sf.add(t, "Acme", "Dev", u)
	addChildren(t, sf, first)
	// an approved letter v1 on the old row
	now := store.FormatTime(sf.t0)
	if err := sf.db.Tx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO documents (application_id, kind, status, version, body, file_path, created_at, updated_at, approved_at)
			VALUES (?, 'letter', 'approved', 1, '{}', 'documents/a_v1.pdf', ?, ?, ?)`, first, now, now, now)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	res, err := sf.svc.Delete(ctx, []int64{first})
	if err != nil {
		t.Fatal(err)
	}
	second := sf.add(t, "Acme", "Dev", u) // re-pasted within the window
	if err := sf.db.Tx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO documents (application_id, kind, status, version, body, file_path, created_at, updated_at, approved_at)
			VALUES (?, 'letter', 'approved', 1, '{}', 'documents/b_v1.pdf', ?, ?, ?)`, second, now, now, now)
		return err
	}); err != nil {
		t.Fatal(err)
	}

	undo, err := sf.svc.Undo(ctx, res.UndoID)
	if err != nil {
		t.Fatal(err)
	}
	if len(undo.Skipped) != 1 || undo.Skipped[0] != first || len(undo.MergedInto) != 1 || undo.MergedInto[0] != second {
		t.Fatalf("undo = %+v", undo)
	}
	sf.timers.fireAll() // the old row is purged...
	// ...but nothing it owned is lost.
	if n := count(t, sf.db, `SELECT count(*) FROM rounds WHERE application_id = ?`, second); n != 1 {
		t.Fatalf("%d rounds on the re-added row, want the old row's 1", n)
	}
	if n := count(t, sf.db, `SELECT count(*) FROM documents WHERE application_id = ? AND version IS NOT NULL`, second); n != 2 {
		t.Fatalf("%d approved letters, want both versions kept", n)
	}
	if n := count(t, sf.db, `SELECT count(DISTINCT version) FROM documents WHERE application_id = ? AND version IS NOT NULL`, second); n != 2 {
		t.Fatal("moved letter versions collide with the re-added row's")
	}
}
