package anthropic

import "time"

// SetNow replaces the clock the capability cache uses.
func SetNow(c *Client, now func() time.Time) { c.now = now }
