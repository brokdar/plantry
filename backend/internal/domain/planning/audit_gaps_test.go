package planning_test

import (
	"fmt"
	"os"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/diet"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

// Replays the live p4 run 2's first reply (3 plates of 12 cells: the model
// used the checker as a to-do list). Completing it costs no repair: two
// real repairs are still left after it.
func TestLLMChooser_SkeletonFirstCallDoesNotSpendARepair(t *testing.T) {
	raw, err := os.ReadFile("testdata/run2_p4_attempt0.json")
	require.NoError(t, err)
	in := run4Input()
	bad := validWeek(t, in)
	bad.session("extra", 125, "2026-11-22", 3, 1) // a second cook on Sunday: H2
	good := validWeek(t, in)

	prop, reqs := chooseWith(t, in, submitRaw("tu_1", raw), submit(t, "tu_2", bad), submit(t, "tu_3", bad), submit(t, "tu_4", good))

	require.Len(t, reqs, 4, "skeleton + two paid repairs + the valid plan")
	assert.Empty(t, violationsOfProposal(in, prop))
	assert.Equal(t, 3, prop.RepairAttempts)
	assert.Contains(t, prop.Notes, "was completed in an extra attempt")
}

// The extra attempt is given once: a second skeleton is a normal repair.
func TestLLMChooser_OnlyOneFreeSkeletonAttempt(t *testing.T) {
	in := run4Input()
	empty := &jsonPlan{Notes: "later"}
	_, reqs := chooseWith(t, in, submit(t, "tu_1", empty), submit(t, "tu_2", empty), submit(t, "tu_3", empty), submit(t, "tu_4", empty))
	assert.Len(t, reqs, 4, "1 + 1 free + 2 repairs")
}

func TestPrompt_StatesTheFillCellsAndSuitability(t *testing.T) {
	in := run4Input()
	sys := planning.RenderPrompt(in)
	assert.Contains(t, sys, fmt.Sprintf("Submit the complete plan in one call: %d fill cells", len(in.Target.Cells)))
	assert.Contains(t, sys, "up to 1.5 portions per eater")
	// Candidate rows say where a recipe suits.
	assert.Contains(t, candidateLine(t, sys, 107), "|main|lunch_dinner|")
	assert.Contains(t, candidateLine(t, sys, 117), "|standalone|breakfast|")
	assert.Contains(t, candidateLine(t, sys, 124), "|side_starch|with_main|")
	// Days with a No-plate meal get a scaled target.
	k := 2200.0
	in.State.Profile.KcalTarget = &k
	sys = planning.RenderPrompt(in)
	assert.Contains(t, sys, "Saturday 2026-11-21: 2 of 3 meals planned, target 1467 kcal")
}

// Runs 6 and 7: a fixed session whose portions are all eaten was hidden, so
// its cook cell was invisible and the model cooked on the same day (H2).
func TestPrompt_FixedCookCellsAreShown(t *testing.T) {
	in := run4Input()
	one := 1.0
	in.State.Sessions = append(in.State.Sessions, planning.Session{
		Confirmed:   true,
		CookSession: plate.CookSession{ID: 26, FoodID: 124, CookDate: day("2026-11-22"), CookSlotID: 3, Portions: 1},
	})
	in.State.Plates = append(in.State.Plates, planning.Plate{
		ID: 5, Date: day("2026-11-22"), SlotID: 3, Eaters: 1, Confirmed: true,
		Components: []planning.Component{{FoodID: 124, Portions: &one, SessionID: 26}},
	})
	in.Target.Cells = slices.DeleteFunc(in.Target.Cells, func(c plate.CellKey) bool {
		return c.Date.Equal(day("2026-11-22")) && c.SlotID == 3
	})
	sys := planning.RenderPrompt(in)
	assert.Contains(t, sys, "Cooking already fixed")
	assert.Contains(t, sys, "- Sunday 2026-11-22 dinner slot_id 3: Steamed rice (session 26)")
	assert.NotContains(t, sys, "Fixed sessions with portions left")
}

// Run 5: the side swap saw neither the rest of the plate nor the role.
func sideSwapInput() planning.ChooseInput {
	in := run4Input()
	one := 1.0
	in.State.Foods[116] = recipe(116, "Tomato salad", food.RoleSideVeg, diet.Vegan, 607)
	in.State.Foods[115] = recipe(115, "Hasselback potatoes", food.RoleSideStarch, diet.Vegan, 300)
	in.State.Sessions = append(in.State.Sessions,
		planning.Session{CookSession: plate.CookSession{ID: 91, FoodID: 125, CookDate: day("2026-11-22"), CookSlotID: 3, Portions: 1}})
	in.State.Plates = append(in.State.Plates, planning.Plate{
		ID: 29, Date: day("2026-11-22"), SlotID: 3, Eaters: 1,
		Components: []planning.Component{{FoodID: 124, Portions: &one}, {FoodID: 125, Portions: &one, SessionID: 91}},
	})
	in.Target = planning.Target{
		Cells:      []plate.CellKey{{Date: day("2026-11-22"), SlotID: 3}},
		SessionIDs: []int64{91},
	}
	in.Side = &planning.SideSwap{
		FoodID: 125, Role: food.RoleSideVeg, CookDate: day("2026-11-22"), CookSlotID: 3, Portions: 1,
		Plates: []planning.SidePlate{{PlateID: 29, Date: day("2026-11-22"), SlotID: 3, Eaters: 1, PerEater: 1}},
	}
	return in
}

func TestPrompt_SideSwapKeepsTheRole(t *testing.T) {
	sys := planning.RenderPrompt(sideSwapInput())
	assert.Contains(t, sys, "the plate keeps Steamed rice (side_starch)")
	assert.Contains(t, sys, "Keep the role: Green salad has role side_veg")
	candidateLine(t, sys, 116)
	assert.NotContains(t, sys, "\n115|", "a starch is no candidate for a veg side")
	assert.NotContains(t, sys, "\n107|", "a main is no candidate for a veg side")
}

func TestLLMChooser_SideSwapToAnotherRoleIsSentBack(t *testing.T) {
	in := sideSwapInput()
	swap := func(food int64) *jsonPlan {
		p := &jsonPlan{Notes: "Swapped the salad."}
		p.session("s1", food, "2026-11-22", 3, 1)
		p.Plates = append(p.Plates, map[string]any{
			"plate_id": 29, "date": "2026-11-22", "slot_id": 3, "reason": "swap",
			"components": []map[string]any{{"food_id": food, "portions": 1, "session_key": "s1"}},
		})
		return p
	}
	prop, reqs := chooseWith(t, in, submit(t, "tu_1", swap(115)), submit(t, "tu_2", swap(116)))
	require.Len(t, reqs, 2)
	assert.Contains(t, feedbackOf(t, reqs[1]), "Hasselback potatoes has role side_starch, but the replaced side Green salad has role side_veg")
	assert.Equal(t, int64(116), prop.Sessions[0].FoodID)
}

// A standalone recipe (oats) is a breakfast, not a lunch or dinner main.
func TestMissingMain_StandaloneIsBreakfastOnly(t *testing.T) {
	in := run4Input()
	p := validWeek(t, in)
	p.Plates[1]["components"] = []map[string]any{{"food_id": 117, "portions": 1}}
	require.EqualValues(t, 2, p.Plates[1]["slot_id"])
	prop, _ := chooseWith(t, in, submit(t, "tu_1", p), submit(t, "tu_2", p), submit(t, "tu_3", p))
	assert.Contains(t, prop.Notes, "has no main (only Overnight oats)")

	// The same plate in the day's first slot (a light meal) is fine.
	in = run4Input()
	in.State.SlotOrder = plate.SlotOrder{2: 0, 1: 1, 3: 2}
	p = validWeek(t, in)
	p.Plates[1]["components"] = []map[string]any{{"food_id": 117, "portions": 1}}
	prop, _ = chooseWith(t, in, submit(t, "tu_1", p), submit(t, "tu_2", p), submit(t, "tu_3", p))
	assert.NotContains(t, prop.Notes, "has no main (only Overnight oats)")
}

// Calories: a day far under its (scaled) target triggers one repair with the
// gap and the changeable meals; what stays is named in the run notes.
func TestLLMChooser_KcalGapTriggersOneRepairAndIsNoted(t *testing.T) {
	in := run4Input()
	k := 2200.0
	in.State.Profile.KcalTarget = &k
	p := validWeek(t, in)

	prop, reqs := chooseWith(t, in, submit(t, "tu_1", p), submit(t, "tu_2", p), submit(t, "tu_3", p))

	require.Len(t, reqs, 2, "no violation: only the kcal repair, once")
	fb := feedbackOf(t, reqs[1])
	assert.Contains(t, fb, "kcal (no rule, but fix it): Sunday 2026-11-22 has ")
	assert.Contains(t, fb, "against the target of 2200")
	assert.Contains(t, fb, "Meals you can change: Sunday 2026-11-22 lunch (slot_id 2)")
	assert.Contains(t, fb, "up to 1.5 portions per eater")
	// Saturday has a No-plate lunch: its target is scaled, never 2200.
	assert.Contains(t, fb, "Saturday 2026-11-21 has ")
	assert.Contains(t, fb, "the target of 2200 scaled to 2 of 3 meals")
	assert.Contains(t, prop.Notes, "Sun is ")
	assert.Contains(t, prop.Notes, "kcal under target")
	assert.NotContains(t, prop.Notes, "Warning, not resolved")
}

func TestLLMChooser_KcalOnTargetNeedsNoRepair(t *testing.T) {
	in := run4Input()
	k := 750.0
	in.State.Profile.KcalTarget = &k
	p := validWeek(t, in)
	prop, reqs := chooseWith(t, in, submit(t, "tu_1", p))
	require.Len(t, reqs, 1)
	assert.NotContains(t, prop.Notes, "kcal")
}

// Meat days (H5): leftovers of an omnivore batch no longer push a week past
// meat_days_per_week_max.
func TestHeuristic_MeatDaysCountLeftovers(t *testing.T) {
	s := run4State()
	for _, id := range []int64{107, 113, 119, 123} {
		f := s.Foods[id]
		f.DietClass = diet.Omnivore
		s.Foods[id] = f
	}
	limit := 2
	s.Profile.MeatDaysPerWeekMax = &limit
	in := planning.ChooseInput{State: s, Target: fillCells(s), Locale: "en"}
	prop, err := planning.Heuristic{}.Choose(t.Context(), in)
	require.NoError(t, err)
	for _, is := range planning.Check(planning.Merge(in.State, in.Target, prop), in.Target) {
		assert.NotEqual(t, "H5", is.Rule, "%+v", is)
	}
}
