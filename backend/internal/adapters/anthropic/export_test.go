package anthropic

import "time"

// SetNow replaces the clock the capability cache uses.
func SetNow(c *Client, now func() time.Time) { c.now = now }

// CapabilityTTLLeft is how long model's cached web search answer has left.
func CapabilityTTLLeft(c *Client, model string) time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.caps[model].until.Sub(c.now())
}
