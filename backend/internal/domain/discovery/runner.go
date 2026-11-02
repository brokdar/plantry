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
	closed  bool
	wg      sync.WaitGroup
	base    context.Context
	cancel  context.CancelFunc
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
	r.wg.Add(1)
	return nil
}

// release frees the slot taken by acquire when no run started.
func (r *runner) release() {
	r.mu.Lock()
	r.running = false
	r.mu.Unlock()
	r.wg.Done()
}

// start runs f in the background under a ctx detached from ctx's
// cancellation, bounded by timeout and cancelled by a shutdown whose wait
// ran out. The slot is freed when f returns.
func (r *runner) start(ctx context.Context, timeout time.Duration, f func(context.Context)) {
	bg, cancel := context.WithTimeout(context.WithoutCancel(ctx), timeout)
	unhook := context.AfterFunc(r.base, cancel)
	go func() {
		defer r.release()
		defer unhook()
		defer cancel()
		f(bg)
	}()
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
