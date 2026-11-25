package chef

import "context"

// ProcessOne runs the oldest queued job in the caller's goroutine, so tests
// can drive simulated days without waiting on workers.
func (r *Runner) ProcessOne(ctx context.Context) (bool, error) { return r.processOne(ctx) }
