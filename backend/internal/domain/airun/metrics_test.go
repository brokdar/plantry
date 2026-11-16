package airun_test

import (
	"context"
	"encoding/json"
	"errors"
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
	eff  *llm.Effective
}

func (c metricInner) Stream(_ context.Context, _ llm.Request, out chan<- llm.Event) (*llm.Response, error) {
	defer close(out)
	out <- llm.Event{Type: llm.EventAssistantDelta, Payload: llm.AssistantDeltaPayload{Text: "hi"}}
	return c.resp, c.err
}

func (c metricInner) Complete(context.Context, llm.Request) (string, error) { return "", nil }

// EffectiveParams: the adapter sent max_tokens 4096 and no temperature.
func (c metricInner) EffectiveParams(llm.Request) llm.Effective {
	if c.eff != nil {
		return *c.eff
	}
	return llm.Effective{MaxTokens: 4096}
}

type provErr struct{}

func (provErr) Error() string             { return "boom with secret-prompt-text" }
func (provErr) ProviderStatus() int       { return 529 }
func (provErr) ProviderErrorType() string { return "overloaded_error" }
func (provErr) ProviderInfo() llm.ProviderErrorInfo {
	return llm.ProviderErrorInfo{
		Status: 529, Type: "overloaded_error", Message: "Overloaded", RequestID: "req_011CX",
		RateLimit: "retry-after=30", RawBody: `{"type":"error","error":{"type":"overloaded_error","message":"Overloaded"}}`,
	}
}

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
	assert.Equal(t, "Overloaded", call["provider_message"])
	assert.Equal(t, "req_011CX", call["provider_request_id"])
	assert.Equal(t, "retry-after=30", call["provider_rate_limit"])
	assert.Equal(t, true, call["after_first_token"], "a delta streamed before the failure")
	assert.NotContains(t, logs.String(), "secret-prompt-text")
}

// A failed call keeps the provider's answer on the step: status, type,
// message, request id and the raw body.
func TestMeter_FailedCallStoresProviderError(t *testing.T) {
	store := newMemStore()
	rec, _ := newRecorder(t, store)
	ctx, run := rec.Start(context.Background(), airun.Start{Kind: airun.KindLookup})
	_, err := drainStream(ctx, rec.Meter(metricInner{err: provErr{}}, "anthropic"), llm.Request{Model: "m"})
	require.Error(t, err)
	run.Finish(ctx, airun.StatusError, "error.ai.provider_error")

	_, _, steps, _ := store.snapshot()
	require.Len(t, steps, 1)
	assert.True(t, steps[0].IsError)
	assert.Empty(t, steps[0].StopReason)
	var body struct {
		Error struct {
			Status          int    `json:"status"`
			Type            string `json:"type"`
			Message         string `json:"message"`
			RequestID       string `json:"request_id"`
			RawBody         string `json:"raw_body"`
			AfterFirstToken bool   `json:"after_first_token"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal([]byte(steps[0].ResponseJSON), &body))
	assert.Equal(t, 529, body.Error.Status)
	assert.Equal(t, "overloaded_error", body.Error.Type)
	assert.Equal(t, "Overloaded", body.Error.Message)
	assert.Equal(t, "req_011CX", body.Error.RequestID)
	assert.Contains(t, body.Error.RawBody, "overloaded_error")
	assert.True(t, body.Error.AfterFirstToken)
}

// A stream that broke off keeps the billed tokens and the text so far, and
// the step says it is partial.
func TestMeter_PartialStreamKeepsTokensAndText(t *testing.T) {
	store := newMemStore()
	rec, logs := newRecorder(t, store)
	ctx, run := rec.Start(context.Background(), airun.Start{Kind: airun.KindChat})
	partial := &llm.Response{
		Message: llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentBlock{{Type: llm.ContentTypeText, Text: "half an ans"}}},
		Usage:   llm.Usage{InputTokens: 300, OutputTokens: 7}, Partial: true,
	}
	_, err := drainStream(ctx, rec.Meter(metricInner{resp: partial, err: errors.New("anthropic: stream scan: unexpected EOF")}, "anthropic"), llm.Request{Model: "m"})
	require.Error(t, err)
	run.Finish(ctx, airun.StatusError, "error.server")

	_, ends, steps, _ := store.snapshot()
	require.Len(t, steps, 1)
	assert.EqualValues(t, 300, steps[0].InputTokens)
	assert.EqualValues(t, 7, steps[0].OutputTokens)
	assert.EqualValues(t, 300, ends[0].InputTokens, "billed tokens count toward the run")
	var body struct {
		Partial bool `json:"partial"`
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
		Error struct {
			Text string `json:"text"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal([]byte(steps[0].ResponseJSON), &body))
	assert.True(t, body.Partial)
	assert.Equal(t, "half an ans", body.Content[0].Text)
	assert.Contains(t, body.Error.Text, "unexpected EOF")
	call := findLog(logLines(t, logs.String()), "ai.llm.call")
	require.NotNil(t, call)
	assert.Equal(t, true, call["partial"])
	assert.Equal(t, float64(300), call["input_tokens"])
}

// The request stores and the log carries what went over the wire and why a
// fallback was taken.
func TestMeter_RecordsSentParamsAndFallbackReasons(t *testing.T) {
	store := newMemStore()
	rec, logs := newRecorder(t, store)
	ctx, run := rec.Start(context.Background(), airun.Start{Kind: airun.KindGenerate})
	eff := llm.Effective{MaxTokens: 8000, ToolChoice: "auto", Fallbacks: []string{"model_always_thinks", "temperature_rejected_by_model"}}
	inner := metricInner{eff: &eff, resp: &llm.Response{StopReason: llm.StopReasonEndTurn}}
	_, err := drainStream(ctx, rec.Meter(inner, "anthropic"), llm.Request{
		Model: "m", ToolChoice: &llm.ToolChoice{Name: "submit_plan"}, Temperature: 0.4,
	})
	require.NoError(t, err)
	run.Finish(ctx, airun.StatusOK, "")

	_, _, steps, _ := store.snapshot()
	var req struct {
		Temperature *float64 `json:"temperature"`
		Sent        struct {
			ToolChoice string   `json:"tool_choice"`
			WebSearch  bool     `json:"web_search"`
			Fallbacks  []string `json:"fallbacks"`
		} `json:"sent"`
	}
	require.NoError(t, json.Unmarshal([]byte(steps[0].RequestJSON), &req))
	assert.Nil(t, req.Temperature, "none was sent")
	assert.Equal(t, "auto", req.Sent.ToolChoice)
	assert.Equal(t, []string{"model_always_thinks", "temperature_rejected_by_model"}, req.Sent.Fallbacks)
	call := findLog(logLines(t, logs.String()), "ai.llm.call")
	require.NotNil(t, call)
	assert.Equal(t, "auto", call["sent_tool_choice"])
	assert.Equal(t, false, call["sent_temperature"])
	assert.Equal(t, "model_always_thinks; temperature_rejected_by_model", call["fallbacks"])
}

// A 400 the adapter retried without a parameter is recorded as a failed step
// of its own, with the reason, before the final step.
type retryingInner struct{ metricInner }

func (c retryingInner) Stream(ctx context.Context, req llm.Request, out chan<- llm.Event) (*llm.Response, error) {
	t0 := time.Now()
	llm.ReportAttempt(ctx, llm.Attempt{
		Started: t0, Duration: 40 * time.Millisecond, Err: provErr{}, Reason: "temperature_rejected_400",
		Sent: llm.Effective{MaxTokens: 100, ToolChoice: "forced", Temperature: ptr(0.4)},
	})
	return c.metricInner.Stream(ctx, req, out)
}

func TestMeter_RetriedAttemptsAreSteps(t *testing.T) {
	store := newMemStore()
	rec, logs := newRecorder(t, store)
	ctx, run := rec.Start(context.Background(), airun.Start{Kind: airun.KindGenerate})
	inner := retryingInner{metricInner{resp: &llm.Response{StopReason: llm.StopReasonEndTurn}}}
	_, err := drainStream(ctx, rec.Meter(inner, "anthropic"), llm.Request{Model: "m", Temperature: 0.4})
	require.NoError(t, err)
	run.Finish(ctx, airun.StatusOK, "")

	_, _, steps, _ := store.snapshot()
	require.Len(t, steps, 2)
	assert.True(t, steps[0].IsError)
	assert.False(t, steps[1].IsError)
	assert.Less(t, steps[0].Seq, steps[1].Seq)
	assert.Contains(t, steps[0].ResponseJSON, `"retry_reason":"temperature_rejected_400"`)
	assert.Contains(t, steps[0].RequestJSON, `"temperature":0.4`)
	var warned bool
	for _, l := range logLines(t, logs.String()) {
		if l["msg"] == "ai.llm.call" && l["retry_reason"] == "temperature_rejected_400" {
			warned = true
		}
	}
	assert.True(t, warned, "the failed attempt is logged with its reason")
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
func ptr[T any](v T) *T { return &v }

// O3: list_foods' search filter is logged; other free text still is not.
func TestRun_ToolStepLogsListFoodsFilter(t *testing.T) {
	store := newMemStore()
	rec, logs := newRecorder(t, store)
	ctx, run := rec.Start(context.Background(), airun.Start{Kind: airun.KindChat})
	run.ToolStep(ctx, "list_foods", json.RawMessage(`{"search":"potato","limit":5,"note":"secret prose"}`),
		json.RawMessage(`{"items":[]}`), false, time.Millisecond)

	call := findLog(logLines(t, logs.String()), "ai.tool.call")
	require.NotNil(t, call)
	assert.Equal(t, `limit=5 search="potato"`, call["args"])
	assert.NotContains(t, logs.String(), "secret prose")
}
