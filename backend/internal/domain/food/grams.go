package food

import (
	"context"
	"fmt"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/units"
)

// PortionLookup is the minimal repository surface ResolveGrams needs to look
// up food-specific (unit → grams) overrides. The full food.Repository
// satisfies it; tests can pass a one-method stub.
type PortionLookup interface {
	ListPortions(ctx context.Context, foodID int64) ([]Portion, error)
}

// ResolveGrams resolves (amount, unit) into grams for a leaf food using the
// shared layered fallback chain:
//
//  1. Food-specific portion override (FDC/OFF-sourced or user-added) — exact.
//  2. Universal default (mass direct, volume water-density) — exact for mass,
//     approximate for volume.
//  3. Manual grams (only when caller passed an explicit manualGrams > 0) —
//     used for count/unknown units that didn't match a portion.
//
// Returns the resolved grams and the matching GramsSource* label. Bare mass
// units (g/kg/mg) skip the portion lookup since they're already exact.
//
// Callers must validate that foodID refers to a leaf food before calling —
// composed foods don't carry their own portion table and have no leaf-style
// quantity on a plate.
func ResolveGrams(
	ctx context.Context,
	portions PortionLookup,
	foodID int64,
	amount float64,
	unit string,
	manualGrams float64,
) (grams float64, source string, err error) {
	normalized := units.Normalize(unit)
	if normalized == "" {
		return 0, "", fmt.Errorf("%w: unit required", domain.ErrInvalidInput)
	}

	// 1. Food-specific portion lookup (skip bare mass units — they're direct).
	if normalized != "g" && normalized != "kg" && normalized != "mg" {
		ps, err := portions.ListPortions(ctx, foodID)
		if err != nil {
			return 0, "", fmt.Errorf("resolve food %d portions: %w", foodID, err)
		}
		for _, p := range ps {
			if units.Normalize(p.Unit) == normalized {
				return amount * p.Grams, GramsSourcePortion, nil
			}
		}
	}

	// 2. Universal default.
	if def, ok := units.LookupDefault(normalized); ok {
		g := amount * def.Grams
		switch {
		case def.Kind == units.KindMass && normalized == "g":
			return g, GramsSourceDirect, nil
		case def.Kind == units.KindMass:
			return g, GramsSourceDefault, nil
		default:
			return g, GramsSourceFallback, nil
		}
	}

	// 3. Manual fallback.
	if manualGrams > 0 {
		return manualGrams, GramsSourceManual, nil
	}

	if units.IsCount(normalized) {
		return 0, "", fmt.Errorf("%w: unit %q requires a portion or manual grams",
			domain.ErrInvalidInput, normalized)
	}
	return 0, "", fmt.Errorf("%w: unknown unit %q", domain.ErrInvalidInput, normalized)
}
