package flavor_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"regexp"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/fake"
	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/chefsets"
	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/airun"
	"github.com/jaltszeimer/plantry/backend/internal/domain/flavor"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
)

// labelTurn is a scripted set_recipe_attributes reply.
func labelTurn(input string) fake.Turn {
	return fake.Turn{
		ForTool: flavor.LabelTool, StopReason: "tool_use",
		Message: fake.ScriptedMessage{Content: []fake.ScriptedBlock{{
			Type: "tool_use", ToolUseID: "toolu_1", ToolName: flavor.LabelTool, ToolInput: json.RawMessage(input),
		}}},
		Usage: fake.ScriptedUsage{InputTokens: 1200, OutputTokens: 300},
	}
}

// newLabeler wires a Labeler over db with client behind the metered run log.
func newLabeler(db *sql.DB, client llm.Client) (*flavor.Labeler, *sqlite.AIRunRepo) {
	runs := sqlite.NewAIRunRepo(db)
	rec := airun.NewRecorder(runs, slog.New(slog.NewTextHandler(io.Discard, nil)), time.Now)
	foods := sqlite.NewFoodRepo(db)
	return flavor.NewLabeler(sqlite.NewFoodAttributeRepo(db), foods, food.NewNutritionResolver(foods),
		sqlite.NewTxRunner(db), llm.StaticResolver(rec.Meter(client, "fake"), "fake-model"), rec), runs
}

func TestLabel_ScriptedFillsAttributesAndDropsInvalid(t *testing.T) {
	ctx := context.Background()
	r := newServiceRig(t)
	creamy := r.recipe(t, "Hähnchen in Sahne", nil, map[string]float64{"Hähnchenbrust": 150, "Sahne": 100, "Zwiebeln": 50})
	spinach := r.recipe(t, "Spinat mit Tomaten", nil, map[string]float64{"Spinat": 300, "Tomaten": 100})
	client := fake.NewFromScript(&fake.Script{Turns: []fake.Turn{labelTurn(`{"recipes":[
		{"key":"r1","cuisine":"french","kind":"pan","heroes":["Hähnchenbrust","Reis"],"texture":"creamy",
		 "taste":{"acid":2,"umami":2},"richness":0,"saucy":true,"technique":"saute"},
		{"key":"r2","cuisine":"klingon","kind":"veg","taste":{"acid":1},"temperature":"warm"},
		{"key":"r9","cuisine":"thai"}
	]}`)}})
	labeler, runs := newLabeler(r.db, client)

	require.NoError(t, labeler.Label(ctx, []int64{creamy.ID, spinach.ID}))

	rec, err := r.store.Get(ctx, creamy.ID)
	require.NoError(t, err)
	assert.Equal(t, "french", rec.Values.Cuisine)
	assert.Equal(t, []string{"Hähnchenbrust"}, rec.Values.Heroes, "a hero that is not an ingredient is dropped")
	require.NotNil(t, rec.Values.Taste)
	assert.Equal(t, flavor.Taste{Umami: 2}, *rec.Values.Taste, "acid without an acid ingredient is dropped")
	assert.Nil(t, rec.Values.Richness, "richness far from the fat share is dropped")
	assert.Equal(t, flavor.SourceAuto, rec.Sources[flavor.FieldCuisine])
	_, hasRichness := rec.Sources[flavor.FieldRichness]
	assert.False(t, hasRichness)

	rec, err = r.store.Get(ctx, spinach.ID)
	require.NoError(t, err)
	assert.Empty(t, rec.Values.Cuisine, "a value outside the vocabulary is dropped")
	assert.Equal(t, "veg", rec.Values.Kind)
	assert.Equal(t, flavor.Taste{Acid: 1}, *rec.Values.Taste, "tomatoes are acidic")

	// The run log: one ok chef_attributes run, its call and what was dropped.
	rows, err := runs.ListRuns(ctx, airun.RunQuery{Kind: airun.KindChefAttributes, Limit: 10})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, airun.StatusOK, rows[0].Status)
	assert.Equal(t, flavor.LabelVersion, rows[0].PromptVersion)
	steps, err := runs.ListSteps(ctx, rows[0].ID)
	require.NoError(t, err)
	require.Len(t, steps, 2)
	assert.Equal(t, airun.StepLLMCall, steps[0].Type)
	assert.Equal(t, "chef.labels", steps[1].Name)
	assert.Contains(t, steps[1].Result, `"richness":0`)
	assert.Contains(t, steps[1].Result, `"cuisine":"klingon"`)
}

func TestLabel_UserValuesWinAndRelabelReplacesAuto(t *testing.T) {
	ctx := context.Background()
	r := newServiceRig(t)
	f := r.recipe(t, "Curry", nil, map[string]float64{"Hähnchenbrust": 300, "Tomaten": 200, "Zwiebeln": 50})
	_, _, err := r.svc.Set(ctx, f.ID, flavor.Patch{Set: flavor.Attributes{Cuisine: "thai"}, Fields: []flavor.Field{flavor.FieldCuisine}})
	require.NoError(t, err)
	labeler, _ := newLabeler(r.db, fake.NewFromScript(&fake.Script{Turns: []fake.Turn{
		labelTurn(`{"recipes":[{"key":"r1","cuisine":"indian","kind":"curry","texture":"soft"}]}`),
	}}))

	require.NoError(t, labeler.Label(ctx, []int64{f.ID}))
	v, err := r.svc.Get(ctx, f.ID)
	require.NoError(t, err)
	assert.Equal(t, "thai", v.Values.Cuisine)
	assert.Equal(t, flavor.SourceUser, v.Sources[flavor.FieldCuisine])
	assert.Equal(t, "curry", v.Values.Kind)
	assert.Equal(t, flavor.SourceAuto, v.Sources[flavor.FieldKind])

	// Labelling again replaces the auto values; the user's stays.
	relabeler, _ := newLabeler(r.db, fake.NewFromScript(&fake.Script{Turns: []fake.Turn{
		labelTurn(`{"recipes":[{"key":"r1","cuisine":"indian","kind":"stew"}]}`),
	}}))
	require.NoError(t, relabeler.Label(ctx, []int64{f.ID}))
	rec, err := r.store.Get(ctx, f.ID)
	require.NoError(t, err)
	assert.Equal(t, "thai", rec.Values.Cuisine)
	assert.Equal(t, "stew", rec.Values.Kind)
	assert.Empty(t, rec.Values.Texture, "an auto value the new labels leave out goes")
}

func TestLabel_SkipsLeavesAndMissingFoods(t *testing.T) {
	ctx := context.Background()
	r := newServiceRig(t)
	client := fake.NewFromScript(&fake.Script{Turns: []fake.Turn{labelTurn(`{"recipes":[]}`)}})
	labeler, runs := newLabeler(r.db, client)

	require.NoError(t, labeler.Label(ctx, []int64{r.leaves["Spinat"], 999999}))
	rows, err := runs.ListRuns(ctx, airun.RunQuery{Kind: airun.KindChefAttributes, Limit: 10})
	require.NoError(t, err)
	assert.Empty(t, rows, "nothing to label starts no run")
}

// oracle answers every labelling call with the DATA-1 labels of the
// recipes it lists, by name.
type oracle struct {
	set   chefsets.Set
	calls int
}

var promptKey = regexp.MustCompile(`(?m)^(r\d+): ("(?:[^"\\]|\\.)*")`)

func (o *oracle) Stream(_ context.Context, req llm.Request, out chan<- llm.Event) (*llm.Response, error) {
	defer close(out)
	o.calls++
	type entry struct {
		Key string `json:"key"`
		chefsets.Attrs
	}
	var recipes []entry
	for _, m := range promptKey.FindAllStringSubmatch(req.Messages[0].Content[0].Text, -1) {
		name, err := strconv.Unquote(m[2])
		if err != nil {
			return nil, err
		}
		recipes = append(recipes, entry{Key: m[1], Attrs: o.set.Foods[name].Attrs})
	}
	input, err := json.Marshal(map[string]any{"recipes": recipes})
	if err != nil {
		return nil, err
	}
	return &llm.Response{
		Message: llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentBlock{{
			Type: llm.ContentTypeToolUse, ToolUseID: "toolu_1", ToolUseName: req.ToolChoice.Name, ToolUseInput: input,
		}}},
		StopReason: llm.StopReasonToolUse,
		Usage:      llm.Usage{InputTokens: 2000, OutputTokens: 900},
	}, nil
}

func (o *oracle) Complete(context.Context, llm.Request) (string, error) { return "", nil }

// seedSet stores the set's ingredients as leaves and its recipes as
// composed foods; it returns the recipe ids by name.
func seedSet(t *testing.T, db *sql.DB, s chefsets.Set) map[string]int64 {
	t.Helper()
	ctx := context.Background()
	foods := sqlite.NewFoodRepo(db)
	leaves := map[string]int64{}
	for _, in := range s.Ingredients {
		src := food.SourceManual
		f := &food.Food{Kind: food.KindLeaf, Source: &src, Name: in.Name, Kcal100g: &in.Kcal, Protein100g: &in.Protein, Fat100g: &in.Fat, Carbs100g: &in.Carbs}
		if err := foods.Create(ctx, f); errors.Is(err, domain.ErrDuplicateName) {
			f.ID, err = foods.LeafIDByName(ctx, in.Name) // the rig's leaves
			require.NoError(t, err)
		} else {
			require.NoError(t, err)
		}
		leaves[in.Name] = f.ID
	}
	recipes := map[string]int64{}
	for name, sf := range s.Foods {
		if sf.Leaf {
			continue
		}
		role := food.Role(sf.Role)
		f := &food.Food{Kind: food.KindComposed, Name: name, Role: &role, ReferencePortions: &sf.Portions, Tags: sf.Tags}
		for i, ing := range sf.Ingredients {
			f.Children = append(f.Children, food.FoodComponent{ChildID: leaves[ing.Name], Amount: ing.Grams, Unit: "g", Grams: ing.Grams, SortOrder: i})
		}
		require.NoError(t, foods.Create(ctx, f))
		recipes[name] = f.ID
	}
	return recipes
}

// inlineQueue runs each queued job at once.
type inlineQueue struct {
	l    *flavor.Labelling
	jobs int
}

func (q *inlineQueue) Enqueue(ctx context.Context, kind airun.Kind, payload string, requested bool) (int64, error) {
	q.jobs++
	if kind != airun.KindChefAttributes || requested {
		return 0, assert.AnError
	}
	return int64(q.jobs), q.l.Handle(ctx, payload)
}

func (q *inlineQueue) Queued(context.Context, airun.Kind, string) (bool, error) { return false, nil }

// holdQueue keeps jobs queued until run.
type holdQueue struct{ waiting map[string]bool }

func (q *holdQueue) Enqueue(_ context.Context, _ airun.Kind, payload string, _ bool) (int64, error) {
	q.waiting[payload] = true
	return int64(len(q.waiting)), nil
}

func (q *holdQueue) Queued(_ context.Context, _ airun.Kind, payload string) (bool, error) {
	return q.waiting[payload], nil
}

type chefSwitch bool

func (c chefSwitch) ChefEnabled(context.Context) (bool, error) { return bool(c), nil }

func flatPrice(_ context.Context, in, out int64) (*float64, error) {
	v := float64(in)*3e-6 + float64(out)*15e-6
	return &v, nil
}

// AC-2: after the Chef is turned on, every composed food of the fixture has
// attributes; user-edited values survive re-labelling.
func TestLabelling_LibraryPassLabelsEveryRecipe(t *testing.T) {
	ctx := context.Background()
	set, err := chefsets.LoadDev()
	require.NoError(t, err)
	r := newServiceRig(t)
	recipes := seedSet(t, r.db, set)
	o := &oracle{set: set}
	labeler, _ := newLabeler(r.db, o)
	q := &inlineQueue{}
	labelling := flavor.NewLabelling(labeler, r.store, q, chefSwitch(true), flatPrice, 2)
	q.l = labelling

	est, err := labelling.Estimate(ctx)
	require.NoError(t, err)
	assert.Equal(t, len(recipes), est.Recipes)
	assert.Equal(t, (len(recipes)+flavor.LabelBatch-1)/flavor.LabelBatch, est.Calls)
	require.NotNil(t, est.CostUSD)
	assert.Positive(t, *est.CostUSD)
	assert.Positive(t, est.Minutes)

	jobs, err := labelling.LabelLibrary(ctx)
	require.NoError(t, err)
	assert.Equal(t, est.Calls, jobs)
	assert.Equal(t, est.Calls, o.calls)

	judgment := []flavor.Field{
		flavor.FieldCuisine, flavor.FieldKind, flavor.FieldTexture, flavor.FieldTemperature,
		flavor.FieldTaste, flavor.FieldTechnique, flavor.FieldFormat,
	}
	for name, id := range recipes {
		v, err := r.svc.Get(ctx, id)
		require.NoError(t, err)
		for _, f := range judgment {
			assert.Equal(t, flavor.SourceAuto, v.Sources[f], "%s: %s", name, f)
		}
	}
	left, err := r.store.UnlabelledRecipes(ctx)
	require.NoError(t, err)
	assert.Empty(t, left, "the library is labelled")
	again, err := labelling.LabelLibrary(ctx)
	require.NoError(t, err)
	assert.Zero(t, again, "a second pass has nothing to label")

	// A user edit survives re-labelling every recipe.
	var some int64
	ids := make([]int64, 0, len(recipes))
	for _, id := range recipes {
		ids = append(ids, id)
		some = id
	}
	_, _, err = r.svc.Set(ctx, some, flavor.Patch{Set: flavor.Attributes{Format: "wrap"}, Fields: []flavor.Field{flavor.FieldFormat}})
	require.NoError(t, err)
	for len(ids) > 0 {
		n := min(flavor.LabelBatch, len(ids))
		require.NoError(t, labeler.Label(ctx, ids[:n]))
		ids = ids[n:]
	}
	v, err := r.svc.Get(ctx, some)
	require.NoError(t, err)
	assert.Equal(t, "wrap", v.Values.Format)
	assert.Equal(t, flavor.SourceUser, v.Sources[flavor.FieldFormat])
}

func TestLabelling_SavedAndOff(t *testing.T) {
	ctx := context.Background()
	r := newServiceRig(t)
	f := r.recipe(t, "Curry", nil, map[string]float64{"Hähnchenbrust": 300, "Tomaten": 200})
	client := fake.NewFromScript(&fake.Script{Turns: []fake.Turn{labelTurn(`{"recipes":[{"key":"r1","kind":"curry"}]}`)}})
	labeler, _ := newLabeler(r.db, client)

	q := &inlineQueue{}
	off := flavor.NewLabelling(labeler, r.store, q, chefSwitch(false), flatPrice, 2)
	q.l = off
	require.NoError(t, off.Saved(ctx, f.ID))
	_, err := off.LabelLibrary(ctx)
	require.ErrorIs(t, err, flavor.ErrChefOff)
	assert.Zero(t, q.jobs, "the Chef off queues nothing")

	on := flavor.NewLabelling(labeler, r.store, q, chefSwitch(true), flatPrice, 2)
	q.l = on
	require.NoError(t, on.Saved(ctx, f.ID))
	assert.Equal(t, 1, q.jobs)
	rec, err := r.store.Get(ctx, f.ID)
	require.NoError(t, err)
	assert.Equal(t, "curry", rec.Values.Kind)
}

func TestLabelling_EstimateUnpricedAndEmpty(t *testing.T) {
	ctx := context.Background()
	r := newServiceRig(t)
	labeler, _ := newLabeler(r.db, fake.NewFromScript(&fake.Script{}))
	unpriced := func(context.Context, int64, int64) (*float64, error) { return nil, nil }
	l := flavor.NewLabelling(labeler, r.store, &inlineQueue{}, chefSwitch(true), unpriced, 2)

	est, err := l.Estimate(ctx)
	require.NoError(t, err)
	assert.Equal(t, flavor.Estimate{CostUSD: new(float64)}, est, "an empty library costs nothing")

	r.recipe(t, "Curry", nil, map[string]float64{"Hähnchenbrust": 300})
	est, err = l.Estimate(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, est.Recipes)
	assert.Nil(t, est.CostUSD, "an unpriced model has no cost estimate")
	assert.Equal(t, 1, est.Minutes)
}

func TestLabelling_SavedSkipsAlreadyQueued(t *testing.T) {
	ctx := context.Background()
	r := newServiceRig(t)
	f := r.recipe(t, "Curry", nil, map[string]float64{"Hähnchenbrust": 300, "Tomaten": 200})
	labeler, _ := newLabeler(r.db, fake.NewFromScript(&fake.Script{}))
	q := &holdQueue{waiting: map[string]bool{}}
	l := flavor.NewLabelling(labeler, r.store, q, chefSwitch(true), flatPrice, 2)

	require.NoError(t, l.Saved(ctx, f.ID))
	require.NoError(t, l.Saved(ctx, f.ID))
	assert.Len(t, q.waiting, 1, "repeat saves share the waiting job")

	clear(q.waiting) // the job ran
	require.NoError(t, l.Saved(ctx, f.ID))
	assert.Len(t, q.waiting, 1, "a save after it ran queues again")
}
