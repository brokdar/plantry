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

// AIHandler exposes /api/ai/* endpoints. When svc is nil, every endpoint
// returns 503 with error.ai.provider_missing — this lets the router register
// routes unconditionally while still disabling AI at runtime.
type AIHandler struct {
	svc       *agent.Service
	provider  string
	model     string
	heartbeat time.Duration
}

// NewAIHandler constructs an AIHandler. svc may be nil to signal that AI is
// disabled (no provider configured).
func NewAIHandler(svc *agent.Service, provider, model string) *AIHandler {
	return &AIHandler{svc: svc, provider: provider, model: model, heartbeat: 15 * time.Second}
}

// Enabled reports whether the AI backend is configured.
func (h *AIHandler) Enabled() bool { return h.svc != nil }

// DebugSystemPrompt handles GET /api/ai/debug/system-prompt?week_id=N.
// Dev-only — the route must be gated by the router (guarded by DevMode). It
// returns the composed system prompt the agent would use for the given week,
// so e2e tests can assert that learned preferences are being injected.
func (h *AIHandler) DebugSystemPrompt(w http.ResponseWriter, r *http.Request) {
	if !h.Enabled() {
		writeError(w, http.StatusServiceUnavailable, "error.ai.provider_missing")
		return
	}
	var weekID *int64
	if v := r.URL.Query().Get("week_id"); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, "error.invalid_body")
			return
		}
		weekID = &id
	}
	prompt, err := h.svc.DebugSystemPrompt(r.Context(), weekID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error.server")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"system_prompt": prompt})
}

type chatRequestBody struct {
	ConversationID *int64 `json:"conversation_id,omitempty"`
	WeekID         *int64 `json:"week_id,omitempty"`
	Mode           string `json:"mode,omitempty"`
	Message        string `json:"message"`
}

// Chat handles POST /api/ai/chat (SSE).
func (h *AIHandler) Chat(w http.ResponseWriter, r *http.Request) {
	if !h.Enabled() {
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

	writer := sse.New(w)
	ctx := r.Context()

	events := make(chan llm.Event, 32)
	chatErr := make(chan error, 1)
	go func() {
		defer close(events)
		chatErr <- h.svc.Chat(ctx, agent.ChatRequest{
			ConversationID: body.ConversationID,
			WeekID:         body.WeekID,
			Mode:           body.Mode,
			UserText:       body.Message,
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
	WeekID    *int64            `json:"week_id,omitempty"`
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
		ID: c.ID, WeekID: c.WeekID, Title: c.Title,
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
	if !h.Enabled() {
		writeError(w, http.StatusServiceUnavailable, "error.ai.provider_missing")
		return
	}
	q := agent.ListQuery{}
	if v := r.URL.Query().Get("week_id"); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, "error.invalid_id")
			return
		}
		q.WeekID = &id
	}
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
	if !h.Enabled() {
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
	if !h.Enabled() {
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

// ---------------------------------------------------------------------------
// Settings (read-only in v1)
// ---------------------------------------------------------------------------

type aiSettingsResponse struct {
	Enabled  bool   `json:"enabled"`
	Provider string `json:"provider,omitempty"`
	Model    string `json:"model,omitempty"`
}

// Settings handles GET /api/settings/ai — reports which provider + model is
// currently active. The api_key is never exposed.
func (h *AIHandler) Settings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, aiSettingsResponse{
		Enabled: h.Enabled(), Provider: h.provider, Model: h.model,
	})
}
