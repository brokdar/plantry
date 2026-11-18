package planning_test

import (
	"encoding/json"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/nutrition"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

func optionsOf(t *testing.T, s planning.State, rule string) []planning.Option {
	t.Helper()
	tgt := windowTarget(s)
	ev := planning.Evaluate(s, tgt)
	for _, is := range planning.IssueOptions(s, tgt, ev, ev.Issues, nil) {
		if is.Rule == rule {
			return is.Options
		}
	}
	t.Fatalf("no %s issue in %v", rule, ev.Issues)
	return nil
}

// An S6 carries verified options: a smaller portion, a lighter swap; each
// has a runnable op and, applied, leaves the plate under the cap.
func TestIssueOptions_S6(t *testing.T) {
	s := altState()
	s.Profile.KcalTarget = f64(2700) // cap 675: the 780 kcal lunch is 105 over
	s.Light = []planning.LightIntent{{Date: sat, Scope: "meal", Slot: "lunch"}}
	ev := planning.Evaluate(s, windowTarget(s))
	for _, p := range ev.Plates {
		t.Log(p.Date, p.SlotID, p.Kcal, p.Intent, p.MaxKcal)
	}
	opts := optionsOf(t, s, "S6")
	require.NotEmpty(t, opts)
	assert.Equal(t, "portions", opts[0].Action, "the least invasive option first")
	assert.Equal(t, 0.75, *opts[0].Portions)
	assert.Equal(t, "set_quantity", opts[0].Op.Op)
	for _, o := range opts {
		b, _ := json.Marshal(o)
		t.Log(string(b))
		assert.NotEmpty(t, o.Op.Op)
		assert.LessOrEqual(t, o.PlateKcal, 675.0, o.Action)
		assert.Equal(t, sat, o.Date)
	}
}

// An S6b carries heartier options only (no removal).
func TestIssueOptions_S6b(t *testing.T) {
	s := altState()
	s.Profile.KcalTarget = f64(2000) // hearty minimum 833
	s.Light = []planning.LightIntent{{Date: sun, Scope: "meal", Slot: "lunch", Level: "hearty"}}
	opts := optionsOf(t, s, "S6b")
	require.NotEmpty(t, opts)
	for _, o := range opts {
		b, _ := json.Marshal(o)
		t.Log(string(b))
		assert.NotEqual(t, "remove", o.Action)
		assert.GreaterOrEqual(t, o.PlateKcal, 833.0, o.Action)
	}
}

// H2: a day over the cook limit offers a verified way out: the batch cooked
// on it moves to an earlier cell.
func TestIssueOptions_H2(t *testing.T) {
	s := baseState()
	s.SlotNames = map[int64]string{breakfast: "slot.breakfast", lunch: "slot.lunch", dinner: "slot.dinner"}
	s.Settings.MaxCookOccasionsPerDay = 1
	s.Sessions = []planning.Session{
		persistedSession(1, curry, sun, lunch, 1, false),
		persistedSession(2, soup, sun, dinner, 1, false),
	}
	s.Plates = []planning.Plate{persisted(10, sun, lunch, false, curry, 1), persisted(11, sun, dinner, false, soup, 2)}
	for i := range s.Plates {
		s.Plates[i].Components[0].ID, s.Plates[i].Components[0].Position = int64(100+i), plate.PositionMain
	}
	opts := optionsOf(t, s, "H2")
	require.NotEmpty(t, opts)
	assert.Equal(t, "move_cook", opts[0].Action)
	for _, o := range opts {
		assert.LessOrEqual(t, o.CookOccasions, 1)
		assert.NotEmpty(t, o.Op.Op)
	}
	assert.Equal(t, "move_cook", opts[0].Op.Op)
	assert.NotNil(t, opts[0].Op.SessionID)
	assert.NotNil(t, opts[0].Op.CookAt)
}

// A far-over-cap plate (main plus composed side plus add-on) gets a
// whole-meal option: no single swap or portion change reaches the cap, but
// replacing the main and dropping the rest does.
func TestIssueOptions_S6WholeMeal(t *testing.T) {
	s := altState()
	s.Profile.KcalTarget = f64(2000) // cap about 500
	s.Light = []planning.LightIntent{{Date: sat, Scope: "meal", Slot: "lunch"}}
	opts := optionsOf(t, s, "S6")
	var meal *planning.Option
	for i, o := range opts {
		b, _ := json.Marshal(o)
		t.Log(string(b))
		if len(o.Ops) > 1 {
			meal = &opts[i]
		}
	}
	require.NotNil(t, meal, "a multi-op option replaces the main and drops the sides")
	assert.Equal(t, "swap", meal.Action)
	assert.Equal(t, sat, meal.Date)
}

// An H9 floor blocks an S6 option: a lighter plate that would take the day
// under the floor (75% of the 2000 kcal target, 1500) is not offered, though
// the day (1780) is above it now.
func TestIssueOptions_S6FloorBlocksOption(t *testing.T) {
	s := altState()
	s.Profile.KcalTarget = f64(2000) // cap 500, floor 1500
	s.Foods[900] = planning.FoodFacts{
		ID: 900, Name: "Muesli", Kind: food.KindLeaf, LeafRole: food.RoleMain, DietClass: "vegan", NoCook: true,
		Macros: nutrition.Macros{Kcal: 400, Protein: 10}, NutritionComplete: true,
	}
	// A breakfast makes Saturday a complete day: 1780 kcal, 280 above the
	// floor. The soup swap (-380) would take it to 1400.
	s.Plates = append(s.Plates, planning.Plate{
		ID: 90, Date: day(sat), SlotID: breakfast, Eaters: 1,
		Components: []planning.Component{{ID: 900, FoodID: 900, Portions: f64(1)}},
	})
	s.Light = []planning.LightIntent{{Date: sat, Scope: "meal", Slot: "lunch"}}
	tgt := windowTarget(s)
	ev := planning.Evaluate(s, tgt)
	require.Empty(t, codeDates(ev.Issues, "H9", planning.CodeKcalBelowFloor), "the day starts above the floor")
	found := false
	for _, is := range planning.IssueOptions(s, tgt, ev, ev.Issues, nil) {
		if is.Rule == "S6" {
			found = true
			assert.Empty(t, is.Options, "every option would put the day under the floor")
		}
	}
	require.True(t, found, "the S6 stays open")
}

// Options are computed for at most MaxOptionIssues issues, the earliest, and
// only those keep accepts.
func TestIssueOptions_Cap(t *testing.T) {
	s := altState()
	tgt := windowTarget(s)
	ev := planning.Evaluate(s, tgt)
	var issues []planning.Issue
	for i := 0; i < planning.MaxOptionIssues+3; i++ {
		is := planning.Issue{Rule: "H2", Code: planning.CodeTooManyCookOccasions, Date: sat, Params: map[string]any{}}
		is.Date = time.Date(2026, 1, 1+i, 0, 0, 0, 0, time.UTC).Format(time.DateOnly)
		issues = append(issues, is)
	}
	slices.Reverse(issues)
	countOpts := func(out []planning.Issue) (n int, last string) {
		for _, is := range out {
			if is.Options != nil {
				n++
				if is.Date > last {
					last = is.Date
				}
			}
		}
		return
	}
	n, last := countOpts(planning.IssueOptions(s, tgt, ev, issues, nil))
	assert.Equal(t, planning.MaxOptionIssues, n)
	assert.Equal(t, "2026-01-05", last, "the earliest by date")
	n, _ = countOpts(planning.IssueOptions(s, tgt, ev, issues, func(is planning.Issue) bool { return is.Date == "2026-01-08" }))
	assert.Equal(t, 1, n)
}
