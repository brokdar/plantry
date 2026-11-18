package agent_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/agent"
	"github.com/jaltszeimer/plantry/backend/internal/domain/availability"
	"github.com/jaltszeimer/plantry/backend/internal/domain/diet"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/nutrition"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
	"github.com/jaltszeimer/plantry/backend/internal/domain/slot"
	"github.com/jaltszeimer/plantry/backend/internal/domain/window"
)

// realisticWeek is a week like the dev data: 22 recipes (16 mains, 2 starch
// and 2 vegetable sides, 2 standalones), 21 cells with an oats routine at
// breakfast every day and Saturday dinner left out, 20 plates, 22 cook
// sessions (7 oats and 9 sides or lunches cooked and eaten on their own cell,
// 6 dinner batches that feed the next lunch too), lunch and dinner add-ons.
func realisticWeek() (planning.State, *profile.Profile) {
	start := time.Date(2026, 11, 21, 0, 0, 0, 0, time.UTC)
	const breakfast, lunch, dinner = int64(1), int64(2), int64(3)
	st := planning.State{
		Window: window.Window{Start: start, End: start.AddDate(0, 0, 6)}, Today: start.AddDate(0, 0, -4),
		SlotOrder: plate.SlotOrder{breakfast: 0, lunch: 1, dinner: 2},
		SlotNames: map[int64]string{breakfast: "slot.breakfast", lunch: "slot.lunch", dinner: "slot.dinner"},
		SlotKind:  map[int64]slot.Kind{breakfast: slot.KindLight},
		Foods:     map[int64]planning.FoodFacts{},
		Settings: planning.Settings{
			BatchSettings: plate.BatchSettings{DefaultKeepDays: 3, FreezerKeepDays: 90}, MaxCookOccasionsPerDay: 1,
			VarietyWeeks: 3, BatchCooking: true,
		},
	}
	kcal := 2200.0
	prof := &profile.Profile{KcalTarget: &kcal, HouseholdPortions: 1, Locale: "en"}
	st.Profile = planning.StateProfile(prof)

	names := []string{
		"Rindergulasch mit Spätzle", "Linsen-Bolognese", "Hähnchen-Gemüse-Curry", "Lachs mit Zitronenbutter",
		"Kichererbsen-Spinat-Pfanne", "Tofu-Gemüse-Pfanne", "Chili sin Carne", "Ofenkürbis mit Feta",
		"Spaghetti Aglio e Olio", "Gebratener Reis mit Ei", "Gemüselasagne", "Kartoffel-Lauch-Suppe",
		"Shakshuka", "Pilzrisotto", "Rinderragù", "Dal mit Kokosmilch",
	}
	id := int64(100)
	add := func(name string, role food.Role, class diet.Class, kcal float64, minutes int) int64 {
		id++
		st.Foods[id] = planning.FoodFacts{
			ID: id, Name: name, Kind: food.KindComposed, Role: role, DietClass: class, NutritionComplete: true,
			Macros: nutrition.Macros{Kcal: kcal, Protein: kcal / 25}, Minutes: minutes,
		}
		return id
	}
	var mains []int64
	for i, n := range names {
		mains = append(mains, add(n, food.RoleMain, diet.Omnivore, 480+float64(i%5)*55, 25+i*5))
	}
	rice := add("Basmatireis", food.RoleSideStarch, diet.Vegan, 210, 20)
	add("Salzkartoffeln", food.RoleSideStarch, diet.Vegan, 160, 25)
	salad := add("Gurken-Tomaten-Salat", food.RoleSideVeg, diet.Vegan, 90, 10)
	add("Rosmarinkartoffeln", food.RoleSideVeg, diet.Vegan, 230, 40)
	oats := add("Overnight Oats", food.RoleStandalone, diet.Vegetarian, 380, 5)
	add("Naturjoghurt mit Beeren", food.RoleStandalone, diet.Vegetarian, 190, 5)
	yoghurt := id + 1
	st.Foods[yoghurt] = planning.FoodFacts{
		ID: yoghurt, Name: "Griechischer Joghurt", Kind: food.KindLeaf, DietClass: diet.Vegetarian, NutritionComplete: true,
		Macros: nutrition.Macros{Kcal: 97, Protein: 9}, LeafRole: food.RoleSideProtein,
	}

	sess := int64(0)
	cookSession := func(foodID int64, d int, slotID int64, portions float64) int64 {
		sess++
		st.Sessions = append(st.Sessions, planning.Session{CookSession: plate.CookSession{
			ID: sess, FoodID: foodID, CookDate: start.AddDate(0, 0, d), CookSlotID: slotID, Portions: portions,
		}})
		return sess
	}
	pid, cid := int64(0), int64(0)
	one := func(v float64) *float64 { return &v }
	addPlate := func(d int, slotID int64, comps ...planning.Component) {
		pid++
		for i := range comps {
			cid++
			comps[i].ID = cid
		}
		st.Plates = append(st.Plates, planning.Plate{ID: pid, Date: start.AddDate(0, 0, d), SlotID: slotID, Eaters: 1, Components: comps})
	}
	for d := range 7 {
		for _, s := range []int64{breakfast, lunch, dinner} {
			mode := availability.ModePlan
			switch {
			case s == breakfast:
				mode = availability.ModeRoutine
			case s == dinner && d == 0:
				mode = availability.ModeNone
			}
			st.Cells = append(st.Cells, planning.Cell{Date: start.AddDate(0, 0, d), SlotID: s, Mode: mode})
		}
		sid := cookSession(oats, d, breakfast, 1)
		addPlate(d, breakfast, planning.Component{FoodID: oats, Portions: one(1), SessionID: sid, Position: plate.PositionMain})
	}
	dinnerSession := map[int]int64{}
	for d := 1; d <= 6; d++ {
		dinnerSession[d] = cookSession(mains[d], d, dinner, 2)
		side := rice
		if d%2 == 0 {
			side = salad
		}
		sideSess := cookSession(side, d, dinner, 1)
		addPlate(d, dinner,
			planning.Component{FoodID: mains[d], Portions: one(1), SessionID: dinnerSession[d], Position: plate.PositionMain},
			planning.Component{FoodID: side, Portions: one(1), SessionID: sideSess, Position: plate.PositionSide})
	}
	for d := range 7 {
		var main planning.Component
		switch {
		case d <= 2:
			sid := cookSession(mains[7+d], d, lunch, 1)
			main = planning.Component{FoodID: mains[7+d], Portions: one(1), SessionID: sid, Position: plate.PositionMain}
		default:
			main = planning.Component{FoodID: mains[d-1], Portions: one(1), SessionID: dinnerSession[d-1], Position: plate.PositionMain}
		}
		comps := []planning.Component{main}
		if d%2 == 1 {
			g := 150.0
			comps = append(comps, planning.Component{FoodID: yoghurt, Grams: &g, Position: plate.PositionExtra})
		}
		addPlate(d, lunch, comps...)
	}
	st.Light = []planning.LightIntent{{Date: start.AddDate(0, 0, 3).Format(time.DateOnly), Scope: "meal", Slot: "dinner"}}
	return st, prof
}

// The chat prompt of a realistic week fits the budget with room to spare: v17
// cut it ("[prompt truncated]") and with it the facts of the later foods and
// the per-day table (audit B1). Run with -v for the byte counts.
func TestChatPrompt_RealisticWeekFitsWithRoomToSpare(t *testing.T) {
	st, prof := realisticWeek()
	require.Len(t, st.Plates, 20)
	require.Len(t, st.Sessions, 22)

	facts := agent.ChatFacts(st)
	summary := planning.RenderPlanSummary(st, agent.ChatIssues(st))
	prompt, cut := agent.ComposePrompt(prof, facts, summary, st.Today)
	t.Logf("chat prompt: %d chars (budget %d): facts %d, summary %d, rest %d", len(prompt), agent.PromptCharBudget, len(facts), len(summary), len(prompt)-len(facts)-len(summary))

	assert.Zero(t, cut, "nothing is cut")
	assert.NotContains(t, prompt, "[prompt truncated]")
	assert.LessOrEqual(t, float64(len(prompt)), 0.85*float64(agent.PromptCharBudget), "at least 15%% under the budget")

	// The facts come before the cells, and every food and day is in them.
	assert.Less(t, strings.Index(prompt, "Foods on the plan"), strings.Index(prompt, "Cells (date"))
	assert.Less(t, strings.Index(prompt, "Kcal per eater and day"), strings.Index(prompt, "Cells (date"))
	assert.Contains(t, prompt, "food_id 110 Gebratener Reis mit Ei", "the later foods are not cut")
	for d := range 7 {
		assert.Contains(t, prompt, fmt.Sprintf("%s: ", st.Window.Start.AddDate(0, 0, d).Format(time.DateOnly)))
	}
	// One-portion sessions on their own cell add nothing to the cell line.
	assert.NotContains(t, prompt, "session_id 1 | food_id")
	assert.Contains(t, prompt, "Cook sessions:")
	// The stored light mark, the routine and the day numbers are there.
	assert.Contains(t, prompt, "plan, light meal")
	assert.Contains(t, prompt, "routine, not cooking")
	assert.Contains(t, prompt, "| band ")
	assert.Contains(t, prompt, "light dinner")
	assert.Contains(t, prompt, "| vegan |", "diet class in the foods list")
}
