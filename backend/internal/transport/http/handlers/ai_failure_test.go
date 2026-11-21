package handlers_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain/agent"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper/testsvc"
	"github.com/jaltszeimer/plantry/backend/internal/transport/http/handlers"
)

// rejectedKey is a provider that answers 401 to every call, the way the
// adapters report it.
type rejectedKey struct{}

func (rejectedKey) Error() string {
	return "anthropic: 401 authentication_error: invalid x-api-key req_0123"
}
func (rejectedKey) ProviderStatus() int                 { return 401 }
func (rejectedKey) ProviderErrorType() string           { return "authentication_error" }
func (rejectedKey) ProviderInfo() llm.ProviderErrorInfo { return llm.ProviderErrorInfo{Status: 401} }

type rejectingClient struct{}

func (rejectingClient) Stream(_ context.Context, _ llm.Request, out chan<- llm.Event) (*llm.Response, error) {
	close(out)
	return nil, rejectedKey{}
}

func (rejectingClient) Complete(context.Context, llm.Request) (string, error) {
	return "", rejectedKey{}
}

func chatHandlerOver(t *testing.T, resolver llm.Resolver) *handlers.AIHandler {
	t.Helper()
	db := testhelper.NewTestDB(t)
	tools, err := agent.NewToolSet(agent.Services{Foods: food.NewService(sqlite.NewFoodRepo(db))})
	require.NoError(t, err)
	svc := agent.NewService(sqlite.NewAIRepo(db), resolver, tools,
		profile.NewService(sqlite.NewProfileRepo(db)), testsvc.Settings(db),
		agent.PlanContext{Build: testsvc.PlanningDeps(db).Builder.Build, Summary: planning.RenderPlanSummary},
		func() time.Time { return time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC) })
	plates, avail := testsvc.Plan(db)
	return handlers.NewAIHandler(svc, testsvc.Changes(db, plates, avail), resolver)
}

func postChat(h *handlers.AIHandler) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/ai/chat", strings.NewReader(`{"message":"plan my week","today":"2026-10-26","start":"2026-10-26"}`))
	w := httptest.NewRecorder()
	h.Chat(w, req)
	return w
}

// Case 8: no key configured. The chat answers 503 with a key before it opens
// a stream, so nothing is started.
func TestChatHTTP_NoProviderConfigured(t *testing.T) {
	w := postChat(chatHandlerOver(t, llm.StaticResolver(nil, "")))

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	assert.Equal(t, "error.ai.provider_missing", messageKey(t, w))
}

// Case 8: a key the provider rejects. The stream carries one error event with
// the key-rejected key and plain text, no provider text or request id.
func TestChatHTTP_ProviderRejectsTheKey(t *testing.T) {
	w := postChat(chatHandlerOver(t, llm.StaticResolver(rejectingClient{}, "m")))

	body := w.Body.String()
	assert.Equal(t, http.StatusOK, w.Code, "the stream was open")
	assert.Equal(t, 1, strings.Count(body, "event: error"))
	assert.Contains(t, body, `"message_key":"error.ai.key_rejected"`)
	for _, jargon := range []string{"anthropic", "authentication_error", "req_0123", "x-api-key"} {
		assert.NotContains(t, body, jargon)
	}
}
