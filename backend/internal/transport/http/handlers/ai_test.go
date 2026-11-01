package handlers_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/fake"
	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/agent"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
	"github.com/jaltszeimer/plantry/backend/internal/domain/window"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper/testsvc"
	"github.com/jaltszeimer/plantry/backend/internal/transport/http/handlers"
)

// newAIHandler wires an AIHandler over a test DB with a fake client that
// ends its turn immediately.
func newAIHandler(t *testing.T) (*handlers.AIHandler, *fake.Client) {
	t.Helper()
	db := testhelper.NewTestDB(t)
	return newAIHandlerWithPlan(t, db, agent.PlanContext{
		Build:   testsvc.PlanningDeps(db).Builder.Build,
		Summary: planning.RenderPlanSummary,
	})
}

// newAIHandlerWithPlan is newAIHandler with the prompt's plan context swapped
// out.
func newAIHandlerWithPlan(t *testing.T, db *sql.DB, plan agent.PlanContext) (*handlers.AIHandler, *fake.Client) {
	t.Helper()
	foodRepo := sqlite.NewFoodRepo(db)
	client := fake.NewFromScript(&fake.Script{Turns: []fake.Turn{{
		StopReason: "end_turn",
		Message:    fake.ScriptedMessage{Content: []fake.ScriptedBlock{{Type: "text", Text: "Done."}}},
	}}})
	resolver := llm.StaticResolver(client, "fake-model")
	tools, err := agent.NewToolSet(agent.Services{Foods: food.NewService(foodRepo)})
	require.NoError(t, err)
	svc := agent.NewService(sqlite.NewAIRepo(db), resolver, tools,
		profile.NewService(sqlite.NewProfileRepo(db)), testsvc.Settings(db), plan,
		func() time.Time { return time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC) })
	return handlers.NewAIHandler(svc, resolver), client
}

func messageKey(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	key, _ := body["message_key"].(string)
	return key
}

// dateCases are the date validation cases shared by chat and the debug
// prompt endpoint. An empty value is omitted. The default shopping day is
// Monday, so 2026-10-31 (a Saturday) is no window start.
var dateCases = []struct {
	name         string
	today, start string
	wantKey      string
}{
	// B3-AC2
	{"missing start", "", "", "error.invalid_body"},
	{"malformed start", "", "31.10.2026", "error.ai.invalid_date"},
	{"malformed today", "yesterday", "2026-11-02", "error.ai.invalid_date"},
	{"start is no shopping day", "", "2026-10-31", "error.plan.invalid_start"},
}

func chatBody(today, start string) string {
	body := map[string]string{"message": "plan my week"}
	for k, v := range map[string]string{"today": today, "start": start} {
		if v != "" {
			body[k] = v
		}
	}
	b, _ := json.Marshal(body)
	return string(b)
}

func TestAIChat_RejectsBadDates(t *testing.T) {
	for _, tt := range dateCases {
		t.Run(tt.name, func(t *testing.T) {
			h, client := newAIHandler(t)
			req := httptest.NewRequest(http.MethodPost, "/api/ai/chat", strings.NewReader(chatBody(tt.today, tt.start)))
			w := httptest.NewRecorder()

			h.Chat(w, req)

			assert.Equal(t, http.StatusBadRequest, w.Code)
			assert.Equal(t, tt.wantKey, messageKey(t, w))
			assert.Empty(t, client.Requests())
		})
	}
}

func TestAIChat_PassesDatesThrough(t *testing.T) {
	tests := []struct {
		name, today, start string
		wantToday          string
	}{
		{"client today", "2026-10-30", "2026-11-02", "Today is Friday 2026-10-30."},
		{"today falls back to server clock", "", "2026-11-02", "Today is Monday 2026-10-26."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, client := newAIHandler(t)
			req := httptest.NewRequest(http.MethodPost, "/api/ai/chat", strings.NewReader(chatBody(tt.today, tt.start)))
			w := httptest.NewRecorder()

			h.Chat(w, req)

			assert.Equal(t, http.StatusOK, w.Code)
			assert.Contains(t, w.Body.String(), "event: done")
			reqs := client.Requests()
			require.NotEmpty(t, reqs)
			assert.Contains(t, reqs[0].System, tt.wantToday)
			assert.Contains(t, reqs[0].System, "The plan week runs from Monday 2026-11-02 to Sunday 2026-11-08")
		})
	}
}

func TestAIConversations_ListIgnoresLegacyFilter(t *testing.T) {
	h, _ := newAIHandler(t)
	w := httptest.NewRecorder()
	h.Chat(w, httptest.NewRequest(http.MethodPost, "/api/ai/chat", strings.NewReader(chatBody("", "2026-11-02"))))
	require.Equal(t, http.StatusOK, w.Code)

	w = httptest.NewRecorder()
	h.ListConversations(w, httptest.NewRequest(http.MethodGet, "/api/ai/conversations?week_id=17", nil))
	require.Equal(t, http.StatusOK, w.Code, "the old filter is ignored")
	var list struct {
		Items []map[string]any `json:"items"`
		Total int              `json:"total"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &list))
	assert.Equal(t, 1, list.Total)
	require.Len(t, list.Items, 1)
	assert.NotContains(t, list.Items[0], "week_id")
}

func failingPlan(err error) agent.PlanContext {
	return agent.PlanContext{
		Build: func(context.Context, window.Window, time.Time, time.Time, time.Time) (planning.State, error) {
			return planning.State{}, err
		},
		Summary: planning.RenderPlanSummary,
	}
}

func TestAIDebugSystemPrompt_MapsServiceErrors(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantKey    string
	}{
		{"invalid input", fmt.Errorf("%w: bad range", domain.ErrInvalidInput), http.StatusBadRequest, "error.invalid_body"},
		{"unexpected", errors.New("disk on fire"), http.StatusInternalServerError, "error.server"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, _ := newAIHandlerWithPlan(t, testhelper.NewTestDB(t), failingPlan(tt.err))
			w := httptest.NewRecorder()

			h.DebugSystemPrompt(w, httptest.NewRequest(http.MethodGet, debugURL("", "2026-11-02"), nil))

			assert.Equal(t, tt.wantStatus, w.Code)
			assert.Equal(t, tt.wantKey, messageKey(t, w))
		})
	}
}

func debugURL(today, start string) string {
	q := []string{}
	for k, v := range map[string]string{"today": today, "start": start} {
		if v != "" {
			q = append(q, k+"="+v)
		}
	}
	return "/api/ai/debug/system-prompt?" + strings.Join(q, "&")
}

func TestAIDebugSystemPrompt_RejectsBadDates(t *testing.T) {
	for _, tt := range dateCases {
		t.Run(tt.name, func(t *testing.T) {
			h, _ := newAIHandler(t)
			w := httptest.NewRecorder()

			h.DebugSystemPrompt(w, httptest.NewRequest(http.MethodGet, debugURL(tt.today, tt.start), nil))

			assert.Equal(t, http.StatusBadRequest, w.Code)
			assert.Equal(t, tt.wantKey, messageKey(t, w))
		})
	}
}

func TestAIDebugSystemPrompt_UsesDates(t *testing.T) {
	h, _ := newAIHandler(t)
	w := httptest.NewRecorder()

	h.DebugSystemPrompt(w, httptest.NewRequest(http.MethodGet, debugURL("2026-10-26", "2026-11-02"), nil))

	require.Equal(t, http.StatusOK, w.Code)
	var body struct {
		SystemPrompt string `json:"system_prompt"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Contains(t, body.SystemPrompt, "Today is Monday 2026-10-26.")
	assert.Contains(t, body.SystemPrompt, "- Sunday 2026-11-08\n")
}
