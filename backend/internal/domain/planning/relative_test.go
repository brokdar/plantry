package planning_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/nutrition"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

const relNuts int64 = 61 // leaf, no-cook, 600 kcal per 100 g, serving 30 g

// relState is one planned Saturday: breakfast oats (350 kcal), lunch curry
// (500) and dinner lasagne with potatoes (1200), for 2000 kcal a day: the
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
	s.NoCook = []int64{relNuts}
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
		one(3, dinner, false, lasagne, potatoes),
	}
	return s
}

func ways(t *testing.T, s planning.State, req planning.WayRequest) planning.WayResult {
	t.Helper()
	res, err := planning.Ways(s, windowTarget(s), req)
	require.NoError(t, err)
	return res
}

func TestWays_Lighter(t *testing.T) {
	res := ways(t, relState(false), planning.WayRequest{PlateID: 3, Goal: planning.GoalLighter})
	assert.InDelta(t, 500, res.MaxKcal, 0.5)
	assert.InDelta(t, 1200, res.Before.MealKcal, 0.5)
	require.NotEmpty(t, res.Ways)
	for _, w := range res.Ways {
		assert.True(t, w.Reaches, w.What)
		assert.LessOrEqual(t, w.After.MealKcal, res.MaxKcal+0.5, w.What)
		assert.GreaterOrEqual(t, w.After.DayKcal, res.FloorKcal, "%s keeps the day above its floor", w.What)
		assert.NotEmpty(t, w.Ops)
		assert.Contains(t, w.Light, planning.LightIntent{Date: sat, Scope: "meal", Slot: "dinner"}, "a way that meets the cap carries its mark")
		if w.Kind == "compound" {
			assert.NotEmpty(t, w.Moved, "a compound way says what moves to the other meal")
		}
	}
	// A dinner at its cap leaves the day under its floor: the first way
	// has to move energy to another meal.
	assert.Equal(t, "compound", res.Ways[0].Kind, res.Ways[0].What)
}

func TestWays_HeartierEvenWhenAboveTheMinimum(t *testing.T) {
	res := ways(t, relState(false), planning.WayRequest{PlateID: 3, Goal: planning.GoalHeartier})
	assert.InDelta(t, 833, res.MinKcal, 0.5)
	require.NotEmpty(t, res.Ways, "1200 kcal is above the minimum and still has to get heartier")
	for _, w := range res.Ways {
		assert.True(t, w.Reaches)
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

func TestWays_NoCompensationAvailable(t *testing.T) {
	// The other meals are confirmed: nothing can lift the day, so a dinner
	// at its cap would break the floor. No way may reach the cap; the answer
	// says how far it gets.
	res := ways(t, relState(true), planning.WayRequest{PlateID: 3, Goal: planning.GoalLighter})
	for _, w := range res.Ways {
		assert.False(t, w.Reaches, w.What)
		assert.NotEmpty(t, w.Why)
		assert.GreaterOrEqual(t, w.After.DayKcal, res.FloorKcal)
	}
	if len(res.Ways) == 0 {
		assert.NotEmpty(t, res.EmptyWhy)
	}
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
			changed, sat, why := planning.JudgeRelative(tt.goal, tt.meal, tt.maxK, tt.minK, tt.before, tt.after)
			assert.Equal(t, tt.changed, changed)
			assert.Equal(t, tt.sat, sat)
			assert.Equal(t, !tt.sat, why != "", "a request that is not satisfied says why")
		})
	}
}
