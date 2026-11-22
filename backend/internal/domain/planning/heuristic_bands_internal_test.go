package planning

import (
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

// PL7 (SB1/SB5): the shape of the spectest weeks 12-05 and 12-12 (dev,
// veg1700, two2600). The protein add-on went on the dinner, which already
// holds the new dish, while the lunch eats a light leftover. The day is in
// its band, but neither meal is in its slot band, and no single portion step
// per meal reaches it: the add-on has to move to the lunch and each meal takes
// a quarter-portion step.
func TestFitMealsToBands_MovesAnAddOnAndStepsTwoMeals(t *testing.T) {
	const (
		breakfast, lunch, dinner                  = int64(1), int64(2), int64(3)
		oats, curry, bake, couscous, yogurt, tofu = int64(10), int64(11), int64(12), int64(13), int64(14), int64(15)
	)
	mon := time.Date(2026, 12, 7, 0, 0, 0, 0, time.UTC)
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
		SlotOrder: plate.SlotOrder{breakfast: 0, lunch: 1, dinner: 2},
		SlotNames: map[int64]string{breakfast: "slot.breakfast", lunch: "slot.lunch", dinner: "slot.dinner"},
		SlotKind:  map[int64]slot.Kind{breakfast: slot.KindLight, lunch: slot.KindMeal, dinner: slot.KindMeal},
		SlotBand: map[int64]slot.Band{
			breakfast: {SlotID: breakfast, MinPct: 20, MaxPct: 30},
			lunch:     {SlotID: lunch, MinPct: 30, MaxPct: 40},
			dinner:    {SlotID: dinner, MinPct: 25, MaxPct: 40},
		},
		SlotShare: map[int64]float64{breakfast: 25, lunch: 35, dinner: 32.5},
		Foods: map[int64]FoodFacts{
			oats:     dish(oats, "Oats", food.RoleStandalone, 450),
			curry:    dish(curry, "Lentil curry", food.RoleMain, 316),
			bake:     dish(bake, "Vegetable bake", food.RoleMain, 700),
			couscous: dish(couscous, "Couscous", food.RoleSideStarch, 250),
			yogurt:   leaf(yogurt, "Natural yogurt", "", 61, 4),
			tofu:     leaf(tofu, "Tofu (firm)", food.RoleSideProtein, 144, 15),
		},
		Settings: Settings{BatchSettings: plate.BatchSettings{DefaultKeepDays: 3}, MaxCookOccasionsPerDay: 3, VarietyWeeks: 3},
		Profile:  Profile{DietType: profile.DietVegan, HouseholdPortions: 1, KcalTarget: &k},
	}
	var tgt Target
	for _, sl := range []int64{breakfast, lunch, dinner} {
		s.Cells = append(s.Cells, Cell{Date: mon, SlotID: sl, Mode: availability.ModePlan})
		tgt.Cells = append(tgt.Cells, plate.CellKey{Date: mon, SlotID: sl})
	}
	h := newPlanner(ChooseInput{State: s, Target: tgt})
	h.prop = Proposal{
		Sessions: []ProposedSession{
			{Key: "o", FoodID: oats, CookDate: mon, CookSlotID: breakfast, Portions: 1},
			{Key: "a", FoodID: curry, CookDate: mon, CookSlotID: lunch, Portions: 1},
			{Key: "b", FoodID: bake, CookDate: mon, CookSlotID: dinner, Portions: 1},
			{Key: "c", FoodID: couscous, CookDate: mon, CookSlotID: dinner, Portions: 1},
		},
		Plates: []ProposedPlate{
			{Date: mon, SlotID: breakfast, Eaters: 1, Components: []ProposedComponent{{FoodID: oats, Portions: 1, SessionKey: "o"}}},
			{Date: mon, SlotID: lunch, Eaters: 1, Components: []ProposedComponent{
				{FoodID: curry, Portions: 1, SessionKey: "a"}, {FoodID: yogurt, Grams: 200},
			}},
			{Date: mon, SlotID: dinner, Eaters: 1, Components: []ProposedComponent{
				{FoodID: bake, Portions: 1, SessionKey: "b"}, {FoodID: couscous, Portions: 1, SessionKey: "c"}, {FoodID: tofu, Grams: 200},
			}},
		},
	}
	inBand := func() (lunchIn, dinnerIn bool, plates map[int64]PlateEval, day float64) {
		m := Merge(h.s, tgt, h.prop)
		ev := evaluateLean(m, tgt)
		x := newIndex(m, tgt)
		dk := x.dayAccounts([]time.Time{mon})[0]
		plates = map[int64]PlateEval{}
		for _, p := range ev.Plates {
			plates[p.SlotID] = p
		}
		lo, hi := dk.bandKcal(s.SlotBand[lunch], lunch)
		lunchIn = plates[lunch].Kcal >= lo && plates[lunch].Kcal <= hi
		lo, hi = dk.bandKcal(s.SlotBand[dinner], dinner)
		dinnerIn = plates[dinner].Kcal >= lo && plates[dinner].Kcal <= hi
		return lunchIn, dinnerIn, plates, dk.Kcal
	}
	l, d, plates, _ := inBand()
	require.False(t, l, "the lunch starts under its band: %.0f kcal", plates[lunch].Kcal)
	require.False(t, d, "the dinner starts over its band: %.0f kcal", plates[dinner].Kcal)
	require.Empty(t, openHard(Merge(h.s, tgt, h.prop), tgt))

	h.fitMealsToBands()

	l, d, plates, day := inBand()
	assert.True(t, l, "lunch %.0f kcal", plates[lunch].Kcal)
	assert.True(t, d, "dinner %.0f kcal", plates[dinner].Kcal)
	assert.GreaterOrEqual(t, day, 0.9*k-0.5, "the day stays in its band")
	assert.Empty(t, openHard(Merge(h.s, tgt, h.prop), tgt), "no hard issue")
	var tofuAt int64
	for _, p := range h.prop.Plates {
		for _, c := range p.Components {
			if c.FoodID == tofu {
				tofuAt = p.SlotID
			}
		}
	}
	assert.Equal(t, lunch, tofuAt, "the protein add-on moved to the lunch")
}
