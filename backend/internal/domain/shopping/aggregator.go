package shopping

import (
	"sort"

	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

// ComponentRef carries the ingredient breakdown for one component together
// with the reference portion count used when the component was built.
type ComponentRef struct {
	ReferencePortions float64
	Ingredients       []ComponentIngredient
}

// ComponentIngredient is one ingredient entry inside a ComponentRef.
type ComponentIngredient struct {
	IngredientID int64
	Name         string
	Grams        float64 // absolute grams for ReferencePortions servings
}

// ShoppingItem is one line on the shopping list.
type ShoppingItem struct {
	IngredientID int64   `json:"ingredient_id"`
	Name         string  `json:"name"`
	TotalGrams   float64 `json:"total_grams"`
}

// FromPlates aggregates total grams per ingredient across all plates in a week.
// Components not present in refs are silently skipped.
//
// Formula per plate_component:
//
//	portionMultiplier = plate_component.Portions / comp.ReferencePortions
//	total_grams[ingredient_id] += ingredient.Grams * portionMultiplier
func FromPlates(plates []plate.Plate, refs map[int64]ComponentRef) []ShoppingItem {
	totals := make(map[int64]ShoppingItem)

	for _, pl := range plates {
		for _, pc := range pl.Components {
			ref, ok := refs[pc.ComponentID]
			if !ok || ref.ReferencePortions <= 0 {
				continue
			}
			mult := pc.Portions / ref.ReferencePortions
			for _, ci := range ref.Ingredients {
				e := totals[ci.IngredientID]
				e.IngredientID = ci.IngredientID
				e.Name = ci.Name
				e.TotalGrams += ci.Grams * mult
				totals[ci.IngredientID] = e
			}
		}
	}

	items := make([]ShoppingItem, 0, len(totals))
	for _, v := range totals {
		items = append(items, v)
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].Name < items[j].Name
	})
	return items
}
