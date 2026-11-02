package airun

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

// Price is a model's price in USD per 1M tokens. A nil cache price falls back
// to a fixed fraction of In that depends on the provider (spec §8.1).
type Price struct {
	In         float64  `json:"in"`
	Out        float64  `json:"out"`
	CacheRead  *float64 `json:"cache_read"`
	CacheWrite *float64 `json:"cache_write"`
	// PerSearch is the USD price of one server-side web search.
	PerSearch float64 `json:"per_search"`
}

// Tokens are a run's summed token counts. For OpenAI, Input includes the
// cached tokens (prompt_tokens); for Anthropic it does not.
type Tokens struct {
	Input      int64
	Output     int64
	CacheRead  int64
	CacheWrite int64
	// WebSearchRequests counts the billed server-side web searches.
	WebSearchRequests int64
}

// PriceSource says where a model's price came from.
type PriceSource string

const (
	PriceBuiltin  PriceSource = "builtin"
	PriceOverride PriceSource = "override"
	PriceNone     PriceSource = "none"
)

// ErrInvalidPrices marks a malformed ai.price_overrides value.
var ErrInvalidPrices = errors.New("invalid price overrides")

// PricesAsOf is the date the built-in table was copied from the vendors'
// public pricing pages.
const PricesAsOf = "2026-10-31"

type priceEntry struct {
	Prefix string
	Price  Price
}

// anthropicPerSearch is Anthropic's web search price: $10 per 1,000 searches.
const anthropicPerSearch = 0.01

func p4(in, out, cacheRead, cacheWrite float64) Price {
	return Price{In: in, Out: out, CacheRead: &cacheRead, CacheWrite: &cacheWrite, PerSearch: anthropicPerSearch}
}

// pOAI is an OpenAI price; cached < 0 means the page lists no cached price.
func pOAI(in, cached, out float64) Price {
	p := Price{In: in, Out: out}
	if cached >= 0 {
		p.CacheRead = &cached
	}
	return p
}

// builtinPrices is keyed by model-id prefix; the longest matching prefix wins,
// so dated snapshot ids (gpt-4o-2024-08-06) resolve to their family.
// Tiers and long-context surcharges are ignored (spec §8.1): Haiku 5.5 uses
// its ≤ 100k-token tier, the OpenAI flagships their < 272k-context price.
//
// Sources, fetched 2026-10-31:
//   - Anthropic: https://platform.claude.com/docs/en/about-claude/pricing
//     (cache write = the 5-minute write price, which the adapter requests)
//   - OpenAI: https://developers.openai.com/api/docs/pricing (Standard tier)
var builtinPrices = []priceEntry{
	// Anthropic: in, out, cache read, 5m cache write.
	{"claude-fable-5-1", p4(10, 50, 0.25, 12.5)},
	{"claude-opus-5-5", p4(4, 20, 0.20, 5)},
	{"claude-sonnet-5-5", p4(2, 10, 0.10, 2.5)},
	{"claude-haiku-5-5", p4(0.10, 0.50, 0.01, 0.125)},
	{"claude-opus-4-7", p4(5, 25, 0.50, 6.25)},
	{"claude-opus-4-6", p4(5, 25, 0.50, 6.25)},
	{"claude-sonnet-4-6", p4(3, 15, 0.30, 3.75)},
	{"claude-haiku-4-5", p4(1, 5, 0.10, 1.25)},

	// OpenAI: in, cached in, out.
	{"gpt-6-astra", pOAI(10, 1, 50)},
	{"gpt-6.1-sol", pOAI(2, 0.10, 10)},
	{"gpt-6.1-terra", pOAI(1, 0.05, 5)},
	{"gpt-6-luna", pOAI(0.10, 0.01, 0.50)},
	{"gpt-6-sol", pOAI(2, 0.20, 10)},
	{"gpt-5.6-sol", pOAI(4, 0.40, 20)},
	{"gpt-5.6-terra", pOAI(2, 0.20, 12)},
	{"gpt-5.6-luna", pOAI(0.20, 0.02, 1.20)},
	{"gpt-5.5", pOAI(5, 0.50, 30)},
	{"gpt-5.5-pro", pOAI(30, -1, 180)},
	{"gpt-5.4", pOAI(2.50, 0.25, 15)},
	{"gpt-5.4-pro", pOAI(30, -1, 180)},
	{"gpt-5.4-mini", pOAI(0.75, 0.075, 4.50)},
	{"gpt-5.4-nano", pOAI(0.20, 0.02, 1.25)},
	{"gpt-5.3-codex", pOAI(1.75, 0.175, 14)},
	{"gpt-5.2", pOAI(1.75, 0.175, 14)},
	{"gpt-5.2-pro", pOAI(21, -1, 168)},
	{"gpt-5.1", pOAI(1.25, 0.125, 10)},
	{"gpt-5", pOAI(1.25, 0.125, 10)},
	{"gpt-5-mini", pOAI(0.25, 0.025, 2)},
	{"gpt-5-nano", pOAI(0.05, 0.005, 0.40)},
	{"gpt-5-pro", pOAI(15, -1, 120)},
	{"gpt-4.1", pOAI(2, 0.50, 8)},
	{"gpt-4.1-mini", pOAI(0.40, 0.10, 1.60)},
	{"gpt-4.1-nano", pOAI(0.10, 0.025, 0.40)},
	{"gpt-4o", pOAI(2.50, 1.25, 10)},
	{"gpt-4o-2024-05-13", pOAI(5, -1, 15)},
	{"gpt-4o-mini", pOAI(0.15, 0.075, 0.60)},
	{"gpt-4-turbo", pOAI(10, -1, 30)},
	{"gpt-3.5-turbo", pOAI(0.50, -1, 1.50)},
	{"o1", pOAI(15, 7.50, 60)},
	{"o1-pro", pOAI(150, -1, 600)},
	{"o3", pOAI(2, 0.50, 8)},
	{"o3-pro", pOAI(20, -1, 80)},
	{"o3-mini", pOAI(1.10, 0.55, 4.40)},
	{"o4-mini", pOAI(1.10, 0.275, 4.40)},
}

// LookupPrice returns the price of model: an override for the exact id, else
// the built-in entry with the longest matching prefix.
func LookupPrice(model string, overrides map[string]Price) (Price, PriceSource) {
	return lookupPrice(builtinPrices, model, overrides)
}

func lookupPrice(table []priceEntry, model string, overrides map[string]Price) (Price, PriceSource) {
	if model == "" {
		return Price{}, PriceNone
	}
	if p, ok := overrides[model]; ok {
		return p, PriceOverride
	}
	best := -1
	for i, e := range table {
		if strings.HasPrefix(model, e.Prefix) && (best < 0 || len(e.Prefix) > len(table[best].Prefix)) {
			best = i
		}
	}
	if best < 0 {
		return Price{}, PriceNone
	}
	return table[best].Price, PriceBuiltin
}

// Cost estimates a run's cost in USD, or nil when the model has no price. A
// run without tokens (a heuristic run) costs 0 whatever its model. Missing
// cache prices default to 0.1× (read) and 1.25× (write) of the input price,
// or 0.5× and 0 for OpenAI, whose input count includes the cached tokens.
func Cost(model, provider string, t Tokens, overrides map[string]Price) *float64 {
	if t == (Tokens{}) {
		zero := 0.0
		return &zero
	}
	p, src := LookupPrice(model, overrides)
	if src == PriceNone {
		return nil
	}
	readFrac, writeFrac := 0.1, 1.25
	input := t.Input
	if provider == "openai" {
		readFrac, writeFrac = 0.5, 0
		input = max(t.Input-t.CacheRead, 0)
	}
	cacheRead, cacheWrite := p.In*readFrac, p.In*writeFrac
	if p.CacheRead != nil {
		cacheRead = *p.CacheRead
	}
	if p.CacheWrite != nil {
		cacheWrite = *p.CacheWrite
	}
	usd := (float64(input)*p.In + float64(t.Output)*p.Out +
		float64(t.CacheRead)*cacheRead + float64(t.CacheWrite)*cacheWrite) / 1e6
	usd += float64(t.WebSearchRequests) * p.PerSearch
	return &usd
}

// overrideEntry requires in and out; the cache prices are optional.
type overrideEntry struct {
	In         *float64 `json:"in"`
	Out        *float64 `json:"out"`
	CacheRead  *float64 `json:"cache_read"`
	CacheWrite *float64 `json:"cache_write"`
	PerSearch  *float64 `json:"per_search"`
}

// ParsePriceOverrides parses ai.price_overrides: a JSON object mapping a model
// id to {in, out, cache_read?, cache_write?} in USD per 1M tokens, plus per_search?
// (USD per web search; omitted keeps the built-in price: 0.01 for Anthropic models, else 0; an explicit 0 is 0). Prices must
// be non-negative. An empty string is an empty map.
func ParsePriceOverrides(raw string) (map[string]Price, error) {
	out := map[string]Price{}
	if strings.TrimSpace(raw) == "" {
		return out, nil
	}
	dec := json.NewDecoder(bytes.NewReader([]byte(raw)))
	dec.DisallowUnknownFields()
	var entries map[string]overrideEntry
	if err := dec.Decode(&entries); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidPrices, err)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("%w: trailing data", ErrInvalidPrices)
	}
	if entries == nil {
		return nil, fmt.Errorf("%w: must be an object", ErrInvalidPrices)
	}
	for model, e := range entries {
		if strings.TrimSpace(model) == "" {
			return nil, fmt.Errorf("%w: empty model id", ErrInvalidPrices)
		}
		if e.In == nil || e.Out == nil {
			return nil, fmt.Errorf("%w: %s needs in and out", ErrInvalidPrices, model)
		}
		for _, v := range []*float64{e.In, e.Out, e.CacheRead, e.CacheWrite, e.PerSearch} {
			if v != nil && *v < 0 {
				return nil, fmt.Errorf("%w: %s has a negative price", ErrInvalidPrices, model)
			}
		}
		price := Price{In: *e.In, Out: *e.Out, CacheRead: e.CacheRead, CacheWrite: e.CacheWrite}
		// An omitted per_search keeps the built-in (family) search price.
		if e.PerSearch != nil {
			price.PerSearch = *e.PerSearch
		} else if b, src := LookupPrice(model, nil); src == PriceBuiltin {
			price.PerSearch = b.PerSearch
		}
		out[model] = price
	}
	return out, nil
}
