package handlers_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain/agent"
	"github.com/jaltszeimer/plantry/backend/internal/domain/component"
	"github.com/jaltszeimer/plantry/backend/internal/domain/ingredient"
	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planner"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
	"github.com/jaltszeimer/plantry/backend/internal/domain/slot"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
	"github.com/jaltszeimer/plantry/backend/internal/transport/http/handlers"
)

type scriptedLLM struct {
	turns []turnScript
	calls int
	fail  error
}

type turnScript struct {
	events     []llm.Event
	stopReason llm.StopReason
	message    llm.Message
}

func (s *scriptedLLM) Stream(ctx context.Context, _ llm.Request, out chan<- llm.Event) (*llm.Response, error) {
	defer close(out)
	if s.fail != nil {
		return nil, s.fail
	}
	if s.calls >= len(s.turns) {
		return nil, errors.New("no more scripted turns")
	}
	t := s.turns[s.calls]
	s.calls++
	for _, e := range t.events {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case out <- e:
		}
	}
	return &llm.Response{Message: t.message, StopReason: t.stopReason}, nil
}

func setupAIRouter(t *testing.T, client llm.Client, enabled bool) (http.Handler, *handlers.AIHandler) {
	t.Helper()
	ctx := context.Background()
	db := testhelper.NewTestDB(t)
	ingRepo := sqlite.NewIngredientRepo(db)
	compRepo := sqlite.NewComponentRepo(db)
	slotRepo := sqlite.NewSlotRepo(db)
	plateRepo := sqlite.NewPlateRepo(db)
	weekRepo := sqlite.NewWeekRepo(db)
	profileRepo := sqlite.NewProfileRepo(db)
	aiRepo := sqlite.NewAIRepo(db)
	txRunner := sqlite.NewTxRunner(db)

	compSvc := component.NewService(compRepo, ingRepo, ingRepo)
	slotSvc := slot.NewService(slotRepo)
	plateSvc := plate.NewService(plateRepo, slotRepo, compRepo)
	plannerSvc := planner.NewService(weekRepo, plateRepo, txRunner)
	profileSvc := profile.NewService(profileRepo)

	// Seed a basic fixture.
	ing := &ingredient.Ingredient{Name: "Chicken", Source: "manual", Kcal100g: 165}
	require.NoError(t, ingRepo.Create(ctx, ing))

	tools, err := agent.NewToolSet(agent.Services{
		Components: compSvc, Planner: plannerSvc, Plates: plateSvc,
		Profile: profileSvc, Slots: slotSvc, Ingredient: ingRepo,
	})
	require.NoError(t, err)

	var h *handlers.AIHandler
	if enabled {
		svc := agent.NewService(aiRepo, client, tools, plannerSvc, profileSvc, "test-model")
		h = handlers.NewAIHandler(svc, "test", "test-model")
	} else {
		h = handlers.NewAIHandler(nil, "", "")
	}

	r := chi.NewRouter()
	r.Route("/api/ai", func(r chi.Router) {
		r.Post("/chat", h.Chat)
		r.Get("/conversations", h.ListConversations)
		r.Get("/conversations/{id}", h.GetConversation)
		r.Delete("/conversations/{id}", h.DeleteConversation)
	})
	r.Get("/api/settings/ai", h.Settings)
	return r, h
}

type sseFrame struct {
	Event string
	Data  string
}

func readAllFrames(t *testing.T, body io.Reader) []sseFrame {
	t.Helper()
	var frames []sseFrame
	scanner := bufio.NewScanner(body)
	var cur sseFrame
	hasPayload := false
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if hasPayload {
				frames = append(frames, cur)
				cur = sseFrame{}
				hasPayload = false
			}
			continue
		}
		if strings.HasPrefix(line, "event: ") {
			cur.Event = strings.TrimPrefix(line, "event: ")
			hasPayload = true
		}
		if strings.HasPrefix(line, "data: ") {
			if cur.Data != "" {
				cur.Data += "\n"
			}
			cur.Data += strings.TrimPrefix(line, "data: ")
			hasPayload = true
		}
	}
	if hasPayload {
		frames = append(frames, cur)
	}
	return frames
}

func TestChat_SSEStreamsEventsAndDone(t *testing.T) {
	client := &scriptedLLM{turns: []turnScript{{
		events: []llm.Event{
			{Type: llm.EventMessageStart, Payload: llm.MessageStartPayload{Model: "test-model"}},
			{Type: llm.EventAssistantDelta, Payload: llm.AssistantDeltaPayload{Text: "hi"}},
		},
		stopReason: llm.StopReasonEndTurn,
		message:    llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentBlock{{Type: llm.ContentTypeText, Text: "hi"}}},
	}}}
	router, _ := setupAIRouter(t, client, true)
	ts := httptest.NewServer(router)
	defer ts.Close()

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, ts.URL+"/api/ai/chat",
		bytes.NewBufferString(`{"message":"hello"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := ts.Client().Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, "text/event-stream; charset=utf-8", resp.Header.Get("Content-Type"))

	frames := readAllFrames(t, resp.Body)
	eventTypes := make([]string, len(frames))
	for i, f := range frames {
		eventTypes[i] = f.Event
	}
	assert.Contains(t, eventTypes, "message_start")
	assert.Contains(t, eventTypes, "assistant_delta")
	assert.Contains(t, eventTypes, "done")
	assert.Equal(t, "done", eventTypes[len(eventTypes)-1])
}

func TestChat_503WhenProviderMissing(t *testing.T) {
	router, _ := setupAIRouter(t, nil, false)
	req := httptest.NewRequest(http.MethodPost, "/api/ai/chat", bytes.NewBufferString(`{"message":"hi"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "error.ai.provider_missing", body["message_key"])
}

func TestChat_400OnEmptyMessage(t *testing.T) {
	router, _ := setupAIRouter(t, &scriptedLLM{}, true)
	req := httptest.NewRequest(http.MethodPost, "/api/ai/chat", bytes.NewBufferString(`{"message":""}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestListAndGetAndDeleteConversations(t *testing.T) {
	client := &scriptedLLM{turns: []turnScript{{
		stopReason: llm.StopReasonEndTurn,
		message:    llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentBlock{{Type: llm.ContentTypeText, Text: "hi"}}},
	}}}
	router, _ := setupAIRouter(t, client, true)
	ts := httptest.NewServer(router)
	defer ts.Close()

	// Chat once to create a conversation.
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, ts.URL+"/api/ai/chat",
		bytes.NewBufferString(`{"message":"hi"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := ts.Client().Do(req)
	require.NoError(t, err)
	_, _ = io.ReadAll(resp.Body)
	_ = resp.Body.Close()

	// List
	resp, err = ts.Client().Get(ts.URL + "/api/ai/conversations")
	require.NoError(t, err)
	var list struct {
		Items []struct {
			ID int64 `json:"id"`
		} `json:"items"`
		Total int64 `json:"total"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&list))
	_ = resp.Body.Close()
	require.Len(t, list.Items, 1)
	convID := list.Items[0].ID

	// Get detail.
	resp, err = ts.Client().Get(ts.URL + "/api/ai/conversations/" + itoa(convID))
	require.NoError(t, err)
	var detail struct {
		ID       int64 `json:"id"`
		Messages []struct {
			Role string `json:"role"`
		} `json:"messages"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&detail))
	_ = resp.Body.Close()
	assert.Equal(t, convID, detail.ID)
	assert.Len(t, detail.Messages, 2)

	// Delete.
	delReq, _ := http.NewRequestWithContext(context.Background(), http.MethodDelete, ts.URL+"/api/ai/conversations/"+itoa(convID), nil)
	resp, err = ts.Client().Do(delReq)
	require.NoError(t, err)
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	_ = resp.Body.Close()

	// 404 after delete.
	resp, err = ts.Client().Get(ts.URL + "/api/ai/conversations/" + itoa(convID))
	require.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	_ = resp.Body.Close()
}

func TestSettings_ReportsCurrentConfig(t *testing.T) {
	router, _ := setupAIRouter(t, &scriptedLLM{}, true)
	req := httptest.NewRequest(http.MethodGet, "/api/settings/ai", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)
	var body struct {
		Enabled  bool   `json:"enabled"`
		Provider string `json:"provider"`
		Model    string `json:"model"`
	}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&body))
	assert.True(t, body.Enabled)
	assert.Equal(t, "test", body.Provider)
	assert.Equal(t, "test-model", body.Model)

	// Disabled form.
	router2, _ := setupAIRouter(t, nil, false)
	req = httptest.NewRequest(http.MethodGet, "/api/settings/ai", nil)
	w = httptest.NewRecorder()
	router2.ServeHTTP(w, req)
	assert.Equal(t, 200, w.Code)
	require.NoError(t, json.NewDecoder(w.Body).Decode(&body))
	assert.False(t, body.Enabled)
}

func TestChat_StreamsErrorEventOnUpstreamFailure(t *testing.T) {
	client := &scriptedLLM{fail: errors.New("upstream down")}
	router, _ := setupAIRouter(t, client, true)
	ts := httptest.NewServer(router)
	defer ts.Close()

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, ts.URL+"/api/ai/chat",
		bytes.NewBufferString(`{"message":"hi"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := ts.Client().Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	frames := readAllFrames(t, resp.Body)
	var sawError bool
	for _, f := range frames {
		if f.Event == "error" {
			sawError = true
		}
	}
	assert.True(t, sawError)
}

// waitForConversationCount polls the list until the expected count appears or
// the short timeout elapses; keeps these tests robust against SSE flush timing.
func waitForConversationCount(t *testing.T, url string, client *http.Client, want int64) {
	t.Helper()
	deadline := time.Now().Add(500 * time.Millisecond)
	for {
		resp, err := client.Get(url + "/api/ai/conversations")
		require.NoError(t, err)
		var list struct {
			Total int64 `json:"total"`
		}
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&list))
		_ = resp.Body.Close()
		if list.Total >= want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timeout waiting for %d conversations (have %d)", want, list.Total)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

var _ = waitForConversationCount // reserved for future tests

func itoa(n int64) string {
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
