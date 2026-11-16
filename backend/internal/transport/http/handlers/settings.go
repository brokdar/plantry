package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"sort"
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
	// anthropicURL and openaiURL are the provider API roots; the setter
	// below lets tests point them at an httptest server.
	anthropicURL string
	openaiURL    string
}

// SetProviderBaseURLs overrides the provider API roots (empty keeps the
// default). For tests.
func (h *SettingsHandler) SetProviderBaseURLs(anthropicURL, openaiURL string) {
	if anthropicURL != "" {
		h.anthropicURL = anthropicURL
	}
	if openaiURL != "" {
		h.openaiURL = openaiURL
	}
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
		openaiURL: "https://api.openai.com",
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
	// Lifecycle is "deprecated" for a model that is still callable but
	// headed for retirement; empty otherwise.
	Lifecycle string `json:"lifecycle,omitempty"`
}

type modelsResponse struct {
	Models    []modelItem `json:"models"`
	Validated bool        `json:"validated"`
}

// providerStatusError is a provider answer other than 200.
type providerStatusError struct{ status int }

func (e *providerStatusError) Error() string { return fmt.Sprintf("provider status %d", e.status) }

// writeKeyCheckError maps a failed models call to the key-test message: 401
// is a rejected key, 403 a valid key that is not permitted (billing, a
// workspace), anything else (network, 5xx, 429) the provider being
// unreachable. The key and the provider's body are never logged or echoed.
func writeKeyCheckError(w http.ResponseWriter, err error) {
	var status int
	var se *providerStatusError
	var ae *anthropic.APIError
	switch {
	case errors.As(err, &se):
		status = se.status
	case errors.As(err, &ae):
		status = ae.Status
	}
	switch status {
	case http.StatusUnauthorized:
		writeError(w, http.StatusUnauthorized, "error.settings.invalid_api_key")
	case http.StatusForbidden:
		writeError(w, http.StatusForbidden, "error.settings.key_not_permitted")
	default:
		writeError(w, http.StatusBadGateway, "error.settings.provider_unreachable")
	}
}

// Models handles GET /api/settings/ai/models?provider=X.
// Optional query param api_key overrides the stored/env key for this request
// only — letting the UI validate a new key before saving it. For both real
// providers the models call is the key check: it needs a valid key and costs
// no tokens, so `validated` is true only when the provider accepted the key.
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
		if apiKey == "" {
			writeError(w, http.StatusUnauthorized, "error.settings.invalid_api_key")
			return
		}
		models, err := fetchOpenAIModels(r.Context(), h.httpClient, h.openaiURL, apiKey)
		if err != nil {
			writeKeyCheckError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, modelsResponse{Models: models, Validated: true})
	case "anthropic":
		if apiKey == "" {
			writeError(w, http.StatusUnauthorized, "error.settings.invalid_api_key")
			return
		}
		opts := []anthropic.Option{anthropic.WithHTTPClient(h.httpClient)}
		if h.anthropicURL != "" {
			opts = append(opts, anthropic.WithBaseURL(h.anthropicURL))
		}
		infos, err := anthropic.New(apiKey, opts...).ListModels(r.Context())
		if err != nil {
			writeKeyCheckError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, modelsResponse{Models: anthropicModelItems(infos), Validated: true})
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

// anthropicModelItems lists the usable models: retired ones are dropped,
// active ones come before deprecated ones, newest first within each.
func anthropicModelItems(infos []anthropic.ModelInfo) []modelItem {
	kept := make([]anthropic.ModelInfo, 0, len(infos))
	for _, m := range infos {
		if m.Lifecycle != "retired" {
			kept = append(kept, m)
		}
	}
	sort.SliceStable(kept, func(i, j int) bool {
		di, dj := kept[i].Lifecycle == "deprecated", kept[j].Lifecycle == "deprecated"
		if di != dj {
			return !di
		}
		return kept[i].CreatedAt > kept[j].CreatedAt
	})
	out := make([]modelItem, len(kept))
	for i, m := range kept {
		out[i] = modelItem{ID: m.ID, DisplayName: m.DisplayName}
		if m.Lifecycle == "deprecated" {
			out[i].Lifecycle = "deprecated"
		}
	}
	return out
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
// chat-capable models, newest first. A non-200 answer is a
// *providerStatusError.
func fetchOpenAIModels(ctx context.Context, client *http.Client, baseURL, apiKey string) ([]modelItem, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/v1/models", nil)
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
		return nil, &providerStatusError{status: resp.StatusCode}
	}
	var payload struct {
		Data []struct {
			ID      string `json:"id"`
			Created int64  `json:"created"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	sort.SliceStable(payload.Data, func(i, j int) bool { return payload.Data[i].Created > payload.Data[j].Created })
	out := make([]modelItem, 0, len(payload.Data))
	for _, m := range payload.Data {
		if isOpenAIChatModel(m.ID) {
			out = append(out, modelItem{ID: m.ID, DisplayName: m.ID})
		}
	}
	return out, nil
}

// openAINonChat are id fragments of models in the chat families that are not
// chat completions models (audio, realtime, image, search, transcription,
// embeddings, moderation, legacy instruct).
var openAINonChat = []string{
	"audio", "realtime", "transcribe", "tts", "whisper", "image", "search",
	"embedding", "moderation", "instruct", "codex", "dall-e", "diarize",
}

var openAIReasoningPrefix = regexp.MustCompile(`^o[0-9]`)

// isOpenAIChatModel filters /v1/models output (it lists every model) to the
// ones that accept messages + tools: the gpt-, chatgpt- and o<digit>
// families minus audio, realtime, image and similar variants.
func isOpenAIChatModel(id string) bool {
	low := strings.ToLower(id)
	if !strings.HasPrefix(low, "gpt-") && !strings.HasPrefix(low, "chatgpt-") && !openAIReasoningPrefix.MatchString(low) {
		return false
	}
	for _, frag := range openAINonChat {
		if strings.Contains(low, frag) {
			return false
		}
	}
	return true
}
