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

// TailIndex is the block of the last message that takes the tail breakpoint
// (-1: none), as encodeRequest decides it when the code tool is sent.
func TailIndex(messages []llm.Message) int {
	return tailIndex(encodeContent(messages[len(messages)-1].Content), codeCalledIDs(messages))
}
