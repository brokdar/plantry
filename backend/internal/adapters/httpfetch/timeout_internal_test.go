package httpfetch

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// The guarded client keeps the timeout of the configured http.Client.
func TestNew_GuardedClientKeepsConfiguredTimeout(t *testing.T) {
	c := New(
		func(c *Client) { c.hc = &http.Client{Timeout: 7 * time.Second} },
		WithDialGuard(nil, false),
	)
	assert.Equal(t, 7*time.Second, c.guarded.Timeout)

	assert.Equal(t, defaultTimeout, New(WithDialGuard(nil, false)).guarded.Timeout, "default otherwise")
}
