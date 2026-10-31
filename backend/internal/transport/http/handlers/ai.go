package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/agent"
	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
	"github.com/jaltszeimer/plantry/backend/internal/transport/http/sse"
)

// AIHandler exposes /api/ai/* endpoints. Enabled state is derived per-request
// from the llm.Resolver, so endpoints reflect the current effective
// configuration (database override or env-var) rather than a snapshot taken
// at startup.
type AIHandler struct {
	svc       *agent.Service
	resolver  llm.Resolver
	heartbeat time.Duration
}

// NewAIHandler constructs an AIHandler. The resolver must be non-nil; it
// determines at request time whether a provider is configured.
func NewAIHandler(svc *agent.Service, resolver llm.Resolver) *AIHandler {
	return &AIHandler{svc: svc, resolver: resolver, heartbeat: 15 * time.Second}
}

// Enabled reports whether the AI backend currently has a provider configured.
// Called by route guards — an unconfigured deployment returns 503.
func (h *AIHandler) Enabled(ctx context.Context) bool {
	if h == nil || h.resolver == nil {
		return false
	}
	_, _, err := h.resolver.Current(ctx)
	return err == nil
}

// DebugSystemPrompt handles GET /api/ai/debug/system-prompt.
// Dev-only — the route must be gated by the router (guarded by DevMode). It
// returns the composed system prompt the agent would use for a chat turn,
// so e2e tests can assert that learned preferences are being injected.
func (h *AIHandler) DebugSystemPrompt(w http.ResponseWriter, r *http.Request) {
	if !h.Enabled(r.Context()) {
		writeError(w, http.StatusServiceUnavailable, "error.ai.provider_missing")
		return
	}
	q := r.URL.Query()
	dates, key := parseChatDates(q.Get("today"), q.Get("from"), q.Get("to"))
	if key != "" {
		writeError(w, http.StatusBadRequest, key)
		return
	}
	prompt, err := h.svc.DebugSystemPrompt(r.Context(), agent.ChatRequest{
		Today: dates.today, From: dates.from, To: dates.to,
	})
	if err != nil {
		status, key := toHTTPWithResource(err, "ai")
		writeError(w, status, key)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"system_prompt": prompt})
}

type chatRequestBody struct {
	ConversationID *int64 `json:"conversation_id,omitempty"`
	Message        string `json:"message"`
	Today          string `json:"today,omitempty"` // YYYY-MM-DD; server clock when empty
	From           string `json:"from"`            // YYYY-MM-DD, required
	To             string `json:"to"`              // YYYY-MM-DD, required
}

// maxChatWindowDays caps the planning window a chat turn may cover.
const maxChatWindowDays = 31

type chatDates struct{ today, from, to time.Time }

// parseChatDates validates the chat planning window. today is optional (zero
// when empty); from and to are required. On failure it returns the i18n key.
func parseChatDates(todayStr, fromStr, toStr string) (chatDates, string) {
	var d chatDates
	if fromStr == "" || toStr == "" {
		return d, "error.ai.window_missing"
	}
	var err error
	if d.from, err = time.Parse("2006-01-02", fromStr); err != nil {
		return d, "error.ai.invalid_date"
	}
	if d.to, err = time.Parse("2006-01-02", toStr); err != nil {
		return d, "error.ai.invalid_date"
	}
	if todayStr != "" {
		if d.today, err = time.Parse("2006-01-02", todayStr); err != nil {
			return d, "error.ai.invalid_date"
		}
	}
	if d.to.Before(d.from) {
		return d, "error.ai.window_inverted"
	}
	if d.to.Sub(d.from) >= maxChatWindowDays*24*time.Hour {
		return d, "error.ai.window_too_long"
	}
	return d, ""
}

// Chat handles POST /api/ai/chat (SSE).
func (h *AIHandler) Chat(w http.ResponseWriter, r *http.Request) {
	if !h.Enabled(r.Context()) {
		writeError(w, http.StatusServiceUnavailable, "error.ai.provider_missing")
		return
	}

	var body chatRequestBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	if body.Message == "" {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	dates, key := parseChatDates(body.Today, body.From, body.To)
	if key != "" {
		writeError(w, http.StatusBadRequest, key)
		return
	}

	writer := sse.New(w)
	ctx := r.Context()

	events := make(chan llm.Event, 32)
	chatErr := make(chan error, 1)
	go func() {
		defer close(events)
		chatErr <- h.svc.Chat(ctx, agent.ChatRequest{
			ConversationID: body.ConversationID,
			UserText:       body.Message,
			Today:          dates.today,
			From:           dates.from,
			To:             dates.to,
		}, events)
	}()

	heartbeat := time.NewTicker(h.heartbeat)
	defer heartbeat.Stop()

	logger := slog.Default()

	// Drain events first. The service goroutine closes `events` when Chat
	// returns, so the channel-close is the authoritative completion signal;
	// chatErr just carries the return value.
	for {
		select {
		case <-ctx.Done():
			return
		case <-heartbeat.C:
			if err := writer.Comment("ping"); err != nil {
				logger.Debug("ai.chat.sse.heartbeat_failed", "error", err)
				return
			}
		case evt, ok := <-events:
			if !ok {
				// Loop finished — read the return value and maybe emit error.
				err := <-chatErr
				if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, agent.ErrIterationsExceeded) {
					// Iterations exceeded already emits its own error event from Run.
					// Other errors may not have — surface them here.
					_ = writer.Send(string(llm.EventError), llm.StreamErrorPayload{
						MessageKey: "error.ai.stream_interrupted",
						Message:    err.Error(),
					})
				}
				return
			}
			if err := writer.Send(string(evt.Type), evt.Payload); err != nil {
				logger.Warn("ai.chat.sse.send_failed", "error", err)
				return
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Conversation CRUD
// ---------------------------------------------------------------------------

type conversationResponse struct {
	ID        int64             `json:"id"`
	Title     *string           `json:"title,omitempty"`
	Messages  []messageResponse `json:"messages,omitempty"`
	CreatedAt string            `json:"created_at"`
	UpdatedAt string            `json:"updated_at"`
}

type messageResponse struct {
	ID        int64           `json:"id"`
	Role      string          `json:"role"`
	Content   json.RawMessage `json:"content"`
	CreatedAt string          `json:"created_at"`
}

type conversationListResponse struct {
	Items []conversationResponse `json:"items"`
	Total int64                  `json:"total"`
}

func toConversationSummary(c *agent.Conversation) conversationResponse {
	return conversationResponse{
		ID: c.ID, Title: c.Title,
		CreatedAt: c.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt: c.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func toConversationDetail(c *agent.Conversation) conversationResponse {
	out := toConversationSummary(c)
	out.Messages = make([]messageResponse, len(c.Messages))
	for i, m := range c.Messages {
		out.Messages[i] = messageResponse{
			ID: m.ID, Role: string(m.Role), Content: m.Content,
			CreatedAt: m.CreatedAt.UTC().Format(time.RFC3339),
		}
	}
	return out
}

// ListConversations handles GET /api/ai/conversations.
func (h *AIHandler) ListConversations(w http.ResponseWriter, r *http.Request) {
	if !h.Enabled(r.Context()) {
		writeError(w, http.StatusServiceUnavailable, "error.ai.provider_missing")
		return
	}
	q := agent.ListQuery{}
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			q.Limit = n
		}
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			q.Offset = n
		}
	}

	res, err := h.svc.ListConversations(r.Context(), q)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error.server")
		return
	}
	items := make([]conversationResponse, len(res.Items))
	for i := range res.Items {
		items[i] = toConversationSummary(&res.Items[i])
	}
	writeJSON(w, http.StatusOK, conversationListResponse{Items: items, Total: res.Total})
}

// GetConversation handles GET /api/ai/conversations/{id}.
func (h *AIHandler) GetConversation(w http.ResponseWriter, r *http.Request) {
	if !h.Enabled(r.Context()) {
		writeError(w, http.StatusServiceUnavailable, "error.ai.provider_missing")
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_id")
		return
	}
	conv, err := h.svc.GetConversation(r.Context(), id)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			writeError(w, http.StatusNotFound, "error.not_found")
			return
		}
		writeError(w, http.StatusInternalServerError, "error.server")
		return
	}
	writeJSON(w, http.StatusOK, toConversationDetail(conv))
}

// DeleteConversation handles DELETE /api/ai/conversations/{id}.
func (h *AIHandler) DeleteConversation(w http.ResponseWriter, r *http.Request) {
	if !h.Enabled(r.Context()) {
		writeError(w, http.StatusServiceUnavailable, "error.ai.provider_missing")
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_id")
		return
	}
	if err := h.svc.DeleteConversation(r.Context(), id); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			writeError(w, http.StatusNotFound, "error.not_found")
			return
		}
		writeError(w, http.StatusInternalServerError, "error.server")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
