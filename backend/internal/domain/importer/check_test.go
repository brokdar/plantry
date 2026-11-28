package importer_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
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
	// Chicken browned in step 1, then a coconut simmer with a stated time.
	"curry": {
		Name:               "Hähnchencurry",
		RecipeYieldNumber:  2,
		RecipeIngredient:   []string{"400 g Hähnchenbrust", "400 ml Kokosmilch", "2 EL Currypaste"},
		RecipeInstructions: []string{"Hähnchen in Stücken anbraten, bis es durch ist.", "Kokosmilch angießen und 10 Minuten köcheln lassen."},
		PrepMinutes:        10, CookMinutes: 20,
	},
	// A stock step is missing between the onions and the finish.
	"risotto": {
		Name:               "Risotto",
		RecipeYieldNumber:  2,
		RecipeIngredient:   []string{"200 g Reis", "1 Zwiebel", "Gemüsebrühe"},
		RecipeInstructions: []string{"Zwiebel und Reis andünsten.", "Mit Käse servieren."},
		PrepMinutes:        5, CookMinutes: 25,
	},
	// Cups with no grams.
	"cups": {
		Name:               "Pancakes",
		RecipeYieldNumber:  4,
		RecipeIngredient:   []string{"2 cups flour", "1 cup milk", "2 eggs"},
		RecipeInstructions: []string{"Mix everything.", "Fry 3 minutes per side."},
		PrepMinutes:        10, CookMinutes: 15,
	},
	"volumes": {
		Name:               "Volumen",
		RecipeYieldNumber:  2,
		RecipeIngredient:   []string{"300 ml Milch", "350 ml Wasser", "500 ml Hackfleisch", "1000 ml Gemüsebrühe"},
		RecipeInstructions: []string{"Alles mischen."},
		PrepMinutes:        10, CookMinutes: 15,
	},
	// An oven temperature in the step, and a plural onion.
	"oven_chicken": {
		Name:               "Ofenhähnchen",
		RecipeYieldNumber:  2,
		RecipeIngredient:   []string{"4 Hähnchenschenkel", "Zwiebeln", "Wasser"},
		RecipeInstructions: []string{"Hähnchenschenkel bei 200 °C im Ofen backen.", "Zwiebel anbraten.", "Wasser kochen."},
		PrepMinutes:        5, CookMinutes: 40,
	},
	"oven_chicken_en": {
		Name:               "Oven chicken",
		RecipeYieldNumber:  2,
		RecipeIngredient:   []string{"4 chicken thighs", "1 tbsp oil"},
		RecipeInstructions: []string{"Bake the chicken thighs at 200 °C in the oven."},
		PrepMinutes:        5, CookMinutes: 40,
	},
	// Protein in a step that does not cook, and one that states its core temperature.
	"cue_skips": {
		Name:               "Zitronenhähnchen",
		RecipeYieldNumber:  2,
		RecipeIngredient:   []string{"400 g Hähnchenbrust", "1 Zitrone", "1 EL Öl"},
		RecipeInstructions: []string{"Hähnchen, Zitrone und Öl zugeben.", "Hähnchen braten, bis die Kerntemperatur 74 °C erreicht ist."},
		PrepMinutes:        5, CookMinutes: 20,
	},
	"english_chicken": {
		Name:               "Fried chicken",
		Description:        "A crispy chicken dish for 2.",
		RecipeYieldNumber:  2,
		RecipeIngredient:   []string{"400 g chicken breast", "2 cloves garlic"},
		RecipeInstructions: []string{"Fry the chicken for 20 minutes.", "Serve the chicken with the rice."},
		PrepMinutes:        5, CookMinutes: 20,
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
	// chicken, a step that drops its number, a
	// given amount, an ingredient.
	r := newCheckRig(t, plantedRecipes["missing_times_raw_chicken"], run, checkTurn(`{
		"keep":{"days":6,"freezable":false},
		"role":{"role":"gourmet"},
		"step_edits":[
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
	require.Len(t, check.Changes, 2)
	assert.Equal(t, importer.ChangeUnit, check.Changes[0].Kind)
	assert.Equal(t, 254.0, d.Ingredients[0].Amount, "480 ml flour by its density, not the model's 250")
	assert.Equal(t, "g", d.Ingredients[0].Unit)
	assert.Equal(t, 247.0, d.Ingredients[1].Amount, "240 ml milk by its density, not the model's 20")
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

// edited runs one step edit through the check and returns the draft's steps
// and the check.
func edited(t *testing.T, recipe, edit string) ([]string, *importer.Check) {
	t.Helper()
	r := newCheckRig(t, plantedRecipes[recipe], run, checkTurn(`{"step_edits":[`+edit+`]}`))
	d := r.draft(t)
	check, err := r.svc.Check(context.Background(), d, "de")
	require.NoError(t, err)
	return d.Instructions, check
}

func TestCheck_ARewriteNeverAddsAStepOrAnIngredientMention(t *testing.T) {
	steps, check := edited(t, "risotto", `{"step":0,"kind":"clarity","reason":"the stock step is missing",
		"text":"Zwiebel und Reis andünsten, bis der Reis glasig ist. Dann nach und nach die Gemüsebrühe zugeben und rühren."}`)
	assert.Equal(t, "Zwiebel und Reis andünsten.", steps[0])
	assert.Empty(t, check.Changes)
	require.Len(t, check.Flags, 1)
	assert.Equal(t, 0, check.Flags[0].Step)
	assert.Contains(t, check.Flags[0].Text, "Schritt scheint zu fehlen")

	// One sentence, but it brings in an ingredient the step did not name.
	steps, check = edited(t, "risotto", `{"step":0,"kind":"clarity","text":"Zwiebel und Reis in Gemüsebrühe andünsten."}`)
	assert.Equal(t, "Zwiebel und Reis andünsten.", steps[0])
	assert.Empty(t, check.Changes)
	assert.Len(t, check.Flags, 1)
}

func TestCheck_ASafetyCueNeedsTheProteinInTheStepAndNeverTouchesATime(t *testing.T) {
	steps, check := edited(t, "curry", `{"step":1,"kind":"safety_cue",
		"text":"Kokosmilch angießen und 10 Minuten köcheln lassen, bis das Hähnchen innen 74 °C erreicht hat."}`)
	assert.Equal(t, "Kokosmilch angießen und 10 Minuten köcheln lassen.", steps[1])
	assert.Empty(t, check.Changes)
}

func TestCheck_ASafetyCueIsTheStepPlusAFixedNote(t *testing.T) {
	// The model's own wording ("bis zu 70 °C") is never used.
	want := "Hähnchen in Stücken anbraten, bis es durch ist (Kerntemperatur mindestens 74 °C)."
	steps, check := edited(t, "curry", `{"step":0,"kind":"safety_cue","text":"Hähnchen anbraten, bis zu 70 °C."}`)
	assert.Equal(t, want, steps[0])
	require.Len(t, check.Changes, 1)
	assert.Equal(t, want, check.Changes[0].Edits[0].After)
}

func TestCheck_ASafetyCueKeepsTheOvenTemperature(t *testing.T) {
	want := "Hähnchenschenkel bei 200 °C im Ofen backen (Kerntemperatur mindestens 74 °C)."
	steps, check := edited(t, "oven_chicken", `{"step":0,"kind":"safety_cue","text":"x"}`)
	assert.Equal(t, want, steps[0])
	assert.Len(t, check.Changes, 1)

	r := newCheckRig(t, plantedRecipes["oven_chicken_en"], run,
		checkTurn(`{"step_edits":[{"step":0,"kind":"safety_cue","text":"x"}]}`))
	d := r.draft(t)
	check, err := r.svc.Check(context.Background(), d, "en")
	require.NoError(t, err)
	assert.Equal(t, "Bake the chicken thighs at 200 °C in the oven (core temperature at least 74 °C).", d.Instructions[0])
	assert.Len(t, check.Changes, 1)
}

func TestCheck_ASafetyCueSkipsANonCookingStepAndOneWithACoreTemperature(t *testing.T) {
	for step := range 2 {
		steps, check := edited(t, "cue_skips", fmt.Sprintf(`{"step":%d,"kind":"safety_cue","text":"Hähnchen braten, 74 °C."}`, step))
		assert.Equal(t, plantedRecipes["cue_skips"].RecipeInstructions[step], steps[step])
		assert.Empty(t, check.Changes)
	}
}

func TestCheck_AClarityRewriteIsOnlyAFlag(t *testing.T) {
	steps, check := edited(t, "oven_chicken", `{"step":0,"kind":"clarity","reason":"zu lang","text":"Hähnchenschenkel im Ofen backen. Die Temperatur beträgt 200 °C."},
		{"step":0,"kind":"clarity","text":"Hähnchenschenkel bei 200 °C backen."}`)
	assert.Equal(t, plantedRecipes["oven_chicken"].RecipeInstructions[0], steps[0], "the step stays as written")
	assert.Empty(t, check.Changes)
	require.Len(t, check.Flags, 1, "one flag per step")
	assert.True(t, check.Flags[0].Clarity)
	assert.Equal(t, 0, check.Flags[0].Step)
	assert.Equal(t, "Könnte klarer sein: Hähnchenschenkel im Ofen backen. Die Temperatur beträgt 200 °C.", check.Flags[0].Text)

	// A new ingredient in the extra sentence is the missing-step flag, once, without the model's reason.
	_, check = edited(t, "oven_chicken", `{"step":1,"kind":"clarity","reason":"missing","text":"Zwiebel anbraten. Dann Wasser zugeben."},
		{"step":1,"kind":"clarity","reason":"missing","text":"Zwiebel anbraten. Dann Wasser zugeben und kochen."}`)
	require.Len(t, check.Flags, 1)
	assert.Equal(t, "Ein Schritt scheint zu fehlen", check.Flags[0].Text)
	assert.False(t, check.Flags[0].Clarity)
}

func TestCheck_IngredientMentionsCompareByStem(t *testing.T) {
	steps, check := edited(t, "oven_chicken", `{"step":1,"kind":"clarity","text":"Zwiebeln anbraten."}`)
	assert.Equal(t, "Zwiebel anbraten.", steps[1])
	require.Len(t, check.Flags, 1)
	assert.True(t, check.Flags[0].Clarity)
	assert.Empty(t, check.Changes)
}

func TestCheck_ASafetyCueNeedsAWholeCookingWord(t *testing.T) {
	for _, step := range []string{
		"Hähnchen mit Petersilie garnieren.", "Hähnchen mit Kochsalz würzen.", "Serve the chicken with garlic bread.",
		"Hähnchen in die Bratensauce legen.", "Hähnchen aus dem Backofen nehmen.", "Chicken with sugar, then search the pan.",
		"Hähnchen auf das Kochfeld stellen.",
	} {
		_, ok := importer.CueText(step, "de")
		assert.False(t, ok, step)
	}
	for _, step := range []string{
		"Hähnchen anbraten.", "Das Hähnchen gebraten servieren.", "Hähnchen garen.", "Hähnchen durchgaren.", "Hähnchen kochen.",
		"Hähnchen backen.", "Cook the chicken.", "Pan-fry the chicken.", "Roast chicken thighs.", "Simmer the chicken in broth.",
	} {
		_, ok := importer.CueText(step, "de")
		assert.True(t, ok, step)
	}
}

func TestCheck_ACueSkipsAStepWithACoreTemperatureInOtherWords(t *testing.T) {
	for _, step := range []string{
		"Hähnchen braten, bis der Kern 74 °C hat.", "Hähnchen braten bis zur Innentemperatur von 74 °C.",
		"Roast the chicken to an internal temperature of 74 °C.",
	} {
		_, ok := importer.CueText(step, "de")
		assert.False(t, ok, step)
	}
}

func TestCheck_ACueComesAfterTheTranslationInTheFinalLanguage(t *testing.T) {
	r := newCheckRig(t, plantedRecipes["english_chicken"], run, checkTurn(`{
		"step_edits":[{"step":0,"kind":"safety_cue","text":"x"}],
		"translation":{"name":"Hähnchen","steps":["Das Hähnchen 20 Minuten braten.","Das Hähnchen mit dem Reis servieren."]}}`))
	d := r.draft(t)
	require.Equal(t, "en", d.Language)
	check, err := r.svc.Check(context.Background(), d, "de")
	require.NoError(t, err)
	assert.Equal(t, "Das Hähnchen 20 Minuten braten (Kerntemperatur mindestens 74 °C).", d.Instructions[0])
	assert.Equal(t, "Das Hähnchen mit dem Reis servieren.", d.Instructions[1])
	require.Len(t, check.Changes, 2)
	assert.Equal(t, importer.ChangeTranslate, check.Changes[0].Kind)
	assert.Equal(t, importer.ChangeSafetyCue, check.Changes[1].Kind)

	// Untranslated, the cue is in the recipe's own language.
	r = newCheckRig(t, plantedRecipes["english_chicken"], run, checkTurn(`{"step_edits":[{"step":0,"kind":"safety_cue","text":"x"}]}`))
	d = r.draft(t)
	_, err = r.svc.Check(context.Background(), d, "de")
	require.NoError(t, err)
	assert.Equal(t, "Fry the chicken for 20 minutes (core temperature at least 74 °C).", d.Instructions[0])
}

func TestCheck_AUnitChangeIsComputedFromTheDensity(t *testing.T) {
	r := newCheckRig(t, plantedRecipes["volumes"], run, checkTurn(`{"amounts":[
		{"ingredient":0,"kind":"unit","amount":300,"unit":"g"},
		{"ingredient":1,"kind":"unit","amount":999,"unit":"g"},
		{"ingredient":2,"kind":"unit","amount":250,"unit":"g"},
		{"ingredient":3,"kind":"unit","amount":10,"unit":"g"}]}`))
	d := r.draft(t)
	check, err := r.svc.Check(context.Background(), d, "en")
	require.NoError(t, err)
	assert.Equal(t, 309.0, d.Ingredients[0].Amount, "milk at 1.03 g/ml, not the model's 300")
	assert.Equal(t, 350.0, d.Ingredients[1].Amount, "water, not the model's 999")
	assert.Equal(t, 500.0, d.Ingredients[2].Amount, "minced meat has no density: unchanged")
	assert.Equal(t, "ml", d.Ingredients[2].Unit)
	assert.Equal(t, 1000.0, d.Ingredients[3].Amount, "broth, not the model's 10")
	assert.Len(t, check.Changes, 3)
	require.Len(t, check.Flags, 1)
	assert.Equal(t, 2, check.Flags[0].Ingredient)
	assert.Contains(t, check.Flags[0].Text, "given as volume")
}

func TestCheck_AMissingStepFlagReplacesAClarityFlagOfTheSameStep(t *testing.T) {
	_, check := edited(t, "oven_chicken", `{"step":1,"kind":"clarity","text":"Zwiebel gut anbraten."},
		{"step":1,"kind":"clarity","text":"Zwiebel anbraten. Dann Wasser zugeben."}`)
	require.Len(t, check.Flags, 1)
	assert.Equal(t, "Ein Schritt scheint zu fehlen", check.Flags[0].Text)
	assert.False(t, check.Flags[0].Clarity)
}

func TestCheck_ATranslatedStepKeepsNoSourceLanguageClarityFlag(t *testing.T) {
	r := newCheckRig(t, plantedRecipes["english"], run, checkTurn(`{
		"step_edits":[{"step":1,"kind":"clarity","text":"Stir the cream in well."},{"step":0,"kind":"clarity","text":"Simmer the tomatoes for 20 minutes, covered."}],
		"translation":{"name":"Tomatensuppe","steps":["Die Tomaten 20 Minuten köcheln lassen.","Die Sahne einrühren."]}}`))
	d := r.draft(t)
	check, err := r.svc.Check(context.Background(), d, "de")
	require.NoError(t, err)
	assert.Empty(t, check.Flags, "both steps were translated")
}
