package agent_test

import (
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain/actor"
	"github.com/jaltszeimer/plantry/backend/internal/domain/airun"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

// B3-AC2: a tool call writes its plate events as the agent of the run in
// ctx, overriding the user actor the HTTP middleware set.
func TestToolSet_ExecuteActsAsAgentOfRun(t *testing.T) {
	e := newPlateToolEnv(t)
	plateID := int64(e.mustExec(t, "create_plate", fmt.Sprintf(`{"date":"2026-11-02","slot_id":%d}`, e.slotID))["id"].(float64))

	rec := airun.NewRecorder(sqlite.NewAIRunRepo(e.db), slog.New(slog.DiscardHandler), time.Now)
	ctx, run := rec.Start(actor.With(e.ctx, actor.User, 0), airun.Start{Kind: airun.KindChat, PromptVersion: "v1"})
	require.NotZero(t, run.ID())
	e.ctx = ctx
	e.mustExec(t, "add_food_to_plate", fmt.Sprintf(`{"plate_id":%d,"food_id":%d,"amount":150,"unit":"g"}`, plateID, e.rice))
	run.Finish(ctx, airun.StatusOK, "")
	rec.Wait()

	evs, err := sqlite.NewPlateEventRepo(e.db).List(e.ctx)
	require.NoError(t, err)
	require.Len(t, evs, 2)
	assert.Equal(t, actor.Agent, evs[0].Actor, "create_plate without a run")
	assert.Nil(t, evs[0].AIRunID)

	ev := evs[1]
	assert.Equal(t, plate.ActionUpdate, ev.Action)
	assert.Equal(t, actor.Agent, ev.Actor)
	require.NotNil(t, ev.AIRunID)
	assert.Equal(t, run.ID(), *ev.AIRunID)
	assert.Equal(t, plateID, *ev.PlateID)
}

// Phase 3 B1-AC2 (D2): rows a tool call creates are drafts tagged with the
// chat run; the tools need no change because the actor is already the agent.
func TestToolSet_CreatesDraftsOfTheChatRun(t *testing.T) {
	e := newPlateToolEnv(t)
	rec := airun.NewRecorder(sqlite.NewAIRunRepo(e.db), slog.New(slog.DiscardHandler), time.Now)
	ctx, run := rec.Start(actor.With(e.ctx, actor.User, 0), airun.Start{Kind: airun.KindChat, PromptVersion: "v1"})
	e.ctx = ctx
	plateID := int64(e.mustExec(t, "create_plate", fmt.Sprintf(`{"date":"2026-11-02","slot_id":%d}`, e.slotID))["id"].(float64))
	e.mustExec(t, "add_food_to_plate", fmt.Sprintf(`{"plate_id":%d,"food_id":%d,"portions":1}`, plateID, e.chili))
	run.Finish(ctx, airun.StatusOK, "")
	rec.Wait()

	p, err := e.plates.Get(e.ctx, plateID)
	require.NoError(t, err)
	assert.Equal(t, plate.StateDraft, p.State)
	require.NotNil(t, p.AIRunID)
	assert.Equal(t, run.ID(), *p.AIRunID)
	sess, err := e.plates.CookSession(e.ctx, *p.Components[0].CookSessionID)
	require.NoError(t, err)
	assert.Equal(t, plate.StateDraft, sess.State)
	assert.Equal(t, run.ID(), *sess.AIRunID)
}
