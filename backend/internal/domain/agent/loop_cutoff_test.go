package agent

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/fake"
	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
)

// A reply cut off at max_tokens runs none of its tool calls: each is answered
// with an error result, the run goes on, and the cut replies stay model turns
// bounded by MaxIterations.
func TestRun_CutoffRunsNothing(t *testing.T) {
	t.Parallel()
	cut := scriptToolUseTurn("tu_cut", "my_tool")
	cut.StopReason = "max_tokens"
	client := fake.NewFromScript(&fake.Script{Turns: []fake.Turn{cut, scriptEndTurn("Done.")}})
	repo := &memRepo{}
	executed := 0
	ts := minimalToolSet(t, "my_tool", json.RawMessage(`{"ok":true}`), ToolEffectNone, nil)
	handler := ts.byName["my_tool"].Handler
	ts.byName["my_tool"].Handler = func(ctx context.Context, in json.RawMessage) (json.RawMessage, ToolEffect, error) {
		executed++
		return handler(ctx, in)
	}

	out := make(chan llm.Event, 64)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); collectEvents(out) }()
	err := Run(context.Background(), makeReq(7), client, ts, repo, out)
	close(out)
	wg.Wait()

	require.NoError(t, err)
	assert.Zero(t, executed)
	reqs := client.Requests()
	require.Len(t, reqs, 2)
	last := reqs[1].Messages[len(reqs[1].Messages)-1]
	require.Len(t, last.Content, 1)
	assert.Equal(t, "tu_cut", last.Content[0].ToolResultID)
	assert.True(t, last.Content[0].ToolResultIsError)
	assert.Contains(t, string(last.Content[0].ToolResultContent), "cut off at the output limit")
	require.Len(t, repo.messages, 3)
	assert.Equal(t, RoleTool, repo.messages[1].Role)

	// Endless cut-offs hit the turn bound.
	client = fake.NewFromScript(&fake.Script{Turns: []fake.Turn{cut}})
	out = make(chan llm.Event, MaxIterations*10)
	err = Run(context.Background(), makeReq(8), client, ts, &memRepo{}, out)
	assert.ErrorIs(t, err, ErrIterationsExceeded)
	assert.Zero(t, executed)
}
