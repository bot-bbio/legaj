// Package jobs runs slow AI work in the background (R23): bulk letter
// generation must leave the UI responsive, use at most two concurrent
// calls (free-tier rate limits), back off when Gemini says "slow down",
// and let one failure stand without stopping the rest.
//
// The queue lives in memory. Drafts are saved in the DB as each job
// finishes, so a restart loses only jobs that hadn't produced anything.
package jobs

import (
	"context"
	"errors"
	"slices"
	"sync"
	"time"

	"github.com/bot-bbio/legaj/internal/apperr"
	"github.com/bot-bbio/legaj/internal/events"
)

// Status is a job's state.
type Status string

// Job states.
const (
	Queued    Status = "queued"
	Running   Status = "running"
	Done      Status = "done"
	Failed    Status = "failed"
	Cancelled Status = "cancelled"
)

// Job is one application's generation.
type Job struct {
	ID       int64  `json:"id"`
	AppID    int64  `json:"appId"`
	Status   Status `json:"status"`
	Error    string `json:"error"` // user-safe reason when failed
	Attempts int    `json:"attempts"`
}

// Worker does one job. It must honour ctx: cancellation is how a deleted
// row's job is stopped (R13).
type Worker func(ctx context.Context, appID int64) error

// Config wires the queue.
type Config struct {
	Work Worker
	Bus  *events.Bus
	// Concurrency is the number of parallel jobs; default 2 (R23).
	Concurrency int
	// Sleep waits between rate-limited attempts; tests inject a recorder.
	Sleep func(ctx context.Context, d time.Duration) error
	// BaseBackoff is the first wait after a rate limit; it doubles. Default 30 s.
	BaseBackoff time.Duration
}

// maxAttempts bounds retries of a rate-limited job (the Gemini client has
// already retried each call before the queue sees the error).
const maxAttempts = 4

// Queue runs jobs with bounded concurrency.
type Queue struct {
	cfg Config

	mu      sync.Mutex
	cond    *sync.Cond
	jobs    []*Job
	pending []*Job
	cancels map[int64]context.CancelFunc // running job ID -> cancel
	nextID  int64
	closed  bool

	ctx  context.Context
	stop context.CancelFunc
	wg   sync.WaitGroup
}

// New creates a queue; call Start to run it.
func New(cfg Config) *Queue {
	if cfg.Concurrency <= 0 {
		cfg.Concurrency = 2
	}
	if cfg.BaseBackoff <= 0 {
		cfg.BaseBackoff = 30 * time.Second
	}
	if cfg.Sleep == nil {
		cfg.Sleep = sleepCtx
	}
	q := &Queue{cfg: cfg, cancels: map[int64]context.CancelFunc{}}
	q.cond = sync.NewCond(&q.mu)
	return q
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// Start launches the workers.
func (q *Queue) Start(ctx context.Context) {
	q.ctx, q.stop = context.WithCancel(ctx)
	for range q.cfg.Concurrency {
		q.wg.Add(1)
		go q.worker()
	}
}

// Close stops the workers (running jobs are cancelled) and waits for them.
func (q *Queue) Close() {
	q.mu.Lock()
	q.closed = true
	q.mu.Unlock()
	if q.stop != nil {
		q.stop()
	}
	q.cond.Broadcast()
	q.wg.Wait()
}

func (q *Queue) emit(appIDs ...int64) {
	q.cfg.Bus.Emit(events.Change(events.EntityJob, appIDs...))
}

// Enqueue adds one job per application. An application that already has
// a queued or running job isn't queued again, so a double click is still
// one job (U6). It returns the jobs added.
func (q *Queue) Enqueue(appIDs []int64) []Job {
	q.mu.Lock()
	busy := map[int64]bool{}
	for _, j := range q.jobs {
		if j.Status == Queued || j.Status == Running {
			busy[j.AppID] = true
		}
	}
	var added []Job
	var touched []int64
	for _, id := range appIDs {
		if busy[id] {
			continue
		}
		busy[id] = true
		q.nextID++
		j := &Job{ID: q.nextID, AppID: id, Status: Queued}
		q.jobs = append(q.jobs, j)
		q.pending = append(q.pending, j)
		added = append(added, *j)
		touched = append(touched, id)
	}
	q.mu.Unlock()
	if len(added) > 0 {
		q.cond.Broadcast()
		q.emit(touched...)
	}
	return added
}

// CancelApps stops every queued or running job for these applications;
// the pipeline calls it when rows are deleted (R13).
func (q *Queue) CancelApps(appIDs []int64) {
	q.mu.Lock()
	var touched []int64
	for _, j := range q.jobs {
		if !slices.Contains(appIDs, j.AppID) {
			continue
		}
		switch j.Status {
		case Queued:
			j.Status = Cancelled
			touched = append(touched, j.AppID)
		case Running:
			if cancel := q.cancels[j.ID]; cancel != nil {
				cancel() // the worker records Cancelled when it returns
			}
		}
	}
	q.pending = slices.DeleteFunc(q.pending, func(j *Job) bool { return j.Status == Cancelled })
	q.mu.Unlock()
	if len(touched) > 0 {
		q.emit(touched...)
	}
}

// List returns a snapshot of every job in the session, oldest first.
func (q *Queue) List() []Job {
	q.mu.Lock()
	defer q.mu.Unlock()
	out := make([]Job, len(q.jobs))
	for i, j := range q.jobs {
		out[i] = *j
	}
	return out
}

// ClearFinished forgets done and cancelled jobs (failures stay until retried).
func (q *Queue) ClearFinished() {
	q.mu.Lock()
	q.jobs = slices.DeleteFunc(q.jobs, func(j *Job) bool { return j.Status == Done || j.Status == Cancelled })
	q.mu.Unlock()
	q.emit()
}

// Dismiss forgets the finished jobs (done, failed or cancelled) of these
// applications: the user has seen a failure and doesn't want to retry it.
// Queued and running jobs are untouched.
func (q *Queue) Dismiss(appIDs []int64) {
	q.mu.Lock()
	q.jobs = slices.DeleteFunc(q.jobs, func(j *Job) bool {
		return slices.Contains(appIDs, j.AppID) && j.Status != Queued && j.Status != Running
	})
	q.mu.Unlock()
	q.emit(appIDs...)
}

// next blocks until a job is pending or the queue closes.
func (q *Queue) next() (*Job, context.Context, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for len(q.pending) == 0 && !q.closed {
		q.cond.Wait()
	}
	if q.closed {
		return nil, nil, false
	}
	j := q.pending[0]
	q.pending = q.pending[1:]
	j.Status = Running
	jctx, cancel := context.WithCancel(q.ctx)
	q.cancels[j.ID] = cancel
	return j, jctx, true
}

func (q *Queue) worker() {
	defer q.wg.Done()
	for {
		j, jctx, ok := q.next()
		if !ok {
			return
		}
		q.emit(j.AppID)
		attempts, err := q.run(jctx, j.AppID)

		q.mu.Lock()
		j.Attempts = attempts
		// Decide the outcome before releasing the job's context: releasing
		// cancels it, which would make every job look cancelled.
		switch {
		case jctx.Err() != nil || errors.Is(err, context.Canceled):
			j.Status = Cancelled
		case err != nil:
			j.Status, j.Error = Failed, apperr.UserMessage(err)
		default:
			j.Status = Done
		}
		q.cancels[j.ID]()
		delete(q.cancels, j.ID)
		q.mu.Unlock()
		q.emit(j.AppID)
	}
}

// run does one job, retrying rate limits with a doubling wait.
func (q *Queue) run(ctx context.Context, appID int64) (int, error) {
	wait := q.cfg.BaseBackoff
	for attempt := 1; ; attempt++ {
		err := q.cfg.Work(ctx, appID)
		if apperr.CodeOf(err) != apperr.CodeAIRateLimited || attempt == maxAttempts {
			return attempt, err
		}
		if err := q.cfg.Sleep(ctx, wait); err != nil {
			return attempt, err
		}
		wait *= 2
	}
}
