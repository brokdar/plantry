package planning_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/fake"
	"github.com/jaltszeimer/plantry/backend/internal/domain/availability"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
	"github.com/jaltszeimer/plantry/backend/internal/domain/nutrition"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
	"github.com/jaltszeimer/plantry/backend/internal/domain/window"
)

// chooseWith runs the LLM chooser over a scripted fake and returns the
// proposal and the requests the model saw.
func chooseWith(t *testing.T, in planning.ChooseInput, turns ...fake.Turn) (planning.Proposal, []llm.Request) {
	t.Helper()
	client := fake.NewFromScript(&fake.Script{Turns: turns})
	c := planning.NewLLMChooser(llm.StaticResolver(client, "fake-model"))
	prop, err := c.Choose(context.Background(), in)
	require.NoError(t, err)
	return prop, client.Requests()
}

func feedbackOf(t *testing.T, req llm.Request) string { return toolResult(t, req) }

// run4State is the live walk-through's window (Sat 2026-11-21 .. Fri
// 2026-11-27): a batch cooker with Saturday shopping, a routine breakfast,
// keep days 3, one cook occasion per day, freezing on; the recipes are the
// walk-through's candidates that run 4 used.
func run4State() planning.State {
	w := window.Window{Start: day("2026-11-21"), End: day("2026-11-27")}
	rec := func(id int64, name string, role food.Role, kcal float64, keep int, freezable bool) planning.FoodFacts {
		f := recipe(id, name, role, "vegan", kcal)
		f.KeepDays, f.Freezable = &keep, freezable
		f.Macros = nutrition.Macros{Kcal: kcal, Protein: kcal / 20, Fat: kcal / 45, Carbs: kcal / 8}
		return f
	}
	foods := map[int64]planning.FoodFacts{}
	for _, f := range []planning.FoodFacts{
		rec(106, "Chili sin carne", food.RoleMain, 729, 4, true),
		rec(107, "Speedy lentil coconut curry", food.RoleMain, 316, 3, true),
		rec(111, "Linsen-Bolognese", food.RoleMain, 389, 4, true),
		rec(113, "Lemon salmon with rice", food.RoleMain, 455, 1, false),
		rec(119, "Einfaches Kuerbisgulasch", food.RoleMain, 290, 1, false),
		rec(122, "Spinach lasagne", food.RoleMain, 556, 4, true),
		rec(123, "Tofu stir-fry", food.RoleMain, 345, 2, false),
		rec(124, "Steamed rice", food.RoleSideStarch, 262, 3, false),
		rec(125, "Green salad", food.RoleSideVeg, 92, 1, false),
		rec(117, "Overnight oats", food.RoleStandalone, 306, 2, false),
	} {
		foods[f.ID] = f
	}
	s := planning.State{
		Window: w, Today: day("2026-11-20"),
		SlotOrder: plate.SlotOrder{1: 0, 2: 1, 3: 2},
		SlotNames: map[int64]string{1: "slot.breakfast", 2: "slot.lunch", 3: "slot.dinner"},
		Foods:     foods,
		Settings: planning.Settings{
			BatchSettings:          plate.BatchSettings{DefaultKeepDays: 3, FreezingEnabled: true, FreezerKeepDays: 90},
			MaxCookOccasionsPerDay: 1, VarietyWeeks: 3, BatchCooking: true,
		},
		Profile: planning.Profile{DietType: profile.DietOmnivore, HouseholdPortions: 1},
	}
	for _, d := range w.Days() {
		ds := d.Format(time.DateOnly)
		for slot := int64(1); slot <= 3; slot++ {
			mode := availability.ModePlan
			switch {
			case slot == 1:
				mode = availability.ModeRoutine
			case ds == "2026-11-21" && slot == 2, ds == "2026-11-26" && slot == 3:
				mode = availability.ModeNone
			}
			s.Cells = append(s.Cells, planning.Cell{Date: d, SlotID: slot, Mode: mode})
		}
	}
	return s
}

func fillCells(s planning.State) planning.Target {
	t := planning.Target{}
	for _, c := range s.Cells {
		if c.Mode == availability.ModePlan {
			t.Cells = append(t.Cells, c.Key())
		}
	}
	return t
}

func run4Input() planning.ChooseInput {
	s := run4State()
	return planning.ChooseInput{State: s, Target: fillCells(s), EatersByDate: map[string]float64{"2026-11-27": 3}, Locale: "en"}
}

// validWeek is a plan without violations for run4Input: a distinct main a
// day cooked on the day's first fill cell and eaten there and at the
// day's other plan cell.
func validWeek(t *testing.T, in planning.ChooseInput) *jsonPlan {
	t.Helper()
	mains := []int64{106, 107, 111, 122, 123, 113, 119}
	p := &jsonPlan{Notes: "One different main a day."}
	for i, d := range in.State.Window.Days() {
		ds := d.Format(time.DateOnly)
		var slots []int64
		for _, c := range in.Target.Cells {
			if c.Date.Equal(d) {
				slots = append(slots, c.SlotID)
			}
		}
		eaters := in.EatersByDate[ds]
		if eaters == 0 {
			eaters = 1
		}
		key := fmt.Sprintf("s%d", i)
		p.session(key, mains[i], ds, slots[0], eaters*float64(len(slots)))
		for _, sl := range slots {
			p.plate(ds, sl, mains[i], key)
		}
	}
	return p
}

// Replays run 4's first reply (stored in the walk-through database): the
// feedback now carries the rule, the numbers, the cells and a fix for each
// problem, and a corrected plan ends the run clean.
func TestLLMChooser_Run4FeedbackAndRepair(t *testing.T) {
	raw, err := os.ReadFile("testdata/run4_attempt0.json")
	require.NoError(t, err)
	in := run4Input()

	prop, reqs := chooseWith(t, in, submitRaw("tu_1", raw), submit(t, "tu_2", validWeek(t, in)))

	require.Len(t, reqs, 2)
	assert.Equal(t, 1, prop.RepairAttempts)
	assert.Empty(t, violationsOfProposal(in, prop))
	fb := feedbackOf(t, reqs[1])

	// The curry session grows by itself (freezable, freezing on): visible.
	assert.Contains(t, fb, "Already corrected for you")
	assert.Contains(t, fb, "Speedy lentil coconut curry: portions raised from 4 to 5")
	assert.Contains(t, fb, "frozen portions from 0 to 1")
	// H2 names the cells and what cooks there.
	assert.Contains(t, fb, "H2: Sunday 2026-11-22 has 2 cooking occasions, at most 1 are allowed")
	assert.Contains(t, fb, "Sunday 2026-11-22 lunch (slot_id 2): s2 (Green salad, food_id 125)")
	assert.Contains(t, fb, "Steamed rice without a session (cooked fresh)")
	// The salad session: eaters x portions = consumption, keep days, no freezer.
	assert.Contains(t, fb, "H3/H4 s2 (Green salad, food_id 125): cooked Sunday 2026-11-22 lunch (slot_id 2) with portions 2")
	assert.Contains(t, fb, "Friday 2026-11-27 lunch (slot_id 2): 3 x 3 = 9 (frozen: after the keep days)")
	assert.Contains(t, fb, "Keep days 1: plates up to 2026-11-23")
	assert.Contains(t, fb, "Green salad is not freezable")
	assert.Contains(t, fb, "portions must be at least")
	// Lunch/dinner plates without a main.
	assert.Contains(t, fb, "Wednesday 2026-11-25 lunch (slot_id 2) has no main (only Green salad)")
	assert.Contains(t, fb, "Reminder: portions on a plate component are per eater")
}

func violationsOfProposal(in planning.ChooseInput, p planning.Proposal) []planning.Issue {
	var out []planning.Issue
	for _, is := range planning.Check(planning.Merge(in.State, in.Target, p), in.Target) {
		if is.Severity == planning.SeverityViolation {
			out = append(out, is)
		}
	}
	return out
}

// An identical reply is called out in the next repair.
func TestLLMChooser_RepeatedProposalIsNamed(t *testing.T) {
	in := run4Input()
	p := validWeek(t, in)
	// Two cooks on Sunday: a violation the model keeps resubmitting.
	p.session("extra", 125, "2026-11-22", 3, 1)
	raw, err := json.Marshal(p)
	require.NoError(t, err)

	_, reqs := chooseWith(t, in, submitRaw("tu_1", raw), submitRaw("tu_2", raw), submit(t, "tu_3", validWeek(t, in)))

	require.Len(t, reqs, 3)
	assert.NotContains(t, feedbackOf(t, reqs[1]), "identical")
	assert.Contains(t, feedbackOf(t, reqs[2]), "Your last reply is identical to the one before it")
}

// The autofix grows a freezable session to its plates and says so in the
// attempt record; nothing is sent back.
func TestAutofix_GrowsFreezableSession(t *testing.T) {
	in := run4Input()
	p := validWeek(t, in)
	// Thursday's lunch eats the Sunday curry (s1, keep 3): 4 days later.
	for _, pl := range p.Plates {
		if pl["date"] == "2026-11-26" {
			pl["components"] = []map[string]any{{"food_id": 107, "portions": 1, "session_key": "s1"}}
		}
	}
	p.Sessions = append(p.Sessions[:5], p.Sessions[6:]...) // Thursday's own session goes
	raw, err := json.Marshal(p)
	require.NoError(t, err)

	prop, reqs := chooseWith(t, in, submitRaw("tu_1", raw))

	if len(reqs) > 1 {
		t.Fatal(feedbackOf(t, reqs[1]))
	}
	require.Len(t, reqs, 1, "no repair needed")
	assert.Empty(t, violationsOfProposal(in, prop))
	var s1 planning.ProposedSession
	for _, s := range prop.Sessions {
		if s.Key == "s1" {
			s1 = s
		}
	}
	assert.Equal(t, 3.0, s1.Portions, "2 on Sunday plus Thursday's leftover")
	assert.Contains(t, prop.Notes, "Speedy lentil coconut curry: portions raised from 2 to 3")
	assert.Equal(t, 1.0, s1.FrozenPortions)
}

// Table of single findings: the feedback line for each.
func TestRepairFeedback_Table(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(in *planning.ChooseInput, p *jsonPlan)
		want   []string
	}{
		{
			name: "leftover past keep days without a freezer",
			mutate: func(in *planning.ChooseInput, p *jsonPlan) {
				in.State.Settings.FreezingEnabled = false
				for _, pl := range p.Plates {
					if pl["date"] == "2026-11-26" {
						pl["components"] = []map[string]any{{"food_id": 107, "portions": 1, "session_key": "s1"}}
					}
				}
				p.Sessions = append(p.Sessions[:5], p.Sessions[6:]...)
			},
			want: []string{"freezing is off, so plates after 2026-11-25 cannot eat from it (2026-11-26)", "Keep days 3: plates up to 2026-11-25"},
		},
		{
			name: "diet",
			mutate: func(in *planning.ChooseInput, p *jsonPlan) {
				in.State.Profile.DietType = profile.DietVegan
				f := in.State.Foods[106]
				f.DietClass = "omnivore"
				in.State.Foods[106] = f
			},
			want: []string{"H5: ", "Chili sin carne (diet class omnivore) but the diet type is vegan"},
		},
		{
			name: "plate without a main",
			mutate: func(in *planning.ChooseInput, p *jsonPlan) {
				p.Plates[1]["components"] = []map[string]any{{"food_id": 124, "portions": 1}}
			},
			want: []string{"has no main (only Steamed rice)"},
		},
		{
			name: "day far under the kcal target (info on a repair that happens anyway)",
			mutate: func(in *planning.ChooseInput, p *jsonPlan) {
				k := 2200.0
				in.State.Profile.KcalTarget = &k
				p.Plates[1]["components"] = []map[string]any{{"food_id": 124, "portions": 1}} // forces the repair
			},
			want: []string{"kcal per person against the target of 2200", "Fix: use the per-portion kcal"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := run4Input()
			p := validWeek(t, in)
			tt.mutate(&in, p)
			// A fresh state per case: foods map was shared by run4State only per call.
			_, reqs := chooseWith(t, in, submit(t, "tu_1", p), submit(t, "tu_2", p), submit(t, "tu_3", p))
			require.GreaterOrEqual(t, len(reqs), 2)
			fb := feedbackOf(t, reqs[1])
			for _, w := range tt.want {
				assert.Contains(t, fb, w)
			}
		})
	}
}

// Run 4's mistake: 3 eaters with portions 3 each (eaters used as portions)
// against a session of 3. It is no plausible arithmetic gap, so Go does not
// grow the session to 18 (shopping would buy it); the model is told.
func TestAutofix_EatersAsPortionsIsSentBack(t *testing.T) {
	in := run4Input()
	p := validWeek(t, in)
	for _, pl := range p.Plates {
		if pl["date"] == "2026-11-27" {
			pl["components"] = []map[string]any{{"food_id": 119, "portions": 3, "session_key": "s6"}}
		}
	}
	for _, s := range p.Sessions {
		if s["key"] == "s6" {
			s["portions"] = 3.0
		}
	}

	prop, reqs := chooseWith(t, in, submit(t, "tu_1", p), submit(t, "tu_2", p), submit(t, "tu_3", p))

	require.Len(t, reqs, 3, "a violation that is not fixed by Go is repaired by the model")
	fb := feedbackOf(t, reqs[1])
	assert.NotContains(t, fb, "Already corrected")
	assert.Contains(t, fb, "3 x 3 = 9")
	assert.Contains(t, fb, "portions must be at least 18")
	for _, s := range prop.Sessions {
		if s.Key == "s6" {
			assert.Equal(t, 3.0, s.Portions, "not grown")
		}
	}
	assert.NotContains(t, prop.Notes, "raised")
}

// Every autofix is a line of the run notes; the gap must be plausible and the
// links valid (freezing on, freezable, within the keep days).
func TestAutofix_NotesAndLimits(t *testing.T) {
	thursdayLeftover := func(in *planning.ChooseInput, p *jsonPlan) {
		for _, pl := range p.Plates {
			if pl["date"] == "2026-11-26" {
				pl["components"] = []map[string]any{{"food_id": 107, "portions": 1, "session_key": "s1"}}
			}
		}
		p.Sessions = append(p.Sessions[:5], p.Sessions[6:]...)
	}
	tests := []struct {
		name      string
		mutate    func(in *planning.ChooseInput, p *jsonPlan)
		wantFixed bool
	}{
		{name: "freezable with freezing on", mutate: thursdayLeftover, wantFixed: true},
		{name: "freezing off", mutate: func(in *planning.ChooseInput, p *jsonPlan) {
			thursdayLeftover(in, p)
			in.State.Settings.FreezingEnabled = false
		}},
		{name: "not freezable", mutate: func(in *planning.ChooseInput, p *jsonPlan) {
			thursdayLeftover(in, p)
			f := in.State.Foods[107]
			f.Freezable = false
			in.State.Foods[107] = f
		}},
		{name: "beyond the freezer keep days", mutate: func(in *planning.ChooseInput, p *jsonPlan) {
			thursdayLeftover(in, p)
			in.State.Settings.FreezerKeepDays = 2
		}},
		{name: "a plate before the cook cell", mutate: func(in *planning.ChooseInput, p *jsonPlan) {
			// Monday's plate eats Tuesday's session (cooked after it).
			for _, pl := range p.Plates {
				if pl["date"] == "2026-11-23" {
					pl["components"] = []map[string]any{{"food_id": 111, "portions": 1, "session_key": "s2"}}
				}
			}
			p.Sessions = append(p.Sessions[:1], p.Sessions[2:]...)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := run4Input()
			p := validWeek(t, in)
			tt.mutate(&in, p)
			prop, reqs := chooseWith(t, in, submit(t, "tu_1", p), submit(t, "tu_2", p), submit(t, "tu_3", p))
			if tt.wantFixed {
				require.Len(t, reqs, 1)
				assert.Contains(t, prop.Notes, "Speedy lentil coconut curry: portions raised from 2 to 3, frozen portions from 0 to 1 to cover its plates.")
				return
			}
			require.Greater(t, len(reqs), 1, "a violation Go does not fix goes back to the model")
			assert.NotContains(t, prop.Notes, "raised")
		})
	}
}

// A missing main still there after the last repair is a warning in the run
// notes, not a silent ok.
func TestLLMChooser_LeftoverAdvisoryIsWarned(t *testing.T) {
	in := run4Input()
	p := validWeek(t, in)
	p.Plates[1]["components"] = []map[string]any{{"food_id": 124, "portions": 1}}

	prop, reqs := chooseWith(t, in, submit(t, "tu_1", p), submit(t, "tu_2", p), submit(t, "tu_3", p))

	require.Len(t, reqs, 3)
	assert.Contains(t, prop.Notes, "One different main a day.")
	assert.Contains(t, prop.Notes, "Warning, not resolved: Sunday 2026-11-22 lunch (slot_id 2) has no main (only Steamed rice).")
	assert.NotContains(t, prop.Notes, "Fix:")
}

// The main is demanded by the slot's name, not its position: breakfast and
// snack plates are exempt wherever they sit, any other slot (also a custom
// one) needs a main, with 2 or 3 slot names alike.
func TestMissingMain_ByRoleNotPosition(t *testing.T) {
	tests := []struct {
		name     string
		names    map[int64]string
		wantFlag bool
	}{
		{"breakfast is the middle slot", map[int64]string{1: "slot.lunch", 2: "slot.breakfast", 3: "slot.dinner"}, false},
		{"2 slots: breakfast and dinner", map[int64]string{1: "slot.dinner", 2: "slot.breakfast"}, false},
		{"2 slots: lunch and dinner", map[int64]string{1: "slot.dinner", 2: "slot.lunch"}, true},
		{"snack in the middle", map[int64]string{1: "slot.breakfast", 2: "slot.snack", 3: "slot.dinner"}, false},
		{"custom slot name needs a main", map[int64]string{1: "slot.breakfast", 2: "slot.elevenses", 3: "slot.dinner"}, true},
		{"first slot that is no breakfast", map[int64]string{1: "slot.lunch", 2: "slot.dinner", 3: "slot.snack"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := run4Input()
			in.State.SlotNames = tt.names
			p := validWeek(t, in)
			p.Plates[1]["components"] = []map[string]any{{"food_id": 124, "portions": 1}}
			require.EqualValues(t, 2, p.Plates[1]["slot_id"])
			prop, _ := chooseWith(t, in, submit(t, "tu_1", p), submit(t, "tu_2", p), submit(t, "tu_3", p))
			if tt.wantFlag {
				assert.Contains(t, prop.Notes, "has no main (only Steamed rice)")
			} else {
				assert.NotContains(t, prop.Notes, "has no main (only Steamed rice)")
			}
		})
	}
}

// A pathological week lists a capped number of problems.
func TestRepairFeedback_Cap(t *testing.T) {
	in := run4Input()
	p := &jsonPlan{Notes: "nothing"}
	_, reqs := chooseWith(t, in, submit(t, "tu_1", p), submit(t, "tu_2", p), submit(t, "tu_3", p))
	fb := feedbackOf(t, reqs[1])
	assert.Contains(t, fb, "11. and 2 more")
	assert.Contains(t, fb, "problem(s) of the same kind")
	assert.NotContains(t, fb, "12. ")
}
