package planning

import (
	"cmp"
	"math"
	"regexp"
	"slices"

	"github.com/jaltszeimer/plantry/backend/internal/domain/diet"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
)

// A cold plate is a meal of its own: no-cook foods of the household's own
// library (a bread, a cheese or spread, a raw vegetable)
// and no recipe, so nothing is cooked for it. It is a plate of leaves only, so
// the add-on rules (S9) leave it alone, and the evaluation names it
// (PlateEval.Kind). composeColdPlate builds one in code; the first user is a
// regenerated day that cannot hold two different cooked mains (E022b), a later
// one can be a whole week's non-cooking days.

// ColdPlateKind is PlateEval.Kind of a cold plate.
const ColdPlateKind = "cold_plate"

// coldProtein is a food a cold plate may take as its protein: eaten as it is.
type coldProtein struct {
	f    FoodFacts
	kind string // spread, cheese, dairy, fish, egg
	tier int    // 0: goes on bread (a spread, a cheese); 1: eaten from the pot or tin
}

// The library carries no kind or tag that says "eaten as it is" (cheese, hummus
// and tofu all have no leaf role and the same no-cook flag), so a cold plate's
// protein is told by name, with the same folded en/de keyword matching as the
// food groups: a spread or cheese that goes on bread first, then yogurt, quark
// and skyr, smoked or tinned fish and a boiled egg. Tofu, tempeh, seitan, raw
// meat or fish and anything fried first (halloumi, bacon) are out.
var (
	coldSpread = regexp.MustCompile(`(?i)\b(hummus|houmous|humus|aufstrich|spread|frischk[aä]se|cream cheese)`)
	coldCheese = regexp.MustCompile(`(?i)(cheese|k[aä]se|cheddar|mozzarella|feta|gouda|emmental|edamer|camembert|brie|parmesan|ricotta|mascarpone)`)
	coldDairy  = regexp.MustCompile(`(?i)(yogurt|yoghurt|joghurt|skyr|quark|kefir)`)
	coldFish   = regexp.MustCompile(`(?i)(smoked|ger[aä]uchert|tuna|thunfisch|sardine|makrele|mackerel|anchov|herring|hering)`)
	coldEgg    = regexp.MustCompile(`(?i)(boiled egg|hard.boiled|gekochte[sn]? ei)`)
	coldNever  = regexp.MustCompile(`(?i)(tofu|tempeh|seitan|\braw\b|\broh|sushi|halloumi|paneer|bacon|speck|vegan cheese|coconut)`)
)

// coldProteinOf classifies a no-cook leaf as a cold plate's protein.
func coldProteinOf(f FoodFacts) (kind string, tier int, ok bool) {
	if coldNever.MatchString(f.Name) || f.Macros.Kcal > 450 || f.Macros.Protein*4/f.Macros.Kcal < 0.12 || f.Macros.Protein*ServingGrams(f)/100 < 3 {
		return "", 0, false
	}
	switch {
	case coldSpread.MatchString(f.Name):
		return "spread", 0, true
	case coldCheese.MatchString(f.Name):
		return "cheese", 0, true
	case coldDairy.MatchString(f.Name):
		return "dairy", 1, true
	case coldFish.MatchString(f.Name):
		return "fish", 1, true
	case coldEgg.MatchString(f.Name):
		return "egg", 1, true
	}
	return "", 0, false
}

// IsColdPlate is isColdPlate for the chat's views.
func (s State) IsColdPlate(p Plate) bool { return s.isColdPlate(p) }

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
	var starch, veg []FoodFacts
	var protein []coldProtein
	for _, id := range s.NoCook {
		f := s.Foods[id]
		if f.Composed() || f.Never || !f.NutritionComplete || f.Macros.Kcal <= 0 || f.Sweet || exclude[id] || !fitsDiet(f.DietClass, household) {
			continue
		}
		switch {
		case f.LeafRole == food.RoleSideStarch:
			starch = append(starch, f)
		case f.LeafRole == food.RoleSideVeg && f.Macros.Kcal <= 30:
			veg = append(veg, f)
		default:
			if kind, tier, ok := coldProteinOf(f); ok {
				protein = append(protein, coldProtein{f, kind, tier})
			}
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
	slices.SortStableFunc(protein, func(a, b coldProtein) int {
		pa, pb := a.f.Macros.Protein*ServingGrams(a.f), b.f.Macros.Protein*ServingGrams(b.f)
		return cmp.Or(cmp.Compare(a.tier, b.tier), cmp.Compare(pb, pa), cmp.Compare(a.f.ID, b.f.ID))
	})
	picks := []FoodFacts{starch[0], protein[0].f, veg[0]}
	// The second protein is of another kind than the first (a spread next to a cheese).
	if i := slices.IndexFunc(protein[1:], func(c coldProtein) bool { return c.kind != protein[0].kind }); i >= 0 {
		picks = append(picks, protein[1+i].f)
	} else if len(protein) > 1 {
		picks = append(picks, protein[1].f)
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
