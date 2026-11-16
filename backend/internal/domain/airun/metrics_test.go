package airun_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/airun"
	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
)

// metricInner is a client that streams one text delta and reports the
// metrics the run log records.
type metricInner struct {
	resp *llm.Response
	err  error
}

func (c metricInner) Stream(_ context.Context, _ llm.Request, out chan<- llm.Event) (*llm.Response, error) {
	defer close(out)
	out <- llm.Event{Type: llm.EventAssistantDelta, Payload: llm.AssistantDeltaPayload{Text: "hi"}}
	return c.resp, c.err
}

func (c metricInner) Complete(context.Context, llm.Request) (string, error) { return "", nil }

// EffectiveParams: the adapter sent max_tokens 4096 and no temperature.
func (c metricInner) EffectiveParams(llm.Request) llm.Effective {
	return llm.Effective{MaxTokens: 4096}
}

type provErr struct{}

func (provErr) Error() string             { return "boom with secret-prompt-text" }
func (provErr) ProviderStatus() int       { return 529 }
func (provErr) ProviderErrorType() string { return "overloaded_error" }

func logLines(t *testing.T, raw string) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(raw), "\n") {
		var m map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &m), line)
		out = append(out, m)
	}
	return out
}

func findLog(lines []map[string]any, msg string) map[string]any {
	for _, m := range lines {
		if m["msg"] == msg {
			return m
		}
	}
	return nil
}

func TestMeter_RecordsStepMetricsAndLogsCall(t *testing.T) {
	store := newMemStore()
	rec, logs := newRecorderWithClock(t, store, steppingClock(10*time.Millisecond))
	thinking := json.RawMessage(`{"type":"thinking","thinking":"héllo","signature":"s"}`)
	inner := metricInner{resp: &llm.Response{
		Message: llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentBlock{
			{Type: llm.ContentTypeServer, Raw: thinking}, {Type: llm.ContentTypeText, Text: "ok"},
		}},
		StopReason: llm.StopReasonEndTurn,
		Usage:      llm.Usage{InputTokens: 100, OutputTokens: 50, CacheReadTokens: 900, CacheWriteTokens: 30, ReasoningTokens: 20},
		Model:      "claude-served-1",
	}}

	conv := int64(7)
	ctx := context.WithValue(context.Background(), middleware.RequestIDKey, "req-9")
	ctx, run := rec.Start(ctx, airun.Start{Kind: airun.KindChat, ConversationID: &conv})
	_, err := drainStream(ctx, rec.Meter(inner, "anthropic"), llm.Request{
		Model: "claude-asked", MaxTokens: 0, Temperature: 0.4,
		ToolChoice: &llm.ToolChoice{Name: "submit_plan"},
		Messages:   []llm.Message{textMsg(llm.RoleUser, "SECRET prompt")},
	})
	require.NoError(t, err)
	run.Finish(ctx, airun.StatusOK, "")

	_, finishes, steps, _ := store.snapshot()
	require.Len(t, steps, 1)
	s := steps[0]
	assert.False(t, s.StartedAt.IsZero())
	require.NotNil(t, s.TTFTMs)
	assert.Positive(t, *s.TTFTMs)
	assert.EqualValues(t, 900, s.CacheReadTokens)
	assert.EqualValues(t, 30, s.CacheWriteTokens)
	assert.EqualValues(t, 20, s.ReasoningTokens)
	assert.EqualValues(t, 5, s.ThinkingChars, "thinking text length in characters")
	assert.Equal(t, "claude-served-1", s.ServedModel)
	assert.Equal(t, "claude-asked", s.Name)

	// request_json stores what was sent: the adapter's max_tokens, no temperature.
	req := decodeRequest(t, s)
	assert.Equal(t, 4096, req.MaxTokens)
	assert.Zero(t, req.Temperature)

	require.Len(t, finishes, 1)
	assert.EqualValues(t, 20, finishes[0].ReasoningTokens)
	assert.Positive(t, finishes[0].DurationMs)

	lines := logLines(t, logs.String())
	call := findLog(lines, "ai.llm.call")
	require.NotNil(t, call)
	assert.Equal(t, float64(1), call["run_id"])
	assert.Equal(t, "req-9", call["request_id"])
	assert.Equal(t, float64(7), call["conversation_id"])
	assert.Equal(t, "anthropic", call["provider"])
	assert.Equal(t, "claude-asked", call["model"])
	assert.Equal(t, "claude-served-1", call["served_model"])
	assert.Equal(t, "submit_plan", call["tool_choice"])
	assert.Equal(t, float64(4096), call["max_tokens"])
	assert.Equal(t, float64(900), call["cache_read_tokens"])
	assert.Equal(t, float64(20), call["reasoning_tokens"])
	assert.Equal(t, "end_turn", call["stop_reason"])
	assert.NotNil(t, call["ttft_ms"])
	assert.NotNil(t, call["duration_ms"])
	assert.Positive(t, call["request_bytes"])
	end := findLog(lines, "ai.run.end")
	require.NotNil(t, end)
	assert.Equal(t, float64(7), end["conversation_id"])
	// Debug level only: the shortened bodies.
	assert.NotNil(t, findLog(lines, "ai.llm.body"))
	assert.NotContains(t, mustMarshal(t, call), "SECRET prompt")
}

func mustMarshal(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return string(b)
}

func TestMeter_FailedCallLogsProviderStatusNotText(t *testing.T) {
	store := newMemStore()
	rec, logs := newRecorder(t, store)
	ctx, run := rec.Start(context.Background(), airun.Start{Kind: airun.KindLookup})
	_, err := drainStream(ctx, rec.Meter(metricInner{err: provErr{}}, "anthropic"), llm.Request{Model: "m"})
	require.Error(t, err)
	run.Finish(ctx, airun.StatusError, "error.ai.provider_error")

	call := findLog(logLines(t, logs.String()), "ai.llm.call")
	require.NotNil(t, call)
	assert.Equal(t, "WARN", call["level"])
	assert.Equal(t, float64(529), call["provider_status"])
	assert.Equal(t, "overloaded_error", call["provider_error_type"])
	assert.Equal(t, "529 overloaded_error", call["error"])
	assert.NotContains(t, logs.String(), "secret-prompt-text")
}

func TestRun_ToolStepLogsSummaryWithoutFreeText(t *testing.T) {
	store := newMemStore()
	rec, logs := newRecorder(t, store)
	ctx, run := rec.Start(context.Background(), airun.Start{Kind: airun.KindChat})
	run.ToolStep(ctx, "set_plate_eaters", json.RawMessage(`{"plate_id":3,"date":"2026-11-02","note":"free text here","eaters":2}`),
		json.RawMessage(`{"error":"error.ai.past_date"}`), true, 12*time.Millisecond)

	call := findLog(logLines(t, logs.String()), "ai.tool.call")
	require.NotNil(t, call)
	assert.Equal(t, "set_plate_eaters", call["tool"])
	assert.Equal(t, float64(12), call["duration_ms"])
	assert.Equal(t, true, call["is_error"])
	assert.Equal(t, "error.ai.past_date", call["error_key"])
	assert.Equal(t, "date=2026-11-02 eaters=2 plate_id=3", call["args"])
	assert.NotContains(t, logs.String(), "free text here")

	_, _, steps, _ := store.snapshot()
	require.Len(t, steps, 1)
	assert.False(t, steps[0].StartedAt.IsZero())
}
