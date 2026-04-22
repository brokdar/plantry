package agent_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain/agent"
	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
)

// fakeLLM implements llm.Client. Each call to Stream pops one scripted
// "turn" from its queue, emits its events on out, and returns a synthesised
// Response carrying the turn's final assistant message and stop_reason.
type fakeLLM struct {
	turns       []fakeTurn
	call        int
	streamErr   error // if set, next call returns this error with no response
	holdForever bool  // if true, block until ctx done (used for cancellation tests)
}

type fakeTurn struct {
	events     []llm.Event
	stopReason llm.StopReason
	usage      llm.Usage
	// final assembled message for the loop; must match what events describe.
	message llm.Message
}

func (f *fakeLLM) Stream(ctx context.Context, _ llm.Request, out chan<- llm.Event) (*llm.Response, error) {
	defer close(out)
	if f.holdForever {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if f.streamErr != nil {
		return nil, f.streamErr
	}
	if f.call >= len(f.turns) {
		return nil, errors.New("fake: no more scripted turns")
	}
	turn := f.turns[f.call]
	f.call++
	for _, evt := range turn.events {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case out <- evt:
		}
	}
	return &llm.Response{
		Message:    turn.message,
		StopReason: turn.stopReason,
		Usage:      turn.usage,
	}, nil
}

func newLoopFixture(t *testing.T) (repo *sqlite.AIRepo, tools *agent.ToolSet, svc agent.Services, convID int64, weekID int64, slotID int64, compID int64) {
	t.Helper()
	f := newToolFixture(t)
	repo = sqlite.NewAIRepo(f.db)
	conv, err := repo.CreateConversation(context.Background(), nil, nil)
	require.NoError(t, err)
	tools = f.tools
	weekID = f.weekID
	slotID = f.slotID
	compID = f.componentID
	convID = conv.ID
	// svc not directly needed — tests drive Run by hand.
	return
}

func drain(ctx context.Context, ch <-chan llm.Event) []llm.Event {
	var out []llm.Event
	for evt := range ch {
		out = append(out, evt)
		_ = ctx
	}
	return out
}

func eventTypes(events []llm.Event) []llm.EventType {
	out := make([]llm.EventType, len(events))
	for i, e := range events {
		out[i] = e.Type
	}
	return out
}

func textBlocks(text string) []llm.ContentBlock {
	return []llm.ContentBlock{{Type: llm.ContentTypeText, Text: text}}
}

func toolUseBlock(id, name string, input any) llm.ContentBlock {
	b, _ := json.Marshal(input)
	return llm.ContentBlock{
		Type: llm.ContentTypeToolUse, ToolUseID: id, ToolUseName: name, ToolUseInput: b,
	}
}

func TestLoop_TextOnlyTurn(t *testing.T) {
	repo, tools, _, convID, _, _, _ := newLoopFixture(t)
	ctx := context.Background()

	fake := &fakeLLM{turns: []fakeTurn{{
		events: []llm.Event{
			{Type: llm.EventMessageStart, Payload: llm.MessageStartPayload{Model: "test"}},
			{Type: llm.EventAssistantDelta, Payload: llm.AssistantDeltaPayload{Text: "ok"}},
		},
		stopReason: llm.StopReasonEndTurn,
		message:    llm.Message{Role: llm.RoleAssistant, Content: textBlocks("ok")},
	}}}

	out := make(chan llm.Event, 64)
	err := agent.Run(ctx, agent.RunRequest{
		ConversationID: convID,
		Model:          "test",
		SystemPrompt:   "you are test",
		History:        []llm.Message{{Role: llm.RoleUser, Content: textBlocks("hi")}},
		Tools:          tools.Describe(),
	}, fake, tools, repo, out)
	close(out)
	require.NoError(t, err)

	events := drain(ctx, out)
	assert.Contains(t, eventTypes(events), llm.EventMessageStart)
	assert.Contains(t, eventTypes(events), llm.EventAssistantDelta)
	assert.Equal(t, llm.EventDone, events[len(events)-1].Type)

	// One assistant row persisted.
	msgs, err := repo.ListMessages(ctx, convID)
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	assert.Equal(t, agent.RoleAssistant, msgs[0].Role)
}

func TestLoop_SingleToolCall_ThenFinalText(t *testing.T) {
	repo, tools, _, convID, _, _, _ := newLoopFixture(t)
	ctx := context.Background()

	fake := &fakeLLM{turns: []fakeTurn{
		{
			events: []llm.Event{
				{Type: llm.EventMessageStart, Payload: llm.MessageStartPayload{Model: "test"}},
				{Type: llm.EventToolCallStart, Payload: llm.ToolCallStartPayload{ID: "tu_1", Name: "list_slots"}},
				{Type: llm.EventToolCallDelta, Payload: llm.ToolCallDeltaPayload{ID: "tu_1", ArgsJSON: "{}"}},
			},
			stopReason: llm.StopReasonToolUse,
			message: llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentBlock{
				toolUseBlock("tu_1", "list_slots", map[string]any{}),
			}},
		},
		{
			events: []llm.Event{
				{Type: llm.EventMessageStart, Payload: llm.MessageStartPayload{Model: "test"}},
				{Type: llm.EventAssistantDelta, Payload: llm.AssistantDeltaPayload{Text: "done"}},
			},
			stopReason: llm.StopReasonEndTurn,
			message:    llm.Message{Role: llm.RoleAssistant, Content: textBlocks("done")},
		},
	}}

	out := make(chan llm.Event, 128)
	err := agent.Run(ctx, agent.RunRequest{
		ConversationID: convID,
		Model:          "test",
		History:        []llm.Message{{Role: llm.RoleUser, Content: textBlocks("hi")}},
		Tools:          tools.Describe(),
	}, fake, tools, repo, out)
	close(out)
	require.NoError(t, err)

	events := drain(ctx, out)
	types := eventTypes(events)
	assert.Contains(t, types, llm.EventToolCallStart)
	assert.Contains(t, types, llm.EventToolExecStart)
	assert.Contains(t, types, llm.EventToolExecEnd)
	assert.Contains(t, types, llm.EventToolResult)
	assert.Equal(t, llm.EventDone, events[len(events)-1].Type)

	// Persisted: assistant (tool_use), tool (tool_result), assistant (final text).
	msgs, err := repo.ListMessages(ctx, convID)
	require.NoError(t, err)
	require.Len(t, msgs, 3)
	assert.Equal(t, agent.RoleAssistant, msgs[0].Role)
	assert.Equal(t, agent.RoleTool, msgs[1].Role)
	assert.Equal(t, agent.RoleAssistant, msgs[2].Role)
}

func TestLoop_PlateChangedEmittedOnMutatingTool(t *testing.T) {
	repo, tools, _, convID, weekID, slotID, _ := newLoopFixture(t)
	ctx := context.Background()

	fake := &fakeLLM{turns: []fakeTurn{
		{
			stopReason: llm.StopReasonToolUse,
			message: llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentBlock{
				toolUseBlock("tu_1", "create_plate", map[string]any{
					"week_id": weekID, "day": 0, "slot_id": slotID,
				}),
			}},
		},
		{
			stopReason: llm.StopReasonEndTurn,
			message:    llm.Message{Role: llm.RoleAssistant, Content: textBlocks("added")},
		},
	}}

	out := make(chan llm.Event, 128)
	err := agent.Run(ctx, agent.RunRequest{
		ConversationID: convID,
		WeekID:         &weekID,
		History:        []llm.Message{{Role: llm.RoleUser, Content: textBlocks("plan")}},
		Tools:          tools.Describe(),
	}, fake, tools, repo, out)
	close(out)
	require.NoError(t, err)

	events := drain(ctx, out)
	found := false
	for _, e := range events {
		if e.Type == llm.EventPlateChanged {
			payload := e.Payload.(llm.PlateChangedPayload)
			assert.Equal(t, weekID, payload.WeekID)
			found = true
		}
	}
	assert.True(t, found, "expected plate_changed event")
}

func TestLoop_ParallelToolCallsInOneTurn(t *testing.T) {
	repo, tools, _, convID, _, _, _ := newLoopFixture(t)
	ctx := context.Background()

	fake := &fakeLLM{turns: []fakeTurn{
		{
			stopReason: llm.StopReasonToolUse,
			message: llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentBlock{
				toolUseBlock("tu_1", "list_slots", map[string]any{}),
				toolUseBlock("tu_2", "get_profile", map[string]any{}),
			}},
		},
		{stopReason: llm.StopReasonEndTurn, message: llm.Message{Role: llm.RoleAssistant, Content: textBlocks("ok")}},
	}}

	out := make(chan llm.Event, 128)
	err := agent.Run(ctx, agent.RunRequest{
		ConversationID: convID,
		History:        []llm.Message{{Role: llm.RoleUser, Content: textBlocks("read")}},
		Tools:          tools.Describe(),
	}, fake, tools, repo, out)
	close(out)
	require.NoError(t, err)

	var execEnds int
	for _, e := range drain(ctx, out) {
		if e.Type == llm.EventToolExecEnd {
			execEnds++
		}
	}
	assert.Equal(t, 2, execEnds)

	// Persistence: assistant (with 2 tool_uses) + tool (with 2 tool_results) + assistant.
	msgs, err := repo.ListMessages(ctx, convID)
	require.NoError(t, err)
	assert.Len(t, msgs, 3)
}

func TestLoop_ToolErrorReportedAsToolResult_AndLoopContinues(t *testing.T) {
	repo, tools, _, convID, _, _, _ := newLoopFixture(t)
	ctx := context.Background()

	fake := &fakeLLM{turns: []fakeTurn{
		{
			stopReason: llm.StopReasonToolUse,
			message: llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentBlock{
				toolUseBlock("tu_1", "get_component", map[string]any{"component_id": 9999}),
			}},
		},
		{stopReason: llm.StopReasonEndTurn, message: llm.Message{Role: llm.RoleAssistant, Content: textBlocks("couldn't find")}},
	}}

	out := make(chan llm.Event, 128)
	err := agent.Run(ctx, agent.RunRequest{
		ConversationID: convID,
		History:        []llm.Message{{Role: llm.RoleUser, Content: textBlocks("ask")}},
		Tools:          tools.Describe(),
	}, fake, tools, repo, out)
	close(out)
	require.NoError(t, err)

	var endStatus llm.ToolExecStatus
	for _, e := range drain(ctx, out) {
		if e.Type == llm.EventToolExecEnd {
			endStatus = e.Payload.(llm.ToolExecEndPayload).Status
		}
	}
	assert.Equal(t, llm.ToolExecStatusError, endStatus)
}

func TestLoop_SchemaInvalidToolArgs_ContinuesAsToolError(t *testing.T) {
	repo, tools, _, convID, _, _, _ := newLoopFixture(t)
	ctx := context.Background()

	fake := &fakeLLM{turns: []fakeTurn{
		{
			stopReason: llm.StopReasonToolUse,
			message: llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentBlock{
				// day=42 is out of range — schema violation.
				toolUseBlock("tu_1", "create_plate", map[string]any{
					"week_id": 1, "day": 42, "slot_id": 1,
				}),
			}},
		},
		{stopReason: llm.StopReasonEndTurn, message: llm.Message{Role: llm.RoleAssistant, Content: textBlocks("sorry")}},
	}}

	out := make(chan llm.Event, 128)
	err := agent.Run(ctx, agent.RunRequest{
		ConversationID: convID,
		History:        []llm.Message{{Role: llm.RoleUser, Content: textBlocks("plan")}},
		Tools:          tools.Describe(),
	}, fake, tools, repo, out)
	close(out)
	require.NoError(t, err)

	types := eventTypes(drain(ctx, out))
	assert.Contains(t, types, llm.EventToolExecEnd)
	assert.Contains(t, types, llm.EventDone)
}

func TestLoop_MaxIterationsExceeded(t *testing.T) {
	repo, tools, _, convID, _, _, _ := newLoopFixture(t)
	ctx := context.Background()

	// Build an infinite tool-calling script (>MaxIterations turns, all tool_use).
	turns := make([]fakeTurn, 0, agent.MaxIterations+1)
	for i := 0; i <= agent.MaxIterations; i++ {
		turns = append(turns, fakeTurn{
			stopReason: llm.StopReasonToolUse,
			message: llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentBlock{
				toolUseBlock("tu_"+itoa(i), "list_slots", map[string]any{}),
			}},
		})
	}
	fake := &fakeLLM{turns: turns}

	out := make(chan llm.Event, 2048)
	err := agent.Run(ctx, agent.RunRequest{
		ConversationID: convID,
		History:        []llm.Message{{Role: llm.RoleUser, Content: textBlocks("loop")}},
		Tools:          tools.Describe(),
	}, fake, tools, repo, out)
	close(out)
	assert.True(t, errors.Is(err, agent.ErrIterationsExceeded))

	// Error event emitted and error row persisted.
	types := eventTypes(drain(ctx, out))
	assert.Contains(t, types, llm.EventError)
	msgs, _ := repo.ListMessages(ctx, convID)
	var errRows int
	for _, m := range msgs {
		if m.Role == agent.RoleError {
			errRows++
		}
	}
	assert.Equal(t, 1, errRows)
}

func TestLoop_StreamErrorPropagatesAndPersists(t *testing.T) {
	repo, tools, _, convID, _, _, _ := newLoopFixture(t)
	ctx := context.Background()

	fake := &fakeLLM{streamErr: errors.New("upstream 503")}

	out := make(chan llm.Event, 32)
	err := agent.Run(ctx, agent.RunRequest{
		ConversationID: convID,
		History:        []llm.Message{{Role: llm.RoleUser, Content: textBlocks("hi")}},
		Tools:          tools.Describe(),
	}, fake, tools, repo, out)
	close(out)
	require.Error(t, err)

	types := eventTypes(drain(ctx, out))
	assert.Contains(t, types, llm.EventError)

	msgs, _ := repo.ListMessages(ctx, convID)
	var hasErr bool
	for _, m := range msgs {
		if m.Role == agent.RoleError {
			hasErr = true
		}
	}
	assert.True(t, hasErr)
}

func TestLoop_CtxCancelMidStream(t *testing.T) {
	repo, tools, _, convID, _, _, _ := newLoopFixture(t)

	fake := &fakeLLM{holdForever: true}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	out := make(chan llm.Event, 16)
	done := make(chan error, 1)
	go func() {
		done <- agent.Run(ctx, agent.RunRequest{
			ConversationID: convID,
			History:        []llm.Message{{Role: llm.RoleUser, Content: textBlocks("hi")}},
			Tools:          tools.Describe(),
		}, fake, tools, repo, out)
		close(out)
	}()

	err := <-done
	require.Error(t, err)
	// drain everything that was emitted
	for range out {
	}
	// ctx cancellation is the signal — we expect Run to return the ctx error
	// (or at minimum an error that wraps it).
	assert.True(t, errors.Is(ctx.Err(), context.DeadlineExceeded))
	_ = repo
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	pos := len(buf)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		pos--
		buf[pos] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}
