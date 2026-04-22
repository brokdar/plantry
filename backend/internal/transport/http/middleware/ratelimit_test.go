package middleware_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/transport/http/middleware"
)

func TestRateLimiter_AllowsUpToBurst(t *testing.T) {
	rl := middleware.NewRateLimiter(5)
	for i := 0; i < 5; i++ {
		assert.True(t, rl.Allow("1.2.3.4"), "request %d should pass", i+1)
	}
	assert.False(t, rl.Allow("1.2.3.4"), "6th should be denied")
}

func TestRateLimiter_KeyedPerIP(t *testing.T) {
	rl := middleware.NewRateLimiter(1)
	assert.True(t, rl.Allow("a"))
	assert.True(t, rl.Allow("b"))
	assert.False(t, rl.Allow("a"))
	assert.False(t, rl.Allow("b"))
}

func TestRateLimiter_DisabledAllowsAll(t *testing.T) {
	rl := middleware.NewRateLimiter(0)
	for i := 0; i < 100; i++ {
		assert.True(t, rl.Allow("x"))
	}
}

func TestRateLimiter_SetLimit_Reconfigures(t *testing.T) {
	rl := middleware.NewRateLimiter(2)
	assert.True(t, rl.Allow("ip"))
	assert.True(t, rl.Allow("ip"))
	assert.False(t, rl.Allow("ip"), "bucket exhausted")

	// Reconfiguring clears existing buckets and lets traffic through again.
	rl.SetLimit(5)
	for i := 0; i < 5; i++ {
		assert.True(t, rl.Allow("ip"), "request %d should pass after reconfig", i+1)
	}
	assert.False(t, rl.Allow("ip"))
}

func TestRateLimiter_SetLimit_ToZero_Disables(t *testing.T) {
	rl := middleware.NewRateLimiter(1)
	assert.True(t, rl.Allow("ip"))
	assert.False(t, rl.Allow("ip"))

	rl.SetLimit(0)
	for i := 0; i < 50; i++ {
		assert.True(t, rl.Allow("ip"))
	}
}

func TestRateLimiter_Middleware_Returns429(t *testing.T) {
	rl := middleware.NewRateLimiter(2)
	h := rl.Middleware("error.ai.rate_limit_exceeded")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))

	call := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		req.RemoteAddr = "10.0.0.1:5000"
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		return w
	}

	assert.Equal(t, 200, call().Code)
	assert.Equal(t, 200, call().Code)
	w := call()
	assert.Equal(t, http.StatusTooManyRequests, w.Code)
	assert.Equal(t, "60", w.Result().Header.Get("Retry-After"))
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "error.ai.rate_limit_exceeded", body["message_key"])
}

func TestRateLimiter_Middleware_DifferentIPsNotShared(t *testing.T) {
	rl := middleware.NewRateLimiter(1)
	h := rl.Middleware("x")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))

	req1 := httptest.NewRequest(http.MethodPost, "/", nil)
	req1.RemoteAddr = "1.1.1.1:1"
	req2 := httptest.NewRequest(http.MethodPost, "/", nil)
	req2.RemoteAddr = "2.2.2.2:2"

	w1 := httptest.NewRecorder()
	w2 := httptest.NewRecorder()
	h.ServeHTTP(w1, req1)
	h.ServeHTTP(w2, req2)
	assert.Equal(t, 200, w1.Code)
	assert.Equal(t, 200, w2.Code)
}

// To keep the suite fast, use a tiny refill interval so we can observe a
// token arriving without a real minute of delay. This exercises the refill
// path directly rather than via Middleware.
func TestRateLimiter_RefillsOverTime(t *testing.T) {
	rl := middleware.NewRateLimiter(60) // 1 token per second
	assert.True(t, rl.Allow("k"))
	// Consume the burst.
	for i := 0; i < 60; i++ {
		rl.Allow("k")
	}
	assert.False(t, rl.Allow("k"))
	// Wait for ~1.2s to get one token back.
	time.Sleep(1100 * time.Millisecond)
	assert.True(t, rl.Allow("k"))
}
