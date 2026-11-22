package planning

import (
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/availability"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/nutrition"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
	"github.com/jaltszeimer/plantry/backend/internal/domain/slot"
	"github.com/jaltszeimer/plantry/backend/internal/domain/window"
)

// Fixed ids of the band fixtures: one day, three slots and the foods of the
// spectest-shaped week (see TestFitMealsToBands_MovesAnAddOnAndStepsTwoMeals).
const (
	bfBreakfast, bfLunch, bfDinner                        = int64(1), int64(2), int64(3)
	bfOats, bfCurry, bfBake, bfCouscous, bfYogurt, bfTofu = int64(10), int64(11), int64(12), int64(13), int64(14), int64(15)
)

var bfMon = time.Date(2026, 12, 7, 0, 0, 0, 0, time.UTC)

// bandsPlanner is a one-day planner (kcal target 2200, slot bands 30-40 %
// lunch, 25-40 % dinner) holding the proposal plates, with the light intents.
func bandsPlanner(light []LightIntent, plates []ProposedPlate) (*planner, Target) {
	mon := bfMon
	k := 2200.0
	leaf := func(id int64, name string, role food.Role, kcal, protein float64) FoodFacts {
		return FoodFacts{
			ID: id, Name: name, Kind: food.KindLeaf, LeafRole: role, NoCook: true, NutritionComplete: true,
			Macros: nutrition.Macros{Kcal: kcal, Protein: protein, Fat: kcal / 45, Carbs: kcal / 10},
		}
	}
	dish := func(id int64, name string, role food.Role, kcal float64) FoodFacts {
		return FoodFacts{
			ID: id, Name: name, Kind: food.KindComposed, Role: role, DietClass: "vegan", NutritionComplete: true,
			ReferencePortions: 4, Macros: nutrition.Macros{Kcal: kcal, Protein: kcal / 25, Fat: kcal / 45, Carbs: kcal / 8},
		}
	}
	s := State{
		Window:    window.Window{Start: mon, End: mon},
		Today:     mon.AddDate(0, 0, -3),
		SlotOrder: plate.SlotOrder{bfBreakfast: 0, bfLunch: 1, bfDinner: 2},
		SlotNames: map[int64]string{bfBreakfast: "slot.breakfast", bfLunch: "slot.lunch", bfDinner: "slot.dinner"},
		SlotKind:  map[int64]slot.Kind{bfBreakfast: slot.KindLight, bfLunch: slot.KindMeal, bfDinner: slot.KindMeal},
		SlotBand: map[int64]slot.Band{
			bfBreakfast: {SlotID: bfBreakfast, MinPct: 20, MaxPct: 30},
			bfLunch:     {SlotID: bfLunch, MinPct: 30, MaxPct: 40},
			bfDinner:    {SlotID: bfDinner, MinPct: 25, MaxPct: 40},
		},
		SlotShare: map[int64]float64{bfBreakfast: 25, bfLunch: 35, bfDinner: 32.5},
		Foods: map[int64]FoodFacts{
			bfOats:     dish(bfOats, "Oats", food.RoleStandalone, 450),
			bfCurry:    dish(bfCurry, "Lentil curry", food.RoleMain, 316),
			bfBake:     dish(bfBake, "Vegetable bake", food.RoleMain, 700),
			bfCouscous: dish(bfCouscous, "Couscous", food.RoleSideStarch, 250),
			bfYogurt:   leaf(bfYogurt, "Natural yogurt", "", 61, 4),
			bfTofu:     leaf(bfTofu, "Tofu (firm)", food.RoleSideProtein, 144, 15),
		},
		Light:    light,
		Settings: Settings{BatchSettings: plate.BatchSettings{DefaultKeepDays: 3}, MaxCookOccasionsPerDay: 3, VarietyWeeks: 3},
		Profile:  Profile{DietType: profile.DietVegan, HouseholdPortions: 1, KcalTarget: &k},
	}
	var tgt Target
	for _, sl := range []int64{bfBreakfast, bfLunch, bfDinner} {
		s.Cells = append(s.Cells, Cell{Date: mon, SlotID: sl, Mode: availability.ModePlan})
		tgt.Cells = append(tgt.Cells, plate.CellKey{Date: mon, SlotID: sl})
	}
	h := newPlanner(ChooseInput{State: s, Target: tgt})
	h.prop = Proposal{
		Sessions: []ProposedSession{
			{Key: "o", FoodID: bfOats, CookDate: mon, CookSlotID: bfBreakfast, Portions: 1},
			{Key: "a", FoodID: bfCurry, CookDate: mon, CookSlotID: bfLunch, Portions: 1},
			{Key: "b", FoodID: bfBake, CookDate: mon, CookSlotID: bfDinner, Portions: 1},
			{Key: "c", FoodID: bfCouscous, CookDate: mon, CookSlotID: bfDinner, Portions: 1},
		},
		Plates: plates,
	}
	return h, tgt
}

func bfPlates(lunch, dinner []ProposedComponent) []ProposedPlate {
	return []ProposedPlate{
		{Date: bfMon, SlotID: bfBreakfast, Eaters: 1, Components: []ProposedComponent{{FoodID: bfOats, Portions: 1, SessionKey: "o"}}},
		{Date: bfMon, SlotID: bfLunch, Eaters: 1, Components: lunch},
		{Date: bfMon, SlotID: bfDinner, Eaters: 1, Components: dinner},
	}
}

func plateOf(h *planner, slot int64) ProposedPlate {
	for _, p := range h.prop.Plates {
		if p.SlotID == slot {
			return p
		}
	}
	return ProposedPlate{}
}

// NU11: a meal with its own light or hearty mark is exempt from the slot band:
// PL7 neither steps it nor moves an add-on to or from it. Unmarked, the light
// dinner would give its tofu to the lunch and the hearty lunch would take a
// quarter-portion step into its band; marked, the meal stays exactly as it is.
func TestFitMealsToBands_LeavesMarkedMealsAlone(t *testing.T) {
	for _, tc := range []struct {
		name          string
		intent        LightIntent
		slot          int64
		lunch, dinner []ProposedComponent
		cooks         [2]int64 // the cook slot of the curry and of the bake
	}{
		{
			"hearty lunch would be stepped up",
			LightIntent{Date: "2026-12-07", Scope: LightScopeMeal, Slot: "lunch", Level: LevelHearty},
			bfLunch,
			[]ProposedComponent{{FoodID: bfBake, Portions: 1, SessionKey: "b"}, {FoodID: bfYogurt, Grams: 200}},
			[]ProposedComponent{{FoodID: bfCurry, Portions: 1, SessionKey: "a"}, {FoodID: bfCouscous, Portions: 1, SessionKey: "c"}, {FoodID: bfTofu, Grams: 200}},
			[2]int64{bfDinner, bfLunch},
		},
		{
			"light dinner would give up the add-on",
			LightIntent{Date: "2026-12-07", Scope: LightScopeMeal, Slot: "dinner"},
			bfDinner,
			[]ProposedComponent{{FoodID: bfCurry, Portions: 1, SessionKey: "a"}, {FoodID: bfYogurt, Grams: 200}},
			[]ProposedComponent{{FoodID: bfBake, Portions: 1, SessionKey: "b"}, {FoodID: bfCouscous, Portions: 1, SessionKey: "c"}, {FoodID: bfTofu, Grams: 200}},
			[2]int64{bfLunch, bfDinner},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, tgt := bandsPlanner([]LightIntent{tc.intent}, bfPlates(tc.lunch, tc.dinner))
			h.prop.Sessions[1].CookSlotID, h.prop.Sessions[2].CookSlotID = tc.cooks[0], tc.cooks[1]
			require.Empty(t, openHard(Merge(h.s, tgt, h.prop), tgt))
			before := plateOf(h, tc.slot)
			h.fitMealsToBands()
			assert.Equal(t, before, plateOf(h, tc.slot), "the marked meal is untouched")
		})
	}
}

// bandsDay is the one-day fixture of the stepping and refusal tests: a lunch
// of curry (1.5 portions) and 600 g yogurt (840 kcal, inside its band) and a
// dinner of the bake and the couscous at the given portions. The bake is rich
// in protein, so shrinking it lowers the day's protein share.
func bandsDay(bake, couscous float64) (*planner, Target) {
	h, tgt := bandsPlanner(nil, bfPlates(
		[]ProposedComponent{{FoodID: bfCurry, Portions: 1.5, SessionKey: "a"}, {FoodID: bfYogurt, Grams: 600}},
		[]ProposedComponent{{FoodID: bfBake, Portions: bake, SessionKey: "b"}, {FoodID: bfCouscous, Portions: couscous, SessionKey: "c"}},
	))
	h.prop.Sessions[1].Portions, h.prop.Sessions[2].Portions, h.prop.Sessions[3].Portions = 1.5, bake, couscous
	bk := h.s.Foods[bfBake]
	bk.Macros.Protein = 90
	h.s.Foods[bfBake] = bk
	return h, tgt
}

func bandsDayKcal(h *planner, tgt Target) (day float64, plates map[int64]float64) {
	ev := evaluateLean(Merge(h.s, tgt, h.prop), tgt)
	plates = map[int64]float64{}
	for _, p := range ev.Plates {
		plates[p.SlotID] = p.Kcal
	}
	return ev.Days[0].Kcal, plates
}

func portionsOf(p ProposedPlate, foodID int64) float64 {
	for _, c := range p.Components {
		if c.FoodID == foodID {
			return c.Portions
		}
	}
	return -1
}

// PL7 (b): a dinner of the bake and the couscous at 1.5 portions each is over
// its band; no portion of one component alone brings it in while the day stays
// in its band, but a step on both does.
func TestFitMealsToBands_StepsTwoComponentsOfOneMeal(t *testing.T) {
	h, tgt := bandsDay(1.5, 1.5)
	require.Empty(t, openHard(Merge(h.s, tgt, h.prop), tgt))
	lunchBefore := plateOf(h, bfLunch)
	h.fitMealsToBands()

	day, plates := bandsDayKcal(h, tgt)
	assert.GreaterOrEqual(t, plates[bfDinner], 0.25*2200-epsilon)
	assert.LessOrEqual(t, plates[bfDinner], 0.4*2200+epsilon)
	assert.GreaterOrEqual(t, day, 0.9*2200-epsilon, "the day stays in its band")
	dinner := plateOf(h, bfDinner)
	assert.NotEqual(t, 1.5, portionsOf(dinner, bfBake), "the bake stepped")
	assert.NotEqual(t, 1.5, portionsOf(dinner, bfCouscous), "the couscous stepped")
	assert.Equal(t, lunchBefore, plateOf(h, bfLunch), "the lunch is in its band and stays")
}

// shrinkBake is the move of the dinner's bake to the given portions; the
// lunch (meal 0) and the dinner (meal 1) are the meals of the fixture.
func shrinkBake(h *planner, portions float64) (meals []bandMeal, moves [][]bandMove) {
	f := h.s.Foods[bfBake]
	meals = []bandMeal{{pi: 1, eaters: 1}, {pi: 2, eaters: 1}}
	moves = [][]bandMove{nil, {{ci: 0, portions: portions, dKcal: (portions - h.prop.Plates[2].Components[0].Portions) * f.Macros.Kcal}}}
	return meals, moves
}

// dayProtein is the day's protein in g and the g/kg-free share of its kcal.
func dayProtein(h *planner, tgt Target) (g, pct float64) {
	d := evaluateLean(Merge(h.s, tgt, h.prop), tgt).Days[0]
	return d.ProteinG, d.ProteinPct
}

// applyAfter applies the shrink of the bake to portions on a copy of the
// proposal and returns the day's protein before and after, restoring it.
func proteinAround(t *testing.T, h *planner, tgt Target, portions float64) (g0, pct0, g1, pct1 float64) {
	t.Helper()
	g0, pct0 = dayProtein(h, tgt)
	meals, moves := shrinkBake(h, portions)
	saved := h.prop
	plates, sessions := slices.Clone(h.prop.Plates), slices.Clone(h.prop.Sessions)
	require.True(t, applyFrom(h, meals, moves), "no protein rule yet: the move holds")
	g1, pct1 = dayProtein(h, tgt)
	saved.Plates, saved.Sessions = plates, sessions
	h.prop = saved
	return g0, pct0, g1, pct1
}

// S3 (spec 2.5): a move may open the day's protein issue (priority 4) only
// when the same step closes that day's kcal-band issue (priority 3); any other
// soft issue it would open refuses it, and a refused move leaves the proposal
// as it was.
func TestApplyMoves_SoftIssueRefusesAndS3Exception(t *testing.T) {
	proteinShare := func(h *planner, pct0, pct1 float64) {
		target := (pct0+pct1)/2 + ProteinMarginPts // the day's floor falls between the two
		h.s.Profile.ProteinPct = &target
	}
	t.Run("closing the day's kcal band may open the day's protein", func(t *testing.T) {
		h, tgt := bandsDay(1.5, 1)
		_, pct0, _, pct1 := proteinAround(t, h, tgt, 0.75)
		require.Less(t, pct1, pct0)
		proteinShare(h, pct0, pct1)
		day, _ := bandsDayKcal(h, tgt)
		require.Greater(t, day, 1.1*2200, "the day starts above its band")
		meals, moves := shrinkBake(h, 0.75)
		assert.True(t, applyFrom(h, meals, moves))
		after, _ := bandsDayKcal(h, tgt)
		assert.LessOrEqual(t, after, 1.1*2200)
		assert.Equal(t, 0.75, portionsOf(plateOf(h, bfDinner), bfBake))
	})
	t.Run("a day already in its band may not open the day's protein", func(t *testing.T) {
		h, tgt := bandsDay(1, 1)
		_, pct0, _, pct1 := proteinAround(t, h, tgt, 0.75)
		require.Less(t, pct1, pct0)
		proteinShare(h, pct0, pct1)
		before := slices.Clone(h.prop.Plates[2].Components)
		sessions := slices.Clone(h.prop.Sessions)
		meals, moves := shrinkBake(h, 0.75)
		assert.False(t, applyFrom(h, meals, moves))
		assert.Equal(t, before, h.prop.Plates[2].Components, "the proposal is restored")
		assert.Equal(t, sessions, h.prop.Sessions, "the batches too")
	})
	t.Run("a week protein issue refuses even when the day band closes", func(t *testing.T) {
		h, tgt := weekOfDays(bandsDay(1.5, 1))
		g0, _, g1, _ := proteinAround(t, h, tgt, 0.75)
		require.Less(t, g1, g0)
		// A g/kg target that the week meets before the move and misses after
		// it: the week average sits between the two.
		w := 1.0
		h.s.Profile.WeightKg = &w
		ev := evaluateLean(Merge(h.s, tgt, h.prop), tgt)
		require.NotNil(t, ev.Week)
		w = (ev.Week.AvgProteinG - (g0-g1)/6) / ev.Week.TargetProteinG // the average over 3 days
		ev = evaluateLean(Merge(h.s, tgt, h.prop), tgt)
		require.NotContains(t, issueCodes(ev.Issues), CodeWeekProteinLow)
		meals, moves := shrinkBake(h, 0.75)
		assert.False(t, applyFrom(h, meals, moves))
		assert.Equal(t, 1.5, portionsOf(plateOf(h, bfDinner), bfBake))
	})
}

func issueCodes(issues []Issue) []string {
	var codes []string
	for _, is := range issues {
		codes = append(codes, is.Code)
	}
	return codes
}

// weekOfDays extends a one-day fixture to MinWeekDays days (two more
// breakfast-only days), so the week-level rules apply.
func weekOfDays(h *planner, tgt Target) (*planner, Target) {
	for i, key := range []string{"o2", "o3"} {
		d := bfMon.AddDate(0, 0, i+1)
		h.s.Window.End = d
		h.s.Cells = append(h.s.Cells, Cell{Date: d, SlotID: bfBreakfast, Mode: availability.ModePlan})
		tgt.Cells = append(tgt.Cells, plate.CellKey{Date: d, SlotID: bfBreakfast})
		h.prop.Sessions = append(h.prop.Sessions, ProposedSession{Key: key, FoodID: bfOats, CookDate: d, CookSlotID: bfBreakfast, Portions: 1})
		h.prop.Plates = append(h.prop.Plates, ProposedPlate{Date: d, SlotID: bfBreakfast, Eaters: 1, Components: []ProposedComponent{{FoodID: bfOats, Portions: 1, SessionKey: key}}})
	}
	h.in.Target = tgt
	return h, tgt
}

// PL7: the best combination of a day is kept only when it opens no soft issue
// the plan had closed; the next one is tried then. In the two-meal fixture the
// best combination (the tofu to the lunch and a step on each meal) lowers the
// day's protein share a little. With the day's protein floor between the share
// before and after, it is refused and the next one, the tofu moving alone,
// is applied. Without the floor the best one is applied.
func TestFitMealsToBands_RefusedCombinationUsesTheNext(t *testing.T) {
	two := func() (*planner, Target) {
		return bandsPlanner(nil, bfPlates(
			[]ProposedComponent{{FoodID: bfCurry, Portions: 1, SessionKey: "a"}, {FoodID: bfYogurt, Grams: 200}},
			[]ProposedComponent{{FoodID: bfBake, Portions: 1, SessionKey: "b"}, {FoodID: bfCouscous, Portions: 1, SessionKey: "c"}, {FoodID: bfTofu, Grams: 200}},
		))
	}
	control, ctgt := two()
	_, pct0 := dayProtein(control, ctgt)
	control.fitMealsToBands()
	_, pct1 := dayProtein(control, ctgt)
	require.Less(t, pct1, pct0)
	require.Equal(t, 0.75, portionsOf(plateOf(control, bfDinner), bfBake), "the best combination steps the bake")

	h, tgt := two()
	floor := (pct0+pct1)/2 + ProteinMarginPts
	h.s.Profile.ProteinPct = &floor
	require.NotContains(t, issueCodes(evaluateLean(Merge(h.s, tgt, h.prop), tgt).Issues), CodeProteinLow)
	h.fitMealsToBands()
	dinner := plateOf(h, bfDinner)
	assert.Equal(t, 1.0, portionsOf(dinner, bfBake), "the best combination was refused")
	assert.Equal(t, 1.0, portionsOf(dinner, bfCouscous))
	assert.Equal(t, 200.0, gramsOf(plateOf(h, bfLunch), bfTofu), "the next combination moved the tofu to the lunch")
	assert.NotContains(t, issueCodes(evaluateLean(Merge(h.s, tgt, h.prop), tgt).Issues), CodeProteinLow)
}

func gramsOf(p ProposedPlate, foodID int64) float64 {
	for _, c := range p.Components {
		if c.FoodID == foodID {
			return c.Grams
		}
	}
	return -1
}

// applyFrom is applyMoves against the issues of the proposal as it stands.
func applyFrom(h *planner, meals []bandMeal, moves [][]bandMove) bool {
	was, wasSoft := h.openIssues()
	return h.applyMoves(meals, moves, nil, was, wasSoft)
}
