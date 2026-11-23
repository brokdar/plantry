package planning_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/nutrition"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

const (
	relNuts  int64 = 61 // leaf, no-cook, 600 kcal per 100 g, serving 30 g
	relBread int64 = 62 // leaf, no-cook, bakery side_starch, 250 kcal per 100 g, serving 60 g
)

// relState is one planned Saturday: breakfast oats (350 kcal), lunch curry
// (500) and dinner chili with potatoes (850), for 2000 kcal a day: the
// even share is 667, the light cap 500, the hearty minimum 833, the floor
// 1500. Only Saturday is in the plan.
func relState(confirmedOthers bool) planning.State {
	s := altState()
	s.Sessions, s.Plates = nil, nil
	s.Cells = s.Cells[:3]
	s.Settings.MaxCookOccasionsPerDay = 3
	s.Profile.KcalTarget = f64(2000)
	s.Foods[relNuts] = planning.FoodFacts{
		ID: relNuts, Name: "Nuts", Kind: food.KindLeaf, LeafRole: food.RoleSideProtein, DietClass: "vegan", NoCook: true, ServingG: 30,
		Macros: nutrition.Macros{Kcal: 600, Protein: 20}, NutritionComplete: true,
	}
	s.Foods[relBread] = planning.FoodFacts{
		ID: relBread, Name: "Bread", Kind: food.KindLeaf, LeafRole: food.RoleSideStarch, DietClass: "vegan", NoCook: true, ServingG: 60,
		Macros: nutrition.Macros{Kcal: 250, Protein: 8}, NutritionComplete: true,
	}
	s.NoCook = []int64{relNuts, relBread}
	one := func(id int64, slot int64, confirmed bool, foods ...int64) planning.Plate {
		p := planning.Plate{ID: id, Date: day(sat), SlotID: slot, Eaters: 1, Confirmed: confirmed}
		for i, f := range foods {
			pos := plate.PositionSide
			if i == 0 {
				pos = plate.PositionMain
			}
			p.Components = append(p.Components, planning.Component{ID: id*10 + int64(i), FoodID: f, Portions: f64(1), SessionID: id*10 + int64(i), Position: pos})
			s.Sessions = append(s.Sessions, persistedSession(id*10+int64(i), f, sat, slot, 1, confirmed))
		}
		return p
	}
	s.Plates = []planning.Plate{
		one(1, breakfast, confirmedOthers, oats),
		one(2, lunch, confirmedOthers, curry),
		one(3, dinner, false, chili, potatoes),
	}
	return s
}

// relLeftover is relState with the dinner's chili eaten from a batch cooked at
// lunch: a smaller serving of it is a smaller serving of a leftover.
func relLeftover() planning.State {
	s := relState(false)
	for i := range s.Sessions {
		if s.Sessions[i].ID == 30 {
			s.Sessions[i].CookSlotID = lunch
		}
	}
	return s
}

func ways(t *testing.T, s planning.State, req planning.WayRequest) planning.WayResult {
	t.Helper()
	res, err := planning.Ways(s, windowTarget(s), req)
	require.NoError(t, err)
	return res
}

func TestWays_LighterChangesOnlyThisMealAndOffersTheRest(t *testing.T) {
	res := ways(t, relLeftover(), planning.WayRequest{PlateID: 3, Goal: planning.GoalLighter})
	assert.InDelta(t, 500, res.MaxKcal, 0.5)
	assert.InDelta(t, 850, res.Before.MealKcal, 0.5)
	require.NotEmpty(t, res.Ways)
	for _, w := range res.Ways {
		assert.NotEqual(t, "compound", w.Kind, "a compound way is never a default")
		assert.False(t, w.ChangesOther)
		assert.Empty(t, w.Moved)
		assert.True(t, planning.MeaningfulMealKcal(res.Before.MealKcal, res.Before.MealKcal-w.After.MealKcal), w.What)
		assert.GreaterOrEqual(t, w.After.DayKcal, res.FloorKcal, "%s keeps the day above its floor", w.What)
		for _, op := range w.Ops {
			assert.Contains(t, []int64{30, 31, 0}, derefInt(op.PlateComponentID)-0, "ops touch dinner only: %v", op)
		}
		if !w.Reaches {
			assert.Empty(t, w.Light, "no mark on a meal over its cap")
		}
	}
	// The first way goes about as far as the floor allows: within one rank
	// bucket (150 kcal) of the lightest, and the own dish before a swap.
	lightest := res.Ways[0].After.MealKcal
	for _, w := range res.Ways {
		lightest = min(lightest, w.After.MealKcal)
	}
	assert.False(t, res.Ways[0].Reaches, "the floor stops a 500 kcal dinner here")
	assert.LessOrEqual(t, res.Ways[0].After.MealKcal-lightest, 150.0)

	// The offer: dinner at its cap, the lunch grows by its own dish and a
	// side or bread, never a filler such as nuts.
	require.NotEmpty(t, res.Offers)
	for _, o := range res.Offers {
		assert.True(t, o.ChangesOther)
		assert.NotEmpty(t, o.Moved)
		// An offer that cannot reach the cap inside the other meal's bound is
		// still a lighter dinner; it says so and stores no mark.
		if o.Reaches {
			assert.LessOrEqual(t, o.After.MealKcal, res.MaxKcal+0.5)
			assert.NotContains(t, o.Light, planning.LightIntent{Date: sat, Scope: "meal", Slot: "dinner"}, "SC3: a comparative stores no mark")
		} else {
			assert.True(t, planning.MeaningfulMealKcal(res.Before.MealKcal, res.Before.MealKcal-o.After.MealKcal), o.What)
			assert.NotContains(t, o.Light, planning.LightIntent{Date: sat, Scope: "meal", Slot: "dinner"})
		}
		assert.GreaterOrEqual(t, o.After.DayKcal, res.FloorKcal, o.What)
		if o.BelowBand {
			assert.NotEmpty(t, o.Why, "an offer under the band says by how much")
			assert.Contains(t, o.Why, "under its band")
		} else {
			assert.GreaterOrEqual(t, o.After.DayKcal, res.BandLow-1, "%s lands the day in its band", o.What)
			assert.Equal(t, "the lunch stays within a filling lunch (833 kcal)", o.Why, "the offer says which bound it kept")
		}
		for _, op := range o.Ops {
			assert.NotEqual(t, relNuts, op.FoodID, "no filler on a dish it does not belong to: %v", o.Moved)
		}
	}
}

func derefInt(p *int64) int64 {
	if p == nil {
		return 0
	}
	return *p
}

func TestWays_HeartierEvenWhenAboveTheMinimum(t *testing.T) {
	res := ways(t, relState(false), planning.WayRequest{PlateID: 3, Goal: planning.GoalHeartier})
	assert.InDelta(t, 833, res.MinKcal, 0.5)
	require.NotEmpty(t, res.Ways, "850 kcal is above the minimum and still has to get heartier")
	for _, w := range res.Ways {
		assert.True(t, planning.MeaningfulMealKcal(res.Before.MealKcal, w.After.MealKcal-res.Before.MealKcal), w.What)
		assert.NotEmpty(t, w.Ops, "never a mark alone")
	}
}

func TestWays_MoreProtein(t *testing.T) {
	for name, req := range map[string]planning.WayRequest{
		"meal": {PlateID: 3, Goal: planning.GoalMoreProtein},
		"day":  {Date: sat, Goal: planning.GoalMoreProtein},
	} {
		t.Run(name, func(t *testing.T) {
			res := ways(t, relState(false), req)
			require.NotEmpty(t, res.Ways)
			for _, w := range res.Ways {
				gain := w.After.DayProteinG - w.Before.DayProteinG
				if name == "meal" {
					gain = w.After.MealProteinG - w.Before.MealProteinG
				}
				assert.GreaterOrEqual(t, gain, planning.RelMinProteinG, w.What)
			}
		})
	}
}

func TestWays_NoOfferWhenTheOtherMealsCannotChange(t *testing.T) {
	// The other meals are confirmed: nothing can lift the day, so a dinner
	// at its cap would break the floor and there is no offer.
	res := ways(t, relState(true), planning.WayRequest{PlateID: 3, Goal: planning.GoalLighter})
	assert.Empty(t, res.Offers)
	for _, w := range res.Ways {
		assert.False(t, w.Reaches, w.What)
		assert.GreaterOrEqual(t, w.After.DayKcal, res.FloorKcal)
	}
}

func TestWays_NothingMeaningfulOnAMealAtTheFloor(t *testing.T) {
	// Dinner 850, breakfast and lunch 850: the day is 1700. Raise the floor
	// to 1700: the dinner cannot lose anything alone.
	s := relState(false)
	s.Profile.KcalTarget = f64(2260) // floor 75 % = 1695
	res := ways(t, s, planning.WayRequest{PlateID: 3, Goal: planning.GoalLighter})
	assert.Empty(t, res.Ways)
	assert.NotEmpty(t, res.EmptyWhy)
	assert.Contains(t, res.EmptyWhy, "kcal")
}

func TestWays_Errors(t *testing.T) {
	s := relState(true)
	for name, req := range map[string]planning.WayRequest{
		"unknown goal":         {PlateID: 3, Goal: "tastier"},
		"unknown plate":        {PlateID: 99, Goal: planning.GoalLighter},
		"confirmed plate":      {PlateID: 1, Goal: planning.GoalMoreProtein},
		"lighter without meal": {Date: sat, Goal: planning.GoalLighter},
		"breakfast is no meal": {PlateID: 1, Goal: planning.GoalLighter},
	} {
		_, err := planning.Ways(s, windowTarget(s), req)
		assert.Error(t, err, name)
	}
}

func TestJudgeRelative(t *testing.T) {
	fig := func(meal, protein, day, dayProtein float64) planning.WayFigures {
		return planning.WayFigures{MealKcal: meal, MealProteinG: protein, DayKcal: day, DayProteinG: dayProtein}
	}
	tests := []struct {
		name          string
		goal          string
		meal          bool
		maxK, minK    float64
		before, after planning.WayFigures
		changed, sat  bool
	}{
		{"lighter under the cap and 80 kcal less", planning.GoalLighter, true, 550, 0, fig(600, 0, 0, 0), fig(500, 0, 0, 0), true, true},
		{"lighter by 10 % of a small meal", planning.GoalLighter, true, 550, 0, fig(300, 0, 0, 0), fig(265, 0, 0, 0), true, true},
		{"lighter but too little", planning.GoalLighter, true, 550, 0, fig(1000, 0, 0, 0), fig(950, 0, 0, 0), false, false},
		{"lighter, changed, still over the cap", planning.GoalLighter, true, 550, 0, fig(1583, 0, 0, 0), fig(1030, 0, 0, 0), true, false},
		{"lighter, food unchanged under the cap", planning.GoalLighter, true, 550, 0, fig(331, 0, 0, 0), fig(331, 0, 0, 0), false, false},
		{"heartier above the minimum, 8 % more is not a step", planning.GoalHeartier, true, 0, 917, fig(1216, 0, 0, 0), fig(1313, 0, 0, 0), false, false},
		{"heartier above the minimum, 10 % more", planning.GoalHeartier, true, 0, 917, fig(1216, 0, 0, 0), fig(1338, 0, 0, 0), true, true},
		{"heartier by exactly 10 % is not more than a tenth (C-7, heartier-23)", planning.GoalHeartier, true, 0, 1103, fig(1080, 0, 0, 0), fig(1188, 0, 0, 0), false, false},
		{"heartier by just over 10 %", planning.GoalHeartier, true, 0, 1103, fig(1080, 0, 0, 0), fig(1189, 0, 0, 0), true, true},
		{"heartier, already above and unchanged", planning.GoalHeartier, true, 0, 917, fig(1216, 0, 0, 0), fig(1216, 0, 0, 0), false, false},
		{"heartier, more but under the minimum", planning.GoalHeartier, true, 0, 917, fig(700, 0, 0, 0), fig(820, 0, 0, 0), true, false},
		{"protein of the meal, 10 g", planning.GoalMoreProtein, true, 0, 0, fig(0, 30, 0, 80), fig(0, 40, 0, 90), true, true},
		{"protein of the meal, 9.9 g", planning.GoalMoreProtein, true, 0, 0, fig(0, 30, 0, 80), fig(0, 39.9, 0, 90), false, false},
		{"protein of the day", planning.GoalMoreProtein, false, 0, 0, fig(0, 0, 0, 80), fig(0, 0, 0, 91), true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			changed, reached, why := planning.JudgeRelative(tt.goal, tt.meal, tt.maxK, tt.minK, tt.before, tt.after)
			assert.Equal(t, tt.changed, changed)
			assert.Equal(t, tt.sat, changed && reached)
			assert.Equal(t, !changed || !reached, why != "", "a request that is not fully met says why")
		})
	}
}

func TestWays_DayGoals(t *testing.T) {
	// Saturday holds 1700 kcal of 2000 (range 1800 to 2200, floor 1500).
	// raise_day changes food inside that range and stores no mark.
	raise := ways(t, relState(false), planning.WayRequest{Date: sat, Goal: planning.GoalRaiseDay})
	require.NotEmpty(t, raise.Ways, raise.EmptyWhy)
	for _, w := range raise.Ways {
		assert.True(t, w.Reaches, w.What)
		assert.GreaterOrEqual(t, w.After.DayKcal, raise.BandLow-0.5, w.What)
		assert.LessOrEqual(t, w.After.DayKcal, raise.BandHigh+0.5, w.What)
		assert.Empty(t, w.Light, "a mark is not stored unasked")
	}
	// lower_day: the day is already at the bottom of its range, so nothing
	// changes inside it; the light-day mark comes back as an offer, not a way.
	lower := ways(t, relState(false), planning.WayRequest{Date: sat, Goal: planning.GoalLowerDay})
	assert.Empty(t, lower.Ways)
	require.NotEmpty(t, lower.Offers)
	assert.Contains(t, lower.EmptyWhy, "light day")
	assert.Contains(t, lower.EmptyWhy, "1800 to 2200")
	for _, o := range lower.Offers {
		assert.LessOrEqual(t, o.After.DayKcal, lower.BandHigh+0.5, o.What)
		assert.GreaterOrEqual(t, o.After.DayKcal, lower.FloorKcal, o.What)
		assert.Contains(t, o.Light, planning.LightIntent{Date: sat, Scope: "day"})
	}
	_, err := planning.Ways(relState(false), windowTarget(relState(false)), planning.WayRequest{Goal: planning.GoalLowerDay})
	assert.Error(t, err, "a day goal needs a date")
}

// A day inside its range with room to spare gets lower by changing food, with
// no mark: "fewer kcal on Monday" does not make Monday a light day.
func TestWays_LowerDayInsideItsRangeStoresNoMark(t *testing.T) {
	s := relState(false)
	s.Profile.KcalTarget = f64(1700) // range 1530 to 1870 around the 1700 kcal day
	res := ways(t, s, planning.WayRequest{Date: sat, Goal: planning.GoalLowerDay})
	require.NotEmpty(t, res.Ways, res.EmptyWhy)
	for _, w := range res.Ways {
		assert.Empty(t, w.Light, w.What)
		assert.GreaterOrEqual(t, w.After.DayKcal, res.BandLow-0.5, "inside the day's own range: %s", w.What)
		assert.Less(t, w.After.DayKcal, res.Before.DayKcal, w.What)
	}
}

// N037: a dish cooked fresh for one plate may go down to half a portion (a
// sauce, a braise, a pan dish): a light dinner asked for is reached at its cap
// with the day still at its minimum, and the offer says the smaller portion.
func TestWays_FreshDishMayGoToHalfAPortion(t *testing.T) {
	res := ways(t, relState(false), planning.WayRequest{PlateID: 3, Goal: planning.GoalLighter, ToLimit: true})
	reaching := 0
	for _, w := range append(append([]planning.Way{}, res.Ways...), res.Offers...) {
		for _, op := range w.Ops {
			if op.Portions != nil {
				assert.GreaterOrEqual(t, *op.Portions, 0.5, "%s", w.What)
			}
		}
		if w.Reaches {
			reaching++
			assert.LessOrEqual(t, w.After.MealKcal, res.MaxKcal+0.5, w.What)
			assert.GreaterOrEqual(t, w.After.DayKcal, res.FloorKcal, w.What)
		}
	}
	require.Positive(t, reaching, "a way reaches the light limit")
	require.NotEmpty(t, res.Offers)
	assert.Contains(t, res.Offers[0].What, "0.5 portions (was 1)", "the smaller portion is told")
}

// E018 rule 2: a lighter meal is still a meal: the main keeps at least half a
// portion and a plate that had a side keeps one (a lighter side before none).
func TestWays_LighterMealKeepsMainAndSide(t *testing.T) {
	res := ways(t, relLeftover(), planning.WayRequest{PlateID: 3, Goal: planning.GoalLighter})
	all := append(append([]planning.Way{}, res.Ways...), res.Offers...)
	require.NotEmpty(t, all)
	for _, w := range all {
		for _, op := range w.Ops {
			if op.Portions != nil {
				assert.GreaterOrEqual(t, *op.Portions, 0.5, "%s: no fragment of a dish", w.What)
			}
		}
		if strings.Contains(w.What, "leave out Potatoes") || strings.Contains(w.What, "leave out Potatoes and") {
			t.Errorf("%s drops the only side", w.What)
		}
	}
	// The offer reaches the cap by swapping the side for a lighter one.
	require.NotEmpty(t, res.Offers)
	assert.Contains(t, res.Offers[0].What, `swap "Potatoes" for`)
}

// E018 rule 5: a number that cannot be met (under the day's minimum) gets no
// way and one offer: the nearest safe option, never under the minimum.
func TestWays_ImpossibleDayNumber(t *testing.T) {
	res := ways(t, relState(false), planning.WayRequest{Date: sat, Goal: planning.GoalLowerDay, TargetKcal: 900})
	assert.True(t, res.Impossible)
	assert.Empty(t, res.Ways)
	assert.Contains(t, res.EmptyWhy, "minimum")
	require.Len(t, res.Offers, 1)
	assert.GreaterOrEqual(t, res.Offers[0].After.DayKcal, res.FloorKcal)
	feasible := ways(t, relState(false), planning.WayRequest{Date: sat, Goal: planning.GoalLowerDay, TargetKcal: 1700})
	assert.False(t, feasible.Impossible)
}

// E018 rule 1: no way or offer of the fixture adds a bread over its add-on
// limit or an add-on that does not fit; nothing a way adds is nuts or cheese.
func TestWays_NoFillersAndNoOverCapBread(t *testing.T) {
	for _, goal := range []string{planning.GoalLighter, planning.GoalHeartier, planning.GoalMoreProtein} {
		res := ways(t, relState(false), planning.WayRequest{PlateID: 3, Goal: goal})
		for _, w := range append(append([]planning.Way{}, res.Ways...), res.Offers...) {
			for _, op := range w.Ops {
				if op.Op == "add" {
					assert.NotEqual(t, relNuts, op.FoodID, "%s: nuts on a chili", goal)
				}
			}
			for _, is := range w.NewIssues {
				assert.NotEqual(t, planning.CodeAddOnExcess, is.Code, "%s %s: no add-on over its limit", goal, w.What)
			}
		}
	}
}

// E018 rule 4: the first alternative is marked recommended, once, with a
// reason; no other is.
func TestAlternatives_FirstIsRecommendedOnce(t *testing.T) {
	out := alts(t, altState(), planning.AltRequest{PlateID: altSatDinner, What: planning.AltMain})
	require.NotEmpty(t, out)
	assert.True(t, out[0].Recommended)
	assert.NotEmpty(t, out[0].Because)
	for _, a := range out[1:] {
		assert.False(t, a.Recommended)
	}
}

// A more-protein way adds or enlarges only protein foods (at least 20 % of
// their energy from protein): bread, nuts and the like are never the answer.
func TestWays_MoreProteinOnlyAddsProteinFoods(t *testing.T) {
	const relTofu int64 = 63
	s := relState(false)
	s.Foods[relTofu] = planning.FoodFacts{
		ID: relTofu, Name: "Tofu", Kind: food.KindLeaf, LeafRole: food.RoleSideProtein, DietClass: "vegan", NoCook: true, ServingG: 100,
		Macros: nutrition.Macros{Kcal: 140, Protein: 16}, NutritionComplete: true,
	}
	s.NoCook = append(s.NoCook, relTofu)
	res := ways(t, s, planning.WayRequest{PlateID: 3, Goal: planning.GoalMoreProtein})
	added := false
	for _, w := range res.Ways {
		for _, op := range w.Ops {
			if op.Op == "add" {
				added = true
				assert.Equal(t, relTofu, op.FoodID, "%s: only a protein food is added", w.What)
			}
		}
		assert.False(t, w.After.DayKcal-w.Before.DayKcal > planning.RelProteinWasteKcal && w.After.DayProteinG-w.Before.DayProteinG < planning.RelProteinWasteG, w.What)
	}
	assert.True(t, added, "the protein food is among the ways")
}

// RL4: a light meal asked for is met at the limit or not at all. No way that
// stops short of it is a way (the model would apply ways[0]); the nearest
// step is the one offer.
func TestWays_ToLimitAppliesNothingThatStopsShort(t *testing.T) {
	res := ways(t, relLeftover(), planning.WayRequest{PlateID: 3, Goal: planning.GoalLighter, ToLimit: true})
	for _, w := range res.Ways {
		assert.True(t, w.Reaches, "way %s stops short of the limit", w.What)
	}
	for _, o := range res.Offers {
		assert.True(t, o.Reaches, o.What)
	}
	if len(res.Ways) == 0 {
		assert.NotEmpty(t, res.EmptyWhy)
	}
}

// SC3: a comparative "lighter" stores no light mark even when the meal ends
// under the light limit; "light" to the limit does.
func TestWays_OnlyToLimitStoresTheMark(t *testing.T) {
	mark := planning.LightIntent{Date: sat, Scope: "meal", Slot: "dinner"}
	for _, toLimit := range []bool{false, true} {
		res := ways(t, relState(false), planning.WayRequest{PlateID: 3, Goal: planning.GoalLighter, ToLimit: toLimit})
		reaching := 0
		for _, w := range append(append([]planning.Way{}, res.Ways...), res.Offers...) {
			if !w.Reaches {
				continue
			}
			reaching++
			if toLimit {
				assert.Contains(t, w.Light, mark, w.What)
			} else {
				assert.NotContains(t, w.Light, mark, w.What)
			}
		}
		assert.Positive(t, reaching, "toLimit=%v: some way reaches the limit in this fixture", toLimit)
	}
}

// KR5: "heartier" grows the main, a starch or a protein food, never fruit or a
// vegetable.
func TestWays_HeartierNeverAddsFruitOrVegetables(t *testing.T) {
	const fruit int64 = 63
	s := relState(false)
	s.Foods[fruit] = planning.FoodFacts{
		ID: fruit, Name: "Dried apricots", Kind: food.KindLeaf, LeafRole: food.RoleSideVeg, DietClass: "vegan", NoCook: true, ServingG: 150,
		Macros: nutrition.Macros{Kcal: 250, Protein: 1}, NutritionComplete: true,
	}
	s.NoCook = append(s.NoCook, fruit)
	res := ways(t, s, planning.WayRequest{PlateID: 3, Goal: planning.GoalHeartier, Limit: 8})
	require.NotEmpty(t, res.Ways)
	for _, w := range append(append([]planning.Way{}, res.Ways...), res.Offers...) {
		for _, op := range w.Ops {
			assert.NotEqual(t, fruit, op.FoodID, "%s", w.What)
		}
	}
}

// KR4: half a portion only for a dish cooked fresh for one meal; a dish other
// meals eat from too goes down to three quarters at the least.
func TestWays_HalfPortionOnlyForADishCookedForOneMeal(t *testing.T) {
	smallest := func(s planning.State) float64 {
		res := ways(t, s, planning.WayRequest{PlateID: 3, Goal: planning.GoalLighter, ToLimit: true, Limit: 8})
		least := 99.0
		for _, w := range append(append([]planning.Way{}, res.Ways...), res.Offers...) {
			for _, op := range w.Ops {
				if op.Portions != nil && derefInt(op.PlateComponentID) == 30 {
					least = min(least, *op.Portions)
				}
			}
		}
		return least
	}
	assert.GreaterOrEqual(t, smallest(relLeftover()), 0.75, "the dish feeds another meal")
	assert.Equal(t, 0.5, smallest(relState(false)), "cooked fresh for this meal alone: half a portion is allowed")
}

// SC3/RL2: a comparative stores no mark, so its search judges the day against
// the range the saved state has: the unmarked one, so the result's range is
// the range of every way. ("light/filling" to the limit stores its mark and
// keeps the marked range.)
func TestWays_ComparativeJudgesTheDayByTheUnmarkedRange(t *testing.T) {
	s := relState(false)
	var plain planning.DayEval
	for _, d := range planning.Evaluate(s, windowTarget(s)).Days {
		if d.Date == sat {
			plain = d
		}
	}
	require.NotZero(t, plain.BandHigh)
	for _, goal := range []string{planning.GoalLighter, planning.GoalHeartier} {
		t.Run(goal, func(t *testing.T) {
			res := ways(t, s, planning.WayRequest{PlateID: 3, Goal: goal, Limit: 8})
			assert.InDelta(t, plain.BandLow, res.BandLow, 0.5)
			assert.InDelta(t, plain.BandHigh, res.BandHigh, 0.5)
		})
	}
}

// B-1 (v1.2, R1): a number named under the day's minimum is refused (C-11) and
// the floor is unchanged: the range the result shows never starts under it.
func TestWays_NamedNumberUnderTheFloorShowsARangeAboveIt(t *testing.T) {
	res := ways(t, relState(false), planning.WayRequest{Date: sat, Goal: planning.GoalLowerDay, TargetKcal: 100, Limit: 8})
	require.True(t, res.Impossible, "a number under the minimum cannot be met")
	require.Positive(t, res.FloorKcal)
	assert.GreaterOrEqual(t, res.BandLow, res.FloorKcal, "range low")
	assert.GreaterOrEqual(t, res.MinKcal, res.FloorKcal, "min kcal")
	assert.Greater(t, res.BandHigh, res.BandLow)
}

// V72 T3 (tune-v2 g2-heartier-07): a dinner already at its largest portion,
// a little under its filling minimum, on a day with room to its top. The adds
// were sized to the 31 kcal gap to the minimum, all below a meaningful step
// (10 % of the meal), so "heartier" found nothing while the day had room. An
// add is now at least a meaningful step.
func TestWays_HeartierAddsAMeaningfulStepAtTheLargestPortion(t *testing.T) {
	s := relState(false)
	s.Profile.KcalTarget = f64(2600)
	delete(s.Foods, lasagne) // every other main is lighter: only an add can make it heartier
	const yogurt int64 = 63
	s.Foods[yogurt] = planning.FoodFacts{
		ID: yogurt, Name: "Greek yogurt", Kind: food.KindLeaf, LeafRole: food.RoleSideProtein, DietClass: "vegetarian", NoCook: true, ServingG: 150,
		Macros: nutrition.Macros{Kcal: 97, Protein: 9}, NutritionComplete: true,
	}
	s.NoCook = append(s.NoCook, yogurt)
	for i := range s.Plates {
		if s.Plates[i].ID == 3 {
			for ci := range s.Plates[i].Components {
				s.Plates[i].Components[ci].Portions = f64(planning.MaxDishPortions)
			}
		}
	}
	for i := range s.Sessions {
		if s.Sessions[i].ID == 30 || s.Sessions[i].ID == 31 {
			s.Sessions[i].Portions = planning.MaxDishPortions
		}
	}
	res := ways(t, s, planning.WayRequest{PlateID: 3, Goal: planning.GoalHeartier})
	require.NotEmpty(t, res.Ways, "the day has room: a meaningful add exists (%s)", res.EmptyWhy)
	for _, w := range res.Ways {
		assert.True(t, planning.MeaningfulMealKcal(res.Before.MealKcal, w.After.MealKcal-res.Before.MealKcal), w.What)
		assert.LessOrEqual(t, w.After.DayKcal, res.BandHigh+0.5, w.What)
	}
}

// V72 T3 (tune-v2 g2-light_limit-17, "Wednesday dinner really filling"): the
// main is at its largest portion (and a bigger one would pass the week's
// meat), no single add reaches the filling minimum, but two fitting adds do.
// A filling-meal request now tries a pair of adds before it says it cannot.
func TestWays_FillingMealReachedByTwoAdds(t *testing.T) {
	s := relState(false)
	const yogurt int64 = 63
	s.Foods[yogurt] = planning.FoodFacts{
		ID: yogurt, Name: "Greek yogurt", Kind: food.KindLeaf, LeafRole: food.RoleSideProtein, DietClass: "vegetarian", NoCook: true, ServingG: 150,
		Macros: nutrition.Macros{Kcal: 97, Protein: 9}, NutritionComplete: true,
	}
	b := s.Foods[relBread]
	b.ServingG = 20 // two servings: 40 g, 100 kcal
	s.Foods[relBread] = b
	s.NoCook = []int64{relBread, yogurt}
	delete(s.Foods, chili) // the heavier mains are out of reach (in the stored run: the week's meat)
	delete(s.Foods, lasagne)
	for i := range s.Plates {
		if s.Plates[i].ID == 3 {
			s.Plates[i].Components = []planning.Component{{ID: 30, FoodID: soup, Portions: f64(planning.MaxDishPortions), SessionID: 30, Position: plate.PositionMain}}
		}
	}
	var sessions []planning.Session
	for _, ss := range s.Sessions {
		switch ss.ID {
		case 30:
			ss.FoodID, ss.Portions = soup, planning.MaxDishPortions
		case 31:
			continue
		}
		sessions = append(sessions, ss)
	}
	s.Sessions = sessions

	res := ways(t, s, planning.WayRequest{PlateID: 3, Goal: planning.GoalHeartier, ToLimit: true})
	require.NotEmpty(t, res.Ways, "two adds reach the filling minimum (%s)", res.EmptyWhy)
	w := res.Ways[0]
	assert.True(t, w.Reaches, w.What)
	assert.GreaterOrEqual(t, w.After.MealKcal, res.MinKcal-0.5, w.What)
	assert.Len(t, w.Ops, 2, "a pair of adds: %v", w.What)
}

// V73 (a) (tune-v2 g2-light_limit-17): a filling-meal search judged the day
// under the mark it would store and told that band (top 1766) while every
// way's own check and the stored plan showed the day's real band (top 1605).
// The told band is the stored day's.
func TestWays_ToLimitTellsTheStoredDayBand(t *testing.T) {
	s := relState(false)
	var day planning.DayEval
	for _, d := range planning.Evaluate(s, windowTarget(s)).Days {
		if d.Date == sat {
			day = d
		}
	}
	require.Positive(t, day.BandHigh)
	for _, goal := range []string{planning.GoalHeartier, planning.GoalLighter} {
		res := ways(t, s, planning.WayRequest{PlateID: 3, Goal: goal, ToLimit: true})
		assert.InDelta(t, day.BandLow, res.BandLow, 0.5, goal)
		assert.InDelta(t, day.BandHigh, res.BandHigh, 0.5, goal)
	}
}
