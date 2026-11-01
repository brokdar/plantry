package planning_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/airun"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
)

// enteredChooser signals when the run chooses, then waits for its context.
type enteredChooser struct{ entered chan struct{} }

func (c enteredChooser) Choose(ctx context.Context, _ planning.ChooseInput) (planning.Proposal, error) {
	close(c.entered)
	<-ctx.Done()
	return planning.Proposal{}, ctx.Err()
}

// Shutdown cancels a running generation at once instead of waiting out its
// timeout, ends it as an error and refuses new runs.
func TestService_ShutdownCancelsRunningRuns(t *testing.T) {
	e := newSvcEnv(t)
	c := enteredChooser{entered: make(chan struct{})}
	e.deps.Go = nil
	e.deps.Choosers = map[airun.Chooser]planning.Chooser{airun.ChooserHeuristic: c}
	e.svc = planning.NewService(e.deps)
	id := e.generate(e.req())
	<-c.entered

	ctx, cancel := context.WithTimeout(e.ctx, 5*time.Second)
	defer cancel()
	require.NoError(t, e.svc.Shutdown(ctx))

	end := e.runs.end(id)
	assert.Equal(t, airun.StatusError, end.Status)
	assert.Equal(t, "error.server", end.ErrorKey)
	_, running := e.svc.Running(e.start)
	assert.False(t, running)
	_, err := e.svc.Generate(e.ctx, e.req())
	assert.Error(t, err, "no new run after Shutdown")
}

// Shutdown gives up waiting when its context ends.
func TestService_ShutdownIsBounded(t *testing.T) {
	e := newSvcEnv(t)
	b := blockingChooser{entered: make(chan struct{}), release: make(chan struct{})}
	e.deps.Go = nil
	e.deps.Choosers = map[airun.Chooser]planning.Chooser{airun.ChooserHeuristic: b}
	e.svc = planning.NewService(e.deps)
	e.generate(e.req())
	<-b.entered

	ctx, cancel := context.WithTimeout(e.ctx, 20*time.Millisecond)
	defer cancel()
	assert.ErrorIs(t, e.svc.Shutdown(ctx), context.DeadlineExceeded)
	close(b.release)
	e.svc.Wait()
}

// panicFinishLog's runs panic when they finish.
type panicFinishLog struct{ *fakeRuns }

type panicFinishRun struct{ planning.Run }

func (l panicFinishLog) Start(ctx context.Context, s planning.RunStart) (context.Context, planning.Run, error) {
	ctx, run, err := l.fakeRuns.Start(ctx, s)
	return ctx, panicFinishRun{run}, err
}

func (panicFinishRun) Finish(context.Context, planning.RunEnd) { panic("finish bug") }

// A run whose finish panics still frees its window.
func TestService_FinishPanicFreesTheWindow(t *testing.T) {
	e := newSvcEnv(t)
	e.deps.RunLog = panicFinishLog{e.runs}
	e.svc = planning.NewService(e.deps)

	assert.Panics(t, func() { _, _ = e.svc.Generate(e.ctx, e.req()) })

	_, running := e.svc.Running(e.start)
	assert.False(t, running)
	e.deps.RunLog = e.runs
	e.svc = planning.NewService(e.deps)
	e.ok(e.generate(e.req()))
}

// lostFinishLog's runs never write their end: the row stays running, as
// when FinishRun fails.
type lostFinishLog struct{ *fakeRuns }

type lostFinishRun struct{ planning.Run }

func (l lostFinishLog) Start(ctx context.Context, s planning.RunStart) (context.Context, planning.Run, error) {
	ctx, run, err := l.fakeRuns.Start(ctx, s)
	return ctx, lostFinishRun{run}, err
}

func (lostFinishRun) Finish(context.Context, planning.RunEnd) {}

// A run row left running that no generation runs reads as an error, so a
// client polling it stops.
func TestService_RunningRowWithoutARunIsAnError(t *testing.T) {
	e := newSvcEnv(t)
	e.deps.RunLog = lostFinishLog{e.runs}
	e.svc = planning.NewService(e.deps)
	id := e.generate(e.req())

	v, err := e.svc.Run(e.ctx, id)

	require.NoError(t, err)
	assert.Equal(t, airun.StatusError, v.Status)
	assert.Equal(t, "error.server", v.ErrorKey)
	assert.Empty(t, v.Stage)
}

// readyChooser is an LLM chooser whose provider reports err.
type readyChooser struct{ err error }

func (readyChooser) Choose(ctx context.Context, in planning.ChooseInput) (planning.Proposal, error) {
	return planning.Heuristic{}.Choose(ctx, in)
}

func (c readyChooser) Ready(context.Context) error { return c.err }

// Without a chooser in the request: the LLM when its provider resolves, the
// heuristic when none is configured, and an error (no silent fallback) when
// a configured one fails.
func TestService_DefaultChooser(t *testing.T) {
	tests := []struct {
		name    string
		ready   error
		want    airun.Chooser
		wantErr error
	}{
		{"provider resolves", nil, airun.ChooserLLM, nil},
		{"no provider", fmt.Errorf("%w: unset", domain.ErrAIProviderMissing), airun.ChooserHeuristic, nil},
		{"provider broken", fmt.Errorf("%w: bad key", domain.ErrAIProviderFailed), "", domain.ErrAIProviderFailed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newSvcEnv(t)
			e.deps.Choosers[airun.ChooserLLM] = readyChooser{err: tt.ready}
			e.svc = planning.NewService(e.deps)

			id, err := e.svc.Generate(e.ctx, e.req())

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				_, running := e.svc.Running(e.start)
				assert.False(t, running)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, e.runs.start(id).Chooser)
		})
	}
}

// meddlingChooser has the user fill an empty target cell while it chooses.
type meddlingChooser struct{ e *svcEnv }

func (c meddlingChooser) Choose(ctx context.Context, in planning.ChooseInput) (planning.Proposal, error) {
	c.e.userPlate(tue, c.e.lunch, c.e.mains[9])
	return planning.Heuristic{}.Choose(ctx, in)
}

// A run whose save finds the plan changed under it ends with its own error
// key and writes nothing.
func TestService_PlanChangedDuringRun(t *testing.T) {
	e := newSvcEnv(t)
	e.deps.Choosers = map[airun.Chooser]planning.Chooser{airun.ChooserHeuristic: meddlingChooser{e}}
	e.svc = planning.NewService(e.deps)

	id := e.generate(e.req())

	end := e.runs.end(id)
	assert.Equal(t, airun.StatusError, end.Status)
	assert.Equal(t, "error.plan.changed_during_run", end.ErrorKey)
	var n int
	require.NoError(t, e.db.QueryRow(`SELECT COUNT(*) FROM plates WHERE ai_run_id = ?`, id).Scan(&n))
	assert.Zero(t, n)
	assert.Len(t, e.window(), 1, "only the user's plate")
}

// lateChooser proposes, then uses up the run's time.
type lateChooser struct{}

func (lateChooser) Choose(ctx context.Context, in planning.ChooseInput) (planning.Proposal, error) {
	p, err := planning.Heuristic{}.Choose(ctx, in)
	<-ctx.Done()
	return p, err
}

// A proposal ready at the run's deadline is still saved: saving has its own
// budget.
func TestService_SavesAProposalReadyAtTheDeadline(t *testing.T) {
	e := newSvcEnv(t)
	e.deps.Choosers = map[airun.Chooser]planning.Chooser{airun.ChooserHeuristic: lateChooser{}}
	e.deps.RunTimeout = 2 * time.Second // resolving fits, even under -race
	e.svc = planning.NewService(e.deps)

	e.ok(e.generate(e.req()))
	assert.NotEmpty(t, e.window())
}
