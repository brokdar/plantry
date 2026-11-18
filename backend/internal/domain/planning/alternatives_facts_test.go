package planning_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/availability"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/nutrition"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

func altResult(t *testing.T, s planning.State, req planning.AltRequest) planning.AltResult {
	t.Helper()
	req.Limit = 10
	res, err := planning.Alternatives(s, windowTarget(s), req)
	require.NoError(t, err)
	return res
}

// A candidate gives its own kcal and the plate's apart, plus its role; the
// replaced food comes as `current` (audit B6).
func TestAlternatives_FoodKcalPlateKcalRoleAndCurrent(t *testing.T) {
	s := altState()
	res := altResult(t, s, planning.AltRequest{PlateID: altSunLunch, What: planning.AltMain})

	require.NotNil(t, res.Current)
	assert.Equal(t, chili, res.Current.FoodID)
	assert.Equal(t, "main", res.Current.Role)
	assert.EqualValues(t, 600, res.Current.FoodKcal)
	assert.EqualValues(t, 600, res.Current.PlateKcal)

	var lasagneAlt *planning.Alternative
	for i, a := range res.Alternatives {
		if a.FoodID == lasagne {
			lasagneAlt = &res.Alternatives[i]
		}
	}
	require.NotNil(t, lasagneAlt)
	assert.EqualValues(t, 950, lasagneAlt.FoodKcal)
	assert.EqualValues(t, 950, lasagneAlt.PlateKcal, "a main alone on its plate")
	assert.Equal(t, "main", lasagneAlt.Role)

	// Replacing a main that has sides: the plate holds more than the food.
	res = altResult(t, s, planning.AltRequest{PlateID: altSatLunch, What: planning.AltMain})
	for _, a := range res.Alternatives {
		if a.FoodID == lasagne {
			assert.EqualValues(t, 950, a.FoodKcal)
			assert.EqualValues(t, 950+200+80, a.PlateKcal, "main plus rice plus broccoli")
		}
	}
	assert.Empty(t, altResult(t, s, planning.AltRequest{PlateID: altSatLunch, What: planning.AltAdd}).Current, "an add replaces nothing")
}

// A meal swap drops the sides and says so (audit F3c).
func TestAlternatives_MealSwapHint(t *testing.T) {
	s := altState()
	res := altResult(t, s, planning.AltRequest{PlateID: altSatLunch, What: planning.AltMeal})
	assert.Contains(t, res.Hint, "Rice, Broccoli")
	assert.Contains(t, res.Hint, "what=main")
	assert.Empty(t, altResult(t, s, planning.AltRequest{PlateID: altSatLunch, What: planning.AltMain}).Hint)
}

// light: lighter keeps only candidates whose plate meets the S6 cap; the empty
// result names the cap and the lightest plate (audit B6/P5).
func TestAlternatives_LightFilter(t *testing.T) {
	s := altState()
	s.Profile.KcalTarget = f64(2000) // 3 meals of 667: cap 500, hearty minimum 833
	lighter := altResult(t, s, planning.AltRequest{PlateID: altSunLunch, What: planning.AltMain, Light: planning.AltLighter})
	ids := altIDs(lighter.Alternatives)
	assert.Contains(t, ids, soup, "400 kcal is under the cap")
	assert.NotContains(t, ids, lasagne, "950 kcal is over the cap")
	for _, a := range lighter.Alternatives {
		assert.LessOrEqual(t, a.PlateKcal, 500.0)
	}
	require.NotNil(t, lighter.Current)
	assert.EqualValues(t, 500, lighter.Current.MaxKcal, "the cap of the marked meal")
	assert.Equal(t, "light", lighter.Current.Intent)

	hearty := altResult(t, s, planning.AltRequest{PlateID: altSunLunch, What: planning.AltMain, Light: planning.AltHeartier})
	assert.Contains(t, altIDs(hearty.Alternatives), lasagne)
	assert.NotContains(t, altIDs(hearty.Alternatives), soup)

	none := altResult(t, s, planning.AltRequest{PlateID: altSunLunch, What: planning.AltMain, Light: planning.AltLighter, Constraints: planning.AltConstraints{Search: "lasagne"}})
	assert.Empty(t, none.Alternatives)
	require.NotNil(t, none.EmptyReason)
	assert.Positive(t, none.EmptyReason.Dropped["S6."+planning.CodeMealNotLight], "counted per try: fresh and cook_at")
	require.Len(t, none.EmptyReason.Why, 1)
	assert.Contains(t, none.EmptyReason.Why[0], "cap of 500 kcal")
	assert.Contains(t, none.EmptyReason.Why[0], "950 kcal")

	_, err := planning.Alternatives(s, windowTarget(s), planning.AltRequest{PlateID: altSunLunch, What: planning.AltMain, Light: "huge"})
	assert.ErrorIs(t, err, domain.ErrInvalidInput)
	s.Profile.KcalTarget = nil
	_, err = planning.Alternatives(s, windowTarget(s), planning.AltRequest{PlateID: altSunLunch, What: planning.AltMain, Light: planning.AltLighter})
	assert.ErrorIs(t, err, domain.ErrInvalidInput, "no kcal target, no cap")
}

// An empty list caused by the kcal floor names the floor and the day it
// would reach (audit F3c).
func TestAlternatives_EmptyReasonNamesTheFloor(t *testing.T) {
	s := altState()
	s.Profile.KcalTarget = f64(2000)
	// Sunday holds only the lunch: 1 of 3 meals, floor max(75% x 2000 / 3, 400) = 500.
	setMode(&s, plate.CellKey{Date: day(sun), SlotID: breakfast}, availability.ModeNone)
	setMode(&s, plate.CellKey{Date: day(sun), SlotID: dinner}, availability.ModeNone)
	res := altResult(t, s, planning.AltRequest{PlateID: altSunLunch, What: planning.AltMain, Constraints: planning.AltConstraints{Search: "soup"}})
	assert.Empty(t, res.Alternatives)
	require.NotNil(t, res.EmptyReason)
	assert.Positive(t, res.EmptyReason.Dropped["H9."+planning.CodeKcalBelowFloor])
	require.NotEmpty(t, res.EmptyReason.Why)
	assert.Contains(t, res.EmptyReason.Why[0], "day energy floor:")
	assert.Contains(t, res.EmptyReason.Why[0], "floor is 500 kcal")
	assert.Contains(t, res.EmptyReason.Why[0], "reaches 400 kcal")
}

// A leaf that is not no-cook is never a side or an add; each food is listed
// once (audit F3d).
func TestAlternatives_SidesAreNoCookAndFoodsAreListedOnce(t *testing.T) {
	s := altState()
	const garlic int64 = 60
	s.Foods[garlic] = planning.FoodFacts{
		ID: garlic, Name: "Garlic", Kind: food.KindLeaf, LeafRole: food.RoleSideVeg, DietClass: "vegan",
		Macros: nutrition.Macros{Kcal: 140, Protein: 6}, NutritionComplete: true,
	}
	for _, what := range []string{planning.AltSide, planning.AltAdd} {
		req := planning.AltRequest{PlateID: altSatLunch, What: what}
		if what == planning.AltSide {
			req.ComponentID = 122
		}
		out := altResult(t, s, req).Alternatives
		assert.NotContains(t, altIDs(out), garlic, what)
		assert.Contains(t, altIDs(out), salad, "a no-cook leaf stays: %s", what)
	}
	seen := map[int64]bool{}
	for _, a := range altResult(t, s, planning.AltRequest{PlateID: altSunLunch, What: planning.AltMain}).Alternatives {
		assert.False(t, seen[a.FoodID], "food %d listed twice", a.FoodID)
		seen[a.FoodID] = true
	}
}

// Replacing a side offers a composed side of the same role before the rest.
func TestAlternatives_SameRoleCompositeSideFirst(t *testing.T) {
	s := altState()
	const peas int64 = 61
	s.Foods[peas] = recipe(peas, "Peas", food.RoleSideVeg, "vegan", 90)
	out := altResult(t, s, planning.AltRequest{PlateID: altSatLunch, What: planning.AltSide, ComponentID: 122}).Alternatives
	require.NotEmpty(t, out)
	assert.Equal(t, peas, out[0].FoodID, "a recipe side of the replaced side's role first")
}
