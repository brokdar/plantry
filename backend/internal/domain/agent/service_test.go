package agent_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/fake"
	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/agent"
	"github.com/jaltszeimer/plantry/backend/internal/domain/availability"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/preset"
	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
	"github.com/jaltszeimer/plantry/backend/internal/domain/settings"
	"github.com/jaltszeimer/plantry/backend/internal/domain/slot"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper/testsvc"
)

func ymd(t *testing.T, s string) time.Time {
	t.Helper()
	d, err := time.Parse("2006-01-02", s)
	require.NoError(t, err)
	return d
}

// windowSources reads the prompt's planning window from db.
func windowSources(db *sql.DB) agent.WindowSources {
	slotRepo := sqlite.NewSlotRepo(db)
	foodRepo := sqlite.NewFoodRepo(db)
	return agent.WindowSources{
		Plates:       testsvc.Plates(db),
		Slots:        slot.NewService(slotRepo),
		Foods:        foodRepo,
		Availability: availabilityService(db),
	}
}

func availabilityService(db *sql.DB) *availability.Service {
	_, svc := testsvc.Plan(db)
	return svc
}

// newPromptService wires an unrecorded service over db with the given client
// and clock.
func newPromptService(t *testing.T, db *sql.DB, client llm.Client, now func() time.Time) *agent.Service {
	t.Helper()
	return newPromptServiceWith(t, db, client, now, nil)
}

func newPromptServiceWith(t *testing.T, db *sql.DB, client llm.Client, now func() time.Time, settingsSvc agent.SettingsReader) *agent.Service {
	t.Helper()
	tools, err := agent.NewToolSet(agent.Services{Foods: food.NewService(sqlite.NewFoodRepo(db))})
	require.NoError(t, err)
	return agent.NewService(sqlite.NewAIRepo(db), llm.StaticResolver(client, chatModel), tools,
		profile.NewService(sqlite.NewProfileRepo(db)), settingsSvc, windowSources(db), now)
}

// stubSettings answers every key from a map; missing keys are empty.
type stubSettings map[string]string

func (s stubSettings) Get(_ context.Context, key string) (settings.Value, error) {
	return settings.Value{Key: key, Raw: s[key]}, nil
}

func strPtr(s string) *string { return &s }

func TestChat_SystemPromptCarriesDatesAndPlates(t *testing.T) {
	ctx := context.Background()
	db := testhelper.NewTestDB(t)
	slotRepo := sqlite.NewSlotRepo(db)
	foodRepo := sqlite.NewFoodRepo(db)
	plateRepo := sqlite.NewPlateRepo(db)

	dinner := &slot.TimeSlot{NameKey: "slot.dinner", Icon: "🍽", Active: true}
	require.NoError(t, slotRepo.Create(ctx, dinner))
	src := food.SourceManual
	pasta := &food.Food{Name: "Pasta", Kind: food.KindLeaf, Source: &src}
	require.NoError(t, foodRepo.Create(ctx, pasta))
	grams, unit, source := 100.0, "g", "direct"
	pc := plate.PlateComponent{FoodID: pasta.ID, Amount: &grams, Unit: &unit, Grams: &grams, GramsSource: &source}
	planned := &plate.Plate{Date: ymd(t, "2026-10-31"), SlotID: dinner.ID, Eaters: 1, Components: []plate.PlateComponent{pc}}
	require.NoError(t, plateRepo.Create(ctx, planned))
	avail := availabilityService(db)
	_, _, err := avail.SetOverride(ctx, availability.OverrideInput{
		Date: ymd(t, "2026-11-01"), SlotID: dinner.ID, Mode: availability.ModeNone, Note: strPtr("dinner out"),
	})
	require.NoError(t, err)
	oats := &preset.Preset{Name: "Oatmeal"}
	require.NoError(t, sqlite.NewPresetRepo(db).Create(ctx, oats))
	// Monday (weekday 0) dinners are a routine by default.
	require.NoError(t, avail.SetDefaults(ctx, []availability.Default{
		{Weekday: 0, SlotID: dinner.ID, Mode: availability.ModeRoutine, PresetID: &oats.ID},
	}))
	outside := &plate.Plate{Date: ymd(t, "2026-11-07"), SlotID: dinner.ID, Eaters: 1, Components: []plate.PlateComponent{pc}}
	require.NoError(t, plateRepo.Create(ctx, outside))

	client := fake.NewFromScript(&fake.Script{Turns: []fake.Turn{endTurn(fake.ScriptedUsage{})}})
	svc := newPromptService(t, db, client, time.Now)

	_, err = chatReq(ctx, svc, agent.ChatRequest{
		UserText: "plan dinners",
		Today:    ymd(t, "2026-10-26"),
		From:     ymd(t, "2026-10-31"),
		To:       ymd(t, "2026-11-06"),
	}, nil)
	require.NoError(t, err)

	reqs := client.Requests()
	require.NotEmpty(t, reqs)
	system := reqs[0].System
	for _, want := range []string{
		"Today is Monday, 2026-10-26.",
		"Planning window: Saturday 2026-10-31 to Friday 2026-11-06 (7 days).",
		"- Saturday 2026-10-31\n",
		"- Sunday 2026-11-01\n",
		"- Monday 2026-11-02\n",
		"- Tuesday 2026-11-03\n",
		"- Wednesday 2026-11-04\n",
		"- Thursday 2026-11-05\n",
		"- Friday 2026-11-06\n",
		fmt.Sprintf("- 2026-10-31 slot.dinner (plate_id=%d, slot_id=%d): Pasta\n", planned.ID, dinner.ID),
		"No-plate and routine cells:\n",
		fmt.Sprintf("- Sunday 2026-11-01 slot.dinner (slot_id=%d): no plate (dinner out)\n", dinner.ID),
		fmt.Sprintf("- Monday 2026-11-02 slot.dinner (slot_id=%d): routine (preset: Oatmeal)\n", dinner.ID),
	} {
		assert.Contains(t, system, want)
	}
	assert.NotContains(t, system, "2026-11-07", "plates outside the window must not be listed")
	assert.NotContains(t, system, "): skipped")
	assert.NotContains(t, system, "skipped=true")
}

func TestChat_ShoppingDayPrintsWeekdayName(t *testing.T) {
	db := testhelper.NewTestDB(t)
	client := fake.NewFromScript(&fake.Script{Turns: []fake.Turn{endTurn(fake.ScriptedUsage{})}})
	svc := newPromptServiceWith(t, db, client, time.Now, stubSettings{settings.KeyPlanShoppingDay: "5"})

	got, err := svc.DebugSystemPrompt(context.Background(), agent.ChatRequest{
		Today: ymd(t, "2026-10-26"), From: ymd(t, "2026-10-26"), To: ymd(t, "2026-10-27"),
	})
	require.NoError(t, err)
	assert.Contains(t, got, "Shopping day: Saturday\n")
	assert.NotContains(t, got, "Shopping day: 5")
}

// cellToolSet seeds a dinner slot, a "none" override on 2026-11-02 and a
// routine default on Tuesdays, and returns a tool set whose plate service
// enforces availability.
func cellToolSet(t *testing.T) (*agent.ToolSet, *sql.DB, int64, int64) {
	t.Helper()
	ctx := context.Background()
	db := testhelper.NewTestDB(t)
	slotRepo := sqlite.NewSlotRepo(db)
	foodRepo := sqlite.NewFoodRepo(db)
	dinner := &slot.TimeSlot{NameKey: "slot.dinner", Icon: "🍽", Active: true}
	require.NoError(t, slotRepo.Create(ctx, dinner))
	avail := availabilityService(db)
	_, _, err := avail.SetOverride(ctx, availability.OverrideInput{
		Date: ymd(t, "2026-11-02"), SlotID: dinner.ID, Mode: availability.ModeNone,
	})
	require.NoError(t, err)
	oats := &preset.Preset{Name: "Oatmeal"}
	require.NoError(t, sqlite.NewPresetRepo(db).Create(ctx, oats))
	require.NoError(t, avail.SetDefaults(ctx, []availability.Default{
		{Weekday: 1, SlotID: dinner.ID, Mode: availability.ModeRoutine, PresetID: &oats.ID},
	}))
	plates := testsvc.Plates(db)
	ts, err := agent.NewToolSet(agent.Services{
		Foods: food.NewService(foodRepo), Plates: plates, Slots: slot.NewService(slotRepo), Availability: avail,
	})
	require.NoError(t, err)
	return ts, db, dinner.ID, oats.ID
}

func TestTool_GetPlatesRange_IncludesAvailability(t *testing.T) {
	ts, _, dinnerID, _ := cellToolSet(t)
	ctx := context.Background()
	_, _, err := ts.Execute(ctx, "create_plate", json.RawMessage(`{"date":"2026-11-04","slot_id":`+strconv.FormatInt(dinnerID, 10)+`}`))
	require.NoError(t, err)

	out, _, err := ts.Execute(ctx, "get_plates_range", json.RawMessage(`{"from":"2026-11-02","to":"2026-11-04"}`))
	require.NoError(t, err)
	var got struct {
		Plates       []map[string]any `json:"plates"`
		Availability []map[string]any `json:"availability"`
	}
	require.NoError(t, json.Unmarshal(out, &got))
	require.Len(t, got.Plates, 1)
	assert.NotContains(t, got.Plates[0], "skipped")
	assert.Equal(t, []map[string]any{
		{"date": "2026-11-02", "slot_id": float64(dinnerID), "mode": "none"},
		{"date": "2026-11-03", "slot_id": float64(dinnerID), "mode": "routine", "preset_name": "Oatmeal"},
	}, got.Availability, "plan cells are not listed")
}

func TestTool_CreatePlateIntoNoneCell_ReturnsKeyedToolError(t *testing.T) {
	ts, db, dinnerID, _ := cellToolSet(t)
	input := json.RawMessage(`{"date":"2026-11-02","slot_id":` + strconv.FormatInt(dinnerID, 10) + `}`)

	_, _, err := ts.Execute(context.Background(), "create_plate", input)
	require.ErrorIs(t, err, domain.ErrCellUnavailable)

	client := fake.NewFromScript(&fake.Script{Turns: []fake.Turn{
		{StopReason: "tool_use", Message: fake.ScriptedMessage{Content: []fake.ScriptedBlock{
			{Type: "tool_use", ToolUseID: "tu_1", ToolName: "create_plate", ToolInput: input},
		}}},
		endTurn(fake.ScriptedUsage{}),
	}})
	svc := agent.NewService(sqlite.NewAIRepo(db), llm.StaticResolver(client, chatModel), ts,
		profile.NewService(sqlite.NewProfileRepo(db)), nil, windowSources(db), time.Now)
	evts, err := chatReq(context.Background(), svc, agent.ChatRequest{
		UserText: "plan monday dinner", From: ymd(t, "2026-11-02"), To: ymd(t, "2026-11-02"),
	}, nil)
	require.NoError(t, err)

	var found bool
	for _, e := range evts {
		if e.Type != llm.EventToolResult {
			continue
		}
		p := e.Payload.(llm.ToolResultPayload)
		require.True(t, p.IsError)
		var body map[string]string
		require.NoError(t, json.Unmarshal(p.Output, &body))
		assert.Equal(t, "error.plate.cell_unavailable", body["message_key"])
		found = true
	}
	assert.True(t, found, "a tool result event must be emitted")
}

func TestChat_TodayDefaultsToServerClock(t *testing.T) {
	db := testhelper.NewTestDB(t)
	client := fake.NewFromScript(&fake.Script{Turns: []fake.Turn{endTurn(fake.ScriptedUsage{})}})
	svc := newPromptService(t, db, client, func() time.Time {
		return time.Date(2026, 10, 27, 23, 30, 0, 0, time.Local)
	})

	_, err := chatReq(context.Background(), svc, agent.ChatRequest{
		UserText: "hi", From: ymd(t, "2026-10-26"), To: ymd(t, "2026-10-28"),
	}, nil)
	require.NoError(t, err)

	system := client.Requests()[0].System
	assert.Contains(t, system, "Today is Tuesday, 2026-10-27.")
	assert.Contains(t, system, "- Tuesday 2026-10-27 (today)\n")
}

func TestChat_RequiresWindow(t *testing.T) {
	db := testhelper.NewTestDB(t)
	client := fake.NewFromScript(&fake.Script{Turns: []fake.Turn{endTurn(fake.ScriptedUsage{})}})
	svc := newPromptService(t, db, client, time.Now)

	_, err := chatReq(context.Background(), svc, agent.ChatRequest{UserText: "hi"}, nil)
	require.Error(t, err)
	assert.Empty(t, client.Requests())
}

func TestDebugSystemPrompt_SamePathAsChat(t *testing.T) {
	db := testhelper.NewTestDB(t)
	svc := newPromptService(t, db, fake.NewFromScript(&fake.Script{}), time.Now)

	got, err := svc.DebugSystemPrompt(context.Background(), agent.ChatRequest{
		Today: ymd(t, "2026-10-26"), From: ymd(t, "2026-10-31"), To: ymd(t, "2026-11-01"),
	})
	require.NoError(t, err)
	assert.Contains(t, got, "Today is Monday, 2026-10-26.")
	assert.Contains(t, got, "- Sunday 2026-11-01\n")
}
