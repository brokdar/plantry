package anthropic_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/anthropic"
	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
)

// rejectingServer answers GET /v1/models/{id} with model and every POST whose
// body contains one of the needles with a 400 naming it; other POSTs stream a
// reply. It records the POST bodies.
type rejectingServer struct {
	mu     sync.Mutex
	bodies []string
	reject map[string]string // body substring → 400 message
}

func (s *rejectingServer) handler(t *testing.T, model string) http.HandlerFunc {
	sse, err := os.ReadFile("testdata/stream_text.sse")
	require.NoError(t, err)
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_, _ = io.WriteString(w, model)
			return
		}
		b, _ := io.ReadAll(r.Body)
		s.mu.Lock()
		s.bodies = append(s.bodies, string(b))
		s.mu.Unlock()
		for needle, msg := range s.reject {
			if strings.Contains(string(b), needle) {
				w.Header().Set("request-id", "req_400")
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"type": "error", "error": map[string]string{"type": "invalid_request_error", "message": msg},
				})
				return
			}
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write(sse)
	}
}

func plannerRequest() llm.Request {
	return llm.Request{
		Model: "m", System: "plan", Temperature: 0.4, MaxTokens: 1000,
		Messages:   []llm.Message{{Role: llm.RoleUser, Content: []llm.ContentBlock{{Type: llm.ContentTypeText, Text: "go"}}}},
		Tools:      []llm.Tool{{Name: "submit_plan", Description: "d", Schema: json.RawMessage(`{"type":"object"}`)}},
		ToolChoice: &llm.ToolChoice{Name: "submit_plan"},
	}
}

func streamObserved(t *testing.T, c *anthropic.Client, req llm.Request) (*llm.Response, []llm.Attempt) {
	t.Helper()
	var attempts []llm.Attempt
	ctx := llm.WithAttemptObserver(context.Background(), func(a llm.Attempt) { attempts = append(attempts, a) })
	out := make(chan llm.Event, 64)
	resp, err := c.Stream(ctx, req, out)
	require.NoError(t, err)
	drainEvents(out)
	return resp, attempts
}

// The planner's temperature goes out; a 400 naming it is retried once without
// it, cached per model, and both attempts are reported with the reason.
func TestStream_TemperatureRejectedRetriesWithoutAndCaches(t *testing.T) {
	srv := &rejectingServer{reject: map[string]string{`"temperature"`: "temperature: this model does not accept temperature"}}
	ts := newTestServer(t, srv.handler(t, modelJSON(true, 64000)))
	c := anthropic.New("k", anthropic.WithBaseURL(ts.URL), anthropic.WithHTTPClient(ts.Client()))

	_, attempts := streamObserved(t, c, plannerRequest())
	require.Len(t, srv.bodies, 2)
	assert.Contains(t, srv.bodies[0], `"temperature":0.4`)
	assert.NotContains(t, srv.bodies[1], `"temperature"`)
	assert.Contains(t, srv.bodies[1], `"tool_choice":{"type":"tool"`, "only the refused parameter is dropped")
	require.Len(t, attempts, 1)
	assert.Equal(t, "temperature_rejected_400", attempts[0].Reason)
	assert.NotNil(t, attempts[0].Sent.Temperature)
	var pe llm.ProviderError
	require.ErrorAs(t, attempts[0].Err, &pe)
	assert.Equal(t, "req_400", pe.ProviderInfo().RequestID)
	assert.Contains(t, pe.ProviderInfo().Message, "temperature")
	assert.Contains(t, pe.ProviderInfo().RawBody, "invalid_request_error")

	eff := c.EffectiveParams(plannerRequest())
	assert.Nil(t, eff.Temperature)
	assert.Equal(t, "forced", eff.ToolChoice)
	assert.Contains(t, eff.Fallbacks, "temperature_rejected_by_model")

	_, attempts = streamObserved(t, c, plannerRequest())
	assert.Empty(t, attempts, "the refusal is cached per model")
	require.Len(t, srv.bodies, 3)
	assert.NotContains(t, srv.bodies[2], `"temperature"`)
}

// A wrong thinking.types.disabled.supported yields a 400 naming tool_choice:
// retried once with auto plus the system line, cached for the model.
func TestStream_ForcedToolRejectedRetriesWithAuto(t *testing.T) {
	srv := &rejectingServer{reject: map[string]string{`"type":"tool"`: "tool_choice: forced tool use is not allowed with thinking"}}
	ts := newTestServer(t, srv.handler(t, modelJSON(true, 64000)))
	c := anthropic.New("k", anthropic.WithBaseURL(ts.URL), anthropic.WithHTTPClient(ts.Client()))

	_, attempts := streamObserved(t, c, plannerRequest())
	require.Len(t, srv.bodies, 2)
	assert.Contains(t, srv.bodies[1], `"tool_choice":{"type":"auto"}`)
	assert.Contains(t, srv.bodies[1], "Answer by calling")
	assert.Contains(t, srv.bodies[1], `"temperature":0.4`)
	require.Len(t, attempts, 1)
	assert.Equal(t, "tool_choice_rejected_400", attempts[0].Reason)
	assert.Equal(t, "forced", attempts[0].Sent.ToolChoice)

	eff := c.EffectiveParams(plannerRequest())
	assert.Equal(t, "auto", eff.ToolChoice)
	assert.Contains(t, eff.Fallbacks, "tool_choice_rejected_by_model")
}

// Other 400s are not retried and come back with every detail.
func TestStream_UnrelatedBadRequestKeepsProviderDetails(t *testing.T) {
	srv := &rejectingServer{reject: map[string]string{`"max_tokens"`: "max_tokens: 1000 > 500, the maximum for this model"}}
	ts := newTestServer(t, srv.handler(t, modelJSON(true, 0)))
	c := anthropic.New("k", anthropic.WithBaseURL(ts.URL), anthropic.WithHTTPClient(ts.Client()))
	_, err := c.Stream(context.Background(), plannerRequest(), make(chan llm.Event, 8))
	require.Error(t, err)
	assert.Len(t, srv.bodies, 1)
	var apiErr *anthropic.APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, "req_400", apiErr.RequestID)
	assert.Equal(t, "invalid_request_error", apiErr.ErrorType)
	assert.Contains(t, apiErr.RawBody, "maximum for this model")
}

// A failed capability lookup logs a warning with the run's attributes and is
// reported as the reason for the auto tool choice.
func TestStream_LookupFailureWarnsAndIsAFallbackReason(t *testing.T) {
	sse, err := os.ReadFile("testdata/stream_text.sse")
	require.NoError(t, err)
	ts := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = io.WriteString(w, `{"type":"error","error":{"type":"overloaded_error","message":"x"}}`)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write(sse)
	})
	var logs bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })
	c := anthropic.New("k", anthropic.WithBaseURL(ts.URL), anthropic.WithHTTPClient(ts.Client()))

	ctx := llm.WithLogAttrs(context.Background(), []any{"run_id", 42})
	out := make(chan llm.Event, 64)
	_, err = c.Stream(ctx, plannerRequest(), out)
	require.NoError(t, err)
	drainEvents(out)

	var line map[string]any
	require.NoError(t, json.Unmarshal(bytes.Split(logs.Bytes(), []byte("\n"))[0], &line))
	assert.Equal(t, "WARN", line["level"])
	assert.Equal(t, "anthropic.model.capabilities failed", line["msg"])
	assert.EqualValues(t, 42, line["run_id"])
	assert.Equal(t, "503 overloaded_error", line["failure"])
	assert.NotContains(t, logs.String(), `"k"`, "never the key")

	eff := c.EffectiveParams(plannerRequest())
	assert.Equal(t, "auto", eff.ToolChoice)
	assert.Equal(t, []string{"capability_lookup_failed: 503 overloaded_error"}, eff.Fallbacks)
}

// A model that always thinks is reported as the reason.
func TestEffectiveParams_AlwaysThinkingReason(t *testing.T) {
	srv := &rejectingServer{}
	ts := newTestServer(t, srv.handler(t, modelJSON(false, 64000)))
	c := anthropic.New("k", anthropic.WithBaseURL(ts.URL), anthropic.WithHTTPClient(ts.Client()))
	streamObserved(t, c, plannerRequest())
	eff := c.EffectiveParams(plannerRequest())
	assert.Equal(t, "auto", eff.ToolChoice)
	assert.Equal(t, []string{"model_always_thinks"}, eff.Fallbacks)
	assert.NotNil(t, eff.Temperature, "temperature is sent when the model has not refused it")
}

// The web-search retry is reported as an attempt with its reason and the
// effective request says search was dropped.
func TestStream_WebSearchRetryIsReported(t *testing.T) {
	srv := &rejectingServer{reject: map[string]string{"web_search_20250305": "Web search is not enabled for this organization."}}
	ts := newTestServer(t, srv.handler(t, `{"id":"m"}`))
	c := anthropic.New("k", anthropic.WithBaseURL(ts.URL), anthropic.WithHTTPClient(ts.Client()))
	req := llm.Request{
		Model:     "m",
		Messages:  []llm.Message{{Role: llm.RoleUser, Content: []llm.ContentBlock{{Type: llm.ContentTypeText, Text: "go"}}}},
		WebSearch: &llm.WebSearch{MaxUses: 3},
	}
	_, attempts := streamObserved(t, c, req)
	require.Len(t, attempts, 1)
	assert.Equal(t, "web_search_rejected_by_org", attempts[0].Reason)
	assert.True(t, attempts[0].Sent.WebSearch)
	eff := c.EffectiveParams(req)
	assert.False(t, eff.WebSearch)
	assert.Contains(t, eff.Fallbacks, "web_search_rejected_by_org")
}

// Chat (no forced tool) is clamped to the model's max_tokens too.
func TestStream_ChatMaxTokensClamped(t *testing.T) {
	srv := &rejectingServer{}
	ts := newTestServer(t, srv.handler(t, modelJSON(true, 8000)))
	c := anthropic.New("k", anthropic.WithBaseURL(ts.URL), anthropic.WithHTTPClient(ts.Client()))
	streamObserved(t, c, llm.Request{Model: "m", MaxTokens: 16000})
	require.Len(t, srv.bodies, 1)
	assert.Contains(t, srv.bodies[0], `"max_tokens":8000`)
}

// A lookup that hangs blocks neither other models nor the request params of
// the same model; it ends at its own timeout and the call goes on with auto.
func TestCapability_SlowLookupDoesNotBlockOthers(t *testing.T) {
	release := make(chan struct{})
	var gets atomic.Int32
	sse, err := os.ReadFile("testdata/stream_text.sse")
	require.NoError(t, err)
	ts := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			gets.Add(1)
			if strings.HasSuffix(r.URL.Path, "/slow") {
				select {
				case <-release:
				case <-r.Context().Done():
				}
				w.WriteHeader(http.StatusGatewayTimeout)
				return
			}
			_, _ = io.WriteString(w, modelJSON(true, 5000))
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write(sse)
	})
	t.Cleanup(func() { close(release) })
	c := anthropic.New("k", anthropic.WithBaseURL(ts.URL), anthropic.WithHTTPClient(ts.Client()))
	anthropic.SetLookupTimeout(c, 300*time.Millisecond)

	slowDone := make(chan struct{})
	go func() {
		defer close(slowDone)
		req := plannerRequest()
		req.Model = "slow"
		out := make(chan llm.Event, 64)
		_, _ = c.Stream(context.Background(), req, out)
		drainEvents(out)
	}()
	require.Eventually(t, func() bool { return gets.Load() == 1 }, 2*time.Second, 5*time.Millisecond)

	// While "slow" is being looked up: another model completes, and
	// EffectiveParams (the cache read) does not wait for the lookup.
	fastDone := make(chan struct{})
	go func() {
		defer close(fastDone)
		req := plannerRequest()
		req.Model = "fast"
		streamObserved(t, c, req)
	}()
	select {
	case <-fastDone:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("a lookup of another model waits behind the slow one")
	}
	paramsDone := make(chan struct{})
	go func() { c.EffectiveParams(llm.Request{Model: "slow"}); close(paramsDone) }()
	select {
	case <-paramsDone:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("EffectiveParams waits for a lookup in progress")
	}

	select {
	case <-slowDone:
	case <-time.After(3 * time.Second):
		t.Fatal("the lookup is not bounded by its timeout")
	}
	slow := plannerRequest()
	slow.Model = "slow"
	fallbacks := c.EffectiveParams(slow).Fallbacks
	require.Len(t, fallbacks, 1)
	assert.Contains(t, fallbacks[0], "capability_lookup_failed")
	assert.Contains(t, fallbacks[0], "deadline exceeded")
}

// Concurrent callers of one model share one lookup.
func TestCapability_ConcurrentCallersShareOneLookup(t *testing.T) {
	var gets atomic.Int32
	sse, err := os.ReadFile("testdata/stream_text.sse")
	require.NoError(t, err)
	ts := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			gets.Add(1)
			time.Sleep(50 * time.Millisecond)
			_, _ = io.WriteString(w, modelJSON(true, 5000))
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write(sse)
	})
	c := anthropic.New("k", anthropic.WithBaseURL(ts.URL), anthropic.WithHTTPClient(ts.Client()))
	var wg sync.WaitGroup
	for range 5 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := c.WebSearch(context.Background(), "m")
			assert.NoError(t, err)
		}()
	}
	wg.Wait()
	assert.EqualValues(t, 1, gets.Load())
}
