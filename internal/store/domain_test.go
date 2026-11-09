package store_test

import (
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/bot-bbio/legaj/internal/store"
	"github.com/bot-bbio/legaj/internal/testenv"
)

func openDomain(t *testing.T) *store.DB {
	t.Helper()
	return mustOpen(t, store.Options{Dir: testenv.Isolate(t)})
}

func exec(t *testing.T, db *store.DB, query string, args ...any) (int64, error) {
	t.Helper()
	var id int64
	err := db.Tx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, query, args...)
		if err != nil {
			return err
		}
		id, err = res.LastInsertId()
		return err
	})
	return id, err
}

func mustExec(t *testing.T, db *store.DB, query string, args ...any) int64 {
	t.Helper()
	id, err := exec(t, db, query, args...)
	if err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return id
}

func now() string { return store.FormatTime(time.Date(2026, 11, 1, 9, 30, 0, 0, time.UTC)) }

func newApp(t *testing.T, db *store.DB, canonical string) int64 {
	t.Helper()
	return mustExec(t, db, `INSERT INTO applications (company, role, url, canonical_url, description, source, stage, created_at, updated_at)
		VALUES ('Acme', 'Engineer', ?, ?, 'Build things.', 'paste', 'lead', ?, ?)`, canonical, canonical, now(), now())
}

func count(t *testing.T, db *store.DB, query string, args ...any) int {
	t.Helper()
	var n int
	if err := db.Reader().QueryRowContext(ctx, query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestR16_DescriptionStoredOnApplication(t *testing.T) {
	db := openDomain(t)
	desc := "We need a Go developer.\n\nRequirements:\n- AT&T experience <5%\n- Łódź office"
	id := mustExec(t, db, `INSERT INTO applications (company, role, description, source, stage, created_at, updated_at)
		VALUES ('Acme', 'Dev', ?, 'clip', 'lead', ?, ?)`, desc, now(), now())

	var got string
	if err := db.Reader().QueryRowContext(ctx, `SELECT description FROM applications WHERE id = ?`, id).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != desc {
		t.Fatalf("description = %q, want %q", got, desc)
	}
	// The description can be empty but never NULL (A9: nothing clears it to nothing).
	if _, err := exec(t, db, `UPDATE applications SET description = NULL WHERE id = ?`, id); err == nil {
		t.Fatal("description accepted NULL")
	}
}

func TestR11_StageMustBeKnown(t *testing.T) {
	db := openDomain(t)
	id := newApp(t, db, "https://jobs.example/1")
	for _, s := range store.Stages {
		if _, err := exec(t, db, `UPDATE applications SET stage = ? WHERE id = ?`, string(s), id); err != nil {
			t.Fatalf("stage %q rejected: %v", s, err)
		}
	}
	if _, err := exec(t, db, `UPDATE applications SET stage = 'hired' WHERE id = ?`, id); err == nil {
		t.Fatal("unknown stage accepted")
	}
	want := []store.Stage{store.StageLead, store.StageReady, store.StageApplied, store.StageInterviewing, store.StageOffer, store.StageRejected, store.StageWithdrawn}
	if len(store.Stages) != len(want) {
		t.Fatalf("Stages = %v", store.Stages)
	}
	for i := range want {
		if store.Stages[i] != want[i] {
			t.Fatalf("Stages = %v, want %v", store.Stages, want)
		}
	}
}

// addChildren gives an application one row in every child table.
func addChildren(t *testing.T, db *store.DB, appID int64) {
	t.Helper()
	mustExec(t, db, `INSERT INTO stage_events (application_id, from_stage, to_stage, cause, reason, at) VALUES (?, NULL, 'lead', 'user', 'paste', ?)`, appID, now())
	mustExec(t, db, `INSERT INTO documents (application_id, kind, status, body, created_at, updated_at) VALUES (?, 'letter', 'draft', '{}', ?, ?)`, appID, now(), now())
	roundID := mustExec(t, db, `INSERT INTO rounds (application_id, starts_at, zone, duration_min, kind, created_at, updated_at) VALUES (?, ?, 'Europe/Warsaw', 60, 'video', ?, ?)`, appID, now(), now(), now())
	mustExec(t, db, `INSERT INTO followups (application_id, round_id, kind, status, created_at) VALUES (?, ?, 'thank_you', 'sent', ?)`, appID, roundID, now())
	mustExec(t, db, `INSERT INTO answers (application_id, question, answer, status, in_bank, created_at, updated_at) VALUES (?, 'Why us?', 'Because.', 'approved', 1, ?, ?)`, appID, now(), now())
}

var childTables = []string{"stage_events", "documents", "rounds", "followups"}

func TestR13_SoftDeleteKeepsChildren(t *testing.T) {
	db := openDomain(t)
	id := newApp(t, db, "https://jobs.example/1")
	addChildren(t, db, id)

	mustExec(t, db, `UPDATE applications SET deleted_at = ? WHERE id = ?`, now(), id)

	for _, table := range childTables {
		if n := count(t, db, `SELECT count(*) FROM `+table+` WHERE application_id = ?`, id); n != 1 {
			t.Errorf("%s: %d rows after soft delete, want 1 (undo must restore them)", table, n)
		}
	}
}

func TestR2_PurgeCascadesToChildrenButKeepsBankedAnswers(t *testing.T) {
	db := openDomain(t)
	id := newApp(t, db, "https://jobs.example/1")
	keep := newApp(t, db, "https://jobs.example/2")
	addChildren(t, db, id)
	addChildren(t, db, keep)

	mustExec(t, db, `DELETE FROM applications WHERE id = ?`, id)

	for _, table := range childTables {
		if n := count(t, db, `SELECT count(*) FROM `+table+` WHERE application_id = ?`, id); n != 0 {
			t.Errorf("%s: %d orphan rows after purge", table, n)
		}
		if n := count(t, db, `SELECT count(*) FROM `+table+` WHERE application_id = ?`, keep); n != 1 {
			t.Errorf("%s: the other application's rows were touched", table)
		}
	}
	// Banked answers are a reusable library: they survive, detached.
	if n := count(t, db, `SELECT count(*) FROM answers WHERE application_id IS NULL AND in_bank = 1`); n != 1 {
		t.Fatalf("banked answer lost on purge (%d detached rows)", n)
	}
}

func TestR13_CanonicalURLUniqueOnlyAmongLiveRows(t *testing.T) {
	db := openDomain(t)
	const u = "https://jobs.example/123"
	first := newApp(t, db, u)
	if _, err := exec(t, db, `INSERT INTO applications (company, role, canonical_url, description, source, stage, created_at, updated_at)
		VALUES ('Acme', 'Dev', ?, '', 'clip', 'lead', ?, ?)`, u, now(), now()); err == nil || !strings.Contains(err.Error(), "UNIQUE") {
		t.Fatalf("duplicate live canonical URL: err = %v, want UNIQUE violation", err)
	}
	mustExec(t, db, `UPDATE applications SET deleted_at = ? WHERE id = ?`, now(), first)
	newApp(t, db, u) // re-clip after delete: a new row, no unique-index error

	// Rows without a URL (manual entries) never collide.
	newApp(t, db, "")
	newApp(t, db, "")
}

func TestR10_ReferencedResumeFileCannotBeDeleted(t *testing.T) {
	db := openDomain(t)
	fileID := mustExec(t, db, `INSERT INTO resume_files (file_name, stored_path, mime_type, sha256, size_bytes, uploaded_at)
		VALUES ('cv.pdf', 'resumes/1-cv.pdf', 'application/pdf', 'abc', 10, ?)`, now())
	app := newApp(t, db, "https://jobs.example/1")
	mustExec(t, db, `UPDATE applications SET stage = 'applied', applied_at = ?, resume_file_id = ? WHERE id = ?`, now(), fileID, app)

	if _, err := exec(t, db, `DELETE FROM resume_files WHERE id = ?`, fileID); err == nil {
		t.Fatal("deleted a resume version an Applied application still points to")
	}
}

func TestR9_ProfileIsASingleJSONRow(t *testing.T) {
	db := openDomain(t)
	mustExec(t, db, `INSERT INTO profile (id, data, updated_at) VALUES (1, '{"fullName":"Ada"}', ?)`, now())
	if _, err := exec(t, db, `INSERT INTO profile (id, data, updated_at) VALUES (2, '{}', ?)`, now()); err == nil {
		t.Fatal("a second profile row was accepted")
	}
	if _, err := exec(t, db, `UPDATE profile SET data = 'not json' WHERE id = 1`); err == nil {
		t.Fatal("invalid JSON accepted as profile data")
	}
}

func TestR2_TimeFormatSortsAndRoundTrips(t *testing.T) {
	a := time.Date(2026, 3, 29, 0, 59, 59, 999_000_000, time.UTC)
	b := a.Add(time.Millisecond)
	sa, sb := store.FormatTime(a), store.FormatTime(b)
	if !(sa < sb) || len(sa) != len(sb) {
		t.Fatalf("%q, %q: fixed width and lexical order required", sa, sb)
	}
	warsaw, _ := time.LoadLocation("Europe/Warsaw")
	got, err := store.ParseTime(store.FormatTime(a.In(warsaw)))
	if err != nil || !got.Equal(a) || got.Location() != time.UTC {
		t.Fatalf("round trip = %v, %v", got, err)
	}
}
