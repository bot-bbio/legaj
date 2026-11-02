package pipeline_test

import (
	"database/sql"
	"testing"
	"time"

	"github.com/bot-bbio/legaj/internal/store"
)

// backdate sets an application's created_at.
func (sf *svcFixture) backdate(t *testing.T, id int64, at time.Time) {
	t.Helper()
	if err := sf.db.Tx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE applications SET created_at = ? WHERE id = ?`, store.FormatTime(at), id)
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

func (sf *svcFixture) round(t *testing.T, appID int64, at time.Time) {
	t.Helper()
	now := store.FormatTime(sf.t0)
	if err := sf.db.Tx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO rounds (application_id, starts_at, zone, kind, created_at, updated_at) VALUES (?, ?, 'Europe/Warsaw', 'video', ?, ?)`,
			appID, store.FormatTime(at), now, now)
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

func TestR14_RecentIsNewest5(t *testing.T) {
	sf := newSvc(t)
	var ids []int64
	for i := range 7 {
		id := sf.add(t, string(rune('A'+i)), "Dev", "")
		// Insertion order deliberately differs from creation time (A14).
		sf.backdate(t, id, sf.t0.Add(time.Duration((i*5)%7)*time.Hour))
		ids = append(ids, id)
	}
	deleted := sf.add(t, "Deleted", "Dev", "")
	sf.backdate(t, deleted, sf.t0.Add(100*time.Hour))
	if _, err := sf.svc.Delete(ctx, []int64{deleted}); err != nil {
		t.Fatal(err)
	}

	st, err := sf.svc.Stats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// Hours by index: 0,5,3,1,6,4,2 -> newest first: idx 4(6h),1(5h),5(4h),2(3h),6(2h).
	want := []int64{ids[4], ids[1], ids[5], ids[2], ids[6]}
	var got []int64
	for _, r := range st.Recent {
		got = append(got, r.ID)
	}
	if !eq(got, want) {
		t.Fatalf("recent = %v, want %v (newest 5 by created time, deleted rows excluded)", got, want)
	}
}

func TestR14_CountsPerStageIgnoreDeleted(t *testing.T) {
	sf := newSvc(t)
	a := sf.add(t, "A", "r", "")
	sf.add(t, "B", "r", "")
	c := sf.add(t, "C", "r", "")
	if _, err := sf.svc.SetStage(ctx, []int64{a}, store.StageApplied); err != nil {
		t.Fatal(err)
	}
	if _, err := sf.svc.Delete(ctx, []int64{c}); err != nil {
		t.Fatal(err)
	}
	st, err := sf.svc.Stats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if st.Counts[store.StageLead] != 1 || st.Counts[store.StageApplied] != 1 || st.Total != 2 {
		t.Fatalf("counts = %v, total = %d", st.Counts, st.Total)
	}
	for _, s := range store.Stages {
		if _, ok := st.Counts[s]; !ok {
			t.Fatalf("stage %s missing from counts; every stage must be present", s)
		}
	}
}

func TestR14_NextInterviewIsEarliestUpcoming(t *testing.T) {
	sf := newSvc(t)
	a := sf.add(t, "Acme", "Dev", "")
	b := sf.add(t, "Globex", "QA", "")
	gone := sf.add(t, "Deleted", "Dev", "")
	sf.round(t, a, sf.t0.Add(-time.Hour))   // past
	sf.round(t, a, sf.t0.Add(48*time.Hour)) // later
	sf.round(t, b, sf.t0.Add(3*time.Hour))  // next
	sf.round(t, gone, sf.t0.Add(time.Hour)) // sooner, but its application is deleted
	if _, err := sf.svc.Delete(ctx, []int64{gone}); err != nil {
		t.Fatal(err)
	}

	st, err := sf.svc.Stats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	n := st.NextInterview
	if n == nil || n.ApplicationID != b || n.Company != "Globex" || !n.StartsAt.Equal(sf.t0.Add(3*time.Hour)) || n.Zone != "Europe/Warsaw" {
		t.Fatalf("next interview = %+v", n)
	}

	empty := newSvc(t)
	st, err = empty.svc.Stats(ctx)
	if err != nil || st.NextInterview != nil || len(st.Recent) != 0 {
		t.Fatalf("empty stats = %+v, %v", st, err)
	}
}
