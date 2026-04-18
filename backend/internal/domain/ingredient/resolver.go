package ingredient

import (
	"context"
	"strings"
)

// Resolver coordinates lookups across external food providers and the local repository.
type Resolver struct {
	repo Repository
	off  BarcodeProvider // may be nil
	fdc  FoodProvider    // may be nil
}

// NewResolver creates a Resolver with optional external providers.
func NewResolver(repo Repository, off BarcodeProvider, fdc FoodProvider) *Resolver {
	return &Resolver{repo: repo, off: off, fdc: fdc}
}

// Lookup searches for food candidates by barcode or text query.
//
// For barcode: tries OFF first, then falls back to FDC branded search.
// For query: searches FDC only (Foundation/SR Legacy).
// Returns an empty slice (not an error) when nothing is found.
func (r *Resolver) Lookup(ctx context.Context, barcode, query, lang string, limit int) ([]Candidate, error) {
	if barcode != "" {
		return r.lookupBarcode(ctx, barcode, limit)
	}
	if query != "" {
		return r.lookupQuery(ctx, query, limit)
	}
	return []Candidate{}, nil
}

func (r *Resolver) lookupBarcode(ctx context.Context, barcode string, _ int) ([]Candidate, error) {
	// Try OFF barcode lookup first.
	if r.off != nil {
		results, err := r.off.LookupBarcode(ctx, barcode)
		if err != nil {
			return nil, err
		}
		if len(results) > 0 {
			fillMissingKcal(results)
			r.enrichExistingIDs(ctx, results)
			return results, nil
		}
	}

	// Fall back to FDC branded search using the barcode as query.
	if r.fdc != nil {
		results, err := r.fdc.SearchByName(ctx, barcode, 5)
		if err != nil {
			return nil, err
		}
		if len(results) > 0 {
			fillMissingKcal(results)
			r.enrichExistingIDs(ctx, results)
			return results, nil
		}
	}

	return []Candidate{}, nil
}

func (r *Resolver) lookupQuery(ctx context.Context, query string, limit int) ([]Candidate, error) {
	if r.fdc == nil {
		return []Candidate{}, nil
	}

	results, err := r.fdc.SearchByName(ctx, query, limit)
	if err != nil {
		return nil, err
	}

	fillMissingKcal(results)
	r.enrichExistingIDs(ctx, results)
	return results, nil
}

// fillMissingKcal derives kcal_100g from macros using Atwater factors
// (4·protein + 4·carbs + 9·fat) whenever the upstream provider returned nil.
// Some FDC entries (notably raw meats) have macros but no kcal; without this
// the UI silently defaults kcal to 0 and every downstream total is wrong.
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

// enrichExistingIDs checks if any candidate matches a local ingredient by name.
func (r *Resolver) enrichExistingIDs(ctx context.Context, candidates []Candidate) {
	for i := range candidates {
		result, err := r.repo.List(ctx, ListQuery{Search: candidates[i].Name, Limit: 1})
		if err != nil || len(result.Items) == 0 {
			continue
		}
		if strings.EqualFold(result.Items[0].Name, candidates[i].Name) {
			id := result.Items[0].ID
			candidates[i].ExistingID = &id
		}
	}
}
