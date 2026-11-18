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
		{
			ID: 13, Date: day(sun), SlotID: lunch, Eaters: 1,
			Components: []planning.Component{{ID: 24, FoodID: chili, Portions: ptr(1.0), SessionID: 4}},
		},
	}
	setMode(&s, plate.CellKey{Date: day(sat), SlotID: lunch}, availability.ModeNone)
	return s
}

// A session whose meals all sit on its own cook cell adds nothing to the cell
// line and is left out of the session list.
func TestRenderPlanSummary_SkipsSessionsOfTheirOwnCell(t *testing.T) {
	s := summaryState()
	s.Sessions = append(s.Sessions, planning.Session{
		CookSession: plate.CookSession{ID: 5, FoodID: oats, CookDate: day(sat), CookSlotID: breakfast, Portions: 1},
	})
	s.Plates[1].Components[0].SessionID = 5
	got := planning.RenderPlanSummary(s, nil)
	assert.NotContains(t, got, "session_id 5 | food_id")
	assert.Contains(t, got, ", session_id 5 cook)", "the cell line still names it")
	assert.Contains(t, got, "session_id 4 | food_id", "a batch that feeds another cell stays")
}

// B3-AC1 (summary half): dates, shopping day, plates with ids/state/eaters,
// sessions with consumption and issues; no candidates; within the budget for
// the 300-recipe state.
func TestRenderPlanSummary(t *testing.T) {
	s := summaryState()
	issues := []planning.Issue{{Rule: "H2", Code: planning.CodeTooManyCookOccasions, Severity: planning.SeverityViolation, Date: sat, Params: map[string]any{"count": 2}}}

	got := planning.RenderPlanSummary(s, issues)

	for _, want := range []string{
		"Today is Friday 2026-11-06.",
		"the shopping day is Saturday",
		"- Saturday 2026-11-07\n",
		"- Friday 2026-11-13\n",
		"plate_id 11, confirmed, 2 eaters: Chili (component_id 21, food_id ",
		", session_id 4 cook)",
		"plate_id 12, draft, 1 eater: Oats (component_id 22",
		"60 g/eater",
		"| none | no plate, leave empty",
		"session_id 4 | food_id ",
		"portions 4, frozen 1 | eaten 3, of which frozen 0 | keeps until 2026-11-10",
		"Issues:\n- cooking occasions per day (violation) " + sat + " count=2\n",
	} {
		assert.Contains(t, got, want)
	}
	assert.NotContains(t, got, "Candidate")
	assert.NotContains(t, got, ", )")
	assert.NotContains(t, got, "1 eaters")
	assert.LessOrEqual(t, len(got), 24_000)
}

func TestRenderPlanSummary_NoIssues(t *testing.T) {
	require.Contains(t, planning.RenderPlanSummary(summaryState(), nil), "Issues: none.")
}
