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
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
	"github.com/jaltszeimer/plantry/backend/internal/domain/slot"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper/testsvc"
	"github.com/jaltszeimer/plantry/backend/internal/transport/http/handlers"
)

// newAIHandler wires an AIHandler over a test DB with a fake client that
// ends its turn immediately.
func newAIHandler(t *testing.T) (*handlers.AIHandler, *fake.Client) {
	t.Helper()
	db := testhelper.NewTestDB(t)
	return newAIHandlerWithPlates(t, db, testsvc.Plates(db))
}

// newAIHandlerWithPlates is newAIHandler with the prompt's plate reader
// swapped out.
func newAIHandlerWithPlates(t *testing.T, db *sql.DB, plates agent.PlateRanger) (*handlers.AIHandler, *fake.Client) {
	t.Helper()
	slotRepo := sqlite.NewSlotRepo(db)
	foodRepo := sqlite.NewFoodRepo(db)
	client := fake.NewFromScript(&fake.Script{Turns: []fake.Turn{{
		StopReason: "end_turn",
		Message:    fake.ScriptedMessage{Content: []fake.ScriptedBlock{{Type: "text", Text: "Done."}}},
	}}})
	resolver := llm.StaticResolver(client, "fake-model")
	tools, err := agent.NewToolSet(agent.Services{Foods: food.NewService(foodRepo)})
	require.NoError(t, err)
	svc := agent.NewService(sqlite.NewAIRepo(db), resolver, tools,
		profile.NewService(sqlite.NewProfileRepo(db)), nil,
		agent.WindowSources{
			Plates: plates,
			Slots:  slot.NewService(slotRepo),
			Foods:  foodRepo,
		},
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
// prompt endpoint. Fields hold today/from/to; an empty value is omitted.
var dateCases = []struct {
	name            string
	today, from, to string
	wantKey         string
}{
	{"missing from", "", "", "2026-11-06", "error.ai.window_missing"},
	{"missing to", "", "2026-10-31", "", "error.ai.window_missing"},
	{"malformed from", "", "31.10.2026", "2026-11-06", "error.ai.invalid_date"},
	{"malformed to", "", "2026-10-31", "2026-11-31", "error.ai.invalid_date"},
	{"malformed today", "yesterday", "2026-10-31", "2026-11-06", "error.ai.invalid_date"},
	{"to before from", "", "2026-11-06", "2026-10-31", "error.ai.window_inverted"},
	{"span over 31 days", "", "2026-10-01", "2026-11-01", "error.ai.window_too_long"},
}

func chatBody(today, from, to string) string {
	body := map[string]string{"message": "plan my week"}
	for k, v := range map[string]string{"today": today, "from": from, "to": to} {
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
			req := httptest.NewRequest(http.MethodPost, "/api/ai/chat", strings.NewReader(chatBody(tt.today, tt.from, tt.to)))
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
		name, today, from, to string
		wantToday, wantWindow string
	}{
		{
			"client today", "2026-10-30", "2026-10-31", "2026-11-06",
			"Today is Friday, 2026-10-30.",
			"Planning window: Saturday 2026-10-31 to Friday 2026-11-06 (7 days).",
		},
		{
			"today falls back to server clock", "", "2026-10-31", "2026-11-06",
			"Today is Monday, 2026-10-26.",
			"Planning window: Saturday 2026-10-31 to Friday 2026-11-06 (7 days).",
		},
		{
			"exactly 31 days inclusive", "2026-10-26", "2026-10-01", "2026-10-31",
			"Today is Monday, 2026-10-26.",
			"Planning window: Thursday 2026-10-01 to Saturday 2026-10-31 (31 days).",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, client := newAIHandler(t)
			req := httptest.NewRequest(http.MethodPost, "/api/ai/chat", strings.NewReader(chatBody(tt.today, tt.from, tt.to)))
			w := httptest.NewRecorder()

			h.Chat(w, req)

			assert.Equal(t, http.StatusOK, w.Code)
			assert.Contains(t, w.Body.String(), "event: done")
			reqs := client.Requests()
			require.NotEmpty(t, reqs)
			assert.Contains(t, reqs[0].System, tt.wantToday)
			assert.Contains(t, reqs[0].System, tt.wantWindow)
		})
	}
}

// B2-AC6: the old chat modes and week ids are unknown fields now: the turn
// runs as a plain chat and the prompt carries no mode hint.
func TestAIChat_IgnoresLegacyFields(t *testing.T) {
	h, client := newAIHandler(t)
	body := `{"message":"plan my week","from":"2026-10-31","to":"2026-11-06","mode":"fill_empty","week_id":17}`
	w := httptest.NewRecorder()

	h.Chat(w, httptest.NewRequest(http.MethodPost, "/api/ai/chat", strings.NewReader(body)))

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "event: done")
	reqs := client.Requests()
	require.NotEmpty(t, reqs)
	assert.NotContains(t, reqs[0].System, "Mode hint")
	assert.NotContains(t, reqs[0].System, "fill the empty slots")
}

func TestAIConversations_ListIgnoresLegacyFilter(t *testing.T) {
	h, _ := newAIHandler(t)
	w := httptest.NewRecorder()
	h.Chat(w, httptest.NewRequest(http.MethodPost, "/api/ai/chat", strings.NewReader(chatBody("", "2026-10-31", "2026-11-06"))))
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

type failingPlates struct{ err error }

func (f failingPlates) Range(context.Context, time.Time, time.Time) ([]plate.Plate, error) {
	return nil, f.err
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
			h, _ := newAIHandlerWithPlates(t, testhelper.NewTestDB(t), failingPlates{tt.err})
			w := httptest.NewRecorder()

			h.DebugSystemPrompt(w, httptest.NewRequest(http.MethodGet, debugURL("", "2026-10-31", "2026-11-01"), nil))

			assert.Equal(t, tt.wantStatus, w.Code)
			assert.Equal(t, tt.wantKey, messageKey(t, w))
		})
	}
}

func debugURL(today, from, to string) string {
	q := []string{}
	for k, v := range map[string]string{"today": today, "from": from, "to": to} {
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

			h.DebugSystemPrompt(w, httptest.NewRequest(http.MethodGet, debugURL(tt.today, tt.from, tt.to), nil))

			assert.Equal(t, http.StatusBadRequest, w.Code)
			assert.Equal(t, tt.wantKey, messageKey(t, w))
		})
	}
}

func TestAIDebugSystemPrompt_UsesDates(t *testing.T) {
	h, _ := newAIHandler(t)
	w := httptest.NewRecorder()

	h.DebugSystemPrompt(w, httptest.NewRequest(http.MethodGet, debugURL("2026-10-26", "2026-10-31", "2026-11-01"), nil))

	require.Equal(t, http.StatusOK, w.Code)
	var body struct {
		SystemPrompt string `json:"system_prompt"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Contains(t, body.SystemPrompt, "Today is Monday, 2026-10-26.")
	assert.Contains(t, body.SystemPrompt, "- Sunday 2026-11-01\n")
}
