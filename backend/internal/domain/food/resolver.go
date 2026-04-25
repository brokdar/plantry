package food

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
)

// Resolver coordinates lookups across external food providers (OFF, FDC) and
// the local catalogue. It only resolves leaf foods — composed foods are user-
// authored and don't come from external sources.
type Resolver struct {
	repo Repository
	off  BarcodeProvider // may be nil
	fdc  FoodProvider    // may be nil
	llm  llm.Resolver    // may be nil — when set, powers AI translation + pick-best
}

// NewResolver creates a Resolver with optional external providers and LLM.
// A nil llm disables AI features entirely.
func NewResolver(repo Repository, off BarcodeProvider, fdc FoodProvider, llmResolver llm.Resolver) *Resolver {
	return &Resolver{repo: repo, off: off, fdc: fdc, llm: llmResolver}
}

// Lookup searches for leaf-food candidates by barcode or text query.
//
// Barcode: tries OFF first, then FDC branded search. Query: FDC only. When an
// LLM is configured and the query is non-English, the query is translated to
// an English FDC search term; after results come back, the LLM picks the best
// semantic match.
//
// Returns (candidates, recommended_index, error). Empty results → index = -1.
func (r *Resolver) Lookup(ctx context.Context, barcode, query, lang string, limit int) ([]Candidate, int, error) {
	if barcode != "" {
		return r.lookupBarcode(ctx, barcode, limit)
	}
	if query != "" {
		return r.lookupQuery(ctx, query, lang, limit)
	}
	return []Candidate{}, -1, nil
}

func (r *Resolver) lookupBarcode(ctx context.Context, barcode string, _ int) ([]Candidate, int, error) {
	trace := TraceFromContext(ctx)

	if r.off != nil {
		start := time.Now()
		results, err := r.off.LookupBarcode(ctx, barcode)
		dur := time.Since(start).Milliseconds()
		if err != nil {
			trace.Add(TraceEntry{
				Step: "off.lookup_barcode", Level: TraceLevelError,
				Summary: "OFF barcode lookup failed", DurationMs: dur,
				Detail: ExternalAPIDetail{Source: "off", Barcode: barcode, Error: err.Error()},
			})
			return nil, -1, err
		}
		trace.Add(TraceEntry{
			Step: "off.lookup_barcode", Level: TraceLevelSuccess,
			Summary: resultCountSummary("OFF", len(results)), DurationMs: dur,
			Detail: ExternalAPIDetail{Source: "off", Barcode: barcode, ResultCount: len(results)},
		})
		if len(results) > 0 {
			fillMissingKcal(results)
			r.enrichExistingIDs(ctx, results)
			return results, r.pickRecommended(ctx, barcode, results, trace), nil
		}
	}

	if r.fdc != nil {
		start := time.Now()
		results, err := r.fdc.SearchByName(ctx, barcode, 5)
		dur := time.Since(start).Milliseconds()
		if err != nil {
			trace.Add(TraceEntry{
				Step: "fdc.search_barcode", Level: TraceLevelError,
				Summary: "FDC barcode search failed", DurationMs: dur,
				Detail: ExternalAPIDetail{Source: "fdc", Query: barcode, Error: err.Error()},
			})
			return nil, -1, err
		}
		trace.Add(TraceEntry{
			Step: "fdc.search_barcode", Level: TraceLevelSuccess,
			Summary: resultCountSummary("FDC", len(results)), DurationMs: dur,
			Detail: ExternalAPIDetail{Source: "fdc", Query: barcode, ResultCount: len(results)},
		})
		if len(results) > 0 {
			fillMissingKcal(results)
			r.enrichExistingIDs(ctx, results)
			return results, r.pickRecommended(ctx, barcode, results, trace), nil
		}
	}

	return []Candidate{}, -1, nil
}

func (r *Resolver) lookupQuery(ctx context.Context, query, lang string, limit int) ([]Candidate, int, error) {
	if r.fdc == nil {
		return []Candidate{}, -1, nil
	}
	trace := TraceFromContext(ctx)

	originalQuery := query
	searchTerm := query

	if r.llm != nil && lang != "" && lang != "en" {
		if client, model, err := r.llm.Current(ctx); err == nil && client != nil {
			searchTerm = translateQuery(ctx, client, model, originalQuery, trace)
		} else if err != nil {
			trace.Add(TraceEntry{
				Step: "ai.translate", Level: TraceLevelInfo,
				Summary: "AI skipped: " + err.Error(),
				Detail:  AITranslationDetail{InputQuery: originalQuery, Error: err.Error()},
			})
		}
	}

	start := time.Now()
	results, err := r.fdc.SearchByName(ctx, searchTerm, limit)
	dur := time.Since(start).Milliseconds()
	if err != nil {
		trace.Add(TraceEntry{
			Step: "fdc.search", Level: TraceLevelError,
			Summary: "FDC search failed", DurationMs: dur,
			Detail: ExternalAPIDetail{Source: "fdc", Query: searchTerm, Error: err.Error()},
		})
		return nil, -1, err
	}
	trace.Add(TraceEntry{
		Step: "fdc.search", Level: TraceLevelSuccess,
		Summary: resultCountSummary("FDC", len(results)), DurationMs: dur,
		Detail: ExternalAPIDetail{Source: "fdc", Query: searchTerm, ResultCount: len(results)},
	})

	fillMissingKcal(results)
	r.enrichExistingIDs(ctx, results)

	for i := range results {
		if results[i].Source == SourceFDC {
			if results[i].SourceName == "" {
				results[i].SourceName = results[i].Name
			}
			results[i].Name = originalQuery
		}
	}

	if len(results) == 0 {
		return results, -1, nil
	}
	return results, r.pickRecommended(ctx, originalQuery, results, trace), nil
}

func (r *Resolver) pickRecommended(ctx context.Context, originalQuery string, candidates []Candidate, trace *LookupTrace) int {
	if r.llm == nil || len(candidates) <= 1 {
		return 0
	}
	client, model, err := r.llm.Current(ctx)
	if err != nil || client == nil {
		if err != nil && !errors.Is(err, context.Canceled) {
			trace.Add(TraceEntry{
				Step: "ai.pick_best", Level: TraceLevelInfo,
				Summary: "AI skipped: " + err.Error(),
			})
		}
		return 0
	}
	return pickBest(ctx, client, model, originalQuery, candidates, trace)
}

// fillMissingKcal derives kcal_100g from macros using Atwater factors whenever
// the upstream provider returned nil.
func fillMissingKcal(cs []Candidate) {
	for i := range cs {
		c := &cs[i]
		if c.Kcal100g != nil {
			continue
		}
		if c.Protein100g == nil && c.Fat100g == nil && c.Carbs100g == nil {
			continue
		}
		var p, f, carbs float64
		if c.Protein100g != nil {
			p = *c.Protein100g
		}
		if c.Fat100g != nil {
			f = *c.Fat100g
		}
		if c.Carbs100g != nil {
			carbs = *c.Carbs100g
		}
		if carbs < 0 {
			carbs = 0
		}
		kcal := 4*p + 4*carbs + 9*f
		c.Kcal100g = &kcal
	}
}

// enrichExistingIDs checks if any candidate matches a local leaf food by name.
func (r *Resolver) enrichExistingIDs(ctx context.Context, candidates []Candidate) {
	for i := range candidates {
		result, err := r.repo.List(ctx, ListQuery{Kind: KindLeaf, Search: candidates[i].Name, Limit: 1})
		if err != nil || len(result.Items) == 0 {
			continue
		}
		if strings.EqualFold(result.Items[0].Name, candidates[i].Name) {
			id := result.Items[0].ID
			candidates[i].ExistingID = &id
		}
	}
}

func resultCountSummary(source string, n int) string {
	switch n {
	case 0:
		return source + " returned 0 results"
	case 1:
		return source + " returned 1 result"
	default:
		return fmt.Sprintf("%s returned %d results", source, n)
	}
}
