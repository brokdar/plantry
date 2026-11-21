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
	t.Parallel()
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
	t.Parallel()
	raw := json.RawMessage(`{"saved":false,"status":"Nothing changed: check","dry_run":true,"dry_run_note":"Nothing was saved.","applied":[{"op":"swap","date":"2026-11-25"}],
		"after":{"days":[{"date":"2026-11-25","kcal":1800}],"message":"Wed 2026-11-25 is over"},"report":{"changed":[]},
		"ways":[{"ops":[{"op":"add","date":"2026-11-25"}],"light":[{"date":"2026-11-25","scope":"meal"}]}]}`)
	out := string(agent.ModelView(raw))
	assert.Regexp(t, `^\{"saved":false,"status":"Nothing changed`, out, "a check opens with saved false and a status")
	assert.Contains(t, out, `"would_apply"`)
	assert.Contains(t, out, `"plan_if_applied"`)
	assert.Contains(t, out, `"report_if_applied"`)
	assert.NotContains(t, out, `"applied"`)
	assert.NotContains(t, out, `"dry_run"`)
	assert.Contains(t, out, `"date":"Wed 25 Nov"`)
	assert.Contains(t, out, `"ops":[{"date":"2026-11-25","op":"add"}]`, "an op the model sends back keeps its date")
	assert.Contains(t, out, `"light":[{"date":"2026-11-25"`, "so does a light entry")

	saved := string(agent.ModelView(json.RawMessage(`{"saved":true,"status":"Saved.","dry_run":false,"applied":[],"after":{},"report":{}}`)))
	assert.Regexp(t, `^\{"saved":true,"status":"Saved\."`, saved)
	assert.Contains(t, saved, `"report"`)

	other := json.RawMessage(`{"items":[{"id":1,"last_eaten":"2026-10-20"}]}`)
	assert.JSONEq(t, `{"items":[{"id":1,"last_eaten":"Tue 20 Oct"}]}`, string(agent.ModelView(other)))
	require.True(t, json.Valid(agent.ModelView(raw)))
}

// E024-2: differences the model used to compute are in the result.
func TestAddDerived(t *testing.T) {
	t.Parallel()
	ways := json.RawMessage(`{"max_kcal":550,"range_low":1815,"range_high":2218,"ways":[{"after":{"meal_kcal":1212,"day_kcal":1697}}]}`)
	var res struct {
		Ways []map[string]any `json:"ways"`
	}
	require.NoError(t, json.Unmarshal(agent.AddDerived("find_ways", ways), &res))
	assert.Equal(t, "662 kcal more than a light meal should hold", res.Ways[0]["meal_vs_limit"])
	assert.Equal(t, "118 kcal below its range", res.Ways[0]["day_vs_range"])

	apply := json.RawMessage(`{"request":{"min_kcal":900,"after":{"meal_kcal":850,"day_kcal":2000}}}`)
	var ap struct {
		Request map[string]any `json:"request"`
	}
	require.NoError(t, json.Unmarshal(agent.AddDerived("apply_changes", apply), &ap))
	assert.Equal(t, "50 kcal less than a filling meal needs", ap.Request["meal_vs_limit"])
	assert.NotContains(t, ap.Request, "day_vs_range", "no range, no claim")

	alts := json.RawMessage(`{"range_low":1815,"range_high":2218,"alternatives":[{"plate_kcal":500,"day_kcal":1900}],"current":{"max_kcal":550}}`)
	var al struct {
		Alternatives []map[string]any `json:"alternatives"`
	}
	require.NoError(t, json.Unmarshal(agent.AddDerived("find_alternatives", alts), &al))
	assert.Equal(t, "inside its range", al.Alternatives[0]["day_vs_range"])
	assert.Equal(t, "within what a light meal should hold", al.Alternatives[0]["meal_vs_limit"])
}

// E024 with E025: the guard and the report read the same warning set, so a
// write is never refused for a worsening the report would not mention.
func TestGuardAndReportShareTheWarningSet(t *testing.T) {
	t.Parallel()
	meat := func(g float64) planning.Issue {
		return issueWith("S7", planning.CodeWeekMeatAboveDGE, "", map[string]any{"meat_g": g, "max_g": 300.0})
	}
	fat := func(p float64) planning.Issue {
		return issueWith("S5", planning.CodeWeekFatRange, "", map[string]any{"pct": p, "range_low_pct": 25.0, "range_high_pct": 35.0})
	}
	ev := func(is ...planning.Issue) planning.Evaluation { return planning.Evaluation{Issues: is} }
	assert.Empty(t, agent.NewWarnings(ev(meat(321), fat(39.7)), ev(meat(325), fat(39.9))), "meat 321 to 325 g and fat 39.7 to 39.9 are the same warnings")
	got := agent.NewWarnings(ev(meat(321), fat(39.7)), ev(meat(350), fat(39.9)))
	require.Len(t, got, 1)
	assert.Equal(t, planning.CodeWeekMeatAboveDGE, got[0].Code)
}

// E030: the model reads the hearty level as "filling" in results and sends
// "filling" back; the tool maps it to the stored level.
func TestModelView_FillingLevel(t *testing.T) {
	t.Parallel()
	raw := json.RawMessage(`{"days":[{"light_level":"hearty","light_meals":["hearty dinner"]}],"request":{"reached_hearty":true},"ways":[{"light":[{"date":"2026-11-25","level":"hearty"}]}]}`)
	out := string(agent.ModelView(raw))
	assert.NotContains(t, out, "hearty")
	assert.Contains(t, out, `"reached_filling":true`)
	assert.Contains(t, out, `"light":[{"date":"2026-11-25","level":"filling"}]`, "a light entry the model passes back says filling")
}

// E036: the offer is the last item of tell, as a ready statement, or nothing.
func TestAddDerived_OfferIsTheLastTellItem(t *testing.T) {
	t.Parallel()
	run := func(in string) []any {
		var res struct {
			Report struct {
				Tell []any `json:"tell"`
			} `json:"report"`
		}
		require.NoError(t, json.Unmarshal(agent.AddDerived("apply_changes", json.RawMessage(in)), &res))
		return res.Report.Tell
	}
	t.Run("an offer of the result", func(t *testing.T) {
		tell := run(`{"dry_run":false,"report":{"tell":["Thu 26 Nov dinner: A replaced by B."]},"offers":[{"what":"swap Mon dinner's Salmon for Tofu","effect":{"day":"Mon 23 Nov","day_kcal_after":1958}}]}`)
		assert.Equal(t, "I can also swap Mon dinner's Salmon for Tofu (Mon 23 Nov would then hold 1958 kcal): say so if you want it.", tell[len(tell)-1])
	})
	t.Run("the first option of a new issue", func(t *testing.T) {
		tell := run(`{"dry_run":false,"report":{"tell":["x"]},"after":{"issues":{"new":[{"options":[{"action":"add","name":"Ciabatta","grams":120,"date":"2026-11-26","day_kcal":2047}]}]}}}`)
		assert.Equal(t, "I can also add 120 g of Ciabatta on Thu 26 Nov (the day would then hold 2047 kcal): say so if you want it.", tell[len(tell)-1])
	})
	t.Run("none, none", func(t *testing.T) {
		assert.Equal(t, []any{"x"}, run(`{"dry_run":false,"report":{"tell":["x"]},"after":{"issues":{"new":[]}}}`))
	})
	t.Run("a check offers nothing", func(t *testing.T) {
		assert.Equal(t, []any{"x"}, run(`{"dry_run":true,"report":{"tell":["x"]},"offers":[{"what":"y"}]}`))
	})
}

// E036: the meat-day count of the window is dropped unless the report says it
// moved; so is the week ledger.
func TestDropAfterWeek_DropsMeatDays(t *testing.T) {
	t.Parallel()
	out := string(agent.DropAfterWeek(json.RawMessage(`{"report":{"tell":[]},"after":{"days":[],"week":{"days":7},"meat_days":3,"meat_days_max":3}}`)))
	assert.NotContains(t, out, "meat_days")
	assert.NotContains(t, out, `"week"`)
	assert.Contains(t, out, `"days":[]`)
}

// E036b: the moved cooking of an away call is told in at most three items,
// and the per-dish lines of the same moves are gone.
func TestAddDerived_CookingMovedIsTold(t *testing.T) {
	t.Parallel()
	in := `{"dry_run":false,"report":{"tell":["Sun 22 Nov is away: the day is not counted.","Mon 23 Nov you cook (1 cooking time): \"A\" at lunch for x.","The \"A\" is now cooked on Mon 23 Nov at lunch instead of Sun 22 Nov at dinner.","To keep one cooking time a day, \"A\" is now cooked at Mon 23 Nov lunch instead of Sun dinner.","Weekly energy: 1935 kcal a day."]},
		"cooking_moved":{"cooked_now":[{"day":"Mon 23 Nov","cooking_time":"lunch","dishes":[{"dish":"\"A\"","feeds":["Mon 23 Nov lunch","Tue 24 Nov lunch"]},{"dish":"\"B\"","feeds":["Mon 23 Nov dinner"]}]}],"nothing_else_changed":true}}`
	var res struct {
		Report struct {
			Tell []string `json:"tell"`
		} `json:"report"`
	}
	require.NoError(t, json.Unmarshal(agent.AddDerived("apply_changes", json.RawMessage(in)), &res))
	assert.Equal(t, []string{
		"Sun 22 Nov is away: the day is not counted.",
		`Everything that moved is now cooked together on Mon 23 Nov at lunch, one cooking time: "A" for Mon 23 Nov lunch, Tue 24 Nov lunch; "B" for Mon 23 Nov dinner.`,
		"No other meal changed.",
		"Weekly energy: 1935 kcal a day.",
	}, res.Report.Tell)
}

// E036b: find_ways gets a ready closing sentence for its first offer.
func TestAddDerived_WaysClosing(t *testing.T) {
	t.Parallel()
	var res map[string]any
	out := agent.AddDerived("find_ways", json.RawMessage(`{"offer":[{"what":"halve the salmon and leave out the yogurt","changes_other_meal":true,"after":{"day_kcal":1934}}],"ways":[]}`))
	require.NoError(t, json.Unmarshal(out, &res))
	assert.Equal(t, "I can also halve the salmon and leave out the yogurt (it also changes another meal; the day would then hold 1934 kcal): say so if you want it.", res["closing"])
	var none map[string]any
	out = agent.AddDerived("find_ways", json.RawMessage(`{"ways":[]}`))
	require.NoError(t, json.Unmarshal(out, &none))
	assert.NotContains(t, none, "closing")
}
