package planning

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/nutrition"
)

// Fillers of the plate-fit filler test, all 250 kcal per 100 g serving.
const (
	pfChickpeas, pfBread, pfCheese, pfBigBread = int64(31), int64(32), int64(33), int64(34)
)

// fillerPlanner is bandsPlanner's day (lunch curry, dinner bake with
// couscous) with three no-cook fillers of the same kcal: chickpeas (a legume
// meal), bread (most carbs) and cheese, plus a bread whose half serving is 375 kcal.
// carbsPct sets the dishes' carb share, so the week's carbs are below or in
// range.
func fillerPlanner(plateFit bool, carbsPct float64, legumeWeek bool) (*planner, Target) {
	h, tgt := bandsPlanner(nil, bfPlates(
		[]ProposedComponent{{FoodID: bfCurry, Portions: 1, SessionKey: "a"}},
		[]ProposedComponent{{FoodID: bfBake, Portions: 1, SessionKey: "b"}, {FoodID: bfCouscous, Portions: 1, SessionKey: "c"}},
	))
	for id, f := range h.s.Foods {
		f.Macros.Carbs = f.Macros.Kcal * carbsPct / 100 / 4
		f.Macros.Fat = f.Macros.Kcal * 0.3 / 9
		h.s.Foods[id] = f
	}
	if legumeWeek {
		f := h.s.Foods[bfCurry]
		f.Groups.LegumeG = 120
		h.s.Foods[bfCurry] = f
	}
	filler := func(id int64, name string, carbs, fat float64, legume float64) FoodFacts {
		f := h.s.Foods[bfYogurt]
		f.ID, f.Name, f.ServingG, f.DietClass = id, name, 100, "vegan"
		f.Macros = nutrition.Macros{Kcal: 250, Protein: 10, Carbs: carbs, Fat: fat}
		f.Groups.LegumeG = legume
		return f
	}
	h.s.Foods[pfChickpeas] = filler(pfChickpeas, "Kichererbsen", 30, 5, 100)
	h.s.Foods[pfBread] = filler(pfBread, "Brot", 50, 2, 0)
	h.s.Foods[pfCheese] = filler(pfCheese, "Käse", 2, 20, 0)
	big := filler(pfBigBread, "Bauernbrot", 50, 2, 0)
	big.ServingG = 300 // its smallest add, half a serving, is 375 kcal
	h.s.Foods[pfBigBread] = big
	h.s.NoCook = []int64{pfCheese, pfBread, pfChickpeas} // the old order: cheese first
	h.s.Settings.PlateFit = plateFit
	h.in.State = h.s
	h.topUpBase, h.adds = map[[2]int]float64{}, map[int]*dayAdds{} // as topUpFloors sets them
	return h, tgt
}

// added is the food addNoCook put on the day's dinner (0: none).
func added(t *testing.T, h *planner, limit float64) int64 {
	t.Helper()
	require.True(t, h.addNoCook(bfMon, false, limit), "a filler is added")
	for _, c := range plateOf(h, bfDinner).Components {
		if c.FoodID != bfBake && c.FoodID != bfCouscous {
			return c.FoodID
		}
	}
	return 0
}

// PLATE-4 for the filler pass: within its kcal bucket a filler serves the
// week first (a legume meal while the week has none, then carb kcal while
// its carb share is below range); the kcal fit still comes before both.
func TestAddNoCook_PlateFitFillerServesTheWeek(t *testing.T) {
	const need = 250 // one 100 g serving of each filler
	tests := []struct {
		name     string
		plateFit bool
		carbsPct float64
		legume   bool
		big      bool // only the large bread beside chickpeas and cheese
		want     int64
	}{
		{name: "plate fit off: the old order", carbsPct: 30, want: pfCheese},
		{name: "legume meal while the week has none", plateFit: true, carbsPct: 30, want: pfChickpeas},
		{name: "carb kcal while carbs are below range", plateFit: true, carbsPct: 30, legume: true, want: pfBread},
		{name: "carbs in range: no carb key", plateFit: true, carbsPct: 55, legume: true, want: pfCheese},
		{name: "kcal fit first: the large bread is outside the bucket", plateFit: true, carbsPct: 30, legume: true, big: true, want: pfChickpeas}, // more carbs than the cheese
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, tgt := fillerPlanner(tt.plateFit, tt.carbsPct, tt.legume)
			if tt.big {
				h.s.NoCook = []int64{pfCheese, pfBigBread, pfChickpeas}
				h.in.State = h.s
			}
			ev := evaluateLean(Merge(h.s, tgt, h.prop), tgt)
			require.Len(t, ev.Days, 1)
			require.NotNil(t, ev.Week)
			assert.Equal(t, tt.legume, ev.Week.LegumeMeals > 0)
			assert.Equal(t, tt.want, added(t, h, ev.Days[0].Kcal+need))
		})
	}
}

// With plate fit on, the day's top-up rounds up to the quarter portion: a
// day 100 kcal short raises its mains by a quarter, where it floors (and
// raises nothing) with plate fit off.
func TestLiftDay_PlateFitRoundsTheTopUpUp(t *testing.T) {
	for _, tc := range []struct {
		plateFit bool
		want     float64
	}{{false, 1}, {true, 1.25}} {
		h, tgt := bandsPlanner(nil, bfPlates(
			[]ProposedComponent{{FoodID: bfCurry, Portions: 1, SessionKey: "a"}},
			[]ProposedComponent{{FoodID: bfBake, Portions: 1, SessionKey: "b"}, {FoodID: bfCouscous, Portions: 1, SessionKey: "c"}},
		))
		h.s.Settings.PlateFit = tc.plateFit
		h.in.State = h.s
		h.topUpBase, h.adds = map[[2]int]float64{}, map[int]*dayAdds{}
		ev := evaluateLean(Merge(h.s, tgt, h.prop), tgt)
		kcal := ev.Days[0].Kcal
		h.liftDay(DayKcal{Date: bfMon, Kcal: kcal}, kcal+100, false, false)
		for _, slot := range []int64{bfLunch, bfDinner} {
			for _, c := range plateOf(h, slot).Components {
				if c.FoodID == bfCurry || c.FoodID == bfBake {
					assert.InDelta(t, tc.want, c.Portions, 1e-9, "plate fit %v, %s", tc.plateFit, h.s.Foods[c.FoodID].Name)
				}
			}
		}
	}
}

// excessFat counts the fat kcal above the range top in 100 kcal steps.
func TestExcessFat(t *testing.T) {
	for _, tc := range []struct {
		name string
		m    nutrition.Macros
		mult float64
		want float64
	}{
		{"lean gives 0", nutrition.Macros{Kcal: 200, Fat: 5}, 1, 0},
		{"at the range top gives 0", nutrition.Macros{Kcal: 200, Fat: 70.0 / 9}, 1, 0},
		{"fatty: 180 fat kcal - 70", nutrition.Macros{Kcal: 200, Fat: 20}, 1, 1},
		{"scales with the portion", nutrition.Macros{Kcal: 200, Fat: 20}, 2, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.InDelta(t, tc.want, excessFat(tc.m, tc.mult, 35), 1e-9)
		})
	}
}

// fattenBake makes the bake 60 % fat kcal (above any fat range top).
func fattenBake(h *planner) {
	f := h.s.Foods[bfBake]
	f.Macros.Fat = f.Macros.Kcal * 0.6 / 9
	h.s.Foods[bfBake] = f
	h.in.State = h.s
}

func liftPlanner() *planner {
	h, _ := bandsPlanner(nil, bfPlates(
		[]ProposedComponent{{FoodID: bfCurry, Portions: 1, SessionKey: "a"}},
		[]ProposedComponent{{FoodID: bfBake, Portions: 1, SessionKey: "b"}, {FoodID: bfCouscous, Portions: 1, SessionKey: "c"}},
	))
	h.s.Settings.PlateFit = true
	fattenBake(h)
	h.topUpBase, h.adds = map[[2]int]float64{}, map[int]*dayAdds{}
	return h
}

// The top-up with leanOnly grows the lean dishes and leaves the fatty ones.
func TestTopUpDay_LeanOnlyGrowsLeanDishesFirst(t *testing.T) {
	h := liftPlanner()
	h.topUpDay(bfMon, 400, true, true)
	assert.Greater(t, portionsOf(plateOf(h, bfLunch), bfCurry), 1.0, "the lean dish grows")
	assert.InDelta(t, 1.0, portionsOf(plateOf(h, bfDinner), bfBake), 1e-9, "the fatty dish stays")

	h = liftPlanner()
	h.topUpDay(bfMon, 400, true, false)
	assert.Greater(t, portionsOf(plateOf(h, bfDinner), bfBake), 1.0, "without leanOnly the fatty dish grows too")
}

// The last week lift lets fatty dishes grow when the week is still short.
func TestLiftWeek_FattyDishesLiftOnlyInTheLastPass(t *testing.T) {
	h := liftPlanner()
	// Only the fatty bake can grow: drop the lean curry from the week.
	h.prop.Plates[1].Components = nil
	h.liftWeek(true)
	assert.InDelta(t, 1.0, portionsOf(plateOf(h, bfDinner), bfBake), 1e-9, "lean-only pass leaves the fatty dish")
	h.liftWeek(false)
	assert.Greater(t, portionsOf(plateOf(h, bfDinner), bfBake), 1.0, "the last pass lifts it")
}

// With plate fit on, a bread filler beside a starch side is no candidate
// (no second starch); off, it stays.
func TestAddNoCook_PlateFitNoSecondStarchFiller(t *testing.T) {
	for _, tc := range []struct {
		plateFit bool
		want     int64
	}{{false, pfBread}, {true, 0}} { // on: the filler goes to the lunch
		h, tgt := fillerPlanner(tc.plateFit, 30, true)
		f := h.s.Foods[pfBread]
		f.Kind, f.LeafRole = food.KindLeaf, food.RoleSideStarch
		h.s.Foods[pfBread] = f
		h.s.NoCook = []int64{pfBread, pfCheese, pfChickpeas}
		h.in.State = h.s
		ev := evaluateLean(Merge(h.s, tgt, h.prop), tgt)
		assert.Equal(t, tc.want, added(t, h, ev.Days[0].Kcal+250), "plate fit %v", tc.plateFit)
	}
}

// The safety floor lift takes any verified food: with only bread reachable
// beside a starch side, the floor lift still adds it.
func TestAddNoCook_FloorLiftIgnoresSecondStarch(t *testing.T) {
	h, tgt := fillerPlanner(true, 30, true)
	f := h.s.Foods[pfBread]
	f.Kind, f.LeafRole = food.KindLeaf, food.RoleSideStarch
	h.s.Foods[pfBread] = f
	h.s.NoCook = []int64{pfBread}
	h.in.State = h.s
	h.floorLift = true
	ev := evaluateLean(Merge(h.s, tgt, h.prop), tgt)
	assert.Equal(t, int64(pfBread), added(t, h, ev.Days[0].Kcal+250))
}

// While the week's fat is above range, the filler with the least fat above
// the range comes before the carb and order keys.
func TestAddNoCook_PlateFitFillerKeepsFatInRange(t *testing.T) {
	for _, tc := range []struct {
		name   string
		fatHot bool
		want   int64
	}{{"fat in range: the old order", false, pfCheese}, {"fat above range: the lean filler", true, pfBread}} {
		t.Run(tc.name, func(t *testing.T) {
			h, tgt := fillerPlanner(true, 55, true) // carbs in range: no carb key
			if tc.fatHot {
				for _, id := range []int64{bfOats, bfCurry, bfBake, bfCouscous} {
					f := h.s.Foods[id]
					f.Macros.Fat = f.Macros.Kcal * 0.5 / 9
					h.s.Foods[id] = f
				}
				h.in.State = h.s
			}
			ev := evaluateLean(Merge(h.s, tgt, h.prop), tgt)
			assert.Equal(t, tc.fatHot, ev.Week.FatPct > h.x.pol.FatHigh)
			assert.Equal(t, tc.want, added(t, h, ev.Days[0].Kcal+250))
		})
	}
}
