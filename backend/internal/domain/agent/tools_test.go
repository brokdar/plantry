package agent_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/agent"
	"github.com/jaltszeimer/plantry/backend/internal/domain/component"
	"github.com/jaltszeimer/plantry/backend/internal/domain/ingredient"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planner"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
	"github.com/jaltszeimer/plantry/backend/internal/domain/slot"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
)

type toolFixture struct {
	db          *sql.DB
	tools       *agent.ToolSet
	slotID      int64
	componentID int64
	altCompID   int64
	weekID      int64
}

func newToolFixture(t *testing.T) *toolFixture {
	t.Helper()
	ctx := context.Background()
	db := testhelper.NewTestDB(t)

	ingRepo := sqlite.NewIngredientRepo(db)
	compRepo := sqlite.NewComponentRepo(db)
	slotRepo := sqlite.NewSlotRepo(db)
	plateRepo := sqlite.NewPlateRepo(db)
	weekRepo := sqlite.NewWeekRepo(db)
	profileRepo := sqlite.NewProfileRepo(db)
	txRunner := sqlite.NewTxRunner(db)

	compSvc := component.NewService(compRepo, ingRepo, ingRepo)
	slotSvc := slot.NewService(slotRepo)
	plateSvc := plate.NewService(plateRepo, slotRepo, compRepo)
	plannerSvc := planner.NewService(weekRepo, plateRepo, txRunner)
	profileSvc := profile.NewService(profileRepo)

	ing := &ingredient.Ingredient{Name: "Chicken", Source: "manual", Kcal100g: 165, Protein100g: 31}
	require.NoError(t, ingRepo.Create(ctx, ing))

	curry := &component.Component{
		Name: "Chicken curry", Role: component.RoleMain, ReferencePortions: 2,
		Tags: []string{"spicy"},
		Ingredients: []component.ComponentIngredient{
			{IngredientID: ing.ID, Amount: 400, Unit: "g", Grams: 400},
		},
		Instructions: []component.Instruction{{StepNumber: 1, Text: "Cook"}},
	}
	require.NoError(t, compSvc.Create(ctx, curry))

	rice := &component.Component{
		Name: "Basmati", Role: component.RoleSideStarch, ReferencePortions: 2,
		Ingredients: []component.ComponentIngredient{
			{IngredientID: ing.ID, Amount: 200, Unit: "g", Grams: 200},
		},
	}
	require.NoError(t, compSvc.Create(ctx, rice))

	ts := &slot.TimeSlot{NameKey: "slot.dinner", Icon: "moon", SortOrder: 2, Active: true}
	require.NoError(t, slotSvc.Create(ctx, ts))

	week, err := plannerSvc.Current(ctx, time.Now().UTC())
	require.NoError(t, err)

	tools, err := agent.NewToolSet(agent.Services{
		Components: compSvc, Planner: plannerSvc, Plates: plateSvc,
		Profile: profileSvc, Slots: slotSvc, Ingredient: ingRepo,
	})
	require.NoError(t, err)

	return &toolFixture{
		db: db, tools: tools,
		slotID: ts.ID, componentID: curry.ID, altCompID: rice.ID, weekID: week.ID,
	}
}

func runTool(t *testing.T, ts *agent.ToolSet, name string, input string) (json.RawMessage, agent.ToolEffect) {
	t.Helper()
	out, effect, err := ts.Execute(context.Background(), name, json.RawMessage(input))
	require.NoError(t, err, "tool %s failed", name)
	return out, effect
}

func runToolErr(t *testing.T, ts *agent.ToolSet, name string, input string) error {
	t.Helper()
	_, _, err := ts.Execute(context.Background(), name, json.RawMessage(input))
	return err
}

func TestToolSet_Describe_CoversExpectedTools(t *testing.T) {
	f := newToolFixture(t)
	want := map[string]bool{
		"list_components": true, "get_component": true, "list_slots": true,
		"get_week": true, "get_week_nutrition": true, "get_profile": true,
		"create_plate": true, "add_component_to_plate": true, "swap_component": true,
		"update_plate_component": true, "remove_plate_component": true,
		"delete_plate": true, "clear_week": true, "record_preference": true,
	}
	got := map[string]bool{}
	for _, t := range f.tools.Describe() {
		got[t.Name] = true
	}
	assert.Equal(t, want, got)
}

func TestToolSet_UnknownTool(t *testing.T) {
	f := newToolFixture(t)
	err := runToolErr(t, f.tools, "does_not_exist", `{}`)
	assert.True(t, errors.Is(err, agent.ErrToolNotFound))
}

func TestToolSet_SchemaViolationSurfacesAsInvalidInput(t *testing.T) {
	f := newToolFixture(t)
	err := runToolErr(t, f.tools, "create_plate", `{}`)
	assert.True(t, errors.Is(err, domain.ErrInvalidInput))

	err = runToolErr(t, f.tools, "create_plate", `{"week_id":1,"day":0,"slot_id":"no"}`)
	assert.True(t, errors.Is(err, domain.ErrInvalidInput))

	err = runToolErr(t, f.tools, "create_plate", `{"week_id":1,"day":9,"slot_id":1}`)
	assert.True(t, errors.Is(err, domain.ErrInvalidInput))
}

func TestTool_ListComponents(t *testing.T) {
	f := newToolFixture(t)
	out, _ := runTool(t, f.tools, "list_components", `{}`)
	var res struct {
		Items []map[string]any `json:"items"`
		Total int64            `json:"total"`
	}
	require.NoError(t, json.Unmarshal(out, &res))
	assert.EqualValues(t, 2, res.Total)
	assert.Len(t, res.Items, 2)

	out, _ = runTool(t, f.tools, "list_components", `{"role":"main"}`)
	require.NoError(t, json.Unmarshal(out, &res))
	assert.EqualValues(t, 1, res.Total)
}

func TestTool_GetComponent(t *testing.T) {
	f := newToolFixture(t)
	out, _ := runTool(t, f.tools, "get_component", jsonMust(map[string]any{"component_id": f.componentID}))
	var m map[string]any
	require.NoError(t, json.Unmarshal(out, &m))
	assert.Equal(t, "Chicken curry", m["name"])
	assert.Equal(t, "main", m["role"])
	assert.Len(t, m["ingredients"], 1)
}

func TestTool_GetComponent_NotFound(t *testing.T) {
	f := newToolFixture(t)
	err := runToolErr(t, f.tools, "get_component", `{"component_id":9999}`)
	assert.True(t, errors.Is(err, domain.ErrNotFound))
}

func TestTool_ListSlots(t *testing.T) {
	f := newToolFixture(t)
	out, _ := runTool(t, f.tools, "list_slots", `{}`)
	var res struct {
		Items []map[string]any `json:"items"`
	}
	require.NoError(t, json.Unmarshal(out, &res))
	require.Len(t, res.Items, 1)
	assert.Equal(t, "slot.dinner", res.Items[0]["name_key"])
}

func TestTool_GetWeek_CurrentByDefault(t *testing.T) {
	f := newToolFixture(t)
	out, _ := runTool(t, f.tools, "get_week", `{}`)
	var m map[string]any
	require.NoError(t, json.Unmarshal(out, &m))
	assert.NotZero(t, m["id"])
	assert.NotNil(t, m["plates"])
}

func TestTool_CreatePlate_AddSwapRemoveDelete_EffectsAndPersistence(t *testing.T) {
	f := newToolFixture(t)

	out, effect := runTool(t, f.tools, "create_plate",
		jsonMust(map[string]any{"week_id": f.weekID, "day": 1, "slot_id": f.slotID}))
	assert.Equal(t, agent.ToolEffectPlateChanged, effect)
	var plateResp map[string]any
	require.NoError(t, json.Unmarshal(out, &plateResp))
	plateID := int64(plateResp["id"].(float64))
	assert.NotZero(t, plateID)

	out, effect = runTool(t, f.tools, "add_component_to_plate",
		jsonMust(map[string]any{"plate_id": plateID, "component_id": f.componentID, "portions": 2}))
	assert.Equal(t, agent.ToolEffectPlateChanged, effect)
	var pcResp map[string]any
	require.NoError(t, json.Unmarshal(out, &pcResp))
	pcID := int64(pcResp["id"].(float64))
	assert.EqualValues(t, f.componentID, pcResp["component_id"])

	_, effect = runTool(t, f.tools, "swap_component",
		jsonMust(map[string]any{"plate_component_id": pcID, "new_component_id": f.altCompID}))
	assert.Equal(t, agent.ToolEffectPlateChanged, effect)

	_, effect = runTool(t, f.tools, "update_plate_component",
		jsonMust(map[string]any{"plate_component_id": pcID, "portions": 3}))
	assert.Equal(t, agent.ToolEffectPlateChanged, effect)

	_, effect = runTool(t, f.tools, "remove_plate_component",
		jsonMust(map[string]any{"plate_component_id": pcID}))
	assert.Equal(t, agent.ToolEffectPlateChanged, effect)

	_, effect = runTool(t, f.tools, "delete_plate", jsonMust(map[string]any{"plate_id": plateID}))
	assert.Equal(t, agent.ToolEffectPlateChanged, effect)

	out, _ = runTool(t, f.tools, "get_week", jsonMust(map[string]any{"week_id": f.weekID}))
	require.NoError(t, json.Unmarshal(out, &plateResp))
	plates := plateResp["plates"].([]any)
	assert.Empty(t, plates)
}

func TestTool_ClearWeek(t *testing.T) {
	f := newToolFixture(t)
	ctx := context.Background()

	_, _, err := f.tools.Execute(ctx, "create_plate",
		json.RawMessage(jsonMust(map[string]any{"week_id": f.weekID, "day": 0, "slot_id": f.slotID})))
	require.NoError(t, err)
	_, _, err = f.tools.Execute(ctx, "create_plate",
		json.RawMessage(jsonMust(map[string]any{"week_id": f.weekID, "day": 1, "slot_id": f.slotID})))
	require.NoError(t, err)

	out, effect := runTool(t, f.tools, "clear_week", jsonMust(map[string]any{"week_id": f.weekID}))
	assert.Equal(t, agent.ToolEffectPlateChanged, effect)
	var res struct {
		Removed int `json:"removed"`
	}
	require.NoError(t, json.Unmarshal(out, &res))
	assert.Equal(t, 2, res.Removed)

	out, effect = runTool(t, f.tools, "clear_week", jsonMust(map[string]any{"week_id": f.weekID}))
	assert.Equal(t, agent.ToolEffectNone, effect)
	require.NoError(t, json.Unmarshal(out, &res))
	assert.Equal(t, 0, res.Removed)
}

func TestTool_CreatePlate_UnknownSlot(t *testing.T) {
	f := newToolFixture(t)
	err := runToolErr(t, f.tools, "create_plate",
		jsonMust(map[string]any{"week_id": f.weekID, "day": 0, "slot_id": 9999}))
	assert.True(t, errors.Is(err, domain.ErrSlotUnknown))
}

func TestTool_AddComponent_UnknownComponent(t *testing.T) {
	f := newToolFixture(t)
	out, _ := runTool(t, f.tools, "create_plate",
		jsonMust(map[string]any{"week_id": f.weekID, "day": 0, "slot_id": f.slotID}))
	var m map[string]any
	require.NoError(t, json.Unmarshal(out, &m))
	plateID := int64(m["id"].(float64))

	err := runToolErr(t, f.tools, "add_component_to_plate",
		jsonMust(map[string]any{"plate_id": plateID, "component_id": 9999}))
	assert.True(t, errors.Is(err, domain.ErrNotFound))
}

func TestTool_GetProfile(t *testing.T) {
	f := newToolFixture(t)
	out, effect := runTool(t, f.tools, "get_profile", `{}`)
	assert.Equal(t, agent.ToolEffectNone, effect)
	var m map[string]any
	require.NoError(t, json.Unmarshal(out, &m))
	assert.Contains(t, m, "dietary_restrictions")
	assert.Contains(t, m, "preferences")
}

func TestTool_RecordPreference_PersistsIntoProfile(t *testing.T) {
	f := newToolFixture(t)
	ctx := context.Background()

	out, effect := runTool(t, f.tools, "record_preference",
		`{"key":"likes_spicy","value":true}`)
	assert.Equal(t, agent.ToolEffectNone, effect)
	var res struct {
		Recorded bool   `json:"recorded"`
		Key      string `json:"key"`
	}
	require.NoError(t, json.Unmarshal(out, &res))
	assert.True(t, res.Recorded)
	assert.Equal(t, "likes_spicy", res.Key)

	profileRepo := sqlite.NewProfileRepo(f.db)
	p, err := profileRepo.Get(ctx)
	require.NoError(t, err)
	require.Contains(t, p.Preferences, "likes_spicy")
	assert.Equal(t, true, p.Preferences["likes_spicy"])
}

func TestTool_RecordPreference_InvalidKey(t *testing.T) {
	f := newToolFixture(t)
	err := runToolErr(t, f.tools, "record_preference", `{"key":"has space","value":1}`)
	assert.True(t, errors.Is(err, domain.ErrInvalidInput))
}

func TestTool_GetWeekNutrition_ReturnsDaysAndWeek(t *testing.T) {
	f := newToolFixture(t)
	ctx := context.Background()

	out, _, err := f.tools.Execute(ctx, "create_plate",
		json.RawMessage(jsonMust(map[string]any{"week_id": f.weekID, "day": 0, "slot_id": f.slotID})))
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(out, &m))
	plateID := int64(m["id"].(float64))
	_, _, err = f.tools.Execute(ctx, "add_component_to_plate",
		json.RawMessage(jsonMust(map[string]any{"plate_id": plateID, "component_id": f.componentID, "portions": 1})))
	require.NoError(t, err)

	out, _ = runTool(t, f.tools, "get_week_nutrition", jsonMust(map[string]any{"week_id": f.weekID}))
	var res struct {
		WeekID int64            `json:"week_id"`
		Days   []map[string]any `json:"days"`
		Week   map[string]any   `json:"week"`
	}
	require.NoError(t, json.Unmarshal(out, &res))
	assert.Equal(t, f.weekID, res.WeekID)
	require.Len(t, res.Days, 1)
	assert.EqualValues(t, 0, res.Days[0]["day"])
	// 400g chicken / 2 reference portions = 1 portion at 200g → 200g * 1.65 kcal/g = 330 kcal.
	macros := res.Days[0]["macros"].(map[string]any)
	assert.InDelta(t, 330, macros["kcal"].(float64), 0.1)
}

func jsonMust(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}

// TestToolSet_SchemasAreOpenAIStrictCompatible ensures every tool's JSON
// schema can be shipped to OpenAI's strict-mode function-calling, which rejects
// object schemas without an explicit "properties" field. Caught a real prod
// incident where get_profile had {"type":"object","additionalProperties":false}
// and OpenAI rejected the whole request with 400 invalid_request_error.
func TestToolSet_SchemasAreOpenAIStrictCompatible(t *testing.T) {
	f := newToolFixture(t)
	for _, tool := range f.tools.Describe() {
		var raw map[string]any
		require.NoError(t, json.Unmarshal(tool.Schema, &raw),
			"tool %q schema is not valid JSON", tool.Name)

		if raw["type"] != "object" {
			continue
		}
		_, hasProps := raw["properties"]
		assert.True(t, hasProps,
			"tool %q has type=object but no properties field; OpenAI strict mode will reject it", tool.Name)
	}
}
