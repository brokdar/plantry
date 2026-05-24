package agent

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xeipuuv/gojsonschema"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/fake"
	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
)

// ---------------------------------------------------------------------------
// In-memory Repository fake (local to loop tests)
// ---------------------------------------------------------------------------

type memRepo struct {
	mu       sync.Mutex
	messages []Message
}

func (r *memRepo) AppendMessage(_ context.Context, m *Message) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.messages = append(r.messages, *m)
	return nil
}

func (r *memRepo) CreateConversation(_ context.Context, weekID *int64, title *string) (*Conversation, error) {
	return &Conversation{ID: 1, WeekID: weekID, Title: title}, nil
}

func (r *memRepo) GetConversation(_ context.Context, id int64) (*Conversation, error) {
	return &Conversation{ID: id}, nil
}

func (r *memRepo) UpdateConversationTitle(_ context.Context, id int64, title *string) (*Conversation, error) {
	return &Conversation{ID: id, Title: title}, nil
}
func (r *memRepo) TouchConversation(_ context.Context, _ int64) error  { return nil }
func (r *memRepo) DeleteConversation(_ context.Context, _ int64) error { return nil }
func (r *memRepo) ListConversations(_ context.Context, _ ListQuery) (*ListResult, error) {
	return &ListResult{}, nil
}

func (r *memRepo) ListMessages(_ context.Context, _ int64) ([]Message, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Message, len(r.messages))
	copy(out, r.messages)
	return out, nil
}

// ---------------------------------------------------------------------------
// blockingClient — blocks on a channel until it is closed or ctx is done.
// Used for the context-cancellation test.
// ---------------------------------------------------------------------------

type blockingClient struct {
	unblock chan struct{} // close to unblock; leave nil to only cancel via ctx
}

func (c *blockingClient) Stream(ctx context.Context, _ llm.Request, out chan<- llm.Event) (*llm.Response, error) {
	defer close(out)
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-c.unblock:
		// deliberately left open — only ctx cancellation is expected in tests
		return nil, errors.New("unblocked unexpectedly")
	}
}

func (c *blockingClient) Complete(ctx context.Context, _ llm.Request) (string, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case <-c.unblock:
		return "", errors.New("unblocked unexpectedly")
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// minimalToolSet builds a ToolSet with one registered tool whose handler
// returns the canned output/effect/error provided at construction time.
func minimalToolSet(t *testing.T, toolName string, output json.RawMessage, effect ToolEffect, handlerErr error) *ToolSet {
	t.Helper()
	ts := &ToolSet{
		byName:  map[string]*Tool{},
		schemas: map[string]*gojsonschema.Schema{},
	}
	err := ts.register(Tool{
		Name:        toolName,
		Description: "test tool",
		// Accept any object input so schema validation never trips us up.
		Schema: json.RawMessage(`{"type":"object","additionalProperties":true}`),
		Handler: func(_ context.Context, _ json.RawMessage) (json.RawMessage, ToolEffect, error) {
			return output, effect, handlerErr
		},
	})
	require.NoError(t, err)
	return ts
}

// collectEvents drains the out channel into a slice. Must run before Run()
// returns, so call it in a goroutine before invoking Run().
func collectEvents(out <-chan llm.Event) []llm.Event {
	var evts []llm.Event
	for e := range out {
		evts = append(evts, e)
	}
	return evts
}

// eventTypes returns just the Type field from each event for easy assertions.
func eventTypes(evts []llm.Event) []llm.EventType {
	types := make([]llm.EventType, len(evts))
	for i, e := range evts {
		types[i] = e.Type
	}
	return types
}

// makeReq returns a minimal RunRequest for the given conversationID.
func makeReq(convID int64) RunRequest {
	return RunRequest{
		ConversationID: convID,
		Model:          "test-model",
	}
}

// scriptTurn builds a fake.Turn that emits a text block and ends with the
// given stop reason and message content.
func scriptEndTurn(text string) fake.Turn {
	return fake.Turn{
		StopReason: "end_turn",
		Message: fake.ScriptedMessage{
			Content: []fake.ScriptedBlock{
				{Type: "text", Text: text},
			},
		},
		Usage: fake.ScriptedUsage{InputTokens: 10, OutputTokens: 5},
	}
}

// scriptToolUseTurn builds a fake.Turn whose message contains a tool_use block.
func scriptToolUseTurn(toolUseID, toolName string) fake.Turn {
	return fake.Turn{
		StopReason: "tool_use",
		Message: fake.ScriptedMessage{
			Content: []fake.ScriptedBlock{
				{Type: "tool_use", ToolUseID: toolUseID, ToolName: toolName, ToolInput: json.RawMessage(`{}`)},
			},
		},
		Usage: fake.ScriptedUsage{InputTokens: 10, OutputTokens: 5},
	}
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// TestRun_NoTools verifies the happy path when the LLM returns end_turn
// immediately with no tool calls.
func TestRun_NoTools(t *testing.T) {
	t.Parallel()

	client := fake.NewFromScript(&fake.Script{
		Turns: []fake.Turn{scriptEndTurn("Hello, I am the assistant.")},
	})
	repo := &memRepo{}
	ts := minimalToolSet(t, "noop", json.RawMessage(`{}`), ToolEffectNone, nil)

	out := make(chan llm.Event, 32)
	var evts []llm.Event
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		evts = collectEvents(out)
	}()

	err := Run(context.Background(), makeReq(1), client, ts, repo, out)
	close(out)
	wg.Wait()

	require.NoError(t, err)

	types := eventTypes(evts)
	assert.Equal(t, llm.EventDone, types[len(types)-1], "last event must be EventDone")

	// Exactly one assistant message persisted (no tool message).
	msgs := repo.messages
	require.Len(t, msgs, 1, "expected exactly one persisted message")
	assert.Equal(t, RoleAssistant, msgs[0].Role)
}

// TestRun_OneToolCallSuccess verifies that a single successful tool call
// produces the expected event sequence and two persisted messages.
func TestRun_OneToolCallSuccess(t *testing.T) {
	t.Parallel()

	const (
		toolUseID = "tu_abc123"
		toolName  = "my_tool"
	)

	client := fake.NewFromScript(&fake.Script{
		Turns: []fake.Turn{
			scriptToolUseTurn(toolUseID, toolName),
			scriptEndTurn("Done."),
		},
	})
	repo := &memRepo{}
	ts := minimalToolSet(t, toolName, json.RawMessage(`{"ok":true}`), ToolEffectNone, nil)

	out := make(chan llm.Event, 64)
	var evts []llm.Event
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		evts = collectEvents(out)
	}()

	err := Run(context.Background(), makeReq(2), client, ts, repo, out)
	close(out)
	wg.Wait()

	require.NoError(t, err)

	types := eventTypes(evts)

	// Verify the mandatory loop-emitted events are present and in order.
	assertContainsInOrder(t, types,
		llm.EventToolExecStart,
		llm.EventToolExecEnd,
		llm.EventToolResult,
		llm.EventDone,
	)

	// Verify tool exec end carries "ok" status.
	for _, e := range evts {
		if e.Type == llm.EventToolExecEnd {
			p := e.Payload.(llm.ToolExecEndPayload)
			assert.Equal(t, llm.ToolExecStatusOK, p.Status)
			assert.Equal(t, toolUseID, p.ID)
		}
	}

	// Two persisted messages: assistant (tool_use) + tool (tool_result).
	msgs := repo.messages
	require.Len(t, msgs, 3, "expected 3 persisted messages: assistant(turn1) + tool + assistant(turn2)")
	assert.Equal(t, RoleAssistant, msgs[0].Role)
	assert.Equal(t, RoleTool, msgs[1].Role)
	assert.Equal(t, RoleAssistant, msgs[2].Role)
}

// TestRun_ToolError verifies that a tool error is fed back to the model as a
// tool_result with is_error=true and that Run() returns nil.
func TestRun_ToolError(t *testing.T) {
	t.Parallel()

	const (
		toolUseID = "tu_err1"
		toolName  = "bad_tool"
	)

	client := fake.NewFromScript(&fake.Script{
		Turns: []fake.Turn{
			scriptToolUseTurn(toolUseID, toolName),
			scriptEndTurn("I see the tool failed, let me handle that."),
		},
	})
	repo := &memRepo{}
	handlerErr := errors.New("something went wrong in the tool")
	ts := minimalToolSet(t, toolName, nil, ToolEffectNone, handlerErr)

	out := make(chan llm.Event, 64)
	var evts []llm.Event
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		evts = collectEvents(out)
	}()

	err := Run(context.Background(), makeReq(3), client, ts, repo, out)
	close(out)
	wg.Wait()

	// Tool errors are recoverable — Run() must return nil.
	require.NoError(t, err, "tool errors must be fed back to the model, not returned as Run() error")

	types := eventTypes(evts)

	assertContainsInOrder(t, types,
		llm.EventToolExecStart,
		llm.EventToolExecEnd,
		llm.EventToolResult,
		llm.EventDone,
	)

	// Verify the tool exec end carries "error" status.
	for _, e := range evts {
		if e.Type == llm.EventToolExecEnd {
			p := e.Payload.(llm.ToolExecEndPayload)
			assert.Equal(t, llm.ToolExecStatusError, p.Status)
		}
	}

	// Verify EventToolResult carries IsError=true.
	for _, e := range evts {
		if e.Type == llm.EventToolResult {
			p := e.Payload.(llm.ToolResultPayload)
			assert.True(t, p.IsError, "tool result must be marked as error")
		}
	}
}

// TestRun_IterationBudgetExceeded verifies that when the LLM never returns
// end_turn the loop hits MaxIterations and returns ErrIterationsExceeded.
func TestRun_IterationBudgetExceeded(t *testing.T) {
	t.Parallel()

	const toolName = "inf_tool"

	// A single tool_use turn; fake client replays the last turn indefinitely.
	client := fake.NewFromScript(&fake.Script{
		Turns: []fake.Turn{
			scriptToolUseTurn("tu_inf", toolName),
		},
	})
	repo := &memRepo{}
	ts := minimalToolSet(t, toolName, json.RawMessage(`{"ok":true}`), ToolEffectNone, nil)

	// Use a large enough buffer so the loop never blocks on the channel.
	out := make(chan llm.Event, MaxIterations*10)
	var evts []llm.Event
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		evts = collectEvents(out)
	}()

	err := Run(context.Background(), makeReq(4), client, ts, repo, out)
	close(out)
	wg.Wait()

	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrIterationsExceeded), "expected ErrIterationsExceeded, got: %v", err)

	// The last event must be EventError (emitted by emitAndPersistError).
	types := eventTypes(evts)
	require.NotEmpty(t, types)
	assert.Equal(t, llm.EventError, types[len(types)-1], "last event must be EventError")
}

// TestRun_ContextCancellation verifies that cancelling the context while the
// LLM is streaming causes Run() to return a non-nil error.
func TestRun_ContextCancellation(t *testing.T) {
	t.Parallel()

	client := &blockingClient{unblock: make(chan struct{})}
	repo := &memRepo{}
	ts := minimalToolSet(t, "noop", json.RawMessage(`{}`), ToolEffectNone, nil)

	ctx, cancel := context.WithCancel(context.Background())

	out := make(chan llm.Event, 32)
	var runErr error
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		runErr = Run(ctx, makeReq(5), client, ts, repo, out)
		close(out)
	}()

	// Drain the channel concurrently so Run() never blocks on send.
	var drainWg sync.WaitGroup
	drainWg.Add(1)
	go func() {
		defer drainWg.Done()
		for range out {
		}
	}()

	// Cancel the context to unblock the client.
	cancel()

	wg.Wait()
	drainWg.Wait()

	require.Error(t, runErr, "Run() must return an error when the context is cancelled")
}

// TestRun_PersistenceOrdering verifies that messages are appended in the
// correct sequence: assistant(turn1) → tool → assistant(turn2).
func TestRun_PersistenceOrdering(t *testing.T) {
	t.Parallel()

	const (
		toolUseID = "tu_order"
		toolName  = "order_tool"
	)

	client := fake.NewFromScript(&fake.Script{
		Turns: []fake.Turn{
			scriptToolUseTurn(toolUseID, toolName),
			scriptEndTurn("All done."),
		},
	})
	repo := &memRepo{}
	ts := minimalToolSet(t, toolName, json.RawMessage(`{"result":"ok"}`), ToolEffectNone, nil)

	out := make(chan llm.Event, 64)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for range out {
		}
	}()

	err := Run(context.Background(), makeReq(6), client, ts, repo, out)
	close(out)
	wg.Wait()

	require.NoError(t, err)

	msgs := repo.messages
	require.Len(t, msgs, 3, "expected assistant(turn1) + tool + assistant(turn2)")

	// Verify roles in order.
	assert.Equal(t, RoleAssistant, msgs[0].Role, "first persisted message must be assistant (tool_use turn)")
	assert.Equal(t, RoleTool, msgs[1].Role, "second persisted message must be tool result")
	assert.Equal(t, RoleAssistant, msgs[2].Role, "third persisted message must be final assistant turn")

	// Verify the tool message content contains a tool_result block.
	var toolBlocks []llm.ContentBlock
	require.NoError(t, json.Unmarshal(msgs[1].Content, &toolBlocks))
	require.Len(t, toolBlocks, 1)
	assert.Equal(t, llm.ContentTypeToolResult, toolBlocks[0].Type)
	assert.Equal(t, toolUseID, toolBlocks[0].ToolResultID)
}

// ---------------------------------------------------------------------------
// Test utilities
// ---------------------------------------------------------------------------

// assertContainsInOrder checks that all wantTypes appear in evtTypes in the
// given order (not necessarily contiguously).
func assertContainsInOrder(t *testing.T, evtTypes []llm.EventType, wantTypes ...llm.EventType) {
	t.Helper()
	idx := 0
	for _, et := range evtTypes {
		if idx < len(wantTypes) && et == wantTypes[idx] {
			idx++
		}
	}
	if idx != len(wantTypes) {
		t.Errorf("event sequence %v does not contain %v in order; found first %d of them",
			evtTypes, wantTypes, idx)
	}
}
