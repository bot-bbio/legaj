package pipeline_test

import (
	"database/sql"
	"testing"
	"time"

	"github.com/bot-bbio/legaj/internal/apperr"
	"github.com/bot-bbio/legaj/internal/store"
)

func (sf *svcFixture) uploadResume(t *testing.T, path string, at time.Time) int64 {
	t.Helper()
	var id int64
	if err := sf.db.Tx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `INSERT INTO resume_files (file_name, stored_path, mime_type, sha256, size_bytes, uploaded_at)
			VALUES ('cv.pdf', ?, 'application/pdf', ?, 1, ?)`, path, path, store.FormatTime(at))
		if err != nil {
			return err
		}
		id, err = res.LastInsertId()
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestR10_OpenResumeUsesAppliedVersion(t *testing.T) {
	sf := newSvc(t)
	v1 := sf.uploadResume(t, "resumes/v1.pdf", sf.t0.Add(-time.Hour))
	applied := sf.add(t, "Acme", "Dev", "")
	if _, err := sf.svc.SetStage(ctx, []int64{applied}, store.StageApplied); err != nil {
		t.Fatal(err)
	}
	v2 := sf.uploadResume(t, "resumes/v2.pdf", sf.t0.Add(time.Hour)) // the user replaces the resume
	lead := sf.add(t, "Globex", "QA", "")

	got, err := sf.svc.ResumeFor(ctx, applied)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != v1 || got.StoredPath != "resumes/v1.pdf" {
		t.Fatalf("applied row opens %+v, want the version it was applied with (v1)", got)
	}
	got, err = sf.svc.ResumeFor(ctx, lead)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != v2 {
		t.Fatalf("a Lead opens %+v, want the current resume (v2)", got)
	}
}

func TestR10_OpenResumeWithoutAnyResume(t *testing.T) {
	sf := newSvc(t)
	id := sf.add(t, "Acme", "Dev", "")
	if _, err := sf.svc.ResumeFor(ctx, id); apperr.CodeOf(err) != apperr.CodeNotFound {
		t.Fatalf("err = %v, want not found", err)
	}
}
