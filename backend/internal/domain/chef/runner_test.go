package chef_test

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/crypto"
	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain/airun"
	"github.com/jaltszeimer/plantry/backend/internal/domain/chef"
	"github.com/jaltszeimer/plantry/backend/internal/domain/settings"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
)

// clock is a stored time the test moves; workers read it concurrently.
type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) Now() time.Time  { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *clock) Set(t time.Time) { c.mu.Lock(); c.t = t; c.mu.Unlock() }

type env struct {
	db       *sql.DB
	settings *settings.Service
	runs     *sqlite.AIRunRepo
	jobs     *sqlite.ChefJobRepo
	clock    *clock
	guard    *chef.Guard
}

// newEnv is a database with the fake provider on a priced model; the
// switch is off (the default) unless on.
func newEnv(t *testing.T, on bool) *env {
	t.Helper()
	ctx := context.Background()
	db := testhelper.NewTestDB(t)
	svc := settings.NewService(sqlite.NewAppSettingsRepo(db), settings.NewEnvSnapshot(nil), crypto.NilCipher{})
	require.NoError(t, svc.Set(ctx, settings.KeyAIProvider, "fake"))
	require.NoError(t, svc.Set(ctx, settings.KeyAIModel, "fake-model"))
	require.NoError(t, svc.Set(ctx, settings.KeyAIPriceOverrides, `{"fake-model":{"in":1,"out":1}}`))
	if on {
		require.NoError(t, svc.Set(ctx, settings.KeyChefEnabled, "true"))
	}
	e := &env{
		db: db, settings: svc, runs: sqlite.NewAIRunRepo(db), jobs: sqlite.NewChefJobRepo(db),
		clock: &clock{t: time.Date(2026, 11, 1, 8, 0, 0, 0, time.UTC)},
	}
	e.guard = &chef.Guard{
		Settings: svc, Spend: e.runs, Now: e.clock.Now,
		Prices: func(ctx context.Context) (map[string]airun.Price, error) {
			v, err := svc.Get(ctx, settings.KeyAIPriceOverrides)
			if err != nil {
				return nil, err
			}
			return airun.ParsePriceOverrides(v.Raw)
		},
	}
	return e
}

func (e *env) runner(handlers map[airun.Kind]chef.Handler, concurrency int) *chef.Runner {
	return chef.NewRunner(chef.RunnerConfig{
		Store: e.jobs, Guard: e.guard, Handlers: handlers, Concurrency: concurrency,
		MaxAttempts: 2, Now: e.clock.Now, Logger: slog.New(slog.DiscardHandler),
	})
}

// recordRun is a handler that logs a run of the job's kind costing cost.
func (e *env) recordRun(cost float64) chef.Handler {
	return func(ctx context.Context, job chef.Job) error {
		id, err := e.runs.CreateRun(ctx, airun.RunStart{Kind: job.Kind, StartedAt: e.clock.Now()})
		if err != nil {
			return err
		}
		return e.runs.FinishRun(ctx, airun.RunEnd{ID: id, Status: airun.StatusOK, FinishedAt: e.clock.Now(), CostUSD: &cost})
	}
}

func (e *env) status(t *testing.T, id int64) (status string, attempts int, errText string) {
	t.Helper()
	require.NoError(t, e.db.QueryRow(`SELECT status, attempts, error FROM chef_jobs WHERE id = ?`, id).Scan(&status, &attempts, &errText))
	return status, attempts, errText
}

func (e *env) runCount(t *testing.T) int {
	t.Helper()
	var n int
	require.NoError(t, e.db.QueryRow(`SELECT COUNT(*) FROM ai_runs`).Scan(&n))
	return n
}

func drain(t *testing.T, r *chef.Runner) {
	t.Helper()
	for {
		ran, err := r.ProcessOne(context.Background())
		require.NoError(t, err)
		if !ran {
			return
		}
	}
}

var autonomousKinds = []airun.Kind{
	airun.KindChefAttributes, airun.KindChefMatch, airun.KindChefImportCheck,
	airun.KindChefFind, airun.KindChefLibraryReview,
}

// AC-1: with chef.enabled off, a simulated month of daily autonomous work
// logs no capped run; the same day with the switch on does.
func TestRunner_SwitchOffMonthLogsNoCappedRun(t *testing.T) {
	ctx := context.Background()
	for _, on := range []bool{false, true} {
		e := newEnv(t, on)
		handlers := map[airun.Kind]chef.Handler{}
		for _, k := range autonomousKinds {
			handlers[k] = e.recordRun(0.001)
		}
		r := e.runner(handlers, 1)
		days := 30
		if on {
			days = 1
		}
		var ids []int64
		for day := range days {
			e.clock.Set(time.Date(2026, 11, 1+day, 8, 0, 0, 0, time.UTC))
			for _, k := range autonomousKinds {
				id, err := r.Enqueue(ctx, k, "", false)
				require.NoError(t, err)
				ids = append(ids, id)
			}
			drain(t, r)
		}
		if !on {
			assert.Zero(t, e.runCount(t), "switch off: no capped run in a month")
			for _, id := range ids {
				st, _, reason := e.status(t, id)
				assert.Equal(t, "skipped", st)
				assert.Equal(t, string(chef.ReasonOff), reason)
			}
			continue
		}
		assert.Equal(t, len(autonomousKinds), e.runCount(t), "switch on: the same jobs run")
	}
}

// AC-17 skeleton: at the cap the runner skips autonomous jobs and runs
// requested ones; the requested check carries the warning.
func TestRunner_CapStopsAutonomousRunsRequested(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t, true)
	r := e.runner(map[airun.Kind]chef.Handler{airun.KindChefFind: e.recordRun(5)}, 1)

	first, err := r.Enqueue(ctx, airun.KindChefFind, "", false)
	require.NoError(t, err)
	drain(t, r)
	st, _, _ := e.status(t, first)
	require.Equal(t, "done", st, "under the cap autonomous work runs")

	// That run cost 5: the month is at the cap.
	auto, err := r.Enqueue(ctx, airun.KindChefFind, "", false)
	require.NoError(t, err)
	req, err := r.Enqueue(ctx, airun.KindChefFind, "", true)
	require.NoError(t, err)
	drain(t, r)
	st, _, reason := e.status(t, auto)
	assert.Equal(t, "skipped", st)
	assert.Equal(t, string(chef.ReasonCap), reason)
	st, _, _ = e.status(t, req)
	assert.Equal(t, "done", st, "requested work runs at the cap")

	d, err := e.guard.Check(ctx, airun.KindChefFind, true)
	require.NoError(t, err)
	assert.True(t, d.Warn, "requested work at the cap warns")
	d, err = e.guard.Check(ctx, airun.KindChefHelp, true)
	require.NoError(t, err)
	assert.Equal(t, chef.Decision{Run: true}, d, "help runs without a warning")

	// Next month the cap resets.
	e.clock.Set(time.Date(2026, 12, 1, 8, 0, 0, 0, time.UTC))
	next, err := r.Enqueue(ctx, airun.KindChefFind, "", false)
	require.NoError(t, err)
	drain(t, r)
	st, _, _ = e.status(t, next)
	assert.Equal(t, "done", st)
}

// At most Concurrency jobs run at once.
func TestRunner_ConcurrencyLimit(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t, true)
	var active, peak atomic.Int32
	started := make(chan struct{}, 8)
	release := make(chan struct{})
	h := func(context.Context, chef.Job) error {
		n := active.Add(1)
		for {
			p := peak.Load()
			if n <= p || peak.CompareAndSwap(p, n) {
				break
			}
		}
		started <- struct{}{}
		<-release
		active.Add(-1)
		return nil
	}
	r := e.runner(map[airun.Kind]chef.Handler{airun.KindChefFind: h}, 2)
	require.NoError(t, r.Start(ctx))
	const jobs = 5
	for range jobs {
		_, err := r.Enqueue(ctx, airun.KindChefFind, "", false)
		require.NoError(t, err)
	}
	<-started
	<-started
	assert.EqualValues(t, 2, active.Load(), "two run, the rest wait")
	for i := range jobs {
		release <- struct{}{}
		if i+2 < jobs {
			<-started
		}
	}
	require.NoError(t, r.Shutdown(ctx))
	assert.EqualValues(t, 2, peak.Load())
	var done int
	require.NoError(t, e.db.QueryRow(`SELECT COUNT(*) FROM chef_jobs WHERE status = 'done'`).Scan(&done))
	assert.Equal(t, jobs, done)
}

// Shutdown waits for the running job to finish.
func TestRunner_ShutdownWaitsForRunningJob(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t, true)
	started, release := make(chan struct{}), make(chan struct{})
	h := func(context.Context, chef.Job) error {
		close(started)
		<-release
		return nil
	}
	r := e.runner(map[airun.Kind]chef.Handler{airun.KindChefFind: h}, 1)
	require.NoError(t, r.Start(ctx))
	id, err := r.Enqueue(ctx, airun.KindChefFind, "", false)
	require.NoError(t, err)
	<-started

	stopped := make(chan error, 1)
	go func() { stopped <- r.Shutdown(ctx) }()
	select {
	case <-stopped:
		t.Fatal("Shutdown returned while a job was running")
	default:
	}
	close(release)
	require.NoError(t, <-stopped)
	st, _, _ := e.status(t, id)
	assert.Equal(t, "done", st)
}

// A shutdown whose wait runs out cancels the job; it stays running, and
// the next boot puts it back to queued, or fails it once it was
// interrupted MaxAttempts times. A recovered job runs again.
func TestRunner_ShutdownCancelsAndBootRecovers(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t, true)
	started := make(chan struct{}, 1)
	blockUntilCancelled := func(ctx context.Context, _ chef.Job) error {
		started <- struct{}{}
		<-ctx.Done()
		return ctx.Err()
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()

	boot := func(h chef.Handler) *chef.Runner {
		r := e.runner(map[airun.Kind]chef.Handler{airun.KindChefFind: h}, 1)
		require.NoError(t, r.Start(ctx))
		return r
	}

	r := boot(blockUntilCancelled)
	id, err := r.Enqueue(ctx, airun.KindChefFind, "", false)
	require.NoError(t, err)
	<-started
	require.ErrorIs(t, r.Shutdown(cancelled), context.Canceled)
	st, attempts, _ := e.status(t, id)
	require.Equal(t, "running", st, "an interrupted job is left for boot recovery")
	require.Equal(t, 1, attempts)

	// Second boot: queued again (1 of 2 attempts) and interrupted again.
	r = boot(blockUntilCancelled)
	<-started
	require.ErrorIs(t, r.Shutdown(cancelled), context.Canceled)
	st, attempts, _ = e.status(t, id)
	require.Equal(t, "running", st)
	require.Equal(t, 2, attempts)

	// Third boot: two interruptions reach MaxAttempts, so it fails.
	ran := make(chan struct{}, 1)
	r = boot(func(context.Context, chef.Job) error { ran <- struct{}{}; return nil })
	require.NoError(t, r.Shutdown(ctx))
	st, _, errText := e.status(t, id)
	assert.Equal(t, "failed", st)
	assert.Contains(t, errText, "interrupted")
	assert.Empty(t, ran, "a failed job does not run")
}

// A shutdown that cancels while the guard is still deciding leaves the job
// running for boot recovery, like a cancel inside the handler.
func TestRunner_ShutdownDuringGuardLeavesJobRunning(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t, true)
	inGuard := make(chan struct{}, 1)
	e.guard.Prices = func(ctx context.Context) (map[string]airun.Price, error) {
		inGuard <- struct{}{}
		<-ctx.Done()
		return nil, ctx.Err()
	}
	handlerRan := false
	r := e.runner(map[airun.Kind]chef.Handler{airun.KindChefFind: func(context.Context, chef.Job) error {
		handlerRan = true
		return nil
	}}, 1)
	require.NoError(t, r.Start(ctx))
	id, err := r.Enqueue(ctx, airun.KindChefFind, "", false)
	require.NoError(t, err)
	<-inGuard
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	require.ErrorIs(t, r.Shutdown(cancelled), context.Canceled)
	st, _, _ := e.status(t, id)
	assert.Equal(t, "running", st, "an interrupted job is left for boot recovery")
	assert.False(t, handlerRan)
}

// A recovered job under the limit runs at boot.
func TestRunner_BootRunsRecoveredJob(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t, true)
	id, err := e.jobs.Enqueue(ctx, airun.KindChefFind, "", false, e.clock.Now())
	require.NoError(t, err)
	_, ok, err := e.jobs.Claim(ctx, e.clock.Now()) // a crash left it running
	require.NoError(t, err)
	require.True(t, ok)

	ran := make(chan struct{})
	r := e.runner(map[airun.Kind]chef.Handler{airun.KindChefFind: func(context.Context, chef.Job) error {
		close(ran)
		return nil
	}}, 1)
	require.NoError(t, r.Start(ctx))
	<-ran
	require.NoError(t, r.Shutdown(ctx))
	st, attempts, _ := e.status(t, id)
	assert.Equal(t, "done", st)
	assert.Equal(t, 2, attempts)
}

func TestRunner_HandlerErrorFailsJob(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t, true)
	r := e.runner(map[airun.Kind]chef.Handler{airun.KindChefFind: func(context.Context, chef.Job) error {
		return errors.New("boom")
	}}, 1)
	id, err := r.Enqueue(ctx, airun.KindChefFind, "", false)
	require.NoError(t, err)
	drain(t, r)
	st, _, errText := e.status(t, id)
	assert.Equal(t, "failed", st)
	assert.Equal(t, "boom", errText)

	_, err = r.Enqueue(ctx, airun.KindChefMatch, "", false)
	assert.Error(t, err, "a kind without a handler is refused")
}

func TestRunner_QueuedUntilClaimed(t *testing.T) {
	e := newEnv(t, true)
	ctx := context.Background()
	r := e.runner(map[airun.Kind]chef.Handler{airun.KindChefAttributes: e.recordRun(0)}, 1)
	_, err := r.Enqueue(ctx, airun.KindChefAttributes, `{"food_ids":[1]}`, false)
	require.NoError(t, err)

	queued, err := r.Queued(ctx, airun.KindChefAttributes, `{"food_ids":[1]}`)
	require.NoError(t, err)
	assert.True(t, queued)
	queued, err = r.Queued(ctx, airun.KindChefAttributes, `{"food_ids":[2]}`)
	require.NoError(t, err)
	assert.False(t, queued, "another payload")

	drain(t, r)
	queued, err = r.Queued(ctx, airun.KindChefAttributes, `{"food_ids":[1]}`)
	require.NoError(t, err)
	assert.False(t, queued, "it ran")
}

// A job the guard skipped or that failed is reported, so what it was for
// does not wait on it.
func TestRunner_ReportsSkippedAndFailedJobs(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t, false) // switch off: the guard skips the job
	var mu sync.Mutex
	got := map[int64]chef.Status{}
	reasons := map[int64]string{}
	r := chef.NewRunner(chef.RunnerConfig{
		Store: e.jobs, Guard: e.guard, Concurrency: 1, MaxAttempts: 2, Now: e.clock.Now,
		Logger: slog.New(slog.DiscardHandler),
		Handlers: map[airun.Kind]chef.Handler{airun.KindChefFind: func(context.Context, chef.Job) error {
			return errors.New("boom")
		}},
		OnUnfinished: func(_ context.Context, job chef.Job, st chef.Status, errText string) {
			mu.Lock()
			defer mu.Unlock()
			got[job.ID], reasons[job.ID] = st, errText
		},
	})
	skipped, err := r.Enqueue(ctx, airun.KindChefFind, "1", false)
	require.NoError(t, err)
	drain(t, r)
	require.NoError(t, e.settings.Set(ctx, settings.KeyChefEnabled, "true"))
	failed, err := r.Enqueue(ctx, airun.KindChefFind, "2", false)
	require.NoError(t, err)
	drain(t, r)
	assert.Equal(t, chef.StatusSkipped, got[skipped])
	assert.Equal(t, string(chef.ReasonOff), reasons[skipped])
	assert.Equal(t, chef.StatusFailed, got[failed])
}
