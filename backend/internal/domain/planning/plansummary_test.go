package planning_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/availability"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

func ptr[T any](v T) *T { return &v }

// summaryState is the bench state (300 recipes) with persisted plates: a
// confirmed chili batch eaten twice and a draft oats plate.
func summaryState() planning.State {
	s, _, _ := benchState()
	s.Sessions = []planning.Session{{
		CookSession: plate.CookSession{ID: 4, FoodID: chili, CookDate: day(sat), CookSlotID: dinner, Portions: 4, FrozenPortions: 1},
		Confirmed:   true,
	}}
	s.Plates = []planning.Plate{
		{
			ID: 11, Date: day(sat), SlotID: dinner, Eaters: 2, Confirmed: true,
			Components: []planning.Component{{ID: 21, FoodID: chili, Portions: ptr(1.0), SessionID: 4}},
		},
		{
			ID: 12, Date: day(sat), SlotID: breakfast, Eaters: 1,
			Components: []planning.Component{{ID: 22, FoodID: oats, Portions: ptr(1.0)}, {ID: 23, FoodID: egg, Grams: ptr(60.0)}},
		},
	}
	setMode(&s, plate.CellKey{Date: day(sat), SlotID: lunch}, availability.ModeNone)
	return s
}

// B3-AC1 (summary half): dates, shopping day, plates with ids/state/eaters,
// sessions with consumption and issues; no candidates; within the budget for
// the 300-recipe state.
func TestRenderPlanSummary(t *testing.T) {
	s := summaryState()
	issues := []planning.Issue{{Rule: "H2", Code: "too_many_cooks", Severity: planning.SeverityViolation, Date: sat, Params: map[string]any{"count": 2}}}

	got := planning.RenderPlanSummary(s, issues)

	for _, want := range []string{
		"Today is Friday 2026-11-06.",
		"the shopping day is Saturday",
		"- Saturday 2026-11-07\n",
		"- Friday 2026-11-13\n",
		"plate_id 11, confirmed, 2 eaters: Chili (component_id 21, food_id ",
		", session_id 4)",
		"plate_id 12, draft, 1 eaters: Oats (component_id 22",
		"60 g/eater",
		"| none | no plate, leave empty",
		"session_id 4 | food_id ",
		"portions 4, frozen 1 | eaten 2, of which frozen 0 | keeps until 2026-11-10",
		"Issues:\n- H2 too_many_cooks (violation) " + sat + " count=2\n",
	} {
		assert.Contains(t, got, want)
	}
	assert.NotContains(t, got, "Candidate")
	assert.LessOrEqual(t, len(got), 24_000)
}

func TestRenderPlanSummary_NoIssues(t *testing.T) {
	require.Contains(t, planning.RenderPlanSummary(summaryState(), nil), "Issues: none.")
}
