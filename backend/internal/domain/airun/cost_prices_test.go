package airun

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Z4-AC4 (AC-7.4): the built-in table pins each Anthropic family's list
// prices (USD per 1M tokens) and the web-search fee as of PricesAsOf.
func TestAnthropicListPricesPinned(t *testing.T) {
	assert.Equal(t, "2026-11-02", PricesAsOf)
	want := []struct {
		model                       string
		in, out, cacheRead, cacheWr float64
	}{
		{"claude-fable-5-1", 10, 50, 0.25, 12.5},
		{"claude-opus-5-5", 4, 20, 0.20, 5},
		{"claude-sonnet-5-5", 2, 10, 0.20, 2.5},
		{"claude-haiku-5-5", 0.10, 0.50, 0.01, 0.125},
		{"claude-opus-4-7", 5, 25, 0.50, 6.25},
		{"claude-opus-4-6", 5, 25, 0.50, 6.25},
		{"claude-sonnet-4-6", 3, 15, 0.30, 3.75},
		{"claude-haiku-4-5", 1, 5, 0.10, 1.25},
	}
	for _, w := range want {
		t.Run(w.model, func(t *testing.T) {
			p, src := LookupPrice(w.model, nil)
			require.Equal(t, PriceBuiltin, src)
			assert.InDelta(t, w.in, p.In, 1e-9)
			assert.InDelta(t, w.out, p.Out, 1e-9)
			require.NotNil(t, p.CacheRead)
			require.NotNil(t, p.CacheWrite)
			assert.InDelta(t, w.cacheRead, *p.CacheRead, 1e-9)
			assert.InDelta(t, w.cacheWr, *p.CacheWrite, 1e-9)
			assert.InDelta(t, 0.01, p.PerSearch, 1e-9, "web search: $10 per 1,000")
		})
	}
}

func TestOpenAIHasNoSearchFee(t *testing.T) {
	p, src := LookupPrice("gpt-5.5", nil)
	require.Equal(t, PriceBuiltin, src)
	assert.Zero(t, p.PerSearch)
}
