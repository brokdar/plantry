package planning_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
)

// E020: evaluate_plan shows the plan as it will be saved.
func TestPlannerView(t *testing.T) {
	t.Run("the evaluation is of the plan the service saves for the same proposal", func(t *testing.T) {
		s := balanceState()
		in := planning.ChooseInput{State: s, Target: windowTarget(s)}
		prop := freshPlan(s)
		view, ev, done := planning.PlannerViewOf(in, prop)
		saved, _ := planning.FinishProposal(in, prop)
		assert.Equal(t, saved, done, "the preview and the save finish the proposal alike")
		require.NotNil(t, ev.Week)
		assert.GreaterOrEqual(t, ev.Week.Pct, 95.0, "the preview shows the balanced week")
		assert.InDelta(t, balanceWeek(s, saved).Pct, ev.Week.Pct, 1e-9)
		assert.Less(t, balanceWeek(s, prop).Pct, ev.Week.Pct, "the proposal as made was shorter")
		assert.NotEmpty(t, view["code_changes"], "the lines the code changed are shown")
		assert.NotContains(t, view, "open_levers", "a balanced week has nothing open")
		for _, is := range ev.Issues {
			if is.Rule == "S2" || is.Rule == "S3" || is.Rule == "S4" || is.Rule == "S9" {
				assert.Empty(t, is.Options, "quantity items are the code's: no options for the model")
			}
		}
	})
	t.Run("a week that stays short says so with a computed lever", func(t *testing.T) {
		s := balanceState()
		s.Profile.KcalTarget = f64(6000)
		in := planning.ChooseInput{State: s, Target: windowTarget(s)}
		view, ev, _ := planning.PlannerViewOf(in, freshPlan(s))
		require.NotNil(t, ev.Week)
		require.Less(t, ev.Week.Pct, 95.0)
		raw, err := json.Marshal(view["open_levers"])
		require.NoError(t, err)
		var levers []string
		require.NoError(t, json.Unmarshal(raw, &levers))
		require.NotEmpty(t, levers)
		assert.Contains(t, levers[0], "of its energy target")
		assert.Contains(t, levers[0], "higher-energy main on ")
		assert.Contains(t, levers[0], "add a cooking day")
		assert.Regexp(t, `about \d+ kcal more per person`, levers[0])
	})
	t.Run("a confirmed plate is untouched in the preview", func(t *testing.T) {
		s := balanceState()
		s.Plates = append(s.Plates, planning.Plate{ID: 900, Date: day(tue), SlotID: dinner, Eaters: 1, Confirmed: true, Components: []planning.Component{{FoodID: 101, Portions: f64(1)}}})
		target := windowTarget(s)
		prop := freshPlan(s)
		prop.Plates = prop.Plates[:0:0]
		for _, c := range s.Cells {
			if c.SlotID != breakfast && (!c.Date.Equal(day(tue)) || c.SlotID != dinner) {
				prop.Plates = append(prop.Plates, planning.ProposedPlate{Date: c.Date, SlotID: c.SlotID, Eaters: 1, Components: []planning.ProposedComponent{{FoodID: 102, Portions: 1}}})
			}
		}
		view, _, done := planning.PlannerViewOf(planning.ChooseInput{State: s, Target: target}, prop)
		m := planning.Merge(s, target, done)
		found := false
		for _, p := range m.Plates {
			if p.ID == 900 {
				found = true
				require.Len(t, p.Components, 1)
				assert.InDelta(t, 1.0, *p.Components[0].Portions, 1e-9)
			}
		}
		assert.True(t, found)
		raw, _ := json.Marshal(view["code_changes"])
		assert.False(t, strings.Contains(string(raw), "Tuesday dinner"), "no change line names the confirmed cell: %s", raw)
	})
}
