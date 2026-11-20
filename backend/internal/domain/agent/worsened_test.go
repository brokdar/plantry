package agent_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/agent"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

func issueWith(rule, code, date string, params map[string]any) planning.Issue {
	return planning.Issue{Rule: rule, Code: code, Severity: planning.SeveritySoft, Date: date, Params: params}
}

func byKey(is ...planning.Issue) map[string]planning.Issue {
	m := map[string]planning.Issue{}
	for _, i := range is {
		m[agent.IssueKey(i)] = i
	}
	return m
}

// An issue open before and after whose numbers got worse is reported with
// both params: it is neither new nor resolved (audit B3).
func TestDeltaOf_Worsened(t *testing.T) {
	t.Parallel()
	before := byKey(
		issueWith("S7", planning.CodeWeekMeatAboveDGE, "", map[string]any{"meat_g": 321.0, "max_g": 300.0}),
		issueWith("S2", planning.CodeKcalBelowBand, "2026-11-10", map[string]any{"kcal": 1700.0, "band_low_kcal": 1800.0, "band_high_kcal": 2200.0}),
		issueWith("S2", planning.CodeKcalBelowBand, "2026-11-11", map[string]any{"kcal": 1700.0, "band_low_kcal": 1800.0, "band_high_kcal": 2200.0}),
		issueWith("S6", planning.CodeMealNotLight, "2026-11-12", map[string]any{"kcal": 800.0, "max_kcal": 550.0}),
	)
	now := byKey(
		issueWith("S7", planning.CodeWeekMeatAboveDGE, "", map[string]any{"meat_g": 471.0, "max_g": 300.0}),
		issueWith("S2", planning.CodeKcalBelowBand, "2026-11-10", map[string]any{"kcal": 1500.0, "band_low_kcal": 1800.0, "band_high_kcal": 2200.0}),
		issueWith("S2", planning.CodeKcalBelowBand, "2026-11-11", map[string]any{"kcal": 1790.0, "band_low_kcal": 1800.0, "band_high_kcal": 2200.0}),
		issueWith("S6", planning.CodeMealNotLight, "2026-11-12", map[string]any{"kcal": 800.0, "max_kcal": 550.0}),
	)
	res := agent.DeltaOf(before, now)
	assert.Empty(t, res["new"])
	assert.Empty(t, res["resolved"])
	worse := asMaps(t, res["worsened"])
	require.Len(t, worse, 2, "meat and the second S2 day got worse; the better and the equal ones did not")
	assert.Equal(t, "2026-11-10", worse[1]["date"], "sorted by date (none first), then rule")
	assert.Equal(t, "S7", worse[0]["rule"])
	assert.Equal(t, map[string]any{"meat_g": 321.0, "max_g": 300.0}, worse[0]["before"])
	assert.Equal(t, map[string]any{"meat_g": 471.0, "max_g": 300.0}, worse[0]["after"])
}

func asMaps(t *testing.T, v any) []map[string]any {
	t.Helper()
	raw, err := json.Marshal(v)
	require.NoError(t, err)
	var out []map[string]any
	require.NoError(t, json.Unmarshal(raw, &out))
	return out
}

func TestDeltaOf_NothingWorseOmitsTheKey(t *testing.T) {
	t.Parallel()
	is := issueWith("S7", planning.CodeWeekMeatAboveDGE, "", map[string]any{"meat_g": 321.0})
	assert.NotContains(t, agent.DeltaOf(byKey(is), byKey(is)), "worsened")
}

func TestDeltaOf_WorsenedIsCapped(t *testing.T) {
	t.Parallel()
	before, now := map[string]planning.Issue{}, map[string]planning.Issue{}
	for i := 1; i <= 14; i++ {
		d := "2026-11-" + string(rune('0'+i/10)) + string(rune('0'+i%10))
		b := issueWith("S6", planning.CodeMealNotLight, d, map[string]any{"kcal": 600.0, "max_kcal": 550.0})
		a := issueWith("S6", planning.CodeMealNotLight, d, map[string]any{"kcal": 700.0, "max_kcal": 550.0})
		before[agent.IssueKey(b)], now[agent.IssueKey(a)] = b, a
	}
	res := agent.DeltaOf(before, now)
	assert.Len(t, asMaps(t, res["worsened"]), 10)
	assert.Equal(t, 4, res["worsened_more"])
}

// A move whose also_changed lists cooks_fresh or batch_resized (no from_date)
// must not add an empty date: that read as "outside the evaluated windows" and
// turned issues_checked off with an empty reason (audit B4).
func TestMealResult_SkipsEmptyDates(t *testing.T) {
	t.Parallel()
	b := agent.NewBatch()
	agent.MealResult(map[string]any{}, plate.MealResult{AlsoChanged: []plate.AlsoChanged{
		{What: plate.AlsoCooksFresh, SessionID: 3, PlateID: 4, Date: "2026-11-09"},
		{What: plate.AlsoBatchResized, SessionID: 5, Date: "2026-11-10"},
		{What: plate.AlsoCookMoved, SessionID: 6, Date: "2026-11-11", FromDate: "2026-11-12"},
	}}, b)
	assert.ElementsMatch(t, []string{"2026-11-09", "2026-11-10", "2026-11-11", "2026-11-12"}, agent.BatchDates(b))
}

// An after-value that cannot be measured is neither worsened nor improved.
func TestDeltaOf_UnmeasurableAfterIsNotWorsened(t *testing.T) {
	t.Parallel()
	before := issueWith("S7", planning.CodeWeekMeatAboveDGE, "", map[string]any{"meat_g": 321.0, "max_g": 300.0})
	after := issueWith("S7", "unknown_code", "", nil)
	assert.NotContains(t, agent.DeltaOf(byKey(before), byKey(after)), "worsened")
}
