package letters_test

import (
	"database/sql"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/bot-bbio/legaj/internal/apperr"
	"github.com/bot-bbio/legaj/internal/store"
)

func (f *fixture) draft(t *testing.T, id int64, company string) {
	t.Helper()
	f.ai.Push(reply(t, company))
	if _, err := f.svc.Draft(ctx, id); err != nil {
		t.Fatal(err)
	}
}

func (f *fixture) stage(t *testing.T, id int64) store.Stage {
	t.Helper()
	var s string
	if err := f.db.Reader().QueryRowContext(ctx, `SELECT stage FROM applications WHERE id = ?`, id).Scan(&s); err != nil {
		t.Fatal(err)
	}
	return store.Stage(s)
}

func (f *fixture) setMaterials(t *testing.T, id int64, m store.Materials) {
	t.Helper()
	if err := f.db.Tx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE applications SET materials = ? WHERE id = ?`, string(m), id)
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

func (f *fixture) versions(t *testing.T, id int64) int {
	t.Helper()
	var n int
	if err := f.db.Reader().QueryRowContext(ctx, `SELECT count(*) FROM documents
		WHERE application_id = ? AND status = 'approved'`, id).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestR23_ApproveAllMakesAllPDFs(t *testing.T) {
	f := newFixture(t)
	var ids []int64
	for _, c := range []string{"Acme", "Initech", "Globex"} {
		id := f.app(t, c, "JD")
		f.draft(t, id, c)
		ids = append(ids, id)
	}
	noDraft := f.app(t, "Umbrella", "JD")

	res := f.svc.ApproveAll(ctx, append(ids, noDraft))
	if len(res.Approved) != 3 {
		t.Fatalf("approved %d, want 3: %+v", len(res.Approved), res)
	}
	for _, v := range res.Approved {
		if _, err := os.Stat(filepath.Join(f.dir, filepath.FromSlash(v.Path))); err != nil {
			t.Errorf("app %d: PDF missing: %v", v.AppID, err)
		}
	}
	if len(res.Failed) != 1 || res.Failed[0].AppID != noDraft || res.Failed[0].Reason == "" {
		t.Fatalf("failures = %+v, want only the row without a draft, with a reason", res.Failed)
	}
	if got := f.svc.ApproveAll(ctx, ids); len(got.Approved) != 0 || len(got.Failed) != 3 {
		t.Fatalf("approving again made versions: %+v", got)
	}
}

func TestR23_SkipReviewMakesPDFDirectly(t *testing.T) {
	f := newFixture(t)

	reviewed := f.app(t, "Acme", "JD")
	f.ai.Push(reply(t, "Acme"))
	if err := f.svc.Generate(ctx, reviewed); err != nil {
		t.Fatal(err)
	}
	if f.versions(t, reviewed) != 0 {
		t.Fatal("with review on, Generate must stop at a draft")
	}
	if _, err := f.svc.CurrentDraft(ctx, reviewed); err != nil {
		t.Fatalf("draft should await review: %v", err)
	}

	f.skip.Store(true)
	direct := f.app(t, "Initech", "JD")
	f.ai.Push(reply(t, "Initech"))
	if err := f.svc.Generate(ctx, direct); err != nil {
		t.Fatal(err)
	}
	if f.versions(t, direct) != 1 {
		t.Fatal("with Skip review on, Generate must make the PDF")
	}
	if _, err := f.svc.CurrentDraft(ctx, direct); apperr.CodeOf(err) != apperr.CodeNotFound {
		t.Fatal("nothing should be left to review")
	}
}

func TestR26_ApprovingTheLetterMakesALetterLeadReady(t *testing.T) {
	f := newFixture(t)
	letter := f.app(t, "Acme", "JD")
	f.setMaterials(t, letter, store.MaterialsLetter)
	both := f.app(t, "Initech", "JD")
	f.setMaterials(t, both, store.MaterialsBoth)
	for _, id := range []int64{letter, both} {
		f.draft(t, id, map[int64]string{letter: "Acme", both: "Initech"}[id])
		if _, err := f.svc.Approve(ctx, id); err != nil {
			t.Fatal(err)
		}
	}
	if s := f.stage(t, letter); s != store.StageReady {
		t.Fatalf("letter-only Lead after approval: %s, want ready", s)
	}
	if s := f.stage(t, both); s != store.StageLead {
		t.Fatalf("letter+answers Lead with only the letter approved: %s, want lead", s)
	}
}

func TestR22_ADraftIsApprovedOnlyOnce(t *testing.T) {
	f := newFixture(t)
	id := f.app(t, "Acme", "JD")
	f.draft(t, id, "Acme")

	var wg sync.WaitGroup
	errs := make([]error, 4)
	for i := range errs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = f.svc.Approve(ctx, id)
		}()
	}
	wg.Wait()
	ok := 0
	for _, err := range errs {
		if err == nil {
			ok++
		}
	}
	if ok != 1 || f.versions(t, id) != 1 {
		t.Fatalf("%d approvals succeeded and %d versions exist; want exactly 1 (errs %v)", ok, f.versions(t, id), errs)
	}
}
