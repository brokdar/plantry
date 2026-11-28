package main

import (
	"context"
	"slices"
	"strings"

	"github.com/jaltszeimer/plantry/backend/internal/domain/diet"
	"github.com/jaltszeimer/plantry/backend/internal/domain/gates"
	"github.com/jaltszeimer/plantry/backend/internal/domain/kitchen"
	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
)

// gatesHousehold reads what the substitution options must fit: the diet, the
// allergens the restriction chips name and the never items.
func gatesHousehold(
	prof func(context.Context) (*profile.Profile, error),
	never func(context.Context) ([]kitchen.Food, error),
) func(context.Context) (gates.Household, error) {
	return func(ctx context.Context) (gates.Household, error) {
		p, err := prof(ctx)
		if err != nil {
			return gates.Household{}, err
		}
		var h gates.Household
		switch p.DietType {
		case profile.DietVegan, profile.DietVegetarian, profile.DietPescetarian:
			h.Diet = diet.Class(p.DietType)
		}
		for _, code := range gates.AllergenCodes {
			if slices.ContainsFunc(p.DietaryRestrictions, func(r string) bool { return strings.Contains(strings.ToLower(r), code) }) {
				h.Allergens = append(h.Allergens, code)
			}
		}
		items, err := never(ctx)
		if err != nil {
			return gates.Household{}, err
		}
		for _, f := range items {
			h.NeverFoodIDs = append(h.NeverFoodIDs, f.ID)
			h.NeverNames = append(h.NeverNames, f.Name)
		}
		return h, nil
	}
}
