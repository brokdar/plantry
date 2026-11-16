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

// A tool call really cut at max_tokens carries its partial JSON as a string.
// It is never run, the stored history and the next request hold `{}` for it,
// and the events show the call starting and ending with an error.
func TestRun_CutoffWithCutInputKeepsHistoryValid(t *testing.T) {
	t.Parallel()
	cut := fake.Turn{StopReason: "max_tokens", Message: fake.ScriptedMessage{Content: []fake.ScriptedBlock{
		{Type: "tool_use", ToolUseID: "tu_cut", ToolName: "my_tool", ToolInput: json.RawMessage(`"{\"ops\": [{\"op\": \"ad"`)},
	}}}
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
	var evts []llm.Event
	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); evts = collectEvents(out) }()
	err := Run(context.Background(), makeReq(9), client, ts, repo, out)
	close(out)
	wg.Wait()

	require.NoError(t, err)
	assert.Zero(t, executed)

	// The next request replays the cut call with an object input.
	reqs := client.Requests()
	require.Len(t, reqs, 2)
	var replayed bool
	for _, m := range reqs[1].Messages {
		for _, b := range m.Content {
			if b.Type == llm.ContentTypeToolUse {
				replayed = true
				assert.JSONEq(t, `{}`, string(b.ToolUseInput))
			}
		}
	}
	assert.True(t, replayed)

	// The stored assistant message (replayed on later turns) holds `{}` too.
	require.Len(t, repo.messages, 3)
	assert.NotContains(t, string(repo.messages[0].Content), "ops")
	assert.Equal(t, RoleTool, repo.messages[1].Role)
	assert.Contains(t, string(repo.messages[1].Content), "cut off at the output limit")

	var types []llm.EventType
	for _, e := range evts {
		if e.Type == llm.EventToolExecStart || e.Type == llm.EventToolExecEnd {
			types = append(types, e.Type)
		}
	}
	assert.Equal(t, []llm.EventType{llm.EventToolExecStart, llm.EventToolExecEnd}, types)
}
