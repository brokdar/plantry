package airun_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5/middleware"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/fake"
	"github.com/jaltszeimer/plantry/backend/internal/domain/airun"
	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
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
	createErr error
	stepErr   error
	upsertErr error
}

func newMemStore() *memStore {
	return &memStore{runs: map[int64]airun.RunStart{}, prompts: map[string]string{}}
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
	if upserts != 1 {
		t.Errorf("UpsertPrompt calls = %d; want 1 (no retry)", upserts)
	}
	if finishes[0].PromptHash != "" {
		t.Errorf("prompt hash = %q; want empty after failed upsert", finishes[0].PromptHash)
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
