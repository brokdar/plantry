package discovery

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
)

// errShuttingDown: Run was called after Shutdown began.
var errShuttingDown = errors.New("discovery: shutting down")

// runner runs one discovery at a time in the background (D7). A run keeps
// the request's values but not its cancellation, ends after its timeout,
// and is waited for by shutdown.
type runner struct {
	mu      sync.Mutex
	running bool
	// id is the running run's ai_runs id, 0 until start.
	id int64
	// ready is closed once id is set or the slot is released.
	ready  chan struct{}
	closed bool
	wg     sync.WaitGroup
	base   context.Context
	cancel context.CancelFunc
}

func newRunner() *runner {
	base, cancel := context.WithCancel(context.Background())
	return &runner{base: base, cancel: cancel}
}

// acquire takes the single run slot.
func (r *runner) acquire() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	switch {
	case r.closed:
		return errShuttingDown
	case r.running:
		return domain.ErrDiscoveryRunning
	}
	r.running = true
	r.ready = make(chan struct{})
	r.wg.Add(1)
	return nil
}

// release frees the slot taken by acquire when no run started.
func (r *runner) release() {
	r.mu.Lock()
	r.running, r.id = false, 0
	r.signalReady()
	r.mu.Unlock()
	r.wg.Done()
}

// start runs f, the run id, in the background under a ctx detached from
// ctx's cancellation, bounded by timeout and cancelled by a shutdown whose
// wait ran out. The slot is freed when f returns.
func (r *runner) start(ctx context.Context, id int64, timeout time.Duration, f func(context.Context)) {
	r.mu.Lock()
	r.id = id
	r.signalReady()
	r.mu.Unlock()
	bg, cancel := context.WithTimeout(context.WithoutCancel(ctx), timeout)
	unhook := context.AfterFunc(r.base, cancel)
	go func() {
		defer r.release()
		defer unhook()
		defer cancel()
		f(bg)
	}()
}

// signalReady closes ready once; r.mu is held.
func (r *runner) signalReady() {
	select {
	case <-r.ready:
	default:
		if r.ready != nil {
			close(r.ready)
		}
	}
}

// current returns the running run's id; false when none runs. A run that
// holds the slot but has no id yet is waited for (its row is being written).
func (r *runner) current() (int64, bool) {
	r.mu.Lock()
	if r.running && r.id == 0 {
		ready := r.ready
		r.mu.Unlock()
		<-ready
		r.mu.Lock()
	}
	defer r.mu.Unlock()
	return r.id, r.running && r.id != 0
}

// wait blocks until no run is running.
func (r *runner) wait() { r.wg.Wait() }

// shutdown refuses new runs and waits for the running one. When ctx ends
// first it cancels the run, waits for it to record its end and returns
// ctx's error.
func (r *runner) shutdown(ctx context.Context) error {
	r.mu.Lock()
	r.closed = true
	r.mu.Unlock()
	done := make(chan struct{})
	go func() {
		r.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
	}
	r.cancel()
	<-done
	return ctx.Err()
}
