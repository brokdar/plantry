// Package devclock is the one clock behind "today", plan windows, eating
// history and fading. In production it is time.Now; a dev-mode run can move it
// (PLANTRY_DEV_CLOCK at startup, PUT /api/dev/clock between simulated days) so
// a time-lapse trial covers weeks in minutes.
package devclock

import (
	"fmt"
	"sync"
	"time"
)

// Clock reads real time plus a movable offset. The zero offset is time.Now.
type Clock struct {
	mu     sync.RWMutex
	offset time.Duration
}

// New returns a clock that reads real time.
func New() *Clock { return &Clock{} }

// Now returns the simulated time; the clock keeps ticking after a Set.
func (c *Clock) Now() time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return time.Now().Add(c.offset)
}

// Set makes the clock read t now.
func (c *Clock) Set(t time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.offset = time.Until(t)
}

// Advance moves the clock forward by d.
func (c *Clock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.offset += d
}

// Parse reads an RFC 3339 time or a YYYY-MM-DD date (taken as 08:00 UTC).
func Parse(s string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	d, err := time.Parse(time.DateOnly, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("want RFC 3339 or YYYY-MM-DD, got %q", s)
	}
	return d.Add(8 * time.Hour), nil
}
