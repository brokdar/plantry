package agent_test

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/fake"
	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain/agent"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
	"github.com/jaltszeimer/plantry/backend/internal/domain/slot"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
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
		Plates: plate.NewService(sqlite.NewPlateRepo(db), slotRepo, foodRepo),
		Slots:  slot.NewService(slotRepo),
		Foods:  foodRepo,
	}
}

// newPromptService wires an unrecorded service over db with the given client
// and clock.
func newPromptService(t *testing.T, db *sql.DB, client llm.Client, now func() time.Time) *agent.Service {
	t.Helper()
	tools, err := agent.NewToolSet(agent.Services{Foods: food.NewService(sqlite.NewFoodRepo(db))})
	require.NoError(t, err)
	return agent.NewService(sqlite.NewAIRepo(db), llm.StaticResolver(client, chatModel), tools,
		profile.NewService(sqlite.NewProfileRepo(db)), nil, windowSources(db), now)
}

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
	pc := plate.QuantityFromLegacyPortions("leaf", 1)
	pc.FoodID = pasta.ID
	planned := &plate.Plate{Date: ymd(t, "2026-10-31"), SlotID: dinner.ID, Components: []plate.PlateComponent{pc}}
	require.NoError(t, plateRepo.Create(ctx, planned))
	skipped := &plate.Plate{Date: ymd(t, "2026-11-01"), SlotID: dinner.ID}
	require.NoError(t, plateRepo.Create(ctx, skipped))
	_, err := plateRepo.SetSkipped(ctx, skipped.ID, true, nil)
	require.NoError(t, err)
	outside := &plate.Plate{Date: ymd(t, "2026-11-07"), SlotID: dinner.ID, Components: []plate.PlateComponent{pc}}
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
		fmt.Sprintf("- 2026-11-01 slot.dinner (plate_id=%d, slot_id=%d): skipped\n", skipped.ID, dinner.ID),
	} {
		assert.Contains(t, system, want)
	}
	assert.NotContains(t, system, "2026-11-07", "plates outside the window must not be listed")
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
