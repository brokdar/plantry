package eval

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/airun"
	"github.com/jaltszeimer/plantry/backend/internal/domain/diet"
	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
	"github.com/jaltszeimer/plantry/backend/internal/domain/window"
)

// The 7 scenarios of spec §8.4.
var specScenarios = []string{
	"away_thu_fri", "family_daily", "many_none_cells", "replan_one_day",
	"single_batch_cooker", "small_library_h8", "vegetarian_variety",
}

func TestLoadScenarios(t *testing.T) {
	all, err := LoadScenarios()
	require.NoError(t, err)
	var names []string
	for _, sc := range all {
		names = append(names, sc.Name)
	}
	assert.Equal(t, specScenarios, names)

	picked, err := LoadScenarios("family_daily")
	require.NoError(t, err)
	require.Len(t, picked, 1)
	assert.Equal(t, "family_daily", picked[0].Name)

	_, err = LoadScenarios("nope")
	assert.ErrorContains(t, err, `unknown scenario "nope"`)
}

func TestLibrary(t *testing.T) {
	lib, err := LoadLibrary()
	require.NoError(t, err)
	assert.Len(t, lib.Recipes, 30)
	for _, in := range lib.Ingredients {
		assert.NotEqual(t, diet.Unknown, diet.Classify(in.Name), "ingredient %s has no diet class", in.Name)
		assert.Positive(t, in.Kcal, in.Name)
	}
}

// B9-AC1: the heuristic plans every scenario without a hard violation and
// meets the scenario's expectations (spec §8.4 CI).
func TestHeuristicHasNoHardViolations(t *testing.T) {
	scs, err := LoadScenarios()
	require.NoError(t, err)
	recs, err := Run(context.Background(), Config{Chooser: airun.ChooserHeuristic, Runs: 1}, scs)
	require.NoError(t, err)
	require.Len(t, recs, len(specScenarios))
	for _, r := range recs {
		t.Run(r.Scenario, func(t *testing.T) {
			assert.Equal(t, string(airun.StatusOK), r.Status, r.ErrorKey)
			assert.Empty(t, r.Violations)
			assert.Empty(t, r.Failed())
			assert.Contains(t, names(r.Expectations), ExpectNoViolations)
			require.NotNil(t, r.Score)
			assert.Empty(t, r.Score.HardViolations)
			assert.Equal(t, "h1", r.PromptVersion)
			assert.Equal(t, Tokens{}, r.Tokens, "the heuristic calls no model")
			if r.Scenario == "small_library_h8" {
				assert.Contains(t, names(r.Expectations), ExpectMinRepeats, "H8 relaxation is pinned")
				assert.GreaterOrEqual(t, r.Score.Variety.Repeats, 7)
			}
		})
	}
}

func TestFrozenSpare(t *testing.T) {
	from := Date{day("2026-11-10")}
	st := Step{Action: ActionReplan, From: &from}
	before := rows{sessions: map[int64]sessionRow{
		1: {CookDate: "2026-11-10", Portions: 6},
		2: {CookDate: "2026-11-09", Portions: 6},
		3: {CookDate: "2026-11-11", Portions: 6, Cooked: true},
	}}
	frozenAt := func(id int64) rows {
		after := rows{sessions: map[int64]sessionRow{}}
		for k, s := range before.sessions {
			if k == id {
				s.FrozenPortions = 1
			}
			after.sessions[k] = s
		}
		return after
	}
	assert.True(t, frozenSpare(st, before, frozenAt(1)).OK)
	assert.False(t, frozenSpare(st, before, frozenAt(2)).OK, "cooks before from")
	assert.False(t, frozenSpare(st, before, frozenAt(3)).OK, "already cooked")
	assert.False(t, frozenSpare(st, before, before).OK)
}

// B9-AC2: with the fake client scripted per scenario, the LLM chooser's
// recorded requests carry the scenario's dates (the "nil dates" regression).
func TestScriptedLLMPromptHasScenarioDates(t *testing.T) {
	scs, err := LoadScenarios()
	require.NoError(t, err)
	recs, err := Run(context.Background(), Config{Chooser: airun.ChooserLLM, Runs: 1}, scs)
	require.NoError(t, err)
	require.Len(t, recs, len(specScenarios))
	for _, r := range recs {
		t.Run(r.Scenario, func(t *testing.T) {
			assert.Empty(t, r.Failed())
			assert.Equal(t, string(airun.StatusOK), r.Status, r.ErrorKey)
			assert.Equal(t, 0, r.RepairAttempts)
			assert.Equal(t, "fake", r.Model)
			assert.Equal(t, "p1", r.PromptVersion)
			// A run that fills cells asks the model; one with nothing to
			// fill (away_thu_fri) may not.
			if r.Plates > 0 {
				assert.Contains(t, names(r.Expectations), ExpectPromptDates)
				assert.Positive(t, r.Tokens.Input, "the metered fake client reports tokens")
				assert.Nil(t, r.CostUSD, "the fake model has no price")
			}
		})
	}
}

func TestPromptDates(t *testing.T) {
	e := &env{window: window.Window{Start: day("2026-11-07"), End: day("2026-11-13")}}
	today := day("2026-11-06")
	var full strings.Builder
	full.WriteString("Today is Friday 2026-11-06.\n")
	for _, d := range e.window.Days() {
		full.WriteString("- " + weekdayDate(d) + "\n")
	}

	ok := promptDates([]llm.Request{{System: full.String()}}, today, e)
	assert.True(t, ok.OK, ok.Detail)

	// The regression: a prompt rendered from zero dates.
	nilDates := promptDates([]llm.Request{{System: "Today is Monday 0001-01-01."}}, today, e)
	assert.False(t, nilDates.OK)
	assert.Contains(t, nilDates.Detail, "Friday 2026-11-06")

	missing := promptDates([]llm.Request{{System: full.String()}, {System: "Today is Friday 2026-11-06."}}, today, e)
	assert.False(t, missing.OK)
	assert.Contains(t, missing.Detail, "request 2")

	none := promptDates(nil, today, e)
	assert.False(t, none.OK)
}

// B9-AC4: "away Thu–Fri" runs the re-plan (No plate on Thu/Fri, nothing
// before Wednesday touched) and "re-plan one day" keeps the confirmed
// neighbours.
func TestReplanScenarios(t *testing.T) {
	scs, err := LoadScenarios("away_thu_fri", "replan_one_day")
	require.NoError(t, err)
	assert.Equal(t, ActionReplan, scs[0].Steps[len(scs[0].Steps)-1].Action)
	assert.Equal(t, ActionRegenerate, scs[1].Steps[len(scs[1].Steps)-1].Action)

	recs, err := Run(context.Background(), Config{Chooser: airun.ChooserHeuristic, Runs: 1}, scs)
	require.NoError(t, err)
	require.Len(t, recs, 2)

	away := recs[0]
	assert.Empty(t, away.Failed())
	assert.Subset(t, names(away.Expectations), []string{ExpectKeptConfirmed, ExpectUnchangedBefore, ExpectNoPlate, ExpectFrozenSpare})

	oneDay := recs[1]
	assert.Empty(t, oneDay.Failed())
	assert.Contains(t, names(oneDay.Expectations), ExpectKeptConfirmed)
	assert.GreaterOrEqual(t, oneDay.Plates, 2)
}

func TestCompareDetectsChanges(t *testing.T) {
	s1 := 0.5
	prev := []Record{
		{GitSHA: "a", Chooser: "llm", PromptVersion: "p1", Model: "m", Scenario: "x", Run: 1, Tokens: Tokens{Input: 100}, CostUSD: &s1},
		{GitSHA: "a", Chooser: "llm", PromptVersion: "p1", Model: "m", Scenario: "x", Run: 2, Tokens: Tokens{Input: 200}, CostUSD: &s1},
	}
	cur := []Record{
		{
			GitSHA: "b", Chooser: "llm", PromptVersion: "p2", Model: "m", Scenario: "x", Run: 1, Tokens: Tokens{Input: 120},
			Expectations: []Expectation{{Name: ExpectNoViolations}},
		},
		{GitSHA: "b", Chooser: "llm", PromptVersion: "p2", Model: "m", Scenario: "y", Run: 1},
	}
	var buf bytes.Buffer
	require.NoError(t, WriteCompare(&buf, prev, cur))
	out := buf.String()
	assert.Contains(t, out, "Compare a llm/p1/m -> b llm/p2/m")
	assert.Regexp(t, `input_tokens\s+150\s+120\s+-30\n`, out)
	assert.Regexp(t, `ok_runs\s+1\s+0\s+-1\n`, out)
	assert.Regexp(t, `cost_usd\s+0.5\s+-\s+-\n`, out)
	assert.Contains(t, out, "\ny\n")
}

func TestJSONLRoundTrip(t *testing.T) {
	c := 0.25
	recs := []Record{{GitSHA: "a", Scenario: "x", Run: 1, Status: "ok", CostUSD: &c, Expectations: []Expectation{{Name: "n", OK: true}}}}
	var buf bytes.Buffer
	require.NoError(t, WriteJSONL(&buf, recs))
	got, err := ReadJSONL(&buf)
	require.NoError(t, err)
	assert.Equal(t, recs, got)

	_, err = ReadJSONL(strings.NewReader("{\n"))
	assert.ErrorContains(t, err, "line 1")
}

func TestRunRejectsUnknownChooser(t *testing.T) {
	_, err := Run(context.Background(), Config{Chooser: "magic"}, nil)
	assert.ErrorContains(t, err, `unknown chooser "magic"`)
}

func names(xs []Expectation) []string {
	out := make([]string, len(xs))
	for i, x := range xs {
		out[i] = x.Name
	}
	return out
}

func day(s string) time.Time {
	t, err := time.Parse(time.DateOnly, s)
	if err != nil {
		panic(err)
	}
	return t
}
