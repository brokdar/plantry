package anthropic

import (
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
)

// SetNow replaces the clock the capability cache uses.
func SetNow(c *Client, now func() time.Time) { c.now = now }

// CapabilityTTLLeft is how long model's cached web search answer has left.
func CapabilityTTLLeft(c *Client, model string) time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.caps[model].until.Sub(c.now())
}

// SetLookupTimeout replaces the bound on one capability lookup.
func SetLookupTimeout(c *Client, d time.Duration) { c.lookupTimeout = d }

// TailIndex is the message and block that take the tail breakpoint (-1, -1:
// none), as encodeRequest decides it when the code tool is sent.
func TailIndex(messages []llm.Message) (msg, block int) {
	msgs := make([]messageBody, len(messages))
	for i, m := range messages {
		msgs[i].Content = encodeContent(m.Content)
	}
	return tailIndex(msgs, codeCalledIDs(messages))
}
