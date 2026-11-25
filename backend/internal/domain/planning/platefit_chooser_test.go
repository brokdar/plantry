package planning_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
)

// plateFitRun4 is run4Input with plate fit on or off: Chili sin carne (106)
// and Steamed rice (124) carry a starch, so the two on one plate clash.
func plateFitRun4(on bool) planning.ChooseInput {
	in := run4Input()
	in.State.Settings.PlateFit = on
	for id, starch := range map[int64]bool{106: true, 124: true, 125: false} {
		f := in.State.Foods[id]
		f.Starch, f.Fresh = ptrB(starch), ptrB(!starch)
		in.State.Foods[id] = f
	}
	return in
}

// clashWeek is validWeek with Steamed rice cooked fresh beside Saturday's
// Chili sin carne, on its cook cell: no rule of the checker is broken.
func clashWeek(t *testing.T, in planning.ChooseInput) *jsonPlan {
	t.Helper()
	p := validWeek(t, in)
	comps := p.Plates[0]["components"].([]map[string]any)
	require.Equal(t, int64(106), comps[0]["food_id"])
	p.Plates[0]["components"] = append(comps, map[string]any{"food_id": 124, "portions": 1})
	return p
}

func hasFood(p planning.Proposal, id int64) bool {
	for _, pl := range p.Plates {
		for _, c := range pl.Components {
			if c.FoodID == id {
				return true
			}
		}
	}
	return false
}

// PLATE-5: a clash is sent back in the chooser's existing repair round and
// the repaired plate comes out; it is never a violation of the result.
func TestLLMChooser_PlateFitClashIsRepaired(t *testing.T) {
	in := plateFitRun4(true)
	clash := clashWeek(t, in)

	prop, reqs := chooseWith(t, in, submit(t, "tu_1", clash), submit(t, "tu_2", validWeek(t, in)))

	require.Len(t, reqs, 2)
	assert.Equal(t, 1, prop.RepairAttempts)
	assert.False(t, hasFood(prop, 124), "the repaired plate has no second starch")
	fb := feedbackOf(t, reqs[1])
	assert.Contains(t, fb, "1. Plate fit: Saturday 2026-11-21 dinner (slot_id 3) puts Chili sin carne + Steamed rice on one plate (two starches or two mains).")
	assert.Empty(t, violationsOfProposal(in, prop))
}

// A clash the repairs leave is no note and no violation: the plan is
// returned as it is, after the existing two repairs and no more.
func TestLLMChooser_PlateFitClashLeftIsNoNote(t *testing.T) {
	in := plateFitRun4(true)
	clash := clashWeek(t, in)

	prop, reqs := chooseWith(t, in, submit(t, "tu_1", clash), submit(t, "tu_2", clash), submit(t, "tu_3", clash))

	require.Len(t, reqs, 3)
	assert.Equal(t, 2, prop.RepairAttempts)
	assert.True(t, hasFood(prop, 124))
	assert.NotContains(t, prop.Notes, "Plate fit")
	assert.NotContains(t, prop.Notes, "Steamed rice")
	assert.Empty(t, violationsOfProposal(in, prop), "a clash never enters the checker")
}

// PLATE-9: with plan.plate_fit off the clash is no repair.
func TestLLMChooser_PlateFitOffNoRepair(t *testing.T) {
	in := plateFitRun4(false)

	prop, reqs := chooseWith(t, in, submit(t, "tu_1", clashWeek(t, in)))

	require.Len(t, reqs, 1)
	assert.Zero(t, prop.RepairAttempts)
	assert.True(t, hasFood(prop, 124))
}
