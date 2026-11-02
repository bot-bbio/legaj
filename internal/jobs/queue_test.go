package jobs_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bot-bbio/legaj/internal/ai"
	"github.com/bot-bbio/legaj/internal/ai/aitest"
	"github.com/bot-bbio/legaj/internal/apperr"
	"github.com/bot-bbio/legaj/internal/events"
	"github.com/bot-bbio/legaj/internal/jobs"
	"github.com/bot-bbio/legaj/internal/letters"
	"github.com/bot-bbio/legaj/internal/pipeline"
	"github.com/bot-bbio/legaj/internal/profile"
	"github.com/bot-bbio/legaj/internal/store"
	"github.com/bot-bbio/legaj/internal/testenv"
)

func TestMain(m *testing.M) { testenv.Main(m) }

var ctx = context.Background()

func newQueue(t *testing.T, work jobs.Worker, rec *events.Recorder) *jobs.Queue {
	t.Helper()
	q := jobs.New(jobs.Config{
		Work: work, Bus: events.NewBus(rec), Concurrency: 2,
		Sleep: func(ctx context.Context, d time.Duration) error { return ctx.Err() }, // no real waiting
	})
	q.Start(ctx)
	t.Cleanup(q.Close)
	return q
}

// waitIdle waits until no job is queued or running.
func waitIdle(t *testing.T, q *jobs.Queue) []jobs.Job {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		all := q.List()
		busy := false
		for _, j := range all {
			if j.Status == jobs.Queued || j.Status == jobs.Running {
				busy = true
			}
		}
		if !busy {
			return all
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("queue never went idle")
	return nil
}

func ids(n int) []int64 {
	out := make([]int64, n)
	for i := range out {
		out[i] = int64(i + 1)
	}
	return out
}

func TestR23_ConcurrencyAtMost2(t *testing.T) {
	var running, peak atomic.Int32
	q := newQueue(t, func(ctx context.Context, appID int64) error {
		n := running.Add(1)
		for {
			p := peak.Load()
			if n <= p || peak.CompareAndSwap(p, n) {
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
		running.Add(-1)
		return nil
	}, &events.Recorder{})

	q.Enqueue(ids(10))
	all := waitIdle(t, q)
	if peak.Load() > 2 {
		t.Fatalf("%d jobs ran at once, want at most 2", peak.Load())
	}
	if peak.Load() < 2 {
		t.Fatalf("peak %d: the queue should use both workers", peak.Load())
	}
	for _, j := range all {
		if j.Status != jobs.Done {
			t.Fatalf("job %+v not done", j)
		}
	}
}

func TestR23_OneClickOneJob(t *testing.T) {
	block := make(chan struct{})
	q := newQueue(t, func(ctx context.Context, appID int64) error { <-block; return nil }, &events.Recorder{})
	q.Enqueue([]int64{7})
	q.Enqueue([]int64{7}) // a double click (U6)
	q.Enqueue([]int64{7, 7})
	if n := len(q.List()); n != 1 {
		t.Fatalf("%d jobs for one row, want 1", n)
	}
	close(block)
	waitIdle(t, q)
}

func TestR23_429BackoffThenSuccess(t *testing.T) {
	var calls atomic.Int32
	var slept []time.Duration
	var mu sync.Mutex
	q := jobs.New(jobs.Config{
		Concurrency: 2, Bus: events.NewBus(&events.Recorder{}),
		Work: func(ctx context.Context, appID int64) error {
			if calls.Add(1) <= 2 {
				return apperr.New(apperr.CodeAIRateLimited, "Gemini's rate limit was reached.")
			}
			return nil
		},
		Sleep: func(ctx context.Context, d time.Duration) error {
			mu.Lock()
			slept = append(slept, d)
			mu.Unlock()
			return nil
		},
	})
	q.Start(ctx)
	defer q.Close()
	q.Enqueue([]int64{1})
	all := waitIdle(t, q)
	if all[0].Status != jobs.Done || calls.Load() != 3 {
		t.Fatalf("job = %+v after %d calls, want done after 2 rate-limited tries", all[0], calls.Load())
	}
	mu.Lock()
	defer mu.Unlock()
	if len(slept) != 2 || slept[1] <= slept[0] {
		t.Fatalf("backoff = %v, want two growing waits", slept)
	}
}

func TestR23_OneFailureOthersComplete(t *testing.T) {
	q := newQueue(t, func(ctx context.Context, appID int64) error {
		if appID == 4 {
			return apperr.New(apperr.CodeInvalid, "The letter doesn't fit on one page. Shorten it and try again.")
		}
		return nil
	}, &events.Recorder{})
	q.Enqueue(ids(10))
	all := waitIdle(t, q)
	done, failed := 0, 0
	for _, j := range all {
		switch j.Status {
		case jobs.Done:
			done++
		case jobs.Failed:
			failed++
			if j.AppID != 4 || j.Error != "The letter doesn't fit on one page. Shorten it and try again." {
				t.Fatalf("failed job = %+v", j)
			}
		}
	}
	if done != 9 || failed != 1 {
		t.Fatalf("done %d, failed %d; want 9 and 1, with the failure kept in the list", done, failed)
	}
}

func TestR23_ErrorsShownAreUserSafe(t *testing.T) {
	q := newQueue(t, func(ctx context.Context, appID int64) error {
		return errors.New("gemini: HTTP 500 at C:\\Users\\x\\AppData\\secret")
	}, &events.Recorder{})
	q.Enqueue([]int64{1})
	j := waitIdle(t, q)[0]
	if j.Status != jobs.Failed || j.Error != apperr.GenericMessage {
		t.Fatalf("job = %+v; raw causes must not reach the UI (R34)", j)
	}
}

func TestR23_ProgressEventsEmitted(t *testing.T) {
	rec := &events.Recorder{}
	q := newQueue(t, func(ctx context.Context, appID int64) error { return nil }, rec)
	q.Enqueue(ids(3))
	waitIdle(t, q)
	n := 0
	for _, e := range rec.Events() {
		if c, ok := e.Payload.(events.Changed); ok && c.Entity == events.EntityJob {
			n++
		}
	}
	// queued (one batch) + running and done for each of 3 jobs
	if n < 1+2*3 {
		t.Fatalf("%d job events, want at least 7", n)
	}
}

func TestR13_DeleteCancelsJobs(t *testing.T) {
	started := make(chan int64, 4)
	q := newQueue(t, func(ctx context.Context, appID int64) error {
		started <- appID
		<-ctx.Done() // a long AI call
		return ctx.Err()
	}, &events.Recorder{})
	q.Enqueue([]int64{1, 2, 3})
	<-started
	<-started // two running, one queued

	q.CancelApps([]int64{1, 2, 3}) // what the pipeline's delete hook calls
	for _, j := range waitIdle(t, q) {
		if j.Status != jobs.Cancelled {
			t.Fatalf("job %+v not cancelled", j)
		}
	}
}

// --- end to end: a job whose row is deleted while it runs writes nothing ---

func TestR13_FinishingJobForDeletedRowWritesNoFile(t *testing.T) {
	dir := testenv.Isolate(t)
	db, st, err := store.Open(ctx, store.Options{Dir: dir})
	if err != nil || st.Kind != store.StartupReady {
		t.Fatalf("open: %v %+v", err, st)
	}
	defer db.Close()
	bus := events.NewBus(&events.Recorder{})
	prof := profile.NewService(db, bus, time.Now)
	m := pipeline.NewMachine(time.Now)
	pipe := pipeline.NewService(pipeline.Config{DB: db, Bus: bus, Machine: m, Now: time.Now})
	var appID int64
	if err := db.Tx(ctx, func(tx *sql.Tx) error {
		var err error
		appID, err = m.CreateLead(ctx, tx, pipeline.NewLead{Company: "Acme", Role: "Dev", Description: "JD", Source: store.SourcePaste})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	fake := &aitest.Fake{Respond: func(req ai.Request) (string, error) {
		// The user deletes the row while Gemini is writing the letter.
		if _, err := pipe.Delete(ctx, []int64{appID}); err != nil {
			return "", err
		}
		b, _ := json.Marshal(map[string]any{
			"salutation": "Dear Hiring Team,", "opening": "Applying to Acme.",
			"whyOptions": []any{"Acme a.", "Acme b.", "Acme c."}, "body": []any{"Body."},
			"closing": "Thanks.", "signOff": "Sincerely,",
		})
		return string(b), nil
	}}
	lt := letters.NewService(letters.Config{DB: db, Bus: bus, AI: fake, Profile: prof, Now: time.Now, Model: func(context.Context) string { return "m" }})
	q := newQueue(t, func(ctx context.Context, id int64) error {
		_, err := lt.Draft(ctx, id)
		return err
	}, &events.Recorder{})
	q.Enqueue([]int64{appID})
	j := waitIdle(t, q)[0]
	if j.Status == jobs.Done {
		t.Fatalf("job finished as done for a deleted row: %+v", j)
	}
	var docs int
	if err := db.Reader().QueryRowContext(ctx, `SELECT count(*) FROM documents`).Scan(&docs); err != nil {
		t.Fatal(err)
	}
	if docs != 0 {
		t.Fatalf("%d documents recorded for a deleted row", docs)
	}
	if entries, _ := os.ReadDir(filepath.Join(dir, "documents")); len(entries) != 0 {
		t.Fatal("files written for a deleted row")
	}
}
