package airun_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/fake"
	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain/airun"
	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
)

// memStore is an in-memory airun.Store. FinishRun and AddStep fail on a
// cancelled ctx, like a real DB driver would.
type memStore struct {
	mu        sync.Mutex
	nextID    int64
	runs      map[int64]airun.RunStart
	finishes  []airun.RunEnd
	steps     []airun.Step
	prompts   map[string]string
	upserts   int
	tools     map[string]string
	toolUps   int
	prunes    []time.Time
	createErr error
	stepErr   error
	upsertErr error
	pruneErr  error
	// failUpserts fails that many UpsertPrompt / UpsertToolSchema calls
	// before they start succeeding.
	failUpserts int
	// failStepAt fails the AddStep call with this step seq.
	failStepAt int
}

func newMemStore() *memStore {
	return &memStore{runs: map[int64]airun.RunStart{}, prompts: map[string]string{}, tools: map[string]string{}}
}

func (s *memStore) UpsertToolSchema(ctx context.Context, hash, json string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.toolUps++
	if s.failUpserts > 0 {
		s.failUpserts--
		return errors.New("db busy")
	}
	s.tools[hash] = json
	return nil
}

func (s *memStore) PruneSteps(ctx context.Context, before time.Time) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prunes = append(s.prunes, before)
	return 0, s.pruneErr
}

func (s *memStore) CreateRun(ctx context.Context, start airun.RunStart) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.createErr != nil {
		return 0, s.createErr
	}
	s.nextID++
	s.runs[s.nextID] = start
	return s.nextID, nil
}

func (s *memStore) FinishRun(ctx context.Context, end airun.RunEnd) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.finishes = append(s.finishes, end)
	return nil
}

func (s *memStore) AddStep(ctx context.Context, step airun.Step) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stepErr != nil {
		return s.stepErr
	}
	if s.failStepAt != 0 && step.Seq == s.failStepAt {
		return errors.New("db busy")
	}
	s.steps = append(s.steps, step)
	return nil
}

func (s *memStore) UpsertPrompt(ctx context.Context, hash, text string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.upserts++
	if s.upsertErr != nil {
		return s.upsertErr
	}
	if s.failUpserts > 0 {
		s.failUpserts--
		return errors.New("db busy")
	}
	s.prompts[hash] = text
	return nil
}

func (s *memStore) snapshot() (runs map[int64]airun.RunStart, finishes []airun.RunEnd, steps []airun.Step, upserts int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	runs = make(map[int64]airun.RunStart, len(s.runs))
	for k, v := range s.runs {
		runs[k] = v
	}
	return runs, append([]airun.RunEnd(nil), s.finishes...), append([]airun.Step(nil), s.steps...), s.upserts
}

var fixedNow = time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)

func newRecorder(t *testing.T, store airun.Store) (*airun.Recorder, *bytes.Buffer) {
	t.Helper()
	return newRecorderWithClock(t, store, func() time.Time { return fixedNow })
}

func newRecorderWithClock(t *testing.T, store airun.Store, now func() time.Time) (*airun.Recorder, *bytes.Buffer) {
	t.Helper()
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	return airun.NewRecorder(store, logger, now), &buf
}

// steppingClock returns a clock that advances by step on every call.
func steppingClock(step time.Duration) func() time.Time {
	var mu sync.Mutex
	t := fixedNow
	return func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		t = t.Add(step)
		return t
	}
}

func textTurn(text string, usage fake.ScriptedUsage) fake.Turn {
	return fake.Turn{
		StopReason: "end_turn",
		Events: []fake.ScriptedEvent{
			{Type: string(llm.EventAssistantDelta), Payload: json.RawMessage(`{"text":"ignored-delta"}`)},
		},
		Message: fake.ScriptedMessage{Content: []fake.ScriptedBlock{{Type: "text", Text: text}}},
		Usage:   usage,
	}
}

func toolTurn(usage fake.ScriptedUsage) fake.Turn {
	return fake.Turn{
		StopReason: "tool_use",
		Message: fake.ScriptedMessage{Content: []fake.ScriptedBlock{
			{Type: "tool_use", ToolUseID: "t1", ToolName: "noop"},
		}},
		Usage: usage,
	}
}

func drainStream(ctx context.Context, c llm.Client, req llm.Request) (*llm.Response, error) {
	out := make(chan llm.Event, 16)
	done := make(chan struct{})
	go func() {
		for range out {
		}
		close(done)
	}()
	resp, err := c.Stream(ctx, req, out)
	<-done
	return resp, err
}

func sha(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func TestMeter_StreamSumsUsageAcrossCalls(t *testing.T) {
	store := newMemStore()
	rec, _ := newRecorder(t, store)
	inner := fake.NewFromScript(&fake.Script{Turns: []fake.Turn{
		toolTurn(fake.ScriptedUsage{InputTokens: 10, OutputTokens: 1, CacheReadTokens: 100, CacheWriteTokens: 1000}),
		toolTurn(fake.ScriptedUsage{InputTokens: 20, OutputTokens: 2, CacheReadTokens: 200, CacheWriteTokens: 2000}),
		textTurn("done", fake.ScriptedUsage{InputTokens: 30, OutputTokens: 3, CacheReadTokens: 300, CacheWriteTokens: 3000}),
	}})
	client := rec.Meter(inner, "anthropic")

	ctx, run := rec.Start(context.Background(), airun.Start{Kind: airun.KindChat, PromptVersion: "v1"})
	if airun.FromContext(ctx) != run {
		t.Fatal("FromContext did not return the started run")
	}
	req := llm.Request{Model: "claude-x", System: "sys"}
	for i := 0; i < 3; i++ {
		if _, err := drainStream(ctx, client, req); err != nil {
			t.Fatalf("stream %d: %v", i, err)
		}
	}
	run.Finish(ctx, airun.StatusOK, "")

	runs, finishes, steps, _ := store.snapshot()
	if len(runs) != 1 || runs[1].Kind != airun.KindChat || runs[1].PromptVersion != "v1" {
		t.Fatalf("runs = %+v; want one chat run v1", runs)
	}
	if len(finishes) != 1 {
		t.Fatalf("finishes = %d; want 1", len(finishes))
	}
	got := finishes[0]
	want := airun.RunEnd{
		ID: 1, Status: airun.StatusOK, FinishedAt: fixedNow,
		Provider: "anthropic", Model: "claude-x", PromptHash: sha("sys"),
		Iterations: 3, InputTokens: 60, OutputTokens: 6, CacheReadTokens: 600, CacheWriteTokens: 6000,
	}
	if got != want {
		t.Errorf("finish = %+v\nwant     %+v", got, want)
	}
	if len(steps) != 3 {
		t.Fatalf("steps = %d; want 3", len(steps))
	}
	for i, s := range steps {
		if s.Seq != i+1 || s.Type != airun.StepLLMCall || s.Name != "claude-x" || s.IsError || s.RunID != 1 {
			t.Errorf("step %d = %+v", i, s)
		}
	}
	if steps[1].InputTokens != 20 || steps[1].OutputTokens != 2 {
		t.Errorf("step 2 tokens = %d/%d; want 20/2", steps[1].InputTokens, steps[1].OutputTokens)
	}
}

func TestMeter_PromptStoredOnce(t *testing.T) {
	store := newMemStore()
	rec, _ := newRecorder(t, store)
	inner := fake.NewFromScript(&fake.Script{Turns: []fake.Turn{textTurn("x", fake.ScriptedUsage{})}})
	client := rec.Meter(inner, "openai")

	ctx, run := rec.Start(context.Background(), airun.Start{Kind: airun.KindLookup})
	for i := 0; i < 3; i++ {
		if _, err := client.Complete(ctx, llm.Request{Model: "m", System: "same system"}); err != nil {
			t.Fatal(err)
		}
	}
	run.Finish(ctx, airun.StatusOK, "")

	_, finishes, _, upserts := store.snapshot()
	if upserts != 1 {
		t.Errorf("UpsertPrompt calls = %d; want 1", upserts)
	}
	if store.prompts[sha("same system")] != "same system" {
		t.Errorf("prompts = %v; want hash of system text", store.prompts)
	}
	if finishes[0].PromptHash != sha("same system") {
		t.Errorf("prompt hash = %q", finishes[0].PromptHash)
	}
}

func TestMeter_CompleteReturnsInnerTextAndRecordsUsage(t *testing.T) {
	turns := func() *fake.Script {
		return &fake.Script{Turns: []fake.Turn{{
			StopReason: "end_turn",
			Message: fake.ScriptedMessage{Content: []fake.ScriptedBlock{
				{Type: "text", Text: "Hello, "},
				{Type: "tool_use", ToolUseID: "x", ToolName: "y"},
				{Type: "text", Text: "world"},
			}},
			Usage: fake.ScriptedUsage{InputTokens: 7, OutputTokens: 5, CacheReadTokens: 2, CacheWriteTokens: 1},
		}}}
	}
	req := llm.Request{Model: "gpt", System: "s"}
	want, err := fake.NewFromScript(turns()).Complete(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}

	store := newMemStore()
	rec, _ := newRecorder(t, store)
	ctx, run := rec.Start(context.Background(), airun.Start{Kind: airun.KindClassify})
	got, err := rec.Meter(fake.NewFromScript(turns()), "openai").Complete(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("Complete = %q; want %q", got, want)
	}
	run.Finish(ctx, airun.StatusOK, "")

	_, finishes, steps, _ := store.snapshot()
	if len(steps) != 1 || steps[0].InputTokens != 7 || steps[0].OutputTokens != 5 {
		t.Errorf("steps = %+v; want one step with 7/5 tokens", steps)
	}
	f := finishes[0]
	if f.Iterations != 1 || f.InputTokens != 7 || f.OutputTokens != 5 || f.CacheReadTokens != 2 || f.CacheWriteTokens != 1 {
		t.Errorf("finish = %+v", f)
	}
}

func TestMeter_InnerErrorRecordedAndReturnedUnchanged(t *testing.T) {
	tests := []struct {
		name string
		call func(ctx context.Context, c llm.Client) error
	}{
		{"stream", func(ctx context.Context, c llm.Client) error {
			_, err := drainStream(ctx, c, llm.Request{Model: "m"})
			return err
		}},
		{"complete", func(ctx context.Context, c llm.Client) error {
			_, err := c.Complete(ctx, llm.Request{Model: "m"})
			return err
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// An empty script makes the fake fail every call.
			wantErr := drainErr(fake.NewFromScript(&fake.Script{}))

			store := newMemStore()
			rec, _ := newRecorder(t, store)
			ctx, run := rec.Start(context.Background(), airun.Start{Kind: airun.KindChat})
			err := tt.call(ctx, rec.Meter(fake.NewFromScript(&fake.Script{}), "anthropic"))
			if err == nil || err.Error() != wantErr.Error() {
				t.Fatalf("err = %v; want %v", err, wantErr)
			}
			run.Finish(ctx, airun.StatusError, "error.ai.failed")

			_, finishes, steps, _ := store.snapshot()
			if len(steps) != 1 || !steps[0].IsError || steps[0].Type != airun.StepLLMCall {
				t.Errorf("steps = %+v; want one error llm_call", steps)
			}
			if finishes[0].Status != airun.StatusError || finishes[0].ErrorKey != "error.ai.failed" || finishes[0].Iterations != 1 {
				t.Errorf("finish = %+v", finishes[0])
			}
		})
	}
}

func drainErr(c llm.Client) error {
	_, err := drainStream(context.Background(), c, llm.Request{})
	return err
}

func TestMeter_InnerErrorIdentityPreserved(t *testing.T) {
	sentinel := errors.New("boom")
	store := newMemStore()
	rec, _ := newRecorder(t, store)
	_, err := drainStream(context.Background(), rec.Meter(errClient{sentinel}, "openai"), llm.Request{Model: "m"})
	if err != sentinel { // identity, not errors.Is: the meter must not wrap
		t.Fatalf("err = %v; want the inner error itself", err)
	}
	_, finishes, _, _ := store.snapshot()
	if len(finishes) != 1 || finishes[0].Status != airun.StatusError {
		t.Errorf("finishes = %+v; want one untracked error run", finishes)
	}
}

func TestMeter_UntrackedCancelledCallFinishesCancelled(t *testing.T) {
	store := newMemStore()
	rec, _ := newRecorder(t, store)
	cancelErr := fmt.Errorf("stream: %w", context.Canceled)
	_, err := drainStream(context.Background(), rec.Meter(errClient{cancelErr}, "openai"), llm.Request{Model: "m"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v; want context.Canceled", err)
	}
	_, finishes, _, _ := store.snapshot()
	if len(finishes) != 1 || finishes[0].Status != airun.StatusCancelled {
		t.Errorf("finishes = %+v; want one untracked cancelled run", finishes)
	}
}

type errClient struct{ err error }

func (c errClient) Stream(_ context.Context, _ llm.Request, out chan<- llm.Event) (*llm.Response, error) {
	close(out)
	return nil, c.err
}

func (c errClient) Complete(context.Context, llm.Request) (string, error) { return "", c.err }

func TestMeter_CallOutsideRunIsUntracked(t *testing.T) {
	store := newMemStore()
	rec, _ := newRecorder(t, store)
	inner := fake.NewFromScript(&fake.Script{Turns: []fake.Turn{textTurn("hi", fake.ScriptedUsage{InputTokens: 3, OutputTokens: 4})}})

	got, err := rec.Meter(inner, "openai").Complete(context.Background(), llm.Request{Model: "gpt"})
	if err != nil || got != "hi" {
		t.Fatalf("Complete = %q, %v", got, err)
	}

	runs, finishes, steps, _ := store.snapshot()
	if len(runs) != 1 || runs[1].Kind != airun.KindUntracked {
		t.Fatalf("runs = %+v; want one untracked run", runs)
	}
	if len(finishes) != 1 || finishes[0].Status != airun.StatusOK || finishes[0].Iterations != 1 ||
		finishes[0].Provider != "openai" || finishes[0].Model != "gpt" || finishes[0].InputTokens != 3 {
		t.Errorf("finishes = %+v", finishes)
	}
	if len(steps) != 1 {
		t.Errorf("steps = %d; want 1", len(steps))
	}
}

func TestRun_FinishTwiceWritesOnce(t *testing.T) {
	store := newMemStore()
	rec, _ := newRecorder(t, store)
	ctx, run := rec.Start(context.Background(), airun.Start{Kind: airun.KindGenerate})
	run.Finish(ctx, airun.StatusOK, "")
	run.Finish(ctx, airun.StatusError, "later")

	_, finishes, _, _ := store.snapshot()
	if len(finishes) != 1 || finishes[0].Status != airun.StatusOK {
		t.Errorf("finishes = %+v; want exactly one ok", finishes)
	}
}

func TestRun_CancelledContextStillRecords(t *testing.T) {
	store := newMemStore()
	rec, _ := newRecorder(t, store)
	ctx, cancel := context.WithCancel(context.Background())
	ctx, run := rec.Start(ctx, airun.Start{Kind: airun.KindChat})
	cancel()

	run.ToolStep(ctx, "noop", json.RawMessage(`{}`), json.RawMessage(`{"ok":true}`), false, time.Millisecond)
	run.Finish(ctx, airun.StatusCancelled, "")

	_, finishes, steps, _ := store.snapshot()
	if len(finishes) != 1 || finishes[0].Status != airun.StatusCancelled {
		t.Errorf("finishes = %+v; want one cancelled", finishes)
	}
	if len(steps) != 1 {
		t.Errorf("steps = %d; want 1 (written despite cancelled ctx)", len(steps))
	}
}

func TestRun_ConcurrentToolStepsHaveUniqueSeq(t *testing.T) {
	const n = 50
	store := newMemStore()
	rec, _ := newRecorder(t, store)
	ctx, run := rec.Start(context.Background(), airun.Start{Kind: airun.KindChat})

	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			run.ToolStep(ctx, "t", json.RawMessage(`{"a":1}`), json.RawMessage(`{}`), i%2 == 0, time.Duration(i)*time.Millisecond)
		}()
	}
	wg.Wait()
	run.Finish(ctx, airun.StatusOK, "")

	_, finishes, steps, _ := store.snapshot()
	if len(steps) != n {
		t.Fatalf("steps = %d; want %d", len(steps), n)
	}
	seen := map[int]bool{}
	for _, s := range steps {
		if seen[s.Seq] {
			t.Errorf("duplicate seq %d", s.Seq)
		}
		seen[s.Seq] = true
		if s.Type != airun.StepToolCall || s.ArgsJSON != `{"a":1}` || s.ResultJSON != `{}` {
			t.Errorf("step = %+v", s)
		}
	}
	if finishes[0].Iterations != 0 {
		t.Errorf("iterations = %d; tool steps must not count", finishes[0].Iterations)
	}
}

func TestRun_NilSafe(t *testing.T) {
	if airun.FromContext(context.Background()) != nil {
		t.Fatal("FromContext on empty ctx should be nil")
	}
	var run *airun.Run
	run.ToolStep(context.Background(), "t", nil, nil, false, 0)
	run.Finish(context.Background(), airun.StatusOK, "")
}

func TestRecorder_StoreFailuresDoNotFailCalls(t *testing.T) {
	store := newMemStore()
	store.createErr = errors.New("db down")
	store.stepErr = errors.New("db down")
	rec, logs := newRecorder(t, store)
	inner := fake.NewFromScript(&fake.Script{Turns: []fake.Turn{textTurn("ok", fake.ScriptedUsage{})}})
	client := rec.Meter(inner, "openai")

	ctx, run := rec.Start(context.Background(), airun.Start{Kind: airun.KindChat})
	got, err := client.Complete(ctx, llm.Request{Model: "m"})
	if err != nil || got != "ok" {
		t.Fatalf("Complete = %q, %v", got, err)
	}
	run.ToolStep(ctx, "t", nil, nil, false, 0)
	run.Finish(ctx, airun.StatusOK, "")

	_, finishes, steps, _ := store.snapshot()
	if len(finishes) != 0 || len(steps) != 0 {
		t.Errorf("a run that failed to start must not write: finishes=%d steps=%d", len(finishes), len(steps))
	}
	if !strings.Contains(logs.String(), `"level":"WARN"`) {
		t.Errorf("expected a warn log, got %s", logs.String())
	}
}

func TestRecorder_LogsStartAndEnd(t *testing.T) {
	store := newMemStore()
	rec, logs := newRecorder(t, store)
	inner := fake.NewFromScript(&fake.Script{Turns: []fake.Turn{textTurn("ok", fake.ScriptedUsage{InputTokens: 9})}})

	ctx := context.WithValue(context.Background(), middleware.RequestIDKey, "req-42")
	ctx, run := rec.Start(ctx, airun.Start{Kind: airun.KindChat})
	if _, err := rec.Meter(inner, "anthropic").Complete(ctx, llm.Request{Model: "claude"}); err != nil {
		t.Fatal(err)
	}
	run.Finish(ctx, airun.StatusOK, "")

	var start, end map[string]any
	for _, line := range strings.Split(strings.TrimSpace(logs.String()), "\n") {
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("bad log line %q: %v", line, err)
		}
		switch m["msg"] {
		case "ai.run.start":
			start = m
		case "ai.run.end":
			end = m
		}
	}
	if start == nil || start["request_id"] != "req-42" || start["kind"] != "chat" || start["run_id"] != float64(1) {
		t.Errorf("start log = %v", start)
	}
	if end == nil || end["request_id"] != "req-42" || end["provider"] != "anthropic" || end["model"] != "claude" ||
		end["status"] != "ok" || end["iterations"] != float64(1) || end["input_tokens"] != float64(9) {
		t.Errorf("end log = %v", end)
	}
	if _, ok := end["duration_ms"]; !ok {
		t.Errorf("end log missing duration_ms: %v", end)
	}
}

func TestRecorder_DurationsFromClock(t *testing.T) {
	store := newMemStore()
	rec, logs := newRecorderWithClock(t, store, steppingClock(5*time.Millisecond))
	inner := fake.NewFromScript(&fake.Script{Turns: []fake.Turn{textTurn("ok", fake.ScriptedUsage{})}})

	ctx, run := rec.Start(context.Background(), airun.Start{Kind: airun.KindChat})
	if _, err := rec.Meter(inner, "openai").Complete(ctx, llm.Request{Model: "m"}); err != nil {
		t.Fatal(err)
	}
	run.Finish(ctx, airun.StatusOK, "")

	_, _, steps, _ := store.snapshot()
	if len(steps) != 1 || steps[0].DurationMs != 5 {
		t.Errorf("steps = %+v; want one step with DurationMs 5", steps)
	}
	// Clock reads: start, call start, call end, finish => 15ms from start.
	if !strings.Contains(logs.String(), `"duration_ms":15`) {
		t.Errorf("end log missing duration_ms 15: %s", logs.String())
	}
}

func TestMeter_PromptHashUnsetWhenUpsertFails(t *testing.T) {
	store := newMemStore()
	store.upsertErr = errors.New("db down")
	rec, _ := newRecorder(t, store)
	inner := fake.NewFromScript(&fake.Script{Turns: []fake.Turn{textTurn("ok", fake.ScriptedUsage{})}})

	ctx, run := rec.Start(context.Background(), airun.Start{Kind: airun.KindChat})
	for i := 0; i < 2; i++ {
		if _, err := rec.Meter(inner, "openai").Complete(ctx, llm.Request{Model: "m", System: "s"}); err != nil {
			t.Fatal(err)
		}
	}
	run.Finish(ctx, airun.StatusOK, "")

	_, finishes, _, upserts := store.snapshot()
	if upserts != 2 {
		t.Errorf("UpsertPrompt calls = %d; want 2 (a failed upsert is retried by the next call)", upserts)
	}
	if finishes[0].PromptHash != "" {
		t.Errorf("prompt hash = %q; want empty after failed upserts", finishes[0].PromptHash)
	}
}

func TestMeter_FailedUpsertsRetriedByNextCall(t *testing.T) {
	store := newMemStore()
	store.failUpserts = 2 // the first prompt and the first tools upsert
	rec, _ := newRecorder(t, store)
	client := rec.Meter(fake.NewFromScript(&fake.Script{Turns: []fake.Turn{textTurn("ok", fake.ScriptedUsage{})}}), "openai")
	tools := []llm.Tool{{Name: "a", Schema: json.RawMessage(`{}`)}}

	ctx, run := rec.Start(context.Background(), airun.Start{Kind: airun.KindChat})
	for i := 0; i < 3; i++ {
		if _, err := client.Complete(ctx, llm.Request{Model: "m", System: "s", Tools: tools}); err != nil {
			t.Fatal(err)
		}
	}
	run.Finish(ctx, airun.StatusOK, "")

	_, finishes, _, upserts := store.snapshot()
	store.mu.Lock()
	toolUps, stored := store.toolUps, len(store.tools)
	store.mu.Unlock()
	if upserts != 2 || toolUps != 2 || stored != 1 {
		t.Errorf("prompt upserts = %d, tool upserts = %d (%d stored); want 2, 2, 1", upserts, toolUps, stored)
	}
	if finishes[0].PromptHash != sha("s") {
		t.Errorf("prompt hash = %q; want the first call's prompt once stored by the retry", finishes[0].PromptHash)
	}
}

func TestMeter_InPlaceReuseOfCallerSliceDoesNotBreakDeltas(t *testing.T) {
	store := newMemStore()
	rec, _ := newRecorder(t, store)
	client := rec.Meter(fake.NewFromScript(&fake.Script{Turns: []fake.Turn{textTurn("ok", fake.ScriptedUsage{})}}), "openai")
	ctx, run := rec.Start(context.Background(), airun.Start{Kind: airun.KindChat})

	// The caller reuses its backing array: the second request overwrites
	// the first one's message in place.
	buf := make([]llm.Message, 1, 4)
	buf[0] = textMsg(llm.RoleUser, "a")
	if _, err := client.Complete(ctx, llm.Request{Model: "m", Messages: buf}); err != nil {
		t.Fatal(err)
	}
	buf[0] = textMsg(llm.RoleUser, "b")
	buf = append(buf, textMsg(llm.RoleUser, "c"))
	if _, err := client.Complete(ctx, llm.Request{Model: "m", Messages: buf}); err != nil {
		t.Fatal(err)
	}
	run.Finish(ctx, airun.StatusOK, "")

	if b := decodeRequest(t, stepsBySeq(store)[1]); b.MessageOffset != 0 || len(b.Messages) != 2 {
		t.Errorf("offset %d, %d messages; want 0, 2 (the recorded prefix changed)", b.MessageOffset, len(b.Messages))
	}
}

func TestMeter_ConcurrentStreamsAndFinishes(t *testing.T) {
	const streams, finishers = 30, 5
	store := newMemStore()
	rec, _ := newRecorder(t, store)
	inner := fake.NewFromScript(&fake.Script{Turns: []fake.Turn{textTurn("ok", fake.ScriptedUsage{InputTokens: 1})}})
	client := rec.Meter(inner, "openai")
	ctx, run := rec.Start(context.Background(), airun.Start{Kind: airun.KindChat})

	var wg sync.WaitGroup
	for i := 0; i < streams; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := drainStream(ctx, client, llm.Request{Model: "m", System: "shared"}); err != nil {
				t.Error(err)
			}
		}()
	}
	for i := 0; i < finishers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			run.Finish(ctx, airun.StatusOK, "")
		}()
	}
	wg.Wait()

	_, finishes, steps, upserts := store.snapshot()
	if upserts != 1 {
		t.Errorf("UpsertPrompt calls = %d; want 1", upserts)
	}
	if len(finishes) != 1 {
		t.Errorf("FinishRun writes = %d; want 1", len(finishes))
	}
	if len(steps) != streams {
		t.Fatalf("steps = %d; want %d", len(steps), streams)
	}
	seen := map[int]bool{}
	for _, s := range steps {
		if seen[s.Seq] {
			t.Errorf("duplicate seq %d", s.Seq)
		}
		seen[s.Seq] = true
	}
}

// requestBody mirrors the request_json shape of an llm_call step.
type requestBody struct {
	MessageOffset int             `json:"message_offset"`
	Messages      []llm.Message   `json:"messages"`
	ToolChoice    *llm.ToolChoice `json:"tool_choice"`
	Temperature   float64         `json:"temperature"`
	MaxTokens     int             `json:"max_tokens"`
	SystemHash    *string         `json:"system_hash"`
	ToolsHash     *string         `json:"tools_hash"`
	Truncated     bool            `json:"truncated"`
}

// responseBody mirrors the response_json shape of an llm_call step.
type responseBody struct {
	Content    []llm.ContentBlock `json:"content"`
	StopReason string             `json:"stop_reason"`
	Truncated  bool               `json:"truncated"`
}

func decodeRequest(t *testing.T, s airun.Step) requestBody {
	t.Helper()
	var b requestBody
	if err := json.Unmarshal([]byte(s.RequestJSON), &b); err != nil {
		t.Fatalf("request_json of step %d is not valid JSON: %v", s.Seq, err)
	}
	return b
}

func decodeResponse(t *testing.T, s airun.Step) responseBody {
	t.Helper()
	var b responseBody
	if err := json.Unmarshal([]byte(s.ResponseJSON), &b); err != nil {
		t.Fatalf("response_json of step %d is not valid JSON: %v", s.Seq, err)
	}
	return b
}

func textMsg(role llm.Role, text string) llm.Message {
	return llm.Message{Role: role, Content: []llm.ContentBlock{{Type: llm.ContentTypeText, Text: text}}}
}

// stepsBySeq returns the snapshot's steps ordered by seq.
func stepsBySeq(store *memStore) []airun.Step {
	_, _, steps, _ := store.snapshot()
	out := make([]airun.Step, len(steps))
	for _, s := range steps {
		out[s.Seq-1] = s
	}
	return out
}

func TestMeter_RecordsRequestDeltas(t *testing.T) {
	store := newMemStore()
	rec, _ := newRecorder(t, store)
	inner := fake.NewFromScript(&fake.Script{Turns: []fake.Turn{
		toolTurn(fake.ScriptedUsage{}), toolTurn(fake.ScriptedUsage{}), textTurn("done", fake.ScriptedUsage{}),
	}})
	client := rec.Meter(inner, "anthropic")
	ctx, run := rec.Start(context.Background(), airun.Start{Kind: airun.KindChat})

	// A chat-like run: each call resends the history plus new messages.
	history := []llm.Message{textMsg(llm.RoleUser, "u1")}
	for i := 0; i < 3; i++ {
		req := llm.Request{Model: "m", System: "sys", Messages: history, Temperature: 0.5, MaxTokens: 1000}
		resp, err := drainStream(ctx, client, req)
		if err != nil {
			t.Fatal(err)
		}
		history = append(history, resp.Message, textMsg(llm.RoleUser, fmt.Sprintf("u%d", i+2)))
	}
	run.Finish(ctx, airun.StatusOK, "")

	steps := stepsBySeq(store)
	wantOffsets, wantLens := []int{0, 1, 3}, []int{1, 2, 2}
	for i, s := range steps {
		b := decodeRequest(t, s)
		if b.MessageOffset != wantOffsets[i] || len(b.Messages) != wantLens[i] {
			t.Errorf("step %d: offset %d, %d messages; want %d, %d", i+1, b.MessageOffset, len(b.Messages), wantOffsets[i], wantLens[i])
		}
		if b.Temperature != 0.5 || b.MaxTokens != 1000 || b.Truncated {
			t.Errorf("step %d params = %+v", i+1, b)
		}
		if b.SystemHash == nil || *b.SystemHash != sha("sys") {
			t.Errorf("step %d system_hash = %v", i+1, b.SystemHash)
		}
		if b.ToolsHash != nil {
			t.Errorf("step %d tools_hash = %q; want null without tools", i+1, *b.ToolsHash)
		}
	}
	if got := decodeRequest(t, steps[1]).Messages[0].Role; got != llm.RoleAssistant {
		t.Errorf("second delta starts with %q; want the assistant reply", got)
	}
}

func TestMeter_RecordsToolChoice(t *testing.T) {
	store := newMemStore()
	rec, _ := newRecorder(t, store)
	inner := fake.NewFromScript(&fake.Script{Turns: []fake.Turn{textTurn("a", fake.ScriptedUsage{}), textTurn("b", fake.ScriptedUsage{})}})
	client := rec.Meter(inner, "anthropic")
	ctx, run := rec.Start(context.Background(), airun.Start{Kind: airun.KindGenerate})
	msgs := []llm.Message{textMsg(llm.RoleUser, "u1")}
	for _, choice := range []*llm.ToolChoice{{Name: "submit_plan"}, nil} {
		if _, err := drainStream(ctx, client, llm.Request{Model: "m", Messages: msgs, ToolChoice: choice}); err != nil {
			t.Fatal(err)
		}
	}
	run.Finish(ctx, airun.StatusOK, "")

	steps := stepsBySeq(store)
	if got := decodeRequest(t, steps[0]).ToolChoice; got == nil || got.Name != "submit_plan" {
		t.Errorf("step 1 tool_choice = %+v; want submit_plan", got)
	}
	if got := decodeRequest(t, steps[1]).ToolChoice; got != nil {
		t.Errorf("step 2 tool_choice = %+v; want none", got)
	}
}

func TestMeter_FailedStepResetsDeltaOffset(t *testing.T) {
	store := newMemStore()
	store.failStepAt = 2
	rec, _ := newRecorder(t, store)
	client := rec.Meter(fake.NewFromScript(&fake.Script{Turns: []fake.Turn{textTurn("ok", fake.ScriptedUsage{})}}), "openai")
	ctx, run := rec.Start(context.Background(), airun.Start{Kind: airun.KindChat})

	history := []llm.Message{textMsg(llm.RoleUser, "u1")}
	for i := 0; i < 3; i++ {
		if _, err := client.Complete(ctx, llm.Request{Model: "m", Messages: history}); err != nil {
			t.Fatal(err)
		}
		history = append(history, textMsg(llm.RoleUser, fmt.Sprintf("u%d", i+2)))
	}
	run.Finish(ctx, airun.StatusOK, "")

	_, _, steps, _ := store.snapshot() // calls are sequential, so in seq order
	if len(steps) != 2 {
		t.Fatalf("stored %d steps; want 2 (the second insert fails)", len(steps))
	}
	// The call after the unstored one must not continue it: replaying from
	// stored steps alone needs its full history.
	b := decodeRequest(t, steps[1])
	if steps[1].Seq != 3 || b.MessageOffset != 0 || len(b.Messages) != 3 {
		t.Errorf("step seq %d: offset %d, %d messages; want seq 3, offset 0, 3 messages", steps[1].Seq, b.MessageOffset, len(b.Messages))
	}
}

func TestMeter_DeltaResetsWhenHistoryDoesNotContinue(t *testing.T) {
	tests := []struct {
		name   string
		second []llm.Message
	}{
		{"shorter history", []llm.Message{textMsg(llm.RoleUser, "a")}},
		{"same length, different messages", []llm.Message{textMsg(llm.RoleUser, "x"), textMsg(llm.RoleUser, "y")}},
		{"longer, different prefix", []llm.Message{textMsg(llm.RoleUser, "x"), textMsg(llm.RoleUser, "y"), textMsg(llm.RoleUser, "z")}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newMemStore()
			rec, _ := newRecorder(t, store)
			client := rec.Meter(fake.NewFromScript(&fake.Script{Turns: []fake.Turn{textTurn("ok", fake.ScriptedUsage{})}}), "openai")
			ctx, run := rec.Start(context.Background(), airun.Start{Kind: airun.KindLookup})

			first := []llm.Message{textMsg(llm.RoleUser, "a"), textMsg(llm.RoleAssistant, "b")}
			for _, msgs := range [][]llm.Message{first, tt.second} {
				if _, err := client.Complete(ctx, llm.Request{Model: "m", Messages: msgs}); err != nil {
					t.Fatal(err)
				}
			}
			run.Finish(ctx, airun.StatusOK, "")

			b := decodeRequest(t, stepsBySeq(store)[1])
			if b.MessageOffset != 0 || len(b.Messages) != len(tt.second) {
				t.Errorf("offset %d, %d messages; want 0, %d", b.MessageOffset, len(b.Messages), len(tt.second))
			}
		})
	}
}

func TestMeter_ToolsStoredOncePerHash(t *testing.T) {
	store := newMemStore()
	rec, _ := newRecorder(t, store)
	client := rec.Meter(fake.NewFromScript(&fake.Script{Turns: []fake.Turn{textTurn("ok", fake.ScriptedUsage{})}}), "anthropic")
	ctx, run := rec.Start(context.Background(), airun.Start{Kind: airun.KindChat})

	toolsA := []llm.Tool{{Name: "a", Description: "A", Schema: json.RawMessage(`{"type":"object"}`)}}
	toolsB := []llm.Tool{{Name: "b", Description: "B", Schema: json.RawMessage(`{"type":"object"}`)}}
	for _, tools := range [][]llm.Tool{toolsA, toolsA, toolsB, toolsA, nil} {
		if _, err := client.Complete(ctx, llm.Request{Model: "m", Tools: tools}); err != nil {
			t.Fatal(err)
		}
	}
	run.Finish(ctx, airun.StatusOK, "")

	store.mu.Lock()
	toolUps, tools := store.toolUps, store.tools
	store.mu.Unlock()
	if toolUps != 2 || len(tools) != 2 {
		t.Fatalf("tool schema upserts = %d (%d hashes); want 2", toolUps, len(tools))
	}
	steps := stepsBySeq(store)
	hashA := decodeRequest(t, steps[0]).ToolsHash
	if hashA == nil {
		t.Fatal("tools_hash is null")
	}
	var stored []llm.Tool
	if err := json.Unmarshal([]byte(tools[*hashA]), &stored); err != nil || len(stored) != 1 || stored[0].Name != "a" {
		t.Errorf("stored tools for hash A = %s (%v)", tools[*hashA], err)
	}
	if h := decodeRequest(t, steps[1]).ToolsHash; h == nil || *h != *hashA {
		t.Errorf("second call tools_hash = %v; want %s", h, *hashA)
	}
	if h := decodeRequest(t, steps[2]).ToolsHash; h == nil || *h == *hashA {
		t.Errorf("toolsB hash = %v; want a different hash", h)
	}
	if h := decodeRequest(t, steps[4]).ToolsHash; h != nil {
		t.Errorf("call without tools has tools_hash %q", *h)
	}
}

func TestMeter_EveryDistinctSystemPromptStored(t *testing.T) {
	store := newMemStore()
	rec, _ := newRecorder(t, store)
	client := rec.Meter(fake.NewFromScript(&fake.Script{Turns: []fake.Turn{textTurn("ok", fake.ScriptedUsage{})}}), "openai")
	ctx, run := rec.Start(context.Background(), airun.Start{Kind: airun.KindLookup})

	for _, sys := range []string{"", "first", "second", "first"} {
		if _, err := client.Complete(ctx, llm.Request{Model: "m", System: sys}); err != nil {
			t.Fatal(err)
		}
	}
	run.Finish(ctx, airun.StatusOK, "")

	_, finishes, _, upserts := store.snapshot()
	if upserts != 2 || store.prompts[sha("first")] != "first" || store.prompts[sha("second")] != "second" {
		t.Errorf("upserts = %d, prompts = %v; want first and second once each", upserts, store.prompts)
	}
	if finishes[0].PromptHash != sha("first") {
		t.Errorf("run prompt hash = %q; want the first call's prompt", finishes[0].PromptHash)
	}
	steps := stepsBySeq(store)
	if h := decodeRequest(t, steps[0]).SystemHash; h != nil {
		t.Errorf("call without system prompt has system_hash %q", *h)
	}
	for i, want := range []string{"first", "second", "first"} {
		if h := decodeRequest(t, steps[i+1]).SystemHash; h == nil || *h != sha(want) {
			t.Errorf("step %d system_hash = %v; want hash of %q", i+2, h, want)
		}
	}
}

func TestMeter_RecordsResponseAndStopReason(t *testing.T) {
	store := newMemStore()
	rec, _ := newRecorder(t, store)
	inner := fake.NewFromScript(&fake.Script{Turns: []fake.Turn{{
		StopReason: "tool_use",
		Message: fake.ScriptedMessage{Content: []fake.ScriptedBlock{
			{Type: "text", Text: "let me look"},
			{Type: "tool_use", ToolUseID: "t1", ToolName: "search", ToolInput: json.RawMessage(`{"q":"fish"}`)},
		}},
	}}})
	ctx, run := rec.Start(context.Background(), airun.Start{Kind: airun.KindChat})
	resp, err := drainStream(ctx, rec.Meter(inner, "anthropic"), llm.Request{Model: "m"})
	if err != nil {
		t.Fatal(err)
	}
	run.Finish(ctx, airun.StatusOK, "")

	s := stepsBySeq(store)[0]
	if s.StopReason != "tool_use" {
		t.Errorf("stop reason = %q", s.StopReason)
	}
	b := decodeResponse(t, s)
	if b.StopReason != "tool_use" || b.Truncated {
		t.Errorf("response = %+v", b)
	}
	got, _ := json.Marshal(b.Content)
	want, _ := json.Marshal(resp.Message.Content)
	if string(got) != string(want) {
		t.Errorf("content = %s\nwant      %s", got, want)
	}
}

func TestMeter_ErrorCallHasNullResponse(t *testing.T) {
	store := newMemStore()
	rec, _ := newRecorder(t, store)
	ctx, run := rec.Start(context.Background(), airun.Start{Kind: airun.KindChat})
	_, err := drainStream(ctx, rec.Meter(errClient{errors.New("boom")}, "openai"),
		llm.Request{Model: "m", Messages: []llm.Message{textMsg(llm.RoleUser, "hi")}})
	if err == nil {
		t.Fatal("want error")
	}
	run.Finish(ctx, airun.StatusError, "")

	s := stepsBySeq(store)[0]
	if !s.IsError || s.ResponseJSON != "" || s.StopReason != "" {
		t.Errorf("step = %+v; want is_error with null response and stop reason", s)
	}
	if b := decodeRequest(t, s); len(b.Messages) != 1 {
		t.Errorf("request messages = %d; want the request recorded", len(b.Messages))
	}
}

func TestMeter_LargeBodiesTruncatedToValidJSON(t *testing.T) {
	const limit = 256 << 10
	big := strings.Repeat("é\"<", 300<<10/4)
	store := newMemStore()
	rec, _ := newRecorder(t, store)
	inner := fake.NewFromScript(&fake.Script{Turns: []fake.Turn{textTurn(big, fake.ScriptedUsage{})}})
	ctx, run := rec.Start(context.Background(), airun.Start{Kind: airun.KindImportFallback})
	req := llm.Request{Model: "m", Messages: []llm.Message{
		textMsg(llm.RoleUser, "keep me"),
		{Role: llm.RoleUser, Content: []llm.ContentBlock{
			{Type: llm.ContentTypeText, Text: big},
			{Type: llm.ContentTypeToolResult, ToolResultID: "r", ToolResultContent: json.RawMessage(`{"blob":"` + strings.Repeat("x", 300<<10) + `"}`)},
		}},
	}}
	if _, err := rec.Meter(inner, "openai").Complete(ctx, req); err != nil {
		t.Fatal(err)
	}
	run.Finish(ctx, airun.StatusOK, "")

	s := stepsBySeq(store)[0]
	if len(s.RequestJSON) > limit || len(s.ResponseJSON) > limit {
		t.Fatalf("bodies = %d / %d bytes; want ≤ %d", len(s.RequestJSON), len(s.ResponseJSON), limit)
	}
	b := decodeRequest(t, s)
	if !b.Truncated {
		t.Error("request truncated = false")
	}
	if b.Messages[0].Content[0].Text != "keep me" {
		t.Errorf("small message changed: %q", b.Messages[0].Content[0].Text)
	}
	if !json.Valid(b.Messages[1].Content[1].ToolResultContent) {
		t.Error("cut tool result is not valid JSON")
	}
	if !strings.HasSuffix(b.Messages[1].Content[0].Text, "…[truncated]") {
		t.Error("cut text lacks the marker")
	}
	if r := decodeResponse(t, s); !r.Truncated || len(r.Content) != 1 {
		t.Errorf("response truncated = %v, %d blocks", r.Truncated, len(r.Content))
	}
	// The caller's request is untouched.
	if req.Messages[1].Content[0].Text != big {
		t.Error("meter mutated the caller's request")
	}
}

func TestRecorder_StoresRunIdentity(t *testing.T) {
	store := newMemStore()
	rec, _ := newRecorder(t, store)

	conv := int64(7)
	_, chatRun := rec.Start(context.Background(), airun.Start{Kind: airun.KindChat, ConversationID: &conv})
	chatRun.Finish(context.Background(), airun.StatusOK, "")

	var seen string
	h := middleware.RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = middleware.GetReqID(r.Context())
		_, run := rec.Start(r.Context(), airun.Start{Kind: airun.KindLookup})
		run.Finish(r.Context(), airun.StatusOK, "")
	}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))

	runs, _, _, _ := store.snapshot()
	if runs[1].ConversationID == nil || *runs[1].ConversationID != 7 || runs[1].RequestID != "" {
		t.Errorf("chat run start = %+v", runs[1])
	}
	if seen == "" || runs[2].RequestID != seen || runs[2].ConversationID != nil {
		t.Errorf("request run id = %q; want %q", runs[2].RequestID, seen)
	}
}

func TestRecorder_StoresGenerationFields(t *testing.T) {
	store := newMemStore()
	rec, _ := newRecorder(t, store)

	ctx, run := rec.Start(context.Background(), airun.Start{
		Kind: airun.KindRegenerate, Chooser: airun.ChooserLLM, ScopeJSON: `{"kind":"day","date":"2026-11-02"}`,
	})
	run.FinishWith(ctx, airun.StatusPartial, "", airun.Result{
		RepairAttempts: 2, ViolationsJSON: `[{"rule":"H2"}]`, ScoreJSON: `{"coverage":1}`, Notes: "Kept Monday.",
	})
	run.Finish(ctx, airun.StatusOK, "") // only the first finish counts

	_, chatRun := rec.Start(context.Background(), airun.Start{Kind: airun.KindChat})
	chatRun.Finish(context.Background(), airun.StatusOK, "")

	runs, finishes, _, _ := store.snapshot()
	assert.Equal(t, airun.ChooserLLM, runs[1].Chooser)
	assert.Equal(t, `{"kind":"day","date":"2026-11-02"}`, runs[1].ScopeJSON)
	assert.Equal(t, airun.Chooser(""), runs[2].Chooser, "chat runs have no chooser")
	require.Len(t, finishes, 2)
	f := finishes[0]
	assert.Equal(t, airun.StatusPartial, f.Status)
	assert.Equal(t, 2, f.RepairAttempts)
	assert.Equal(t, `[{"rule":"H2"}]`, f.ViolationsJSON)
	assert.Equal(t, `{"coverage":1}`, f.ScoreJSON)
	assert.Equal(t, "Kept Monday.", f.Notes)
	assert.Equal(t, airun.RunEnd{ID: 2, Status: airun.StatusOK, FinishedAt: finishes[1].FinishedAt}, finishes[1])
}

// manualClock is a clock tests move by hand.
type manualClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *manualClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *manualClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

func TestRecorder_PruneAfterFinishAtMostHourly(t *testing.T) {
	store := newMemStore()
	clock := &manualClock{t: fixedNow}
	rec, _ := newRecorderWithClock(t, store, clock.now)
	rec.WithRetention(func(context.Context) (int, error) { return 180, nil })

	finish := func() {
		ctx, run := rec.Start(context.Background(), airun.Start{Kind: airun.KindChat})
		run.Finish(ctx, airun.StatusOK, "")
	}
	finish() // prunes
	clock.advance(30 * time.Minute)
	finish() // throttled
	clock.advance(31 * time.Minute)
	finish() // prunes again
	finish() // throttled
	rec.Wait()
	// Prunes run in the background, so they may land in either order.
	slices.SortFunc(store.prunes, time.Time.Compare)

	store.mu.Lock()
	prunes := append([]time.Time(nil), store.prunes...)
	store.mu.Unlock()
	want := []time.Time{
		fixedNow.Add(-180 * 24 * time.Hour),
		fixedNow.Add(61*time.Minute - 180*24*time.Hour),
	}
	if len(prunes) != len(want) || !prunes[0].Equal(want[0]) || !prunes[1].Equal(want[1]) {
		t.Errorf("prunes = %v; want %v", prunes, want)
	}
}

func TestRecorder_PruneNowAndFailuresLogged(t *testing.T) {
	store := newMemStore()
	store.pruneErr = errors.New("db locked")
	clock := &manualClock{t: fixedNow}
	rec, logs := newRecorderWithClock(t, store, clock.now)
	rec.WithRetention(func(context.Context) (int, error) { return 30, nil })

	rec.Prune(context.Background()) // startup prune
	ctx, run := rec.Start(context.Background(), airun.Start{Kind: airun.KindChat})
	run.Finish(ctx, airun.StatusOK, "") // within the hour: no second prune
	rec.Wait()

	store.mu.Lock()
	prunes := append([]time.Time(nil), store.prunes...)
	store.mu.Unlock()
	if len(prunes) != 1 || !prunes[0].Equal(fixedNow.Add(-30*24*time.Hour)) {
		t.Errorf("prunes = %v", prunes)
	}
	if !strings.Contains(logs.String(), `"level":"WARN","msg":"ai.run.prune failed"`) {
		t.Errorf("want a warn log for the failed prune, got %s", logs.String())
	}
}

func TestRecorder_NoRetentionNeverPrunes(t *testing.T) {
	store := newMemStore()
	rec, _ := newRecorder(t, store)
	rec.Prune(context.Background())
	ctx, run := rec.Start(context.Background(), airun.Start{Kind: airun.KindChat})
	run.Finish(ctx, airun.StatusOK, "")
	rec.Wait()
	if len(store.prunes) != 0 {
		t.Errorf("prunes = %v; want none without a retention source", store.prunes)
	}
}

func TestRecorder_RetentionPrunesRealDB(t *testing.T) {
	db := testhelper.NewTestDB(t)
	repo := sqlite.NewAIRunRepo(db)
	clock := &manualClock{t: fixedNow.Add(-200 * 24 * time.Hour)}
	rec, _ := newRecorderWithClock(t, repo, clock.now)
	inner := fake.NewFromScript(&fake.Script{Turns: []fake.Turn{textTurn("ok", fake.ScriptedUsage{})}})

	call := func() {
		ctx, run := rec.Start(context.Background(), airun.Start{Kind: airun.KindLookup})
		if _, err := rec.Meter(inner, "fake").Complete(ctx, llm.Request{Model: "m"}); err != nil {
			t.Fatal(err)
		}
		run.Finish(ctx, airun.StatusOK, "")
	}
	call() // 200 days old
	clock.advance(190 * 24 * time.Hour)
	call() // 10 days old
	clock.advance(10 * 24 * time.Hour)

	rec.WithRetention(func(context.Context) (int, error) { return 180, nil })
	rec.Prune(context.Background())
	rec.Wait()

	rows, err := db.Query(`SELECT r.id, COUNT(s.id) FROM ai_runs r LEFT JOIN ai_run_steps s ON s.run_id = r.id GROUP BY r.id ORDER BY r.id`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var got [][2]int64
	for rows.Next() {
		var id, n int64
		if err := rows.Scan(&id, &n); err != nil {
			t.Fatal(err)
		}
		got = append(got, [2]int64{id, n})
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if want := [][2]int64{{1, 0}, {2, 1}}; fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("runs/steps = %v; want %v (old run kept without steps)", got, want)
	}
}

func TestRecorder_ParallelFinishesPruneOnce(t *testing.T) {
	const n = 20
	store := newMemStore()
	rec, _ := newRecorder(t, store) // fixed clock: every Finish is within the hour
	rec.WithRetention(func(context.Context) (int, error) { return 180, nil })
	t.Cleanup(rec.Wait)

	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, run := rec.Start(context.Background(), airun.Start{Kind: airun.KindChat})
			run.Finish(ctx, airun.StatusOK, "")
		}()
	}
	wg.Wait()
	rec.Wait()

	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.prunes) != 1 {
		t.Errorf("PruneSteps calls = %d; want 1", len(store.prunes))
	}
}

func TestRecorder_PruneAfterFinishIgnoresCancelledCtx(t *testing.T) {
	store := newMemStore()
	rec, _ := newRecorder(t, store)
	rec.WithRetention(func(ctx context.Context) (int, error) { return 180, ctx.Err() })

	ctx, cancel := context.WithCancel(context.Background())
	ctx, run := rec.Start(ctx, airun.Start{Kind: airun.KindChat})
	cancel()
	run.Finish(ctx, airun.StatusCancelled, "")
	rec.Wait()

	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.prunes) != 1 {
		t.Errorf("PruneSteps calls = %d; want 1 despite the cancelled request ctx", len(store.prunes))
	}
}

func TestRecorder_NoPruneAfterWait(t *testing.T) {
	store := newMemStore()
	rec, _ := newRecorder(t, store)
	rec.WithRetention(func(context.Context) (int, error) { return 180, nil })
	rec.Wait() // shutdown: the DB is about to close

	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// A request that outlived srv.Shutdown's timeout.
			ctx, run := rec.Start(context.Background(), airun.Start{Kind: airun.KindChat})
			run.Finish(ctx, airun.StatusOK, "")
			rec.Prune(context.Background())
		}()
	}
	wg.Wait()
	rec.Wait()

	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.prunes) != 0 {
		t.Errorf("prunes = %v; want none after Wait", store.prunes)
	}
}

// blockingPruneStore holds PruneSteps until release is closed.
type blockingPruneStore struct {
	*memStore
	release chan struct{}
}

func (s *blockingPruneStore) PruneSteps(ctx context.Context, before time.Time) (int64, error) {
	<-s.release
	return s.memStore.PruneSteps(ctx, before)
}

func TestRecorder_PruneRunsInBackground(t *testing.T) {
	store := &blockingPruneStore{memStore: newMemStore(), release: make(chan struct{})}
	rec, _ := newRecorder(t, store)
	rec.WithRetention(func(context.Context) (int, error) { return 30, nil })

	rec.Prune(context.Background()) // must return without waiting for the store
	close(store.release)
	rec.Wait()

	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.prunes) != 1 {
		t.Errorf("prunes = %v; want one, covered by Wait", store.prunes)
	}
}
