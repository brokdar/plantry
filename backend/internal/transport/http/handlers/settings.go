package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/anthropic"
	"github.com/jaltszeimer/plantry/backend/internal/adapters/crypto"
	"github.com/jaltszeimer/plantry/backend/internal/domain/settings"
	plantrymw "github.com/jaltszeimer/plantry/backend/internal/transport/http/middleware"
)

// SystemInfo is the static read-only payload shown on the System tab. It is
// supplied by the caller because the settings package must not reach into
// config directly (layering).
type SystemInfo struct {
	Port            int    `json:"port"`
	DBPath          string `json:"db_path"`
	LogLevel        string `json:"log_level"`
	ImagePath       string `json:"image_path"`
	DevMode         bool   `json:"dev_mode"`
	Version         string `json:"version"`
	BuildCommit     string `json:"build_commit"`
	CipherAvailable bool   `json:"cipher_available"`
}

// SettingsHandler exposes /api/settings endpoints for editable and read-only
// configuration.
type SettingsHandler struct {
	svc        *settings.Service
	system     SystemInfo
	rateLimit  *plantrymw.RateLimiter
	httpClient *http.Client
}

// NewSettingsHandler constructs the handler. rateLimit is optional — when
// non-nil, changes to ai.rate_limit_per_min are pushed into it so the effect
// is immediate.
func NewSettingsHandler(svc *settings.Service, system SystemInfo, rateLimit *plantrymw.RateLimiter) *SettingsHandler {
	return &SettingsHandler{
		svc:       svc,
		system:    system,
		rateLimit: rateLimit,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

type settingItem struct {
	Key           string `json:"key"`
	Value         string `json:"value,omitempty"`
	Source        string `json:"source"`
	IsSecret      bool   `json:"is_secret"`
	MaskedPreview string `json:"masked_preview,omitempty"`
	EnvAlsoSet    bool   `json:"env_also_set"`
}

type settingListResponse struct {
	Items           []settingItem `json:"items"`
	CipherAvailable bool          `json:"cipher_available"`
}

// List handles GET /api/settings.
func (h *SettingsHandler) List(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	values, err := h.svc.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error.server")
		return
	}
	items := make([]settingItem, len(values))
	for i, v := range values {
		items[i] = settingItem{
			Key:           v.Key,
			Value:         v.Raw,
			Source:        string(v.Source),
			IsSecret:      v.IsSecret,
			MaskedPreview: v.MaskedPreview,
			EnvAlsoSet:    v.EnvAlsoSet,
		}
	}
	writeJSON(w, http.StatusOK, settingListResponse{Items: items, CipherAvailable: h.svc.CipherAvailable()})
}

type settingUpsertRequest struct {
	Value string `json:"value"`
}

// Set handles PUT /api/settings/{key}.
func (h *SettingsHandler) Set(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	var req settingUpsertRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	if err := h.svc.Set(r.Context(), key, req.Value); err != nil {
		writeSettingsError(w, err)
		return
	}
	// Side-effect: rate limiter reconfigures immediately on change.
	if key == settings.KeyAIRateLimit && h.rateLimit != nil {
		if cfg, err := h.svc.EffectiveAI(r.Context()); err == nil {
			h.rateLimit.SetLimit(cfg.RateLimitPerMin)
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// Delete handles DELETE /api/settings/{key}.
func (h *SettingsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	if err := h.svc.Delete(r.Context(), key); err != nil {
		writeSettingsError(w, err)
		return
	}
	if key == settings.KeyAIRateLimit && h.rateLimit != nil {
		if cfg, err := h.svc.EffectiveAI(r.Context()); err == nil {
			h.rateLimit.SetLimit(cfg.RateLimitPerMin)
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// System handles GET /api/settings/system.
func (h *SettingsHandler) System(w http.ResponseWriter, r *http.Request) {
	info := h.system
	info.CipherAvailable = h.svc.CipherAvailable()
	writeJSON(w, http.StatusOK, info)
}

type modelItem struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name,omitempty"`
}

type modelsResponse struct {
	Models    []modelItem `json:"models"`
	Validated bool        `json:"validated"`
}

// Models handles GET /api/settings/ai/models?provider=X.
// Optional query param api_key overrides the stored/env key for this request
// only — letting the UI validate a new key before saving it.
func (h *SettingsHandler) Models(w http.ResponseWriter, r *http.Request) {
	provider := strings.ToLower(r.URL.Query().Get("provider"))
	apiKey := r.URL.Query().Get("api_key")

	if apiKey == "" {
		cfg, err := h.svc.EffectiveAI(r.Context())
		if err == nil {
			apiKey = cfg.APIKey
		}
	}

	switch provider {
	case "openai":
		models, err := fetchOpenAIModels(r.Context(), h.httpClient, apiKey)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "error.settings.invalid_api_key")
			return
		}
		writeJSON(w, http.StatusOK, modelsResponse{Models: models, Validated: true})
	case "anthropic":
		if apiKey == "" {
			writeError(w, http.StatusUnauthorized, "error.settings.invalid_api_key")
			return
		}
		out := make([]modelItem, 0, len(anthropic.KnownModels()))
		for _, m := range anthropic.KnownModels() {
			out = append(out, modelItem{ID: m.ID, DisplayName: m.DisplayName})
		}
		writeJSON(w, http.StatusOK, modelsResponse{Models: out, Validated: true})
	case "fake":
		writeJSON(w, http.StatusOK, modelsResponse{
			Models: []modelItem{
				{ID: "fake-default", DisplayName: "Fake (default)"},
				{ID: "fake-tools", DisplayName: "Fake (tools)"},
			},
			Validated: true,
		})
	default:
		writeError(w, http.StatusBadRequest, "error.settings.unknown_provider")
	}
}

// AISummary handles GET /api/settings/ai — BC endpoint for the existing
// frontend, reporting the currently effective provider + model.
func (h *SettingsHandler) AISummary(w http.ResponseWriter, r *http.Request) {
	cfg, err := h.svc.EffectiveAI(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error.server")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"enabled":  cfg.Provider != "",
		"provider": cfg.Provider,
		"model":    cfg.Model,
	})
}

func writeSettingsError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, settings.ErrUnknownKey):
		writeError(w, http.StatusBadRequest, "error.settings.unknown_key")
	case errors.Is(err, settings.ErrReadOnlyKey):
		writeError(w, http.StatusBadRequest, "error.settings.readonly_key")
	case errors.Is(err, settings.ErrInvalidKind):
		writeError(w, http.StatusBadRequest, "error.settings.invalid_value")
	case errors.Is(err, crypto.ErrSecretKeyMissing):
		writeError(w, http.StatusServiceUnavailable, "error.settings.secret_key_missing")
	default:
		writeError(w, http.StatusInternalServerError, "error.server")
	}
}

// fetchOpenAIModels calls GET /v1/models and filters the result to
// chat-capable models. The caller maps any error to 401.
func fetchOpenAIModels(ctx context.Context, client *http.Client, apiKey string) ([]modelItem, error) {
	if apiKey == "" {
		return nil, errors.New("missing api key")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.openai.com/v1/models", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("openai models: status %d: %s", resp.StatusCode, string(body))
	}
	var payload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	out := make([]modelItem, 0, len(payload.Data))
	for _, m := range payload.Data {
		if isOpenAIChatModel(m.ID) {
			out = append(out, modelItem{ID: m.ID, DisplayName: m.ID})
		}
	}
	return out, nil
}

// isOpenAIChatModel filters /v1/models output to chat-capable families. The
// endpoint lists every model (including embeddings, audio, moderation) — we
// want only the ones that accept messages + tools.
func isOpenAIChatModel(id string) bool {
	low := strings.ToLower(id)
	for _, prefix := range []string{"gpt-", "chatgpt-", "o1-", "o3-", "o4-"} {
		if strings.HasPrefix(low, prefix) {
			return true
		}
	}
	return false
}
