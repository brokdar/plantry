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

const (
	bvLunch, bvDinner    = int64(2), int64(3)
	bvMain, bvBatchSide  = int64(10), int64(11)
	bvLeafA, bvLeafB     = int64(21), int64(22)
	bvMainKey, bvSideKey = "m", "s"
	bvKcalSide           = 150.0
	bvMon, bvTue, bvWed  = 0, 1, 2 // days from bvStart
)

var bvStart = time.Date(2026, 12, 7, 0, 0, 0, 0, time.UTC)

func bvDay(n int) time.Time { return bvStart.AddDate(0, 0, n) }

// bvState is a three-day state with a batch: the main and a shared side are
// cooked on the Monday dinner and served again on the Tuesday dinner. The two
// leaf sides are the variants the Tuesday plate may take.
func bvState(plateFit bool, a, b FoodFacts) (State, Target, Proposal) {
	k := 2200.0
	s := State{
		Window:    window.Window{Start: bvDay(bvMon), End: bvDay(bvWed)},
		Today:     bvDay(bvMon).AddDate(0, 0, -3),
		SlotOrder: plate.SlotOrder{bvLunch: 0, bvDinner: 1},
		SlotNames: map[int64]string{bvLunch: "slot.lunch", bvDinner: "slot.dinner"},
		SlotKind:  map[int64]slot.Kind{bvLunch: slot.KindMeal, bvDinner: slot.KindMeal},
		SlotBand: map[int64]slot.Band{
			bvLunch:  {SlotID: bvLunch, MinPct: 0, MaxPct: 100},
			bvDinner: {SlotID: bvDinner, MinPct: 0, MaxPct: 100},
		},
		SlotShare: map[int64]float64{bvLunch: 50, bvDinner: 50},
		Foods: map[int64]FoodFacts{
			bvMain: {
				ID: bvMain, Name: "Lentil curry", Kind: food.KindComposed, Role: food.RoleMain, DietClass: "vegan",
				NutritionComplete: true, ReferencePortions: 1, Macros: nutrition.Macros{Kcal: 600, Protein: 25, Fat: 13, Carbs: 75},
				Fresh: new(bool), // known not fresh: a non-fresh side leaves the fresh gap raised
			},
			bvBatchSide: {
				ID: bvBatchSide, Name: "Rice", Kind: food.KindComposed, Role: food.RoleSideVeg, DietClass: "vegan",
				NutritionComplete: true, ReferencePortions: 1, Macros: nutrition.Macros{Kcal: bvKcalSide, Fat: 3, Carbs: 30},
			},
			a.ID: a,
			b.ID: b,
		},
		NoCook:   []int64{a.ID, b.ID},
		Settings: Settings{BatchSettings: plate.BatchSettings{DefaultKeepDays: 3}, MaxCookOccasionsPerDay: 3, VarietyWeeks: 3, BatchVariation: true, PlateFit: plateFit},
		Profile:  Profile{DietType: profile.DietVegan, HouseholdPortions: 1, KcalTarget: &k},
	}
	var tgt Target
	for n := bvMon; n <= bvWed; n++ {
		for _, sl := range []int64{bvLunch, bvDinner} {
			s.Cells = append(s.Cells, Cell{Date: bvDay(n), SlotID: sl, Mode: availability.ModePlan})
			tgt.Cells = append(tgt.Cells, plate.CellKey{Date: bvDay(n), SlotID: sl})
		}
	}
	batch := []ProposedComponent{{FoodID: bvMain, Portions: 1, SessionKey: bvMainKey}, {FoodID: bvBatchSide, Portions: 1, SessionKey: bvSideKey}}
	in := Proposal{
		Sessions: []ProposedSession{
			{Key: bvMainKey, FoodID: bvMain, CookDate: bvDay(bvMon), CookSlotID: bvDinner, Portions: 2},
			{Key: bvSideKey, FoodID: bvBatchSide, CookDate: bvDay(bvMon), CookSlotID: bvDinner, Portions: 2},
		},
		Plates: []ProposedPlate{
			{Date: bvDay(bvMon), SlotID: bvDinner, Eaters: 1, Components: batch},
			{Date: bvDay(bvTue), SlotID: bvDinner, Eaters: 1, Components: batch},
		},
	}
	return s, tgt, in
}

func bvLeaf(id int64, name string, kcal float64, fresh bool) FoodFacts {
	return FoodFacts{
		ID: id, Name: name, Kind: food.KindLeaf, LeafRole: food.RoleSideVeg, DietClass: "vegan", NoCook: true,
		NutritionComplete: true, ServingG: 100, Fresh: &fresh, Macros: nutrition.Macros{Kcal: kcal, Fat: 1, Carbs: 10},
	}
}

// bvLeftoverSide is the side the batch's leftover plate (Tuesday dinner) eats
// after varyBatches.
func bvLeftoverSide(t *testing.T, in Proposal) int64 {
	t.Helper()
	for _, p := range in.Plates {
		if p.Date.Equal(bvDay(bvTue)) && p.SlotID == bvDinner {
			for _, c := range p.Components {
				if c.FoodID != bvMain {
					return c.FoodID
				}
			}
		}
	}
	require.Fail(t, "no Tuesday dinner plate")
	return 0
}

// A side another plate of the same day serves is no variant with plate fit on
// (PLATE-6); with plate fit off the old picks stay: the closest side by kcal
// wins even when the day already serves it.
func TestVaryBatches_TodaySkipIsPlateFits(t *testing.T) {
	for _, tc := range []struct {
		name     string
		plateFit bool
		want     int64
	}{
		{"plate fit on: the day's side is skipped", true, bvLeafB},
		{"plate fit off: the day's side is a variant", false, bvLeafA},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// A is within the old side's kcal bucket (and lifts the day a little), B is two buckets away; A is already
			// served by the day's lunch.
			s, tgt, in := bvState(tc.plateFit, bvLeaf(bvLeafA, "Leaf A", bvKcalSide+10, true), bvLeaf(bvLeafB, "Leaf B", bvKcalSide+60, true))
			in.Plates = append(in.Plates, ProposedPlate{Date: bvDay(bvTue), SlotID: bvLunch, Eaters: 1, Components: []ProposedComponent{{FoodID: bvLeafA, Portions: 1}}})
			out := varyBatches(s, tgt, in, nil)
			assert.Equal(t, tc.want, bvLeftoverSide(t, out))
		})
	}
}

// Rotation ranks before raised gaps: the side the week uses less wins over
// one with a clean plate fit that the week already uses more.
func TestVaryBatches_RotationBeforeRaisedGaps(t *testing.T) {
	// A is fresh (no raised gap) but used on Wednesday's lunch; B is not fresh
	// (a raised gap) and unused.
	s, tgt, in := bvState(true, bvLeaf(bvLeafA, "Leaf A", bvKcalSide+10, true), bvLeaf(bvLeafB, "Leaf B", bvKcalSide+10, false))
	in.Plates = append(in.Plates, ProposedPlate{Date: bvDay(bvWed), SlotID: bvLunch, Eaters: 1, Components: []ProposedComponent{{FoodID: bvLeafA, Portions: 1}}})
	out := varyBatches(s, tgt, in, nil)
	assert.Equal(t, bvLeafB, bvLeftoverSide(t, out))
}
