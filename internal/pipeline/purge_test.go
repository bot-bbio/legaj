package pipeline_test

import (
	"database/sql"
	"slices"
	"sync"
	"testing"
)

func TestR13_PurgeReportsIDsAndDropsUnbankedAnswers(t *testing.T) {
	sf := newSvc(t)
	keep := sf.add(t, "Keep", "Dev", "")
	gone := sf.add(t, "Gone", "Dev", "")
	var mu sync.Mutex
	var purged []int64
	sf.svc.OnPurge(func(ids []int64) {
		mu.Lock()
		defer mu.Unlock()
		purged = append(purged, ids...)
	})
	if err := sf.tx(t, func(tx *sql.Tx) error {
		for _, q := range []string{
			`INSERT INTO answers (application_id, question, answer, status, in_bank, created_at, updated_at) VALUES (?, 'Draft?', '', 'draft', 0, '', '')`,
			`INSERT INTO answers (application_id, question, answer, status, in_bank, created_at, updated_at) VALUES (?, 'Banked?', 'Yes.', 'approved', 1, '', '')`,
		} {
			if _, err := tx.ExecContext(ctx, q, gone); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := sf.svc.Delete(ctx, []int64{gone}); err != nil {
		t.Fatal(err)
	}
	sf.timers.fireAll()

	mu.Lock()
	got := slices.Clone(purged)
	mu.Unlock()
	if !slices.Equal(got, []int64{gone}) {
		t.Fatalf("purge hook got %v, want [%d]", got, gone)
	}
	if n := count(t, sf.db, `SELECT count(*) FROM answers WHERE question = 'Draft?'`); n != 0 {
		t.Fatal("a draft answer outlived its application")
	}
	if n := count(t, sf.db, `SELECT count(*) FROM answers WHERE question = 'Banked?' AND application_id IS NULL`); n != 1 {
		t.Fatal("a banked answer must survive, unlinked")
	}
	if n := count(t, sf.db, `SELECT count(*) FROM applications WHERE id = ?`, keep); n != 1 {
		t.Fatal("the other application was purged")
	}
}

func TestR13_StartupPurgeAlsoReportsIDs(t *testing.T) {
	sf := newSvc(t)
	id := sf.add(t, "Gone", "Dev", "")
	if _, err := sf.svc.Delete(ctx, []int64{id}); err != nil {
		t.Fatal(err)
	}
	var got []int64
	sf.svc.OnPurge(func(ids []int64) { got = append(got, ids...) })
	if _, err := sf.svc.PurgeDeleted(ctx); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, []int64{id}) {
		t.Fatalf("startup purge reported %v", got)
	}
}
