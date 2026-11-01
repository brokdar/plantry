package planning

import (
	"fmt"
	"sync"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
)

// Stage is where a running generation is (D35).
type Stage string

// Stages, in order.
const (
	StageResolving Stage = "resolving"
	StageChoosing  Stage = "choosing"
	StageChecking  Stage = "checking"
	StageRepairing Stage = "repairing"
	StageSaving    Stage = "saving"
)

// Progress is the in-memory registry of generation runs (D9): the stage of
// each running run and the one run each window may have running. A finished
// run drops out at once: its ai_runs row is the record from then on. It
// implements planview's Progress port. Safe for concurrent use.
type Progress struct {
	mu sync.Mutex
	// windows maps a window start to its running run (0 while it starts).
	windows map[string]int64
	runs    map[int64]*progressEntry
}

type progressEntry struct {
	window string
	stage  Stage
	// done is closed when the run finished (Await).
	done chan struct{}
}

// NewProgress returns an empty registry.
func NewProgress() *Progress {
	return &Progress{windows: map[string]int64{}, runs: map[int64]*progressEntry{}}
}

func windowKey(start time.Time) string { return start.Format(time.DateOnly) }

// reserve claims the window for a run about to start, or fails with
// ErrGenerationRunning while another one runs there.
func (p *Progress) reserve(start time.Time) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	k := windowKey(start)
	if _, busy := p.windows[k]; busy {
		return fmt.Errorf("%w: %s", domain.ErrGenerationRunning, k)
	}
	p.windows[k] = 0
	return nil
}

// release frees a window whose run did not start.
func (p *Progress) release(start time.Time) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.windows, windowKey(start))
}

// begin records the run that started on a reserved window.
func (p *Progress) begin(start time.Time, runID int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	k := windowKey(start)
	p.windows[k] = runID
	p.runs[runID] = &progressEntry{window: k, stage: StageResolving, done: make(chan struct{})}
}

func (p *Progress) setStage(runID int64, st Stage) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if e, ok := p.runs[runID]; ok {
		e.stage = st
	}
}

// finish drops the run, frees its window and wakes its waiters.
func (p *Progress) finish(runID int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	e, ok := p.runs[runID]
	if !ok {
		return
	}
	if p.windows[e.window] == runID {
		delete(p.windows, e.window)
	}
	delete(p.runs, runID)
	close(e.done)
}

// doneCh returns the channel closed when a running run finished; ok is false
// for any other run.
func (p *Progress) doneCh(runID int64) (<-chan struct{}, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	e, ok := p.runs[runID]
	if !ok {
		return nil, false
	}
	return e.done, true
}

// Stage returns a running run's stage; ok is false for any other run.
func (p *Progress) Stage(runID int64) (Stage, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	e, ok := p.runs[runID]
	if !ok {
		return "", false
	}
	return e.stage, true
}

// Running returns the run generating the window that starts on start.
func (p *Progress) Running(start time.Time) (int64, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	id := p.windows[windowKey(start)]
	return id, id != 0
}
