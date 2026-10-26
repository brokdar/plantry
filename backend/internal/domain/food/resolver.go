package food

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
)

// Resolver coordinates lookups across external food providers (OFF, FDC) and
// the local catalogue. It only resolves leaf foods — composed foods are user-
// authored and don't come from external sources.
type Resolver struct {
	repo       Repository
	off        BarcodeProvider // may be nil
	fdc        FoodProvider    // may be nil
	translator QueryTranslator
	picker     CandidatePicker
}

// SourceLocal marks a lookup candidate that already exists in the local
// catalogue; its ExistingID is always set.
const SourceLocal Source = "local"

const (
	maxLocalCandidates = 5
	leafPageSize       = 500
)

type noopTranslator struct{}

func (noopTranslator) Translate(_ context.Context, q string, _ *LookupTrace) string { return q }

type noopPicker struct{}

func (noopPicker) Pick(_ context.Context, _ string, _ []Candidate, _ *LookupTrace) int { return 0 }

// NewResolver creates a Resolver with optional external providers and LLM.
// A nil llmResolver disables AI features entirely.
func NewResolver(repo Repository, off BarcodeProvider, fdc FoodProvider, llmResolver llm.Resolver) *Resolver {
	var t QueryTranslator = noopTranslator{}
	var p CandidatePicker = noopPicker{}
	if llmResolver != nil {
		t = &llmTranslator{resolver: llmResolver}
		p = &llmPicker{resolver: llmResolver}
	}
	return &Resolver{repo: repo, off: off, fdc: fdc, translator: t, picker: p}
}

// Lookup searches for leaf-food candidates by barcode or text query.
//
// Barcode: tries OFF first, then FDC branded search. Query: local catalogue
// leaves first (exact, prefix, contains; at most 5), then FDC, then OFF by
// name. When FDC is queried with an LLM configured and a non-English query,
// the query is translated to an English FDC search term and the LLM picks the
// best semantic match, unless an exact local match is already recommended.
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
			r.enrichFromCatalogue(ctx, results)
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
			r.enrichFromCatalogue(ctx, results)
			return results, r.pickRecommended(ctx, barcode, results, trace), nil
		}
	}

	return []Candidate{}, -1, nil
}

func (r *Resolver) lookupQuery(ctx context.Context, query, lang string, limit int) ([]Candidate, int, error) {
	trace := TraceFromContext(ctx)

	originalQuery := query

	// A catalogue failure must not hide external results: trace it and carry
	// on without local leaves.
	leaves, err := r.listLeaves(ctx)
	if err != nil {
		trace.Add(TraceEntry{
			Step: "local.search", Level: TraceLevelError,
			Summary: "Local catalogue search failed",
			Detail:  ExternalAPIDetail{Source: SourceLocal, Query: originalQuery, Error: err.Error()},
		})
	}
	results, exactLocal := localCandidates(leaves, normalizeName(originalQuery))

	var external []Candidate
	if r.fdc != nil {
		searchTerm := query
		if lang != "" && lang != "en" {
			searchTerm = r.translator.Translate(ctx, originalQuery, trace)
		}

		start := time.Now()
		fdcResults, err := r.fdc.SearchByName(ctx, searchTerm, limit)
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
			Summary: resultCountSummary("FDC", len(fdcResults)), DurationMs: dur,
			Detail: ExternalAPIDetail{Source: "fdc", Query: searchTerm, ResultCount: len(fdcResults)},
		})

		for i := range fdcResults {
			if fdcResults[i].Source == SourceFDC {
				if fdcResults[i].SourceName == "" {
					fdcResults[i].SourceName = fdcResults[i].Name
				}
				fdcResults[i].Name = originalQuery
			}
		}
		external = append(external, fdcResults...)
	}

	if r.off != nil {
		// OFF is a supplementary source: a failure is traced but does not
		// discard the local and FDC results.
		start := time.Now()
		offResults, err := r.off.SearchByName(ctx, originalQuery, limit)
		dur := time.Since(start).Milliseconds()
		if err != nil {
			trace.Add(TraceEntry{
				Step: "off.search", Level: TraceLevelError,
				Summary: "OFF search failed", DurationMs: dur,
				Detail: ExternalAPIDetail{Source: "off", Query: originalQuery, Error: err.Error()},
			})
		} else {
			trace.Add(TraceEntry{
				Step: "off.search", Level: TraceLevelSuccess,
				Summary: resultCountSummary("OFF", len(offResults)), DurationMs: dur,
				Detail: ExternalAPIDetail{Source: "off", Query: originalQuery, ResultCount: len(offResults)},
			})
			external = append(external, offResults...)
		}
	}

	fillMissingKcal(external)
	enrichExistingIDs(leaves, external)
	results = dedupeCandidates(append(results, external...))

	switch {
	case len(results) == 0:
		return results, -1, nil
	case exactLocal || r.fdc == nil:
		return results, 0, nil
	}
	return results, r.pickRecommended(ctx, originalQuery, results, trace), nil
}

func (r *Resolver) pickRecommended(ctx context.Context, originalQuery string, candidates []Candidate, trace *LookupTrace) int {
	if len(candidates) <= 1 {
		return 0
	}
	return r.picker.Pick(ctx, originalQuery, candidates, trace)
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

// enrichFromCatalogue sets ExistingID on barcode results that match a local
// leaf. Catalogue errors are ignored: enrichment is best-effort.
func (r *Resolver) enrichFromCatalogue(ctx context.Context, candidates []Candidate) {
	leaves, err := r.listLeaves(ctx)
	if err != nil {
		return
	}
	enrichExistingIDs(leaves, candidates)
}

// catalogueLeaf pairs a local leaf with its normalized name, computed once
// per lookup.
type catalogueLeaf struct {
	food *Food
	norm string
}

// listLeaves loads every leaf food in the catalogue, page by page. This
// assumes a small single-user catalogue; if it grows large, switch to
// List{Search} and re-rank the hits with normalizeName instead.
func (r *Resolver) listLeaves(ctx context.Context) ([]catalogueLeaf, error) {
	var out []catalogueLeaf
	for offset := 0; ; offset += leafPageSize {
		page, err := r.repo.List(ctx, ListQuery{Kind: KindLeaf, Limit: leafPageSize, Offset: offset})
		if err != nil {
			return nil, fmt.Errorf("list local leaves (offset %d): %w", offset, err)
		}
		for i := range page.Items {
			out = append(out, catalogueLeaf{food: &page.Items[i], norm: normalizeName(page.Items[i].Name)})
		}
		if len(page.Items) < leafPageSize {
			return out, nil
		}
	}
}

// localCandidates returns up to maxLocalCandidates leaves whose normalized
// name equals the normalized query, then those starting with it, then those
// containing it. exact reports whether the first candidate is an exact match.
func localCandidates(leaves []catalogueLeaf, normQuery string) (cs []Candidate, exact bool) {
	if normQuery == "" {
		return []Candidate{}, false
	}
	var exacts, prefix, contains []Candidate
	for _, l := range leaves {
		switch {
		case l.norm == normQuery:
			exacts = append(exacts, localCandidate(l.food))
		case strings.HasPrefix(l.norm, normQuery):
			prefix = append(prefix, localCandidate(l.food))
		case strings.Contains(l.norm, normQuery):
			contains = append(contains, localCandidate(l.food))
		}
	}
	out := append(append(exacts, prefix...), contains...)
	if len(out) > maxLocalCandidates {
		out = out[:maxLocalCandidates]
	}
	if out == nil {
		return []Candidate{}, false
	}
	return out, len(exacts) > 0
}

func localCandidate(f *Food) Candidate {
	id := f.ID
	c := Candidate{
		Name:             f.Name,
		Source:           SourceLocal,
		ExistingID:       &id,
		Kcal100g:         f.Kcal100g,
		Protein100g:      f.Protein100g,
		Fat100g:          f.Fat100g,
		Carbs100g:        f.Carbs100g,
		Fiber100g:        f.Fiber100g,
		Sodium100g:       f.Sodium100g,
		SaturatedFat100g: f.SaturatedFat100g,
		TransFat100g:     f.TransFat100g,
		Cholesterol100g:  f.Cholesterol100g,
		Sugar100g:        f.Sugar100g,
		Potassium100g:    f.Potassium100g,
		Calcium100g:      f.Calcium100g,
		Iron100g:         f.Iron100g,
		Magnesium100g:    f.Magnesium100g,
		Phosphorus100g:   f.Phosphorus100g,
		Zinc100g:         f.Zinc100g,
		VitaminA100g:     f.VitaminA100g,
		VitaminC100g:     f.VitaminC100g,
		VitaminD100g:     f.VitaminD100g,
		VitaminB12100g:   f.VitaminB12100g,
		VitaminB6100g:    f.VitaminB6100g,
		Folate100g:       f.Folate100g,
	}
	if f.Barcode != nil {
		c.Barcode = *f.Barcode
	}
	return c
}

// enrichExistingIDs sets ExistingID on candidates whose normalized display
// name matches a local leaf. Query lookups call it after FDC results have
// been renamed to the original query, so the match is against the query and
// not the FDC description.
func enrichExistingIDs(leaves []catalogueLeaf, candidates []Candidate) {
	byName := make(map[string]int64, len(leaves))
	for _, l := range leaves {
		if _, ok := byName[l.norm]; !ok {
			byName[l.norm] = l.food.ID
		}
	}
	for i := range candidates {
		if id, ok := byName[normalizeName(candidates[i].Name)]; ok {
			candidates[i].ExistingID = &id
		}
	}
}

// dedupeCandidates drops repeated (source, external id) pairs, keeping the
// first occurrence. Candidates without an external id are always kept.
func dedupeCandidates(cs []Candidate) []Candidate {
	seen := make(map[string]struct{}, len(cs))
	out := cs[:0]
	for _, c := range cs {
		key := candidateKey(c)
		if key != "" {
			if _, dup := seen[key]; dup {
				continue
			}
			seen[key] = struct{}{}
		}
		out = append(out, c)
	}
	return out
}

func candidateKey(c Candidate) string {
	switch {
	case c.Source == SourceLocal && c.ExistingID != nil:
		return fmt.Sprintf("local:%d", *c.ExistingID)
	case c.Source == SourceFDC && c.FdcID != 0:
		return fmt.Sprintf("fdc:%d", c.FdcID)
	case c.Source == SourceOFF && c.Barcode != "":
		return "off:" + c.Barcode
	}
	return ""
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
