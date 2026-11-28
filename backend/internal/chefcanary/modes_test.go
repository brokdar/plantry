package chefcanary_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/fake"
	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/chefcanary"
	"github.com/jaltszeimer/plantry/backend/internal/chefsets"
	"github.com/jaltszeimer/plantry/backend/internal/domain/airun"
	"github.com/jaltszeimer/plantry/backend/internal/domain/chefhelp"
	"github.com/jaltszeimer/plantry/backend/internal/domain/finds"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/importer"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
)

func toolTurn(tool, input string, in, out int) fake.Turn {
	return fake.Turn{
		ForTool: tool, StopReason: "tool_use",
		Message: fake.ScriptedMessage{Content: []fake.ScriptedBlock{{Type: "tool_use", ToolUseID: "toolu_" + tool, ToolName: tool, ToolInput: json.RawMessage(input)}}},
		Usage:   fake.ScriptedUsage{InputTokens: in, OutputTokens: out},
	}
}

// dear prices the fake model at $1000 per million input tokens: a call of 900
// input tokens costs $0.90.
var dear = map[string]airun.Price{"fake-model": {In: 1000}}

func env(t *testing.T, turns ...fake.Turn) chefcanary.Env {
	t.Helper()
	return chefcanary.Env{
		DB: testhelper.NewTestDB(t), Client: fake.NewFromScript(&fake.Script{Turns: turns, Capabilities: fake.ScriptedCapabilities{WebSearch: true}}),
		Provider: "fake", Model: "fake-model", Prices: dear,
	}
}

func metric(t *testing.T, r chefcanary.ModeReport, name string) chefcanary.Metric {
	t.Helper()
	for _, m := range r.Metrics {
		if m.Name == name {
			return m
		}
	}
	require.Failf(t, "no such metric", "%s in %s", name, r.Summary())
	return chefcanary.Metric{}
}

func TestImportCases_EveryPlantedErrorHasAnExpectedFinding(t *testing.T) {
	set, err := chefsets.LoadImportCheck()
	require.NoError(t, err)
	cases := chefcanary.ImportCases(set)
	clean := map[string]*importer.Draft{}
	for _, c := range cases {
		if c.Planted == nil {
			clean[strings.TrimSuffix(c.ID, "-clean")] = c.Draft
		}
	}
	require.Len(t, clean, len(set.Recipes))
	planted := 0
	for _, c := range cases {
		if c.Planted == nil {
			continue
		}
		planted++
		base := clean[c.Planted.Base]
		assert.False(t, c.Expect.Recipe && c.Expect.Ingredient != 0, c.ID)
		if !c.Expect.Recipe {
			require.Less(t, c.Expect.Ingredient, len(c.Draft.Ingredients), c.ID)
			assert.NotEqual(t, base.Ingredients[c.Expect.Ingredient], c.Draft.Ingredients[c.Expect.Ingredient], "%s changed nothing", c.ID)
		}
		switch c.Planted.Kind {
		case chefsets.PlantDropStep:
			assert.Len(t, c.Draft.Instructions, len(base.Instructions)-1, c.ID)
		case chefsets.PlantScaleServe:
			assert.Greater(t, c.Draft.ReferencePortions, base.ReferencePortions, c.ID)
		}
	}
	assert.Len(t, cases, len(set.Recipes)+len(set.Planted))
	assert.GreaterOrEqual(t, planted, 20)
}

// A tiny set: line 1 of "a" has its amount planted, "b" lost a step, "c" is clean.
func tinySet() chefsets.ImportCheckSet {
	rec := func(id string) chefsets.CheckRecipe {
		return chefsets.CheckRecipe{
			ID: id, Name: id, Language: "en", Portions: 2, Prep: 5, Cook: 10, Steps: []string{"Mix.", "Cook.", "Serve."},
			Ingredients: []chefsets.CheckIngredient{{Text: "100 g rice", Amount: 100, Unit: "g", Name: "rice"}, {Text: "200 g beans", Amount: 200, Unit: "g", Name: "beans"}},
		}
	}
	return chefsets.ImportCheckSet{
		AllowedChanges: []string{"keep"}, Recipes: []chefsets.CheckRecipe{rec("a"), rec("b"), rec("c")},
		Planted: []chefsets.Planted{
			{ID: "a-x10", Base: "a", Kind: chefsets.PlantAmountX10, Ingredient: 1},
			{ID: "b-step", Base: "b", Kind: chefsets.PlantDropStep, Step: 1},
		},
	}
}

func TestRunImportCheck_ReportsDetectionWrongFixesAndCleanChanges(t *testing.T) {
	// The reply flags line 1 and applies a keep-days change plus a role change
	// (not allowed): line 1 of the planted "a" is found, the recipe-level
	// error of "b" is missed.
	e := env(t, toolTurn(importer.CheckTool, `{"keep":{"days":3,"freezable":true},"role":{"role":"main"},"flags":[{"ingredient":1,"text":"ten times too much"}]}`, 100, 20))
	e.Prices = nil
	rep, err := chefcanary.RunImportCheck(context.Background(), chefcanary.ImportCheckConfig{Env: e, Set: tinySet()})
	require.NoError(t, err)
	assert.Equal(t, 5, rep.Ran)
	assert.InDelta(t, 50, metric(t, rep, "detection_rate").Value, 0.01)
	assert.False(t, *metric(t, rep, "detection_rate").Met)
	assert.Zero(t, metric(t, rep, "wrong_fix_rate").Value, "a role change is a designed change to judge, not a wrong fix")
	assert.InDelta(t, 100, metric(t, rep, "wrong_fix_rate.legacy").Value, 0.01, "legacy: the role change is outside the allowed kinds, in both planted cases")
	assert.Equal(t, 5.0, metric(t, rep, "changes_to_judge").Value)
	assert.Contains(t, rep.Artifacts["importcheck-changes.md"], "a-x10: role")
	assert.Equal(t, 3.0, metric(t, rep, "clean_recipes_changed").Value)
	assert.Equal(t, 3.0, metric(t, rep, "clean_recipes_flagged").Value)
	assert.InDelta(t, 100, metric(t, rep, "detection_rate.kind.amount_x10").Value, 0.01)
	assert.Zero(t, metric(t, rep, "detection_rate.kind.drop_step").Value)
	assert.True(t, rep.Unpriced)
	assert.Contains(t, rep.Summary(), "detection_rate")
}

// A unit planted on line 0 of "a" (g to ml) and a clean recipe "m" with a ml line.
func unitSet() chefsets.ImportCheckSet {
	set := tinySet()
	// Water: its 1 g per ml puts the line back to the same grams.
	a := set.Recipes[0]
	a.Ingredients = append([]chefsets.CheckIngredient{{Text: "100 g water", Amount: 100, Unit: "g", Name: "water"}}, a.Ingredients[1:]...)
	set.Recipes = []chefsets.CheckRecipe{a, {
		ID: "m", Name: "m", Language: "en", Portions: 2, Steps: []string{"Mix."},
		Ingredients: []chefsets.CheckIngredient{{Text: "100 ml milk", Amount: 100, Unit: "ml", Name: "milk"}},
	}}
	set.Planted = []chefsets.Planted{{ID: "a-unit", Base: "a", Kind: chefsets.PlantWrongUnit, Ingredient: 0, ToUnit: "ml"}}
	return set
}

func TestRunImportCheck_RestoringThePlantedLineIsACorrectFix(t *testing.T) {
	// 100 g on line 0 puts the planted line back; on the clean recipe "m" the
	// same ml-to-g change of a clean line is wrong.
	e := env(t, toolTurn(importer.CheckTool, `{"amounts":[{"ingredient":0,"kind":"unit","amount":100,"unit":"g"}]}`, 100, 20))
	rep, err := chefcanary.RunImportCheck(context.Background(), chefcanary.ImportCheckConfig{Env: e, Set: unitSet()})
	require.NoError(t, err)
	assert.Equal(t, 1.0, metric(t, rep, "planted_fixed_correctly").Value)
	assert.InDelta(t, 100, metric(t, rep, "wrong_fix_rate.legacy").Value, 0.01, "legacy: any edit of the planted line counts, also the restoring one")
	assert.InDelta(t, 100.0/3, metric(t, rep, "wrong_fix_rate").Value, 0.01, "only the clean ml line changed wrongly (1 of 3 recipes)")
	byID := map[string]chefcanary.CaseResult{}
	for _, r := range rep.Results {
		byID[r.ID] = r
	}
	require.Len(t, byID["a-unit"].Changes, 1)
	assert.Equal(t, "correct_fix", byID["a-unit"].Changes[0].Verdict)
	require.Len(t, byID["m-clean"].Changes, 1)
	assert.Equal(t, "wrong", byID["m-clean"].Changes[0].Verdict)
}

func TestRunImportCheck_TheModelsOwnNumberOnThePlantedLineIsNeverUsed(t *testing.T) {
	e := env(t, toolTurn(importer.CheckTool, `{"amounts":[{"ingredient":0,"kind":"unit","amount":50,"unit":"g"}]}`, 100, 20))
	rep, err := chefcanary.RunImportCheck(context.Background(), chefcanary.ImportCheckConfig{Env: e, Set: unitSet()})
	require.NoError(t, err)
	assert.Equal(t, 1.0, metric(t, rep, "planted_fixed_correctly").Value, "the grams are computed from the density, not the model's 50")
	for _, r := range rep.Results {
		if r.ID == "a-unit" {
			assert.Equal(t, "correct_fix", r.Changes[0].Verdict)
		}
	}
}

func TestRunImportCheck_ClarityProposalsAreListedAsFlags(t *testing.T) {
	e := env(t, toolTurn(importer.CheckTool, `{"step_edits":[{"step":0,"kind":"clarity","text":"Stir the rice well.","reason":"clearer"}]}`, 100, 20))
	rep, err := chefcanary.RunImportCheck(context.Background(), chefcanary.ImportCheckConfig{Env: e, Set: tinySet()})
	require.NoError(t, err)
	assert.Zero(t, metric(t, rep, "wrong_fix_rate").Value)
	assert.Zero(t, metric(t, rep, "changes_to_judge").Value)
	assert.Zero(t, metric(t, rep, "clean_recipes_flagged").Value, "a clarity proposal is no false alarm")
	md := rep.Artifacts["importcheck-changes.md"]
	assert.Contains(t, md, "Clarity proposals")
	assert.Contains(t, md, "Could be clearer: Stir the rice well.")
}

func TestRunImportCheck_StopsAtTheSpendCap(t *testing.T) {
	e := env(t, toolTurn(importer.CheckTool, `{}`, 900, 0))
	e.SpendCap = 1 // a call costs $0.90: the second one ends the budget
	rep, err := chefcanary.RunImportCheck(context.Background(), chefcanary.ImportCheckConfig{Env: e, Set: tinySet()})
	require.NoError(t, err)
	assert.True(t, rep.StoppedBySpendCap)
	assert.Equal(t, 2, rep.Ran)
	assert.Equal(t, 5, rep.Cases)
	assert.InDelta(t, 1.8, rep.CostUSD, 0.001)
}

func TestRunMonth_ComputesFromTheRunLog(t *testing.T) {
	e := env(t, toolTurn(importer.CheckTool, `{}`, 900, 0))
	_, err := chefcanary.RunImportCheck(context.Background(), chefcanary.ImportCheckConfig{Env: e, Set: tinySet()})
	require.NoError(t, err)
	rep, err := chefcanary.RunMonth(context.Background(), chefcanary.MonthConfig{DB: e.DB, Prices: dear})
	require.NoError(t, err)
	// Five import checks of $0.90: 4 x $0.90 in a month; nothing else was run.
	assert.InDelta(t, chefcanary.MonthImportChecks*0.9, metric(t, rep, "month.import_checks").Value, 0.001)
	assert.InDelta(t, chefcanary.MonthImportChecks*0.9, metric(t, rep, "month_cost").Value, 0.001)
	assert.Equal(t, 4.0, metric(t, rep, "kinds_without_runs").Value)
	assert.Contains(t, metric(t, rep, "month_cost").Note, "INCOMPLETE")
	assert.False(t, *metric(t, rep, "month_cost").Met, "$3.60 is under $5 but the month is incomplete")
}

func TestRunSubs_ChecksOptionsByRule(t *testing.T) {
	db := testhelper.NewTestDB(t)
	src := food.SourceManual
	kcal, p, f, c := 700.0, 1.0, 80.0, 1.0
	for _, name := range []string{"Butter", "Margarine", "Olivenöl"} {
		require.NoError(t, sqlite.NewFoodRepo(db).Create(context.Background(), &food.Food{Kind: food.KindLeaf, Source: &src, Name: name, Kcal100g: &kcal, Protein100g: &p, Fat100g: &f, Carbs100g: &c}))
	}
	line := func(n string) []chefsets.SubLine {
		return []chefsets.SubLine{{Name: n, Grams: 30}, {Name: "Mehl", Grams: 250}}
	}
	rep, err := chefcanary.RunSubs(context.Background(), chefcanary.SubsConfig{DB: db, Cases: []chefsets.SubCase{
		{ID: "butter-dairy", Recipe: "Pfannkuchen", Remove: "Butter", Lines: line("Butter"), Allergens: []string{"dairy"}},
		{ID: "unknown-food", Recipe: "Seltsam", Remove: "Fantasiezutat", Lines: line("Fantasiezutat")},
	}})
	require.NoError(t, err)
	assert.Equal(t, 2, rep.Ran)
	assert.Equal(t, "pass", rep.Results[0].Outcome, rep.Results[0].Detail)
	assert.Equal(t, "no_option", rep.Results[1].Outcome)
	assert.InDelta(t, 50, metric(t, rep, "pass_rate").Value, 0.01)
	assert.Zero(t, rep.CostUSD)

	var n int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM foods WHERE name IN ('Mehl', 'Fantasiezutat') OR name LIKE 'subs %'`).Scan(&n))
	assert.Zero(t, n, "the cases ran on a copy")
}

func TestRunHelp_ReportsLatencyShapeAndStopsAtTheCap(t *testing.T) {
	set := chefsets.HelpSet{
		Recipes: []chefsets.HelpRecipe{{
			ID: "r", Name: "Soup", Portions: 2, Ingredients: []chefsets.SubLine{{Name: "Zwiebel", Grams: 100}, {Name: "Brühe", Grams: 500}},
			Steps: []string{"Chop the onion.", "Simmer the stock.", "Season and serve."},
		}},
		Questions: []chefsets.HelpQuestion{
			{Recipe: "r", Step: 0, Kind: chefhelp.KindExplainStep},
			{Recipe: "r", Step: 2, Kind: chefhelp.KindWentWrong, Problem: "too_salty"},
			{Recipe: "r", Step: 1, Kind: chefhelp.KindExplainStep},
		},
	}
	turn := toolTurn(chefhelp.Tool, `{"lines":["Cut it small and even.","Then fry it until it is soft."]}`, 900, 20)
	e := env(t, turn)
	rep, err := chefcanary.RunHelp(context.Background(), chefcanary.HelpConfig{Env: e, Set: set})
	require.NoError(t, err)
	require.Equal(t, 3, rep.Ran, rep.Summary())
	assert.Equal(t, "answered", rep.Results[0].Outcome, rep.Results[0].Detail)
	assert.Zero(t, metric(t, rep, "failed_answers").Value)
	assert.Zero(t, metric(t, rep, "answers_breaking_the_line_rule").Value)
	assert.Zero(t, metric(t, rep, "answers_asked_twice").Value)
	assert.InDelta(t, 0.9, metric(t, rep, "cost_per_help.p50").Value, 0.001)
	assert.NotNil(t, metric(t, rep, "latency.p90").Met)

	e = env(t, turn)
	e.SpendCap = 1
	rep, err = chefcanary.RunHelp(context.Background(), chefcanary.HelpConfig{Env: e, Set: set})
	require.NoError(t, err)
	assert.True(t, rep.StoppedBySpendCap)
	assert.Equal(t, 2, rep.Ran)
}

// pageFetcher serves the recorded DATA-3 pages under their URLs.
type pageFetcher struct{ t *testing.T }

func (f pageFetcher) Fetch(_ context.Context, u string) (string, string, error) {
	pages := map[string]string{
		"https://www.bbcgoodfood.com/recipes/chicken-curry": "en_chicken_curry_1.html",
		"https://www.budgetbytes.com/chicken-curry/":        "en_chicken_curry_2.html",
		"https://www.thewoksoflife.com/chicken-curry/":      "en_chicken_curry_3.html",
	}
	name, ok := pages[u]
	if !ok {
		return "", "", errors.New("not recorded")
	}
	b, err := os.ReadFile(filepath.Join("..", "adapters", "jsonld", "testdata", "finds", name))
	require.NoError(f.t, err)
	return string(b), u, nil
}

// matchAll matches every line to a catalogue food.
type matchAll struct{}

func (matchAll) Match(_ context.Context, req food.MatchRequest) (*food.MatchResult, error) {
	res := &food.MatchResult{}
	for i, l := range req.Lines {
		id := int64(100 + i)
		res.Lines = append(res.Lines, food.LineMatch{Status: food.MatchMatched, Name: l.Name, Pick: 0, Candidates: []food.Candidate{{Name: l.Name, ExistingID: &id}}})
	}
	return res, nil
}

func findsTurns() []fake.Turn {
	return []fake.Turn{
		toolTurn(finds.SearchTool, `{"urls":["https://www.bbcgoodfood.com/recipes/chicken-curry","https://www.budgetbytes.com/chicken-curry/","https://www.thewoksoflife.com/chicken-curry/"]}`, 900, 100),
		toolTurn(finds.AdaptTool, `{"name":"Mild curry","steps":["Fry the onion until soft, about 5 minutes.","Add the paste and the rest, simmer 20 minutes."],"swaps":[],"requirements":{"equipment":["stovetop"],"active_minutes":25}}`, 900, 100),
	}
}

func TestRunFinds_ReportsRatesCostLatencyAndSearches(t *testing.T) {
	e := env(t, findsTurns()...)
	reqs := []chefsets.FindRequest{
		{ID: "indian-mild", Dish: "chicken curry", Theme: "indian", Constraint: "mild_child", RatherNot: []string{"spicy"}},
		{ID: "indian-30", Dish: "chicken curry", Theme: "indian", Constraint: "max_30min", MaxMinutes: 30},
	}
	rep, err := chefcanary.RunFinds(context.Background(), chefcanary.FindsConfig{Env: e, Requests: reqs, Fetcher: pageFetcher{t}, Matcher: matchAll{}})
	require.NoError(t, err)
	require.Equal(t, 2, rep.Ran, rep.Summary())
	for _, r := range rep.Results {
		assert.Equal(t, "ready", r.Outcome, r.Detail)
		assert.Contains(t, r.Detail, "unmatched=0")
	}
	assert.InDelta(t, 100, metric(t, rep, "ready_rate").Value, 0.01)
	assert.InDelta(t, 100, metric(t, rep, "ready_rate.theme.indian").Value, 0.01)
	assert.InDelta(t, 100, metric(t, rep, "ready_rate.constraint.max_30min").Value, 0.01)
	assert.Zero(t, metric(t, rep, "ready_finds_with_unmatched_lines").Value)
	assert.True(t, *metric(t, rep, "ready_finds_with_unmatched_lines").Met)
	assert.Positive(t, metric(t, rep, "cost_per_find.p50").Value)
	assert.GreaterOrEqual(t, metric(t, rep, "cost_per_find.p90").Value, metric(t, rep, "cost_per_find.p50").Value)
	assert.NotNil(t, metric(t, rep, "latency.p90"))
	assert.Contains(t, rep.Summary(), "cost_per_find.p90")
}

func TestRunFinds_StopsAtTheSpendCap(t *testing.T) {
	e := env(t, findsTurns()...)
	e.SpendCap = 1 // one find costs two calls of $0.90
	reqs := []chefsets.FindRequest{
		{ID: "a", Dish: "chicken curry", Theme: "indian", Constraint: "vegetarian"},
		{ID: "b", Dish: "chicken curry", Theme: "indian", Constraint: "german"},
	}
	rep, err := chefcanary.RunFinds(context.Background(), chefcanary.FindsConfig{Env: e, Requests: reqs, Fetcher: pageFetcher{t}, Matcher: matchAll{}})
	require.NoError(t, err)
	assert.True(t, rep.StoppedBySpendCap)
	assert.Equal(t, 1, rep.Ran)
	assert.Equal(t, 2, rep.Cases)
}
