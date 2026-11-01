package agent

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/fake"
	"github.com/jaltszeimer/plantry/backend/internal/domain/airun"
	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

type stubRunStore struct{ airun.Store }

func (stubRunStore) CreateRun(context.Context, airun.RunStart) (int64, error) { return 7, nil }
func (stubRunStore) AddStep(context.Context, airun.Step) error                { return nil }

type stubChangeStore struct {
	ChangeStore
	markErr error
}

func (s stubChangeStore) LastEventID(context.Context) (int64, error) { return 0, s.markErr }
func (stubChangeStore) RunEvents(context.Context, int64, int64, []int64) ([]plate.Event, error) {
	return nil, nil
}

func (stubChangeStore) CreateChange(_ context.Context, c *Change) error {
	c.ID = 9
	return nil
}

// failingChangeTool is a tool set whose one tool notes a change, then fails.
func failingChangeTool(t *testing.T, store ChangeStore) *ToolSet {
	ts := minimalToolSet(t, "w", nil, ToolEffectNone, nil)
	ts.byName["w"].Handler = func(ctx context.Context, _ json.RawMessage) (json.RawMessage, ToolEffect, error) {
		noteChange(ctx, "k", map[string]any{})
		return nil, ToolEffectNone, errors.New("wait ended")
	}
	ts.changes = store
	return ts
}

func runWithChangeTool(t *testing.T, ts *ToolSet) ([]llm.Event, *memRepo) {
	t.Helper()
	rec := airun.NewRecorder(stubRunStore{}, slog.New(slog.DiscardHandler), time.Now)
	ctx, _ := rec.Start(context.Background(), airun.Start{Kind: airun.KindChat})
	client := fake.NewFromScript(&fake.Script{Turns: []fake.Turn{scriptToolUseTurn("tu", "w"), scriptEndTurn("ok")}})
	repo := &memRepo{}
	out := make(chan llm.Event, 64)
	var evts []llm.Event
	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); evts = collectEvents(out) }()
	require.NoError(t, Run(ctx, makeReq(3), client, ts, repo, out))
	close(out)
	wg.Wait()
	return evts, repo
}

// A failed call that still changed the plan emits its change line, and the
// error result carries the change id.
func TestRun_ChangeAfterFailedCall(t *testing.T) {
	evts, _ := runWithChangeTool(t, failingChangeTool(t, stubChangeStore{}))
	assertContainsInOrder(t, eventTypes(evts), llm.EventToolResult, llm.EventChange, llm.EventDone)
	for _, e := range evts {
		switch p := e.Payload.(type) {
		case llm.ChangePayload:
			assert.True(t, p.Recorded)
			assert.EqualValues(t, 9, p.ID)
		case llm.ToolResultPayload:
			assert.True(t, p.IsError)
			var res map[string]any
			require.NoError(t, json.Unmarshal(p.Output, &res))
			assert.EqualValues(t, 9, res["change_id"])
			assert.Equal(t, "wait ended", res["error"])
		}
	}
}

// When the watermark cannot be read the change is made but not recorded, on
// a failed call as on a successful one.
func TestRun_WatermarkFailureIsNotRecorded(t *testing.T) {
	evts, _ := runWithChangeTool(t, failingChangeTool(t, stubChangeStore{markErr: errors.New("db gone")}))
	var seen int
	for _, e := range evts {
		switch p := e.Payload.(type) {
		case llm.ChangePayload:
			seen++
			assert.False(t, p.Recorded)
			assert.False(t, p.Undoable)
			assert.Zero(t, p.ID)
		case llm.ToolResultPayload:
			var res map[string]any
			require.NoError(t, json.Unmarshal(p.Output, &res))
			assert.Equal(t, "change_not_recorded", res["warning"])
			assert.NotContains(t, res, "change_id")
		}
	}
	assert.Equal(t, 1, seen)
}
