package chef

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain/airun"
)

// Status is a job's state in the queue.
type Status string

const (
	StatusQueued  Status = "queued"
	StatusRunning Status = "running"
	StatusDone    Status = "done"
	StatusFailed  Status = "failed"
	// StatusSkipped: the guard refused the job; its error holds the Reason.
	StatusSkipped Status = "skipped"
)

// Job is one unit of capped work. Kind is the capped run kind its handler
// records.
type Job struct {
	ID        int64
	Kind      airun.Kind
	Payload   string
	Requested bool
	Attempts  int
}

// JobStore persists the queue.
type JobStore interface {
	Enqueue(ctx context.Context, kind airun.Kind, payload string, requested bool, now time.Time) (int64, error)
	// Claim marks the oldest queued job running, counting an attempt; false
	// when none is queued.
	Claim(ctx context.Context, now time.Time) (Job, bool, error)
	Finish(ctx context.Context, id int64, status Status, errText string, now time.Time) error
	// Recover puts running jobs back to queued, or fails those with at
	// least maxAttempts attempts. It returns how many it touched.
	Recover(ctx context.Context, maxAttempts int, now time.Time) (int64, error)
	// Queued reports whether a job of kind with payload waits, not yet running.
	Queued(ctx context.Context, kind airun.Kind, payload string) (bool, error)
}

// Checker is the cap guard as the runner uses it.
type Checker interface {
	Check(ctx context.Context, kind airun.Kind, requested bool) (Decision, error)
}

// Handler does one job's work.
type Handler func(ctx context.Context, job Job) error

// RunnerConfig wires a Runner.
type RunnerConfig struct {
	Store    JobStore
	Guard    Checker
	Handlers map[airun.Kind]Handler
	// Concurrency is how many jobs run at once (at least 1).
	Concurrency int
	// MaxAttempts: a job interrupted this often fails at boot recovery.
	MaxAttempts int
	Now         func() time.Time
	Logger      *slog.Logger
}

// Runner runs queued capped jobs in the background: at most Concurrency at
// a time, each after the cap guard allowed it. Jobs survive restarts in the
// store; Start recovers the ones a crash or shutdown left running.
type Runner struct {
	cfg  RunnerConfig
	wake chan struct{}
	stop chan struct{}
	once sync.Once
	wg   sync.WaitGroup
	// base is cancelled when a shutdown's wait runs out.
	base   context.Context
	cancel context.CancelFunc
}

// NewRunner returns a runner; Start begins work.
func NewRunner(cfg RunnerConfig) *Runner {
	cfg.Concurrency = max(cfg.Concurrency, 1)
	cfg.MaxAttempts = max(cfg.MaxAttempts, 1)
	base, cancel := context.WithCancel(context.Background())
	return &Runner{cfg: cfg, wake: make(chan struct{}, 1), stop: make(chan struct{}), base: base, cancel: cancel}
}

// Start recovers jobs left running and starts the workers.
func (r *Runner) Start(ctx context.Context) error {
	n, err := r.cfg.Store.Recover(ctx, r.cfg.MaxAttempts, r.cfg.Now())
	if err != nil {
		return fmt.Errorf("chef runner: recover: %w", err)
	}
	if n > 0 {
		r.cfg.Logger.InfoContext(ctx, "chef.jobs.recovered", "count", n)
	}
	for range r.cfg.Concurrency {
		r.wg.Add(1)
		go r.work()
	}
	r.signal()
	return nil
}

// Enqueue queues a job of kind; requested marks work the user asked for.
func (r *Runner) Enqueue(ctx context.Context, kind airun.Kind, payload string, requested bool) (int64, error) {
	if _, ok := r.cfg.Handlers[kind]; !ok {
		return 0, fmt.Errorf("chef runner: no handler for %q", kind)
	}
	id, err := r.cfg.Store.Enqueue(ctx, kind, payload, requested, r.cfg.Now())
	if err != nil {
		return 0, fmt.Errorf("chef runner: enqueue %s: %w", kind, err)
	}
	r.signal()
	return id, nil
}

// Queued reports whether a job of kind with payload waits to run.
func (r *Runner) Queued(ctx context.Context, kind airun.Kind, payload string) (bool, error) {
	return r.cfg.Store.Queued(ctx, kind, payload)
}

// Shutdown stops taking jobs and waits for the running ones. When ctx ends
// first it cancels them, waits for their handlers to return and returns
// ctx's error; the cancelled jobs stay running for boot recovery.
func (r *Runner) Shutdown(ctx context.Context) error {
	r.once.Do(func() { close(r.stop) })
	done := make(chan struct{})
	go func() {
		r.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		r.cancel()
		<-done
		return ctx.Err()
	}
}

// signal wakes one idle worker.
func (r *Runner) signal() {
	select {
	case r.wake <- struct{}{}:
	default:
	}
}

func (r *Runner) work() {
	defer r.wg.Done()
	for {
		select {
		case <-r.stop:
			return
		default:
		}
		ran, err := r.processOne(r.base)
		if err != nil {
			r.cfg.Logger.Error("chef.jobs.claim", "error", err)
		}
		if ran {
			continue
		}
		select {
		case <-r.wake:
		case <-r.stop:
			return
		}
	}
}

// processOne claims and runs the oldest queued job; false when none was
// queued or the claim failed.
func (r *Runner) processOne(ctx context.Context) (bool, error) {
	job, ok, err := r.cfg.Store.Claim(ctx, r.cfg.Now())
	if err != nil || !ok {
		return false, err
	}
	r.signal() // another worker may take the next job
	status, errText := r.run(ctx, job)
	if status == StatusRunning {
		return true, nil // cancelled by shutdown: boot recovery decides
	}
	if err := r.cfg.Store.Finish(context.WithoutCancel(ctx), job.ID, status, errText, r.cfg.Now()); err != nil {
		r.cfg.Logger.Error("chef.jobs.finish", "job", job.ID, "error", err)
	}
	return true, nil
}

// run asks the guard and runs the job's handler.
func (r *Runner) run(ctx context.Context, job Job) (Status, string) {
	log := r.cfg.Logger.With("job", job.ID, "kind", job.Kind, "requested", job.Requested)
	handler, ok := r.cfg.Handlers[job.Kind]
	if !ok {
		log.Error("chef.jobs.no_handler")
		return StatusFailed, "no handler"
	}
	d, err := r.cfg.Guard.Check(ctx, job.Kind, job.Requested)
	if err != nil {
		if r.base.Err() != nil {
			return StatusRunning, ""
		}
		log.Error("chef.jobs.guard", "error", err)
		return StatusFailed, err.Error()
	}
	if !d.Run {
		log.Info("chef.jobs.skipped", "reason", d.Reason)
		return StatusSkipped, string(d.Reason)
	}
	if err := handler(ctx, job); err != nil {
		if r.base.Err() != nil {
			return StatusRunning, ""
		}
		log.Warn("chef.jobs.failed", "error", err)
		return StatusFailed, err.Error()
	}
	return StatusDone, ""
}
