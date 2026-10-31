package airun

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func ptr(f float64) *float64 { return &f }

func TestLookupPrice(t *testing.T) {
	table := []priceEntry{
		{Prefix: "claude-opus-5", Price: Price{In: 5, Out: 25}},
		{Prefix: "claude-opus-5-5", Price: Price{In: 4, Out: 20}},
		{Prefix: "gpt-5", Price: Price{In: 1.25, Out: 10}},
	}
	overrides := map[string]Price{"claude-opus-5-5": {In: 1, Out: 2}, "my-local": {In: 0.5, Out: 0.5}}

	tests := []struct {
		name      string
		model     string
		overrides map[string]Price
		wantIn    float64
		wantSrc   PriceSource
	}{
		{"longest prefix wins", "claude-opus-5-5", nil, 4, PriceBuiltin},
		{"longest prefix wins for a dated id", "claude-opus-5-5-20261001", nil, 4, PriceBuiltin},
		{"shorter prefix", "claude-opus-5-20260101", nil, 5, PriceBuiltin},
		{"override for the exact id wins", "claude-opus-5-5", overrides, 1, PriceOverride},
		{"override is exact, not a prefix", "claude-opus-5-5-20261001", overrides, 4, PriceBuiltin},
		{"override adds a model", "my-local", overrides, 0.5, PriceOverride},
		{"unknown model", "llama-3", overrides, 0, PriceNone},
		{"empty model", "", nil, 0, PriceNone},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, src := lookupPrice(table, tt.model, tt.overrides)
			assert.Equal(t, tt.wantSrc, src)
			assert.InDelta(t, tt.wantIn, p.In, 1e-9)
		})
	}
}

func TestCost(t *testing.T) {
	overrides := map[string]Price{
		// No cache prices: the provider fractions apply.
		"anthro-x": {In: 10, Out: 20},
		"openai-x": {In: 10, Out: 20},
		"explicit": {In: 10, Out: 20, CacheRead: ptr(1), CacheWrite: ptr(3)},
	}
	million := int64(1_000_000)
	tests := []struct {
		name     string
		model    string
		provider string
		tokens   Tokens
		want     *float64
	}{
		{
			name: "anthropic cache defaults: read 0.1x, write 1.25x", model: "anthro-x", provider: "anthropic",
			tokens: Tokens{Input: million, Output: million, CacheRead: million, CacheWrite: million},
			// 10 + 20 + 1 + 12.5
			want: ptr(43.5),
		},
		{
			name: "openai cached tokens are subtracted; cache defaults read 0.5x, write 0", model: "openai-x", provider: "openai",
			tokens: Tokens{Input: 2 * million, Output: million, CacheRead: million, CacheWrite: million},
			// (2M-1M)*10 + 1M*20 + 1M*5 + 0
			want: ptr(35),
		},
		{
			name: "explicit cache prices", model: "explicit", provider: "anthropic",
			tokens: Tokens{Input: million, CacheRead: million, CacheWrite: million},
			want:   ptr(14),
		},
		{
			name: "builtin price", model: "claude-sonnet-4-6", provider: "anthropic",
			tokens: Tokens{Input: 1000},
			want:   ptr(0.003),
		},
		{name: "unknown model", model: "llama-3", provider: "ollama", tokens: Tokens{Input: 10}, want: nil},
		{name: "no tokens cost nothing, even unpriced", model: "", provider: "", tokens: Tokens{}, want: ptr(0)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Cost(tt.model, tt.provider, tt.tokens, overrides)
			if tt.want == nil {
				assert.Nil(t, got)
				return
			}
			require.NotNil(t, got)
			assert.InDelta(t, *tt.want, *got, 1e-9)
		})
	}
}

func TestBuiltinPricesCoverTheKnownModels(t *testing.T) {
	for _, id := range []string{
		"claude-opus-5-5", "claude-sonnet-5-5", "claude-haiku-5-5", "claude-fable-5-1",
		"claude-opus-4-7", "claude-opus-4-6", "claude-sonnet-4-6", "claude-haiku-4-5",
		"gpt-5", "gpt-5-mini", "gpt-4.1", "gpt-4o", "o3", "o4-mini",
	} {
		_, src := LookupPrice(id, nil)
		assert.Equal(t, PriceBuiltin, src, id)
	}
	for _, e := range builtinPrices {
		assert.Positive(t, e.Price.In, e.Prefix)
		assert.Positive(t, e.Price.Out, e.Prefix)
	}
}

func TestParsePriceOverrides(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    map[string]Price
		wantErr bool
	}{
		{name: "empty", raw: "", want: map[string]Price{}},
		{name: "empty object", raw: "{}", want: map[string]Price{}},
		{
			name: "full entry", raw: `{"m":{"in":1,"out":2,"cache_read":0.1,"cache_write":1.25}}`,
			want: map[string]Price{"m": {In: 1, Out: 2, CacheRead: ptr(0.1), CacheWrite: ptr(1.25)}},
		},
		{name: "cache prices optional", raw: `{"m":{"in":1,"out":2}}`, want: map[string]Price{"m": {In: 1, Out: 2}}},
		{name: "zero prices allowed", raw: `{"m":{"in":0,"out":0}}`, want: map[string]Price{"m": {}}},
		{name: "array", raw: `[]`, wantErr: true},
		{name: "string", raw: `"x"`, wantErr: true},
		{name: "not json", raw: `{`, wantErr: true},
		{name: "entry not an object", raw: `{"m":5}`, wantErr: true},
		{name: "negative input", raw: `{"m":{"in":-1,"out":2}}`, wantErr: true},
		{name: "negative cache read", raw: `{"m":{"in":1,"out":2,"cache_read":-0.1}}`, wantErr: true},
		{name: "missing out", raw: `{"m":{"in":1}}`, wantErr: true},
		{name: "unknown field", raw: `{"m":{"in":1,"out":2,"foo":1}}`, wantErr: true},
		{name: "empty model id", raw: `{"":{"in":1,"out":2}}`, wantErr: true},
		{name: "trailing data", raw: `{} {}`, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParsePriceOverrides(tt.raw)
			if tt.wantErr {
				assert.ErrorIs(t, err, ErrInvalidPrices)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
