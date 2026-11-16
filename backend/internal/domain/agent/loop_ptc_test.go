package agent

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/fake"
	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
)

// programmaticTurn is a reply that paused on tool calls made by code: the
// code's server tool use, and one tool_use per tool, each with a code caller.
func programmaticTurn(container string, expires time.Time, tools ...string) fake.Turn {
	blocks := []fake.ScriptedBlock{{
		Type: "server",
		Raw:  json.RawMessage(`{"type":"server_tool_use","id":"srv1","name":"code_execution","input":{"code":"print(1)"}}`),
	}}
	for i, name := range tools {
		blocks = append(blocks, fake.ScriptedBlock{
			Type: "tool_use", ToolUseID: "tu" + string(rune('a'+i)), ToolName: name, ToolInput: json.RawMessage(`{}`),
			Caller: &llm.Caller{Type: llm.CallerCodeExecution, ToolID: "srv1"},
		})
	}
	return fake.Turn{
		StopReason: "tool_use", ContainerID: container, ContainerExpiresAt: expires,
		Message: fake.ScriptedMessage{Content: blocks},
		Usage:   fake.ScriptedUsage{InputTokens: 10, OutputTokens: 5},
	}
}

func runLoop(t *testing.T, req RunRequest, client llm.Client, ts *ToolSet) error {
	t.Helper()
	out := make(chan llm.Event, 64)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); collectEvents(out) }()
	err := Run(context.Background(), req, client, ts, &memRepo{}, out)
	close(out)
	wg.Wait()
	return err
}

func TestRun_ProgrammaticCalls_ContainerKeptAndResultsOnly(t *testing.T) {
	t.Parallel()
	expires := time.Now().Add(time.Hour)
	client := fake.NewFromScript(&fake.Script{Turns: []fake.Turn{
		programmaticTurn("c1", expires, "my_tool", "my_tool"),
		scriptEndTurn("Done."),
	}})
	var callers []*llm.Caller
	var mu sync.Mutex
	ts := minimalToolSet(t, "my_tool", json.RawMessage(`{"ok":true}`), ToolEffectNone, nil)
	ts.byName["my_tool"].CodeCallable = true
	inner := ts.byName["my_tool"].Handler
	ts.byName["my_tool"].Handler = func(ctx context.Context, in json.RawMessage) (json.RawMessage, ToolEffect, error) {
		mu.Lock()
		callers = append(callers, llm.CallerFrom(ctx))
		mu.Unlock()
		return inner(ctx, in)
	}
	var saved []string
	req := makeReq(1)
	req.CodeExecution = true
	req.Tools = ts.Describe()
	req.SaveContainer = func(_ context.Context, id string, _ time.Time) { saved = append(saved, id) }

	require.NoError(t, runLoop(t, req, client, ts))

	reqs := client.Requests()
	require.Len(t, reqs, 2)
	assert.True(t, reqs[0].CodeExecution)
	assert.Empty(t, reqs[0].Container, "the first call opens a container")
	assert.Equal(t, "c1", reqs[1].Container, "the continuation must carry the container")
	assert.Equal(t, []string{llm.CallerDirect, llm.CallerCodeExecution}, reqs[0].Tools[0].AllowedCallers)
	assert.Equal(t, []string{"c1"}, saved)

	// The continuation answers every programmatic call with tool_result
	// blocks only (no text).
	last := reqs[1].Messages[len(reqs[1].Messages)-1]
	assert.Equal(t, llm.RoleUser, last.Role)
	require.Len(t, last.Content, 2)
	for _, b := range last.Content {
		assert.Equal(t, llm.ContentTypeToolResult, b.Type)
	}
	// The replayed assistant message keeps the server block and the callers.
	prev := reqs[1].Messages[len(reqs[1].Messages)-2]
	assert.Equal(t, llm.ContentTypeServer, prev.Content[0].Type)
	assert.True(t, prev.Content[1].Caller.FromCode())

	require.Len(t, callers, 2)
	for _, c := range callers {
		assert.True(t, c.FromCode())
		assert.Equal(t, "srv1", c.ToolID)
	}
}

func TestRun_CodeCallOfNonCallableToolIsRefused(t *testing.T) {
	t.Parallel()
	client := fake.NewFromScript(&fake.Script{Turns: []fake.Turn{
		programmaticTurn("c1", time.Now().Add(time.Hour), "my_tool"),
		scriptEndTurn("Done."),
	}})
	called := false
	ts := minimalToolSet(t, "my_tool", json.RawMessage(`{}`), ToolEffectNone, nil)
	ts.byName["my_tool"].Handler = func(context.Context, json.RawMessage) (json.RawMessage, ToolEffect, error) {
		called = true
		return json.RawMessage(`{}`), ToolEffectNone, nil
	}
	req := makeReq(1)
	req.CodeExecution = true
	require.NoError(t, runLoop(t, req, client, ts))

	assert.False(t, called)
	last := client.Requests()[1].Messages
	res := last[len(last)-1].Content[0]
	assert.True(t, res.ToolResultIsError)
	assert.Contains(t, string(res.ToolResultContent), "cannot be called from code")
}

func TestApplyChanges_FromCodeMustBeDryRun(t *testing.T) {
	t.Parallel()
	ts, err := NewToolSet(Services{})
	require.NoError(t, err)
	code := llm.WithCaller(context.Background(), &llm.Caller{Type: llm.CallerCodeExecution, ToolID: "srv1"})

	_, _, _, err = ts.Execute(code, applyChangesName, json.RawMessage(`{"ops":[{"op":"delete_plate","plate_id":1}]}`))
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrCodeCallRefused))
	assert.Contains(t, err.Error(), "dry_run")

	// A dry run passes the guard (it then fails for lack of services).
	_, _, _, err = ts.Execute(code, applyChangesName, json.RawMessage(`{"ops":[{"op":"delete_plate","plate_id":1}],"dry_run":true}`))
	assert.False(t, errors.Is(err, ErrCodeCallRefused))

	// Writes other than apply_changes are not callable from code at all.
	_, _, _, err = ts.Execute(code, "confirm", json.RawMessage(`{}`))
	assert.True(t, errors.Is(err, ErrCodeCallRefused))
}

func TestDescribe_OnlyReadAndDryRunToolsAllowCode(t *testing.T) {
	t.Parallel()
	ts, err := NewToolSet(Services{})
	require.NoError(t, err)
	allowed := map[string]bool{}
	for _, tl := range ts.Describe() {
		if len(tl.AllowedCallers) > 0 {
			allowed[tl.Name] = true
			assert.Equal(t, []string{llm.CallerDirect, llm.CallerCodeExecution}, tl.AllowedCallers)
		}
	}
	assert.True(t, allowed["find_foods"])
	assert.True(t, allowed[applyChangesName])
	assert.False(t, allowed["confirm"])
	for _, tl := range withoutCallers(ts.Describe()) {
		assert.Empty(t, tl.AllowedCallers)
	}
}

// failAfter wraps a client and fails every request after the first n.
type failAfter struct {
	llm.Client
	n, calls int
}

func (f *failAfter) Stream(ctx context.Context, req llm.Request, out chan<- llm.Event) (*llm.Response, error) {
	f.calls++
	if f.calls > f.n {
		close(out)
		return nil, errors.New("connection reset")
	}
	return f.Client.Stream(ctx, req, out)
}

func runLoopRepo(t *testing.T, req RunRequest, client llm.Client, ts *ToolSet, repo *memRepo) error {
	t.Helper()
	out := make(chan llm.Event, 256)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); collectEvents(out) }()
	err := Run(context.Background(), req, client, ts, repo, out)
	close(out)
	wg.Wait()
	return err
}

func codeToolSet(t *testing.T, calls *int) *ToolSet {
	t.Helper()
	ts := minimalToolSet(t, "my_tool", json.RawMessage(`{"ok":true}`), ToolEffectNone, nil)
	ts.byName["my_tool"].CodeCallable = true
	inner := ts.byName["my_tool"].Handler
	ts.byName["my_tool"].Handler = func(ctx context.Context, in json.RawMessage) (json.RawMessage, ToolEffect, error) {
		*calls++
		return inner(ctx, in)
	}
	return ts
}

// Sequential code-called tool calls are separate requests but not model
// turns: they do not hit MaxIterations. At their own bound the script gets
// errors, the model closes the turn with a message, and that message is stored.
func TestRun_CodeContinuationsHaveTheirOwnBound(t *testing.T) {
	t.Parallel()
	exp := time.Now().Add(time.Hour)
	var turns []fake.Turn
	for range MaxCodeContinuations + 1 {
		turns = append(turns, programmaticTurn("c1", exp, "my_tool"))
	}
	turns = append(turns, scriptEndTurn("Stopped early."))
	client := fake.NewFromScript(&fake.Script{Turns: turns})
	var calls int
	ts := codeToolSet(t, &calls)
	req := makeReq(1)
	req.CodeExecution = true
	req.Tools = ts.Describe()
	repo := &memRepo{}

	require.NoError(t, runLoopRepo(t, req, client, ts, repo))

	assert.Greater(t, MaxCodeContinuations, MaxIterations)
	assert.Equal(t, MaxCodeContinuations, calls, "the tools stop running at the bound")
	reqs := client.Requests()
	require.Len(t, reqs, MaxCodeContinuations+2)
	last := reqs[len(reqs)-1].Messages
	res := last[len(last)-1].Content[0]
	assert.True(t, res.ToolResultIsError)
	assert.Contains(t, string(res.ToolResultContent), "limit reached")
	msgs, _ := repo.ListMessages(context.Background(), 1)
	assert.Equal(t, RoleAssistant, msgs[len(msgs)-1].Role)
	assert.Contains(t, string(msgs[len(msgs)-1].Content), "Stopped early.")
}

// A script that never stops, whatever the model does, ends at MaxRequests.
func TestRun_RequestsAreBoundedAndTurnIsClosed(t *testing.T) {
	t.Parallel()
	exp := time.Now().Add(time.Hour)
	var turns []fake.Turn
	for range MaxRequests + 5 {
		turns = append(turns, programmaticTurn("c1", exp, "my_tool"))
	}
	client := fake.NewFromScript(&fake.Script{Turns: turns})
	var calls int
	ts := codeToolSet(t, &calls)
	req := makeReq(1)
	req.CodeExecution = true
	req.Tools = ts.Describe()
	repo := &memRepo{}

	err := runLoopRepo(t, req, client, ts, repo)

	require.ErrorIs(t, err, ErrIterationsExceeded)
	assert.Len(t, client.Requests(), MaxRequests)
	msgs, _ := repo.ListMessages(context.Background(), 1)
	for _, m := range msgs {
		assert.NotContains(t, string(m.Content), "server_tool_use", "an unfinished code turn is not stored")
	}
}

// A request that fails while a code turn is open must not leave the stored
// conversation inside it: the next user message after unanswered programmatic
// calls would be a 400 on every later turn.
func TestRun_FailureInsideCodeTurnStoresNoOpenCode(t *testing.T) {
	t.Parallel()
	client := &failAfter{
		Client: fake.NewFromScript(&fake.Script{Turns: []fake.Turn{programmaticTurn("c1", time.Now().Add(time.Hour), "my_tool")}}),
		n:      1,
	}
	var calls int
	ts := codeToolSet(t, &calls)
	req := makeReq(1)
	req.CodeExecution = true
	req.Tools = ts.Describe()
	repo := &memRepo{}

	require.Error(t, runLoopRepo(t, req, client, ts, repo))

	assert.Equal(t, 1, calls, "the code-called tool ran")
	msgs, _ := repo.ListMessages(context.Background(), 1)
	require.Len(t, msgs, 1, "only the error row is stored")
	assert.Equal(t, RoleError, msgs[0].Role)
}

// A closed code turn is stored as it happened.
func TestRun_ClosedCodeTurnIsStoredInOrder(t *testing.T) {
	t.Parallel()
	final := fake.Turn{
		StopReason: "end_turn", ContainerID: "c1", ContainerExpiresAt: time.Now().Add(time.Hour),
		Message: fake.ScriptedMessage{Content: []fake.ScriptedBlock{
			{Type: "server", Raw: json.RawMessage(`{"type":"code_execution_tool_result","tool_use_id":"srv1","content":{"type":"code_execution_result","stdout":"","stderr":"","return_code":0,"content":[]}}`)},
			{Type: "text", Text: "Done."},
		}},
		Usage: fake.ScriptedUsage{InputTokens: 10, OutputTokens: 5},
	}
	client := fake.NewFromScript(&fake.Script{Turns: []fake.Turn{programmaticTurn("c1", time.Now().Add(time.Hour), "my_tool"), final}})
	var calls int
	ts := codeToolSet(t, &calls)
	req := makeReq(1)
	req.CodeExecution = true
	req.Tools = ts.Describe()
	repo := &memRepo{}

	require.NoError(t, runLoopRepo(t, req, client, ts, repo))

	msgs, _ := repo.ListMessages(context.Background(), 1)
	var roles []Role
	for _, m := range msgs {
		roles = append(roles, m.Role)
	}
	assert.Equal(t, []Role{RoleAssistant, RoleTool, RoleAssistant}, roles)
}

// The container's rolling expiry is stored with every reply, not only when
// the id changes.
func TestRun_ContainerExpiryRefreshedEveryReply(t *testing.T) {
	t.Parallel()
	e1, e2 := time.Now().Add(time.Hour), time.Now().Add(2*time.Hour)
	client := fake.NewFromScript(&fake.Script{Turns: []fake.Turn{
		programmaticTurn("c1", e1, "my_tool"),
		programmaticTurn("c1", e2, "my_tool"),
		scriptEndTurn("Done."),
	}})
	var calls int
	ts := codeToolSet(t, &calls)
	var saved []time.Time
	req := makeReq(1)
	req.CodeExecution = true
	req.Tools = ts.Describe()
	req.SaveContainer = func(_ context.Context, _ string, exp time.Time) { saved = append(saved, exp) }

	require.NoError(t, runLoopRepo(t, req, client, ts, &memRepo{}))

	require.Len(t, saved, 2)
	assert.True(t, saved[1].After(saved[0]))
}
