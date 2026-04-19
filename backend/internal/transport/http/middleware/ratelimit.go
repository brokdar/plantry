// Package middleware holds chi-compatible middleware local to Plantry.
package middleware

import (
	"net"
	"net/http"
	"sync"
	"time"
)

// RateLimiter implements a per-IP token bucket. It's safe for concurrent use
// across requests. The bucket map grows over the process lifetime; use
// StartJanitor to periodically evict idle keys.
type RateLimiter struct {
	capacity int           // max burst
	refill   time.Duration // time between token adds
	now      func() time.Time

	mu      sync.Mutex
	buckets map[string]*bucket
}

type bucket struct {
	tokens   int
	lastFill time.Time
	lastSeen time.Time
}

// NewRateLimiter creates a limiter admitting at most perMinute requests per
// IP on average, with a burst equal to perMinute. Setting perMinute <= 0
// disables rate limiting entirely.
func NewRateLimiter(perMinute int) *RateLimiter {
	if perMinute <= 0 {
		return &RateLimiter{capacity: 0}
	}
	return &RateLimiter{
		capacity: perMinute,
		refill:   time.Minute / time.Duration(perMinute),
		now:      time.Now,
		buckets:  map[string]*bucket{},
	}
}

// SetLimit reconfigures the per-minute limit at runtime. Existing buckets
// are cleared so the new capacity applies immediately; setting perMinute to
// 0 disables rate limiting.
func (r *RateLimiter) SetLimit(perMinute int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if perMinute <= 0 {
		r.capacity = 0
		r.refill = 0
		r.buckets = nil
		return
	}
	r.capacity = perMinute
	r.refill = time.Minute / time.Duration(perMinute)
	r.buckets = map[string]*bucket{}
	if r.now == nil {
		r.now = time.Now
	}
}

// Allow reports whether the key gets a token right now. If it does, the bucket
// is debited by 1.
func (r *RateLimiter) Allow(key string) bool {
	if r.capacity == 0 {
		return true
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	now := r.now()
	b := r.buckets[key]
	if b == nil {
		b = &bucket{tokens: r.capacity, lastFill: now}
		r.buckets[key] = b
	}
	// Refill based on elapsed time.
	elapsed := now.Sub(b.lastFill)
	add := int(elapsed / r.refill)
	if add > 0 {
		b.tokens += add
		if b.tokens > r.capacity {
			b.tokens = r.capacity
		}
		b.lastFill = b.lastFill.Add(time.Duration(add) * r.refill)
	}
	b.lastSeen = now
	if b.tokens <= 0 {
		return false
	}
	b.tokens--
	return true
}

// Middleware returns an http middleware that 429s over-limit requests.
// messageKey is written into the JSON error body.
func (r *RateLimiter) Middleware(messageKey string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if !r.Allow(clientIP(req)) {
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Retry-After", "60")
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = w.Write([]byte(`{"message_key":"` + messageKey + `","status":429}`))
				return
			}
			next.ServeHTTP(w, req)
		})
	}
}

// StartJanitor periodically evicts buckets that haven't been touched for
// retainFor. It runs until ctx is done. Call in a goroutine from main.
func (r *RateLimiter) StartJanitor(stop <-chan struct{}, interval, retainFor time.Duration) {
	if r.capacity == 0 {
		return
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case now := <-ticker.C:
			r.mu.Lock()
			for k, b := range r.buckets {
				if now.Sub(b.lastSeen) > retainFor {
					delete(r.buckets, k)
				}
			}
			r.mu.Unlock()
		}
	}
}

// clientIP extracts the best-effort client IP from the request. Expects chi's
// RealIP middleware to have already normalised X-Forwarded-For if present.
func clientIP(r *http.Request) string {
	if r.RemoteAddr == "" {
		return ""
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
