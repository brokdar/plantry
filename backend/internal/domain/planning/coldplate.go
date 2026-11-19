package planning

import (
	"cmp"
	"math"
	"slices"

	"github.com/jaltszeimer/plantry/backend/internal/domain/diet"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
)

// A cold plate is a meal of its own: no-cook foods of the household's own
// library (a bread, a protein such as cheese, hummus or tofu, a raw vegetable)
// and no recipe, so nothing is cooked for it. It is a plate of leaves only, so
// the add-on rules (S9) leave it alone, and the evaluation names it
// (PlateEval.Kind). composeColdPlate builds one in code; the first user is a
// regenerated day that cannot hold two different cooked mains (E022b), a later
// one can be a whole week's non-cooking days.

// ColdPlateKind is PlateEval.Kind of a cold plate.
const ColdPlateKind = "cold_plate"

// maxColdServingG caps the serving of a protein food in a cold plate's pick: bread
// companions (cheese, hummus, tofu, fish) rather than bowls (yogurt, quark).
const maxColdServingG = 100

// isColdPlate reports whether p is a plate of no-cook leaves only (at least two).
func (s State) isColdPlate(p Plate) bool {
	if len(p.Components) < 2 {
		return false
	}
	for _, c := range p.Components {
		f, ok := s.Foods[c.FoodID]
		if !ok || f.Composed() || !f.NoCook || c.Grams == nil {
			return false
		}
	}
	return true
}

// composeColdPlate picks a bread, a protein and a raw vegetable among the
// state's no-cook leaves and sizes them toward aimKcal per eater, each within
// its serving cap (addCapG, at most one and a half servings). A second protein
// is added when the first three at their caps stay under the aim. false when
// the library lacks a bread, a protein or a vegetable that fits the diet.
func (s State) composeColdPlate(exclude map[int64]bool, aimKcal, dayTarget float64) ([]ProposedComponent, bool) {
	var household diet.Class
	if DietRestricts(s.Profile.DietType) {
		household = diet.Class(s.Profile.DietType)
	}
	var starch, veg, protein []FoodFacts
	for _, id := range s.NoCook {
		f := s.Foods[id]
		if f.Composed() || !f.NutritionComplete || f.Macros.Kcal <= 0 || f.Sweet || exclude[id] || !fitsDiet(f.DietClass, household) {
			continue
		}
		switch {
		case f.LeafRole == food.RoleSideStarch:
			starch = append(starch, f)
		case f.LeafRole == food.RoleSideVeg && f.Macros.Kcal <= 30:
			veg = append(veg, f)
		case (f.LeafRole == "" || f.LeafRole == food.RoleSideProtein) && f.Macros.Kcal <= 450 && ServingGrams(f) <= maxColdServingG &&
			f.Macros.Protein*4/f.Macros.Kcal >= 0.12 && f.Macros.Protein*ServingGrams(f)/100 >= 4:
			protein = append(protein, f)
		}
	}
	if len(starch) == 0 || len(veg) == 0 || len(protein) == 0 {
		return nil, false
	}
	slices.SortStableFunc(starch, func(a, b FoodFacts) int {
		return cmp.Or(cmp.Compare(a.Macros.Kcal, b.Macros.Kcal), cmp.Compare(a.ID, b.ID))
	})
	slices.SortStableFunc(veg, func(a, b FoodFacts) int {
		return cmp.Or(cmp.Compare(a.Macros.Kcal, b.Macros.Kcal), cmp.Compare(a.ID, b.ID))
	})
	slices.SortStableFunc(protein, func(a, b FoodFacts) int {
		pa, pb := a.Macros.Protein*ServingGrams(a), b.Macros.Protein*ServingGrams(b)
		return cmp.Or(cmp.Compare(pb, pa), cmp.Compare(a.ID, b.ID))
	})
	picks := []FoodFacts{starch[0], protein[0], veg[0]}
	if len(protein) > 1 {
		picks = append(picks, protein[1])
	}
	grams := make([]float64, len(picks))
	capOf := func(f FoodFacts) float64 {
		return math.Min(addCapG(f, dayTarget), math.Round(1.5*ServingGrams(f)/roundGrams)*roundGrams)
	}
	kcal := func() (sum float64) {
		for i, f := range picks {
			sum += f.Macros.Kcal * grams[i] / 100
		}
		return sum
	}
	// One serving each (the second protein starts at nothing), then half servings
	// of the starch and the proteins while the aim is not reached.
	for i, f := range picks {
		if i == 3 {
			continue
		}
		grams[i] = math.Min(ServingGrams(f), capOf(f))
	}
	for grown := true; grown && kcal() < aimKcal-1e-9; {
		grown = false
		for _, i := range []int{0, 1, 3} {
			f := picks[i%len(picks)]
			if i >= len(picks) || kcal() >= aimKcal-1e-9 || grams[i] >= capOf(f)-1e-9 {
				continue
			}
			step := math.Max(roundGrams, ServingGrams(f)/2)
			if grams[i] == 0 {
				step = minAddGrams
			}
			// Never past the aim: a light meal's cap is its aim.
			room := math.Floor((grams[i]+(aimKcal-kcal())*100/f.Macros.Kcal)/roundGrams) * roundGrams
			next := math.Min(math.Min(capOf(f), grams[i]+step), room)
			if next <= grams[i]+1e-9 || (grams[i] == 0 && next < minAddGrams) {
				continue
			}
			grams[i] = next
			grown = true
		}
	}
	var out []ProposedComponent
	for i, f := range picks {
		if grams[i] <= 0 {
			continue
		}
		out = append(out, ProposedComponent{FoodID: f.ID, Grams: math.Round(grams[i]/roundGrams) * roundGrams})
	}
	return out, len(out) >= 3
}
