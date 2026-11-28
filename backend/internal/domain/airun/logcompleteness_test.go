package airun_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/airun"
	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
)

// L3: the provider's request id is stored on a successful call (from the
// response), and on a failed one (from the provider error), and logged.
func TestMeter_StoresProviderRequestID(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		store := newMemStore()
		rec, logs := newRecorder(t, store)
		ctx, run := rec.Start(context.Background(), airun.Start{Kind: airun.KindChat})
		_, err := drainStream(ctx, rec.Meter(metricInner{resp: &llm.Response{StopReason: llm.StopReasonEndTurn, RequestID: "req_ok_1"}}, "anthropic"), llm.Request{Model: "m"})
		require.NoError(t, err)
		run.Finish(ctx, airun.StatusOK, "")

		_, _, steps, _ := store.snapshot()
		require.Len(t, steps, 1)
		assert.Equal(t, "req_ok_1", steps[0].ProviderRequestID)
		call := findLog(logLines(t, logs.String()), "ai.llm.call")
		require.NotNil(t, call)
		assert.Equal(t, "req_ok_1", call["provider_request_id"])
	})
	t.Run("provider error", func(t *testing.T) {
		store := newMemStore()
		rec, _ := newRecorder(t, store)
		ctx, run := rec.Start(context.Background(), airun.Start{Kind: airun.KindChat})
		_, err := drainStream(ctx, rec.Meter(metricInner{err: provErr{}}, "anthropic"), llm.Request{Model: "m"})
		require.Error(t, err)
		run.Finish(ctx, airun.StatusError, "error.ai.provider_error")

		_, _, steps, _ := store.snapshot()
		require.Len(t, steps, 1)
		assert.Equal(t, "req_011CX", steps[0].ProviderRequestID)
	})
	t.Run("partial stream", func(t *testing.T) {
		store := newMemStore()
		rec, _ := newRecorder(t, store)
		ctx, run := rec.Start(context.Background(), airun.Start{Kind: airun.KindChat})
		partial := &llm.Response{Partial: true, RequestID: "req_part_1"}
		_, err := drainStream(ctx, rec.Meter(metricInner{resp: partial, err: assert.AnError}, "anthropic"), llm.Request{Model: "m"})
		require.Error(t, err)
		run.Finish(ctx, airun.StatusError, "error.server")

		_, _, steps, _ := store.snapshot()
		require.Len(t, steps, 1)
		assert.Equal(t, "req_part_1", steps[0].ProviderRequestID)
	})
	t.Run("none known", func(t *testing.T) {
		store := newMemStore()
		rec, _ := newRecorder(t, store)
		ctx, run := rec.Start(context.Background(), airun.Start{Kind: airun.KindChat})
		_, err := drainStream(ctx, rec.Meter(metricInner{resp: &llm.Response{}}, "fake"), llm.Request{Model: "m"})
		require.NoError(t, err)
		run.Finish(ctx, airun.StatusOK, "")
		_, _, steps, _ := store.snapshot()
		require.Len(t, steps, 1)
		assert.Empty(t, steps[0].ProviderRequestID)
	})
}

// L4: the recorder stamps every run with the app version.
func TestRecorder_StoresAppVersion(t *testing.T) {
	store := newMemStore()
	rec, _ := newRecorder(t, store)
	rec.WithAppVersion("v1.2.3+abc1234")
	_, run := rec.Start(context.Background(), airun.Start{Kind: airun.KindChat})
	runs, _, _, _ := store.snapshot()
	assert.Equal(t, "v1.2.3+abc1234", runs[run.ID()].AppVersion)
}

// L5: request_json.sent carries every effective parameter beyond tools,
// system and messages.
func TestMeter_RecordsEveryEffectiveParameter(t *testing.T) {
	store := newMemStore()
	rec, _ := newRecorder(t, store)
	ctx, run := rec.Start(context.Background(), airun.Start{Kind: airun.KindChat})
	eff := llm.Effective{
		MaxTokens: 8000, Thinking: true, ThinkingType: "adaptive", ThinkingDisplay: "summarized",
		Effort: "high", Betas: []string{"beta-1"}, CacheBreakpoints: []string{"system", "tools"},
	}
	_, err := drainStream(ctx, rec.Meter(metricInner{eff: &eff, resp: &llm.Response{}}, "anthropic"), llm.Request{Model: "m"})
	require.NoError(t, err)
	run.Finish(ctx, airun.StatusOK, "")

	_, _, steps, _ := store.snapshot()
	var body struct {
		Sent map[string]any `json:"sent"`
	}
	require.NoError(t, json.Unmarshal([]byte(steps[0].RequestJSON), &body))
	assert.Equal(t, "adaptive", body.Sent["thinking_type"])
	assert.Equal(t, "summarized", body.Sent["thinking_display"])
	assert.Equal(t, "high", body.Sent["effort"])
	assert.Equal(t, []any{"beta-1"}, body.Sent["betas"])
	assert.Equal(t, []any{"system", "tools"}, body.Sent["cache_breakpoints"])
	assert.Equal(t, true, body.Sent["thinking"])
}

func TestBuildVersion_NeverPanics(t *testing.T) {
	// `go test` binaries carry no VCS stamp: empty is valid; a stamp is a
	// short revision.
	v := airun.BuildVersion()
	assert.LessOrEqual(t, len(v), 12+len("-dirty"))
}

// A run that ends in error is one error-level line, matchable to its row by
// run_id and request_id; a good run stays info.
func TestRecorder_FailedRunEndsAtErrorLevel(t *testing.T) {
	for status, level := range map[airun.Status]string{airun.StatusError: "ERROR", airun.StatusOK: "INFO"} {
		rec, logs := newRecorder(t, newMemStore())
		ctx, run := rec.Start(context.Background(), airun.Start{Kind: airun.KindChefFind})
		run.Finish(ctx, status, "error.server")
		end := findLog(logLines(t, logs.String()), "ai.run.end")
		require.NotNil(t, end)
		assert.Equal(t, level, end["level"], "status %s", status)
		assert.Contains(t, end, "run_id")
		assert.Contains(t, end, "request_id")
		assert.Equal(t, string(airun.KindChefFind), end["kind"])
	}
}
