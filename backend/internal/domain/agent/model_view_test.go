package agent_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/agent"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
)

// E024-1: an issue open before and after is the same warning unless it got
// worse by a real step; a share that moved 0.2 points or meat 4 g further out
// is not news.
func TestWorsened_NeedsARealStep(t *testing.T) {
	fat := func(pct float64) planning.Issue {
		return issueWith("S5", planning.CodeWeekFatRange, "", map[string]any{"pct": pct, "range_low_pct": 25.0, "range_high_pct": 35.0})
	}
	meat := func(g float64) planning.Issue {
		return issueWith("S7", planning.CodeWeekMeatAboveDGE, "", map[string]any{"meat_g": g, "max_g": 300.0})
	}
	day := func(kcal float64) planning.Issue {
		return issueWith("S2", planning.CodeKcalBelowBand, "2026-11-10", map[string]any{"kcal": kcal, "band_low_kcal": 1800.0, "band_high_kcal": 2200.0})
	}
	cases := []struct {
		name          string
		before, after planning.Issue
		want          bool
	}{
		{"fat 39.7 to 39.9", fat(39.7), fat(39.9), false},
		{"fat 39.7 to 40.7", fat(39.7), fat(40.7), true},
		{"meat 321 to 325", meat(321), meat(325), false},
		{"meat 321 to 346", meat(321), meat(346), true},
		{"day 40 kcal further under", day(1700), day(1660), false},
		{"day 50 kcal further under", day(1700), day(1650), true},
		{"better is never worse", day(1700), day(1790), false},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, agent.Worsened(c.before, c.after), c.name)
	}
}

// E024-3 and E024-5: the model reads days as "Wed 25 Nov" (ids and the fields
// it sends back keep ISO dates), and a check never looks like a write.
func TestModelView_DatesAndDryRun(t *testing.T) {
	raw := json.RawMessage(`{"dry_run":true,"dry_run_note":"Nothing was saved.","applied":[{"op":"swap","date":"2026-11-25"}],
		"after":{"days":[{"date":"2026-11-25","kcal":1800}],"message":"Wed 2026-11-25 is over"},"report":{"changed":[]},
		"ways":[{"ops":[{"op":"add","date":"2026-11-25"}],"light":[{"date":"2026-11-25","scope":"meal"}]}]}`)
	out := string(agent.ModelView(raw))
	assert.Regexp(t, `^\{"saved":false,"nothing_changed":"`, out, "a check opens with saved false")
	assert.Contains(t, out, `"would_apply"`)
	assert.Contains(t, out, `"plan_if_applied"`)
	assert.Contains(t, out, `"report_if_applied"`)
	assert.NotContains(t, out, `"applied"`)
	assert.NotContains(t, out, `"dry_run"`)
	assert.Contains(t, out, `"date":"Wed 25 Nov"`)
	assert.Contains(t, out, `"ops":[{"date":"2026-11-25","op":"add"}]`, "an op the model sends back keeps its date")
	assert.Contains(t, out, `"light":[{"date":"2026-11-25"`, "so does a light entry")

	saved := string(agent.ModelView(json.RawMessage(`{"dry_run":false,"applied":[],"after":{},"report":{}}`)))
	assert.Regexp(t, `^\{"saved":true`, saved)
	assert.Contains(t, saved, `"report"`)

	other := json.RawMessage(`{"items":[{"id":1,"last_eaten":"2026-10-20"}]}`)
	assert.JSONEq(t, `{"items":[{"id":1,"last_eaten":"Tue 20 Oct"}]}`, string(agent.ModelView(other)))
	require.True(t, json.Valid(agent.ModelView(raw)))
}

// E024-2: differences the model used to compute are in the result.
func TestAddDerived(t *testing.T) {
	ways := json.RawMessage(`{"max_kcal":550,"range_low":1815,"range_high":2218,"ways":[{"after":{"meal_kcal":1212,"day_kcal":1697}}]}`)
	var res struct {
		Ways []map[string]any `json:"ways"`
	}
	require.NoError(t, json.Unmarshal(agent.AddDerived("find_ways", ways), &res))
	assert.Equal(t, "662 kcal over the most a light meal holds", res.Ways[0]["meal_vs_limit"])
	assert.Equal(t, "118 kcal below its range", res.Ways[0]["day_vs_range"])

	apply := json.RawMessage(`{"request":{"min_kcal":900,"after":{"meal_kcal":850,"day_kcal":2000}}}`)
	var ap struct {
		Request map[string]any `json:"request"`
	}
	require.NoError(t, json.Unmarshal(agent.AddDerived("apply_changes", apply), &ap))
	assert.Equal(t, "50 kcal under the least a filling meal holds", ap.Request["meal_vs_limit"])
	assert.NotContains(t, ap.Request, "day_vs_range", "no range, no claim")

	alts := json.RawMessage(`{"range_low":1815,"range_high":2218,"alternatives":[{"plate_kcal":500,"day_kcal":1900}],"current":{"max_kcal":550}}`)
	var al struct {
		Alternatives []map[string]any `json:"alternatives"`
	}
	require.NoError(t, json.Unmarshal(agent.AddDerived("find_alternatives", alts), &al))
	assert.Equal(t, "inside its range", al.Alternatives[0]["day_vs_range"])
	assert.Equal(t, "within the most a light meal holds", al.Alternatives[0]["meal_vs_limit"])
}
