package agent_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/fake"
	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/agent"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
	"github.com/jaltszeimer/plantry/backend/internal/domain/slot"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper/testsvc"
)

// B3-AC3: get_plan returns the cells, sessions and issues GET /api/plan reads
// for the chat's window, field by field on a seeded window.
func TestToolGetPlan_MatchesPlanReadModel(t *testing.T) {
	ctx := context.Background()
	db := testhelper.NewTestDB(t)
	insert := func(q string) int64 {
		res, err := db.ExecContext(ctx, q)
		require.NoError(t, err, q)
		id, err := res.LastInsertId()
		require.NoError(t, err)
		return id
	}
	dinner := insert(`INSERT INTO time_slots (name_key, icon, sort_order, active) VALUES ('slot.dinner', 'moon', 1, 1)`)
	rice := insert(`INSERT INTO foods (name, kind, source) VALUES ('Rice', 'leaf', 'manual')`)
	chili := insert(`INSERT INTO foods (name, kind, role, reference_portions) VALUES ('Chili', 'composed', 'main', 4)`)
	planView := testsvc.PlanView(db)
	ts, err := agent.NewToolSet(agent.Services{
		Foods:    food.NewService(sqlite.NewFoodRepo(db)),
		Plates:   testsvc.Plates(db),
		Profile:  profile.NewService(sqlite.NewProfileRepo(db)),
		Slots:    slot.NewService(sqlite.NewSlotRepo(db)),
		PlanView: planView,
	})
	require.NoError(t, err)

	exec := func(name, input string) json.RawMessage {
		t.Helper()
		out, _, err := ts.Execute(ctx, name, json.RawMessage(input))
		require.NoError(t, err)
		return out
	}
	var created struct {
		ID int64 `json:"id"`
	}
	require.NoError(t, json.Unmarshal(exec("create_plate", fmt.Sprintf(`{"date":"2026-11-03","slot_id":%d}`, dinner)), &created))
	exec("add_food_to_plate", fmt.Sprintf(`{"plate_id":%d,"food_id":%d,"amount":150,"unit":"g"}`, created.ID, rice))
	exec("add_food_to_plate", fmt.Sprintf(`{"plate_id":%d,"food_id":%d,"portions":1}`, created.ID, chili))

	start := time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)
	today := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	want, err := planView.Plan(ctx, &start, today)
	require.NoError(t, err)
	require.NotEmpty(t, want.Sessions, "the seeded window has a cook session")

	// Run get_plan inside a chat turn over the window, as the model would.
	client := fake.NewFromScript(&fake.Script{Turns: []fake.Turn{
		toolUseTurn(fake.ScriptedUsage{}, "get_plan"),
		endTurn(fake.ScriptedUsage{}),
	}})
	svc := agent.NewService(sqlite.NewAIRepo(db), llm.StaticResolver(client, chatModel), ts,
		profile.NewService(sqlite.NewProfileRepo(db)), testsvc.Settings(db), planContext(db), time.Now)
	evts, err := chatReq(ctx, svc, agent.ChatRequest{UserText: "show the plan", Today: today, Start: start}, nil)
	require.NoError(t, err)
	var raw json.RawMessage
	for _, e := range evts {
		if e.Type == llm.EventToolResult {
			p := e.Payload.(llm.ToolResultPayload)
			require.False(t, p.IsError, string(p.Output))
			raw = p.Output
		}
	}
	require.NotNil(t, raw, "get_plan produced a tool result")
	var got struct {
		Window struct{ Start, End string } `json:"window"`
		Cells  []struct {
			Date   string `json:"date"`
			SlotID int64  `json:"slot_id"`
			Mode   string `json:"mode"`
			Plate  *struct {
				ID         int64   `json:"id"`
				State      string  `json:"state"`
				Eaters     float64 `json:"eaters"`
				Components []struct {
					ID        int64    `json:"id"`
					FoodID    int64    `json:"food_id"`
					Portions  *float64 `json:"portions"`
					Amount    *float64 `json:"amount"`
					SessionID *int64   `json:"session_id"`
				} `json:"components"`
			} `json:"plate"`
		} `json:"cells"`
		Sessions []struct {
			ID             int64   `json:"id"`
			FoodID         int64   `json:"food_id"`
			CookDate       string  `json:"cook_date"`
			CookSlotID     int64   `json:"cook_slot_id"`
			Portions       float64 `json:"portions"`
			FrozenPortions float64 `json:"frozen_portions"`
			State          string  `json:"state"`
			Consumed       float64 `json:"consumed"`
			FrozenConsumed float64 `json:"frozen_consumed"`
			Links          []any   `json:"links"`
			Warnings       []any   `json:"warnings"`
		} `json:"sessions"`
		Issues json.RawMessage `json:"issues"`
	}
	require.NoError(t, json.Unmarshal(raw, &got))

	assert.Equal(t, "2026-11-02", got.Window.Start)
	assert.Equal(t, "2026-11-08", got.Window.End)
	require.Len(t, got.Cells, len(want.Cells))
	for i, c := range want.Cells {
		g := got.Cells[i]
		assert.Equal(t, c.Date.Format(time.DateOnly), g.Date)
		assert.Equal(t, c.SlotID, g.SlotID)
		assert.Equal(t, string(c.Mode), g.Mode)
		if c.Plate == nil {
			assert.Nil(t, g.Plate, "cell %d", i)
			continue
		}
		require.NotNil(t, g.Plate, "cell %d", i)
		assert.Equal(t, c.Plate.ID, g.Plate.ID)
		assert.Equal(t, string(c.Plate.State), g.Plate.State)
		assert.Equal(t, c.Plate.Eaters, g.Plate.Eaters)
		require.Len(t, g.Plate.Components, len(c.Plate.Components))
		for j, pc := range c.Plate.Components {
			assert.Equal(t, pc.ID, g.Plate.Components[j].ID)
			assert.Equal(t, pc.FoodID, g.Plate.Components[j].FoodID)
			assert.Equal(t, pc.Portions, g.Plate.Components[j].Portions)
			assert.Equal(t, pc.Amount, g.Plate.Components[j].Amount)
			assert.Equal(t, pc.CookSessionID, g.Plate.Components[j].SessionID)
		}
	}
	require.Len(t, got.Sessions, len(want.Sessions))
	for i, v := range want.Sessions {
		g := got.Sessions[i]
		assert.Equal(t, v.ID, g.ID)
		assert.Equal(t, v.FoodID, g.FoodID)
		assert.Equal(t, v.CookDate.Format(time.DateOnly), g.CookDate)
		assert.Equal(t, v.CookSlotID, g.CookSlotID)
		assert.Equal(t, v.Portions, g.Portions)
		assert.Equal(t, v.FrozenPortions, g.FrozenPortions)
		assert.Equal(t, string(v.State), g.State)
		assert.Equal(t, v.Consumed, g.Consumed)
		assert.Equal(t, v.FrozenConsumed, g.FrozenConsumed)
		assert.Len(t, g.Links, len(v.Links))
		assert.Len(t, g.Warnings, len(v.Warnings))
	}
	wantIssues, err := json.Marshal(want.Issues)
	require.NoError(t, err)
	assert.JSONEq(t, string(wantIssues), string(got.Issues))
}

func TestToolGetPlan_NeedsAWindow(t *testing.T) {
	db := testhelper.NewTestDB(t)
	ts, err := agent.NewToolSet(agent.Services{PlanView: testsvc.PlanView(db)})
	require.NoError(t, err)

	_, _, err = ts.Execute(context.Background(), "get_plan", json.RawMessage(`{}`))

	assert.ErrorIs(t, err, domain.ErrInvalidInput)
}

func TestToolGetPlan_NotRegisteredWithoutPlanView(t *testing.T) {
	ts, err := agent.NewToolSet(agent.Services{})
	require.NoError(t, err)
	for _, tool := range ts.Describe() {
		assert.NotEqual(t, "get_plan", tool.Name)
	}
}
