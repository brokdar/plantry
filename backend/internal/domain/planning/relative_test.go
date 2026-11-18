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

func ways(t *testing.T, s planning.State, req planning.WayRequest) planning.WayResult {
	t.Helper()
	res, err := planning.Ways(s, windowTarget(s), req)
	require.NoError(t, err)
	return res
}

func TestWays_LighterChangesOnlyThisMealAndOffersTheRest(t *testing.T) {
	res := ways(t, relState(false), planning.WayRequest{PlateID: 3, Goal: planning.GoalLighter})
	assert.InDelta(t, 500, res.MaxKcal, 0.5)
	assert.InDelta(t, 850, res.Before.MealKcal, 0.5)
	require.NotEmpty(t, res.Ways)
	for _, w := range res.Ways {
		assert.NotEqual(t, "compound", w.Kind, "a compound way is never a default")
		assert.False(t, w.ChangesOther)
		assert.Empty(t, w.Moved)
		assert.True(t, planning.MeaningfulKcal(res.Before.MealKcal, res.Before.MealKcal-w.After.MealKcal), w.What)
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
		assert.True(t, o.Reaches)
		assert.LessOrEqual(t, o.After.MealKcal, res.MaxKcal+0.5)
		assert.GreaterOrEqual(t, o.After.DayKcal, res.FloorKcal, o.What)
		if o.BelowBand {
			assert.NotEmpty(t, o.Why, "an offer under the band says by how much")
			assert.Contains(t, o.Why, "under its band")
		} else {
			assert.GreaterOrEqual(t, o.After.DayKcal, res.BandLow-1, "%s lands the day in its band", o.What)
			assert.Empty(t, o.Why)
		}
		assert.Contains(t, o.Light, planning.LightIntent{Date: sat, Scope: "meal", Slot: "dinner"})
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
		assert.True(t, planning.MeaningfulKcal(res.Before.MealKcal, w.After.MealKcal-res.Before.MealKcal), w.What)
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
		{"heartier above the minimum, 97 kcal more", planning.GoalHeartier, true, 0, 917, fig(1216, 0, 0, 0), fig(1313, 0, 0, 0), true, true},
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
	// Saturday holds 1700 kcal of 2000: lower_day aims at 85 % (band 1530 to
	// 1870, floor 1500); raise_day at 115 % (band 2070 to 2530).
	for goal, in := range map[string]func(planning.WayResult, planning.Way){
		planning.GoalLowerDay: func(res planning.WayResult, w planning.Way) {
			assert.LessOrEqual(t, w.After.DayKcal, res.BandHigh+0.5, w.What)
			assert.GreaterOrEqual(t, w.After.DayKcal, res.FloorKcal, w.What)
			assert.Contains(t, w.Light, planning.LightIntent{Date: sat, Scope: "day"})
		},
		planning.GoalRaiseDay: func(res planning.WayResult, w planning.Way) {
			assert.GreaterOrEqual(t, w.After.DayKcal, res.BandLow-0.5, w.What)
			assert.LessOrEqual(t, w.After.DayKcal, res.BandHigh+0.5, w.What)
			assert.Contains(t, w.Light, planning.LightIntent{Date: sat, Scope: "day", Level: "hearty"})
		},
	} {
		t.Run(goal, func(t *testing.T) {
			res := ways(t, relState(false), planning.WayRequest{Date: sat, Goal: goal})
			require.NotEmpty(t, res.Ways, res.EmptyWhy)
			for _, w := range res.Ways {
				assert.True(t, w.Reaches, w.What)
				assert.NotEmpty(t, w.Ops)
				in(res, w)
			}
			assert.Empty(t, res.Offers)
		})
	}
	_, err := planning.Ways(relState(false), windowTarget(relState(false)), planning.WayRequest{Goal: planning.GoalLowerDay})
	assert.Error(t, err, "a day goal needs a date")
}

// E018 rule 2: a lighter meal is still a meal: the main keeps at least half a
// portion and a plate that had a side keeps one (a lighter side before none).
func TestWays_LighterMealKeepsMainAndSide(t *testing.T) {
	res := ways(t, relState(false), planning.WayRequest{PlateID: 3, Goal: planning.GoalLighter})
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
	assert.Contains(t, res.Offers[0].What, "swap Potatoes for")
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
