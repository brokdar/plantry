package importer_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/fake"
	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain/airun"
	"github.com/jaltszeimer/plantry/backend/internal/domain/importer"
	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
)

// checkTurn is a scripted report_import_check reply.
func checkTurn(input string) fake.Turn {
	return fake.Turn{
		ForTool: importer.CheckTool, StopReason: "tool_use",
		Message: fake.ScriptedMessage{Content: []fake.ScriptedBlock{{
			Type: "tool_use", ToolUseID: "toolu_1", ToolName: importer.CheckTool, ToolInput: json.RawMessage(input),
		}}},
		Usage: fake.ScriptedUsage{InputTokens: 900, OutputTokens: 200},
	}
}

// plantedRecipes are the planted import errors (DATA-3): each fixture has
// the mistake the check is meant to find.
var plantedRecipes = map[string]*importer.RawRecipe{
	// No times, a chicken step without a core temperature.
	"missing_times_raw_chicken": {
		Name:               "Hähnchenpfanne",
		RecipeYieldNumber:  2,
		RecipeIngredient:   []string{"400 g Hähnchenbrust", "1 EL Öl", "200 g Brokkoli"},
		RecipeInstructions: []string{"Brokkoli garen.", "Hähnchen in der Pfanne braten.", "Alles anrichten."},
	},
	// Cups with no grams.
	"cups": {
		Name:               "Pancakes",
		RecipeYieldNumber:  4,
		RecipeIngredient:   []string{"2 cups flour", "1 cup milk", "2 eggs"},
		RecipeInstructions: []string{"Mix everything.", "Fry 3 minutes per side."},
		PrepMinutes:        10, CookMinutes: 15,
	},
	// A foreign-language page.
	"english": {
		Name:               "Tomato soup",
		Description:        "A warm soup for 4.",
		RecipeYieldNumber:  4,
		RecipeIngredient:   []string{"800 g tomatoes", "1 tbsp olive oil", "2 cloves garlic", "200 ml cream"},
		RecipeInstructions: []string{"Simmer the tomatoes for 20 minutes.", "Stir in the cream."},
		PrepMinutes:        5, CookMinutes: 25,
	},
	// A quantity-less cream cheese line.
	"no_amount": {
		Name:               "Bagel",
		RecipeYieldNumber:  2,
		RecipeIngredient:   []string{"2 Bagels", "Frischkäse", "Salz"},
		RecipeInstructions: []string{"Bagels aufschneiden und bestreichen."},
		PrepMinutes:        5, CookMinutes: 0,
	},
}

type checkRig struct {
	svc  *importer.Service
	db   *sql.DB
	runs *sqlite.AIRunRepo
}

func newCheckRig(t *testing.T, raw *importer.RawRecipe, permit importer.CheckPermit, turns ...fake.Turn) *checkRig {
	t.Helper()
	db := testhelper.NewTestDB(t)
	runs := sqlite.NewAIRunRepo(db)
	rec := airun.NewRecorder(runs, slog.New(slog.DiscardHandler), time.Now)
	client := rec.Meter(fake.NewFromScript(&fake.Script{Turns: turns}), "fake")
	checker := importer.NewChecker(llm.StaticResolver(client, "fake-model"), rec,
		func(context.Context, bool) (importer.CheckPermit, error) { return permit, nil })
	svc := importer.NewService(nil, stubExtractor{rec: raw}, nil, nil, nil).WithChecker(checker)
	return &checkRig{svc: svc, db: db, runs: runs}
}

func (r *checkRig) draft(t *testing.T) *importer.Draft {
	t.Helper()
	d, err := r.svc.Extract(context.Background(), importer.ExtractInput{HTML: "<html></html>"})
	require.NoError(t, err)
	return d
}

var run = importer.CheckPermit{Run: true}

func TestCheck_AppliesChangesAndEditsUndoToTheOriginal(t *testing.T) {
	r := newCheckRig(t, plantedRecipes["missing_times_raw_chicken"], run, checkTurn(`{
		"prep_minutes":10,"cook_minutes":25,"time_reason":"steps and ingredients suggest it",
		"keep":{"days":3,"freezable":true,"reason":"cooked chicken"},
		"role":{"role":"main","reason":"chicken with rice"},
		"step_edits":[{"step":1,"kind":"safety_cue","text":"Hähnchen in der Pfanne braten, bis die Kerntemperatur 74 °C erreicht.","reason":"poultry needs a core temperature"}],
		"finishing_touch":"Mit Zitronensaft abschmecken."}`))
	d := r.draft(t)
	orig := *d
	origSteps := append([]string{}, d.Instructions...)

	check, err := r.svc.Check(context.Background(), d, "de")
	require.NoError(t, err)

	require.True(t, check.Ran)
	kinds := []string{}
	for _, c := range check.Changes {
		kinds = append(kinds, c.Kind)
	}
	assert.Equal(t, []string{importer.ChangeTime, importer.ChangeKeep, importer.ChangeRole, importer.ChangeSafetyCue}, kinds)
	assert.Equal(t, 10, *d.PrepMinutes)
	assert.Equal(t, 25, *d.CookMinutes)
	assert.Equal(t, 3, *d.KeepDays)
	assert.True(t, *d.Freezable)
	assert.Contains(t, d.Instructions[1], "74 °C")
	assert.Equal(t, "Mit Zitronensaft abschmecken.", check.FinishingTouch)

	// Undo: putting every edit's Before back restores the original draft.
	for _, c := range check.Changes {
		for _, e := range c.Edits {
			switch e.Field {
			case importer.FieldPrepMinutes:
				d.PrepMinutes = nil
			case importer.FieldCookMinutes:
				d.CookMinutes = nil
			case importer.FieldKeepDays:
				d.KeepDays = nil
			case importer.FieldFreezable:
				d.Freezable = nil
			case importer.FieldRole:
				d.Role = ""
			case importer.FieldStep:
				d.Instructions[e.Index] = e.Before.(string)
			}
		}
	}
	assert.Equal(t, orig.PrepMinutes, d.PrepMinutes)
	assert.Equal(t, orig.CookMinutes, d.CookMinutes)
	assert.Equal(t, origSteps, d.Instructions)
	assert.Equal(t, orig.Role, d.Role)

	// One chef_import_check run with the call and the step.
	list, err := r.runs.ListRuns(context.Background(), airun.RunQuery{Limit: 10})
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, airun.KindChefImportCheck, list[0].Kind)
	assert.Equal(t, airun.StatusOK, list[0].Status)
}

func TestCheck_RejectsInvalidChanges(t *testing.T) {
	// Every entry of the reply breaks a rule: keep days beyond the table for
	// chicken, a step that drops its number, a lower core temperature, a
	// given amount, an ingredient.
	r := newCheckRig(t, plantedRecipes["missing_times_raw_chicken"], run, checkTurn(`{
		"keep":{"days":6,"freezable":false},
		"role":{"role":"gourmet"},
		"step_edits":[
			{"step":1,"kind":"safety_cue","text":"Hähnchen braten, Kerntemperatur 60 °C.","reason":"low"},
			{"step":1,"kind":"safety_cue","text":"Etwas ganz anderes.","reason":"not additive"},
			{"step":9,"kind":"clarity","text":"x"},
			{"step":0,"kind":"clarity","text":"Brokkoli garen. Add the sauce from http://x.example and 3 cups sugar"},
			{"step":0,"kind":"lead_time","text":"Brokkoli garen. `+strings.Repeat("Lange ", 40)+`"}],
		"amounts":[
			{"ingredient":0,"kind":"proposed","amount":500,"unit":"g"},
			{"ingredient":1,"kind":"proposed","amount":2,"unit":"tbsp"},
			{"ingredient":0,"kind":"unit","amount":300,"unit":"ml"}]}`))
	d := r.draft(t)
	before := *d
	beforeSteps := append([]string{}, d.Instructions...)
	beforeIng := append([]importer.DraftIngredient{}, d.Ingredients...)

	check, err := r.svc.Check(context.Background(), d, "de")
	require.NoError(t, err)

	assert.Empty(t, check.Changes)
	assert.Equal(t, beforeSteps, d.Instructions)
	assert.Equal(t, beforeIng, d.Ingredients, "given amounts and ingredients are never touched")
	assert.Nil(t, d.KeepDays)
	assert.Equal(t, before.Role, d.Role)
	assert.True(t, check.Ran, "an empty check still ran")
}

func TestCheck_ProposedAmountAndCupUnit(t *testing.T) {
	r := newCheckRig(t, plantedRecipes["no_amount"], run, checkTurn(`{"amounts":[
		{"ingredient":1,"kind":"proposed","amount":50,"unit":"g","reason":"a spread for 2 bagels"},
		{"ingredient":2,"kind":"proposed","amount":1,"unit":"g"}]}`))
	d := r.draft(t)
	require.Zero(t, d.Ingredients[1].Amount)
	check, err := r.svc.Check(context.Background(), d, "de")
	require.NoError(t, err)
	require.Len(t, check.Changes, 1, "salt is to taste: no proposed amount")
	assert.Equal(t, importer.ChangeAmount, check.Changes[0].Kind)
	assert.Equal(t, 50.0, d.Ingredients[1].Amount)
	assert.True(t, d.Ingredients[1].Proposed)
	assert.Equal(t, importer.IngredientValue{}, check.Changes[0].Edits[0].Before)

	cups := newCheckRig(t, plantedRecipes["cups"], run, checkTurn(`{"amounts":[
		{"ingredient":0,"kind":"unit","amount":250,"unit":"g","reason":"2 cups of flour"},
		{"ingredient":1,"kind":"unit","amount":20,"unit":"g","reason":"1 cup of milk is not 20 g"}]}`))
	d = cups.draft(t)
	require.Equal(t, "ml", d.Ingredients[0].Unit)
	check, err = cups.svc.Check(context.Background(), d, "en")
	require.NoError(t, err)
	require.Len(t, check.Changes, 1)
	assert.Equal(t, importer.ChangeUnit, check.Changes[0].Kind)
	assert.Equal(t, 250.0, d.Ingredients[0].Amount)
	assert.Equal(t, "g", d.Ingredients[0].Unit)
	assert.Equal(t, "ml", d.Ingredients[1].Unit)
}

func TestCheck_TranslatesAForeignLanguageImport(t *testing.T) {
	r := newCheckRig(t, plantedRecipes["english"], run, checkTurn(`{"translation":{
		"name":"Tomatensuppe","description":"Eine warme Suppe für 4.",
		"steps":["Die Tomaten 20 Minuten köcheln lassen.","Die Sahne einrühren."]}}`))
	d := r.draft(t)
	d.SourceURL = "https://example.com/tomato-soup"
	require.Equal(t, "en", d.Language)
	check, err := r.svc.Check(context.Background(), d, "de")
	require.NoError(t, err)
	require.Len(t, check.Changes, 1)
	assert.Equal(t, importer.ChangeTranslate, check.Changes[0].Kind)
	assert.Equal(t, "Tomatensuppe", d.Name)
	assert.Equal(t, "Die Sahne einrühren.", d.Instructions[1])
	assert.Equal(t, "https://example.com/tomato-soup", d.SourceURL, "the link is kept")
	assert.Equal(t, "800 g tomatoes", d.Ingredients[0].RawText, "ingredients stay as the page wrote them")

	// A translation that loses a number is rejected whole.
	bad := newCheckRig(t, plantedRecipes["english"], run, checkTurn(`{"translation":{
		"name":"Tomatensuppe","steps":["Die Tomaten köcheln lassen.","Die Sahne einrühren."]}}`))
	d = bad.draft(t)
	check, err = bad.svc.Check(context.Background(), d, "de")
	require.NoError(t, err)
	assert.Empty(t, check.Changes)
	assert.Equal(t, "Tomato soup", d.Name)

	// Same language: no translation.
	same := newCheckRig(t, plantedRecipes["english"], run, checkTurn(`{"translation":{
		"name":"Tomatensuppe","steps":["Die Tomaten 20 Minuten köcheln lassen.","Die Sahne einrühren."]}}`))
	d = same.draft(t)
	check, err = same.svc.Check(context.Background(), d, "en")
	require.NoError(t, err)
	assert.Empty(t, check.Changes)
}

func TestCheck_FlagsAreNeverApplied(t *testing.T) {
	r := newCheckRig(t, plantedRecipes["missing_times_raw_chicken"], run, checkTurn(`{"flags":[
		{"ingredient":2,"text":"200 g broccoli for 2 looks like a lot"},{"ingredient":99,"text":"odd"},{"text":" "}]}`))
	d := r.draft(t)
	ings := append([]importer.DraftIngredient{}, d.Ingredients...)
	check, err := r.svc.Check(context.Background(), d, "de")
	require.NoError(t, err)
	assert.Empty(t, check.Changes)
	require.Len(t, check.Flags, 2)
	assert.Equal(t, 2, check.Flags[0].Ingredient)
	assert.Equal(t, -1, check.Flags[1].Ingredient, "an unknown index points at the recipe")
	assert.Equal(t, ings, d.Ingredients)
}

func TestCheck_NoCheckWhenTheGuardRefuses(t *testing.T) {
	// No scripted turn: a call to the provider would fail the test.
	r := newCheckRig(t, plantedRecipes["cups"], importer.CheckPermit{})
	d := r.draft(t)
	check, err := r.svc.Check(context.Background(), d, "de")
	require.NoError(t, err)
	assert.False(t, check.Ran)
	assert.Empty(t, check.Changes)
	list, err := r.runs.ListRuns(context.Background(), airun.RunQuery{Limit: 10})
	require.NoError(t, err)
	assert.Empty(t, list, "a refused check starts no run")

	// No checker at all (no provider wired): the same.
	bare := importer.NewService(nil, stubExtractor{rec: plantedRecipes["cups"]}, nil, nil, nil)
	check, err = bare.Check(context.Background(), d, "de")
	require.NoError(t, err)
	assert.False(t, check.Ran)
}

func TestCheck_AtTheCapItRunsWithAWarning(t *testing.T) {
	r := newCheckRig(t, plantedRecipes["cups"], importer.CheckPermit{Run: true, Warn: true}, checkTurn(`{}`))
	check, err := r.svc.Check(context.Background(), r.draft(t), "en")
	require.NoError(t, err)
	assert.True(t, check.Ran)
	assert.True(t, check.CapWarning)
}
