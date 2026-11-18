package planning_test

import (
	"encoding/json"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/availability"
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

// H2 options never cook on a cell marked No plate, however near it is.
func TestIssueOptions_H2SkipsNoneCells(t *testing.T) {
	build := func(none ...plateCellKey) planning.State {
		s := baseState()
		s.Settings.MaxCookOccasionsPerDay = 1
		s.Sessions = []planning.Session{
			persistedSession(1, curry, sun, lunch, 1, false),
			persistedSession(2, soup, sun, dinner, 1, false),
		}
		s.Plates = []planning.Plate{persisted(10, sun, lunch, false, curry, 1), persisted(11, sun, dinner, false, soup, 2)}
		for i := range s.Plates {
			s.Plates[i].Components[0].ID, s.Plates[i].Components[0].Position = int64(100+i), plate.PositionMain
		}
		for _, c := range none {
			setMode(&s, c, availability.ModeNone)
		}
		return s
	}
	free := optionsOf(t, build(), "H2")
	require.NotEmpty(t, free)
	for _, tc := range []struct {
		name string
		none []plateCellKey
	}{
		{"first option's cell", []plateCellKey{{Date: day(sun), SlotID: free[0].SlotID}}},
		{"the cell of every move", func() []plateCellKey {
			var out []plateCellKey
			for _, o := range free {
				if o.Action == "move_cook" {
					d, err := time.Parse(time.DateOnly, o.Date)
					require.NoError(t, err)
					out = append(out, plateCellKey{Date: d, SlotID: o.SlotID})
				}
			}
			return out
		}()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := build(tc.none...)
			tgt := planning.WindowTarget(s)
			ev := planning.Evaluate(s, tgt)
			for _, is := range planning.IssueOptions(s, tgt, ev, ev.Issues, nil) {
				for _, o := range is.Options {
					if o.Action != "move_cook" {
						continue
					}
					for _, n := range tc.none {
						assert.False(t, o.Date == n.Date.Format(time.DateOnly) && o.SlotID == n.SlotID, "option %+v cooks on a none cell", o)
					}
				}
			}
		})
	}
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

// h2DayState is a Sunday over the cook limit (one occasion) with every other
// day marked No plate, so the only way out is within the day.
func h2DayState(sessions []planning.Session, plates []planning.Plate) planning.State {
	s := baseState()
	s.Sessions, s.Plates = sessions, plates
	for i := range s.Plates {
		for j := range s.Plates[i].Components {
			s.Plates[i].Components[j].ID, s.Plates[i].Components[j].Position = int64(100+10*i+j), plate.PositionMain
		}
	}
	for _, c := range s.Cells {
		if c.Key().Date.Format(time.DateOnly) != sun {
			setMode(&s, c.Key(), availability.ModeNone)
		}
	}
	return s
}

// moved applies move_cook ops to s.
func moved(t *testing.T, s planning.State, ops []planning.OptionOp) planning.State {
	t.Helper()
	s.Sessions = slices.Clone(s.Sessions)
	for _, op := range ops {
		require.Equal(t, "move_cook", op.Op)
		d, err := time.Parse(time.DateOnly, op.CookAt.Date)
		require.NoError(t, err)
		for i := range s.Sessions {
			if s.Sessions[i].ID == *op.SessionID {
				s.Sessions[i].CookDate, s.Sessions[i].CookSlotID = d, op.CookAt.SlotID
			}
		}
	}
	return s
}

// H2 on a day whose other days are away: the way out is a cook cell of the
// day itself, one batch alone or all of the day's batches together.
func TestIssueOptions_H2SameDay(t *testing.T) {
	freshPlate := persisted(12, sun, breakfast, false, chili, 0)
	for _, tc := range []struct {
		name string
		s    planning.State
		ops  int // ops of the first option
		cell planning.OptionCell
	}{
		{
			// Breakfast cooks the curry, lunch the soup: the soup moves to
			// the breakfast cell (or the curry to lunch).
			name: "one batch to the cook cell of the day",
			s: h2DayState(
				[]planning.Session{persistedSession(1, curry, sun, breakfast, 1, false), persistedSession(2, soup, sun, lunch, 1, false)},
				[]planning.Plate{persisted(10, sun, breakfast, false, curry, 1), persisted(11, sun, lunch, false, soup, 2)}),
			ops: 1, cell: planning.OptionCell{Date: sun, SlotID: breakfast},
		},
		{
			// Breakfast cooks fresh, lunch and dinner each cook a batch: no
			// single move leaves fewer than two occasions, both together
			// into the breakfast cell leave one.
			name: "all batches into one cell",
			s: h2DayState(
				[]planning.Session{persistedSession(1, curry, sun, lunch, 1, false), persistedSession(2, soup, sun, dinner, 1, false)},
				[]planning.Plate{freshPlate, persisted(10, sun, lunch, false, curry, 1), persisted(11, sun, dinner, false, soup, 2)}),
			ops: 2, cell: planning.OptionCell{Date: sun, SlotID: breakfast},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tgt := windowTarget(tc.s)
			require.Contains(t, codes(planning.Check(tc.s, tgt), "H2"), planning.CodeTooManyCookOccasions)
			opts := optionsOf(t, tc.s, "H2")
			require.NotEmpty(t, opts)
			first := opts[0]
			assert.Equal(t, "move_cook", first.Action)
			ops := first.Ops
			if len(ops) == 0 {
				ops = []planning.OptionOp{first.Op}
			}
			require.Len(t, ops, tc.ops)
			assert.Equal(t, ops[0], first.Op, "op is the first of ops")
			for _, op := range ops {
				assert.Equal(t, &tc.cell, op.CookAt)
			}
			assert.Equal(t, 1, first.CookOccasions)
			for _, o := range opts {
				ops := o.Ops
				if len(ops) == 0 {
					ops = []planning.OptionOp{o.Op}
				}
				assert.NotContains(t, codes(planning.Check(moved(t, tc.s, ops), tgt), "H2"), planning.CodeTooManyCookOccasions, "option %+v leaves H2", o)
			}
		})
	}
}
