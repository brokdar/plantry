package handlers_test

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/crypto"
	"github.com/jaltszeimer/plantry/backend/internal/domain/settings"
	"github.com/jaltszeimer/plantry/backend/internal/transport/http/handlers"
)

// modelsRouter wires GET /ai/models against fake provider servers.
func modelsRouter(t *testing.T, anthropicURL, openaiURL string) *chi.Mux {
	t.Helper()
	svc := settings.NewService(newMemRepo(), settings.NewEnvSnapshot(nil), crypto.NilCipher{})
	h := handlers.NewSettingsHandler(svc, handlers.SystemInfo{}, nil)
	h.SetProviderBaseURLs(anthropicURL, openaiURL)
	r := chi.NewRouter()
	r.Post("/ai/models", h.Models)
	return r
}

func fakeProvider(t *testing.T, status int, body string, seenKey *string) *httptest.Server {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if seenKey != nil {
			*seenKey = r.Header.Get("x-api-key") + r.Header.Get("Authorization")
		}
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(ts.Close)
	return ts
}

func getModels(r http.Handler, query string) *httptest.ResponseRecorder {
	q, _ := url.ParseQuery(query)
	body, _ := json.Marshal(map[string]string{"provider": q.Get("provider"), "api_key": q.Get("api_key")})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/ai/models", bytes.NewReader(body)))
	return w
}

func TestModels_AnthropicRequiresKey(t *testing.T) {
	w := getModels(modelsRouter(t, "", ""), "provider=anthropic")
	assert.Equal(t, 401, w.Code)
}

// The list is the live API list: active first, newest first, deprecated
// marked, retired dropped.
func TestModels_AnthropicLiveList(t *testing.T) {
	var key string
	ts := fakeProvider(t, 200, `{"has_more":false,"data":[
		{"id":"old","display_name":"Old","lifecycle":"deprecated","created_at":"2026-09-01T00:00:00Z"},
		{"id":"mid","display_name":"Mid","lifecycle":"active","created_at":"2026-02-01T00:00:00Z"},
		{"id":"gone","display_name":"Gone","lifecycle":"retired","created_at":"2025-01-01T00:00:00Z"},
		{"id":"new","display_name":"New","lifecycle":"active","created_at":"2026-08-01T00:00:00Z"}]}`, &key)
	w := getModels(modelsRouter(t, ts.URL, ""), "provider=anthropic&api_key=sk-test")
	require.Equal(t, 200, w.Code)
	assert.Equal(t, "sk-test", key)
	var got struct {
		Models []map[string]string `json:"models"`
		Valid  bool                `json:"validated"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.True(t, got.Valid)
	var ids []string
	for _, m := range got.Models {
		ids = append(ids, m["id"])
	}
	assert.Equal(t, []string{"new", "mid", "old"}, ids)
	assert.Equal(t, "New", got.Models[0]["display_name"])
	assert.Equal(t, "deprecated", got.Models[2]["lifecycle"])
	assert.Empty(t, got.Models[0]["lifecycle"])
}

func TestModels_KeyCheckErrors(t *testing.T) {
	const bogus = `{"type":"error","error":{"type":"authentication_error","message":"invalid x-api-key"}}`
	tests := []struct {
		name     string
		provider string
		status   int
		body     string
		want     int
		key      string
	}{
		{"anthropic bogus key", "anthropic", 401, bogus, 401, "error.settings.invalid_api_key"},
		{"anthropic not permitted", "anthropic", 403, `{}`, 403, "error.settings.key_not_permitted"},
		{"anthropic 5xx", "anthropic", 529, `{}`, 502, "error.settings.provider_unreachable"},
		{"openai bogus key", "openai", 401, `{}`, 401, "error.settings.invalid_api_key"},
		{"openai not permitted", "openai", 403, `{}`, 403, "error.settings.key_not_permitted"},
		{"openai 5xx", "openai", 503, `{}`, 502, "error.settings.provider_unreachable"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := fakeProvider(t, tt.status, tt.body, nil)
			w := getModels(modelsRouter(t, ts.URL, ts.URL), "provider="+tt.provider+"&api_key=sk-secret")
			assert.Equal(t, tt.want, w.Code)
			assert.Contains(t, w.Body.String(), tt.key)
			assert.NotContains(t, w.Body.String(), "sk-secret")
		})
	}
}

func TestModels_ProviderUnreachable(t *testing.T) {
	ts := fakeProvider(t, 200, `{}`, nil)
	url := ts.URL
	ts.Close()
	for _, p := range []string{"anthropic", "openai"} {
		w := getModels(modelsRouter(t, url, url), "provider="+p+"&api_key=sk-x")
		assert.Equal(t, 502, w.Code, p)
		assert.Contains(t, w.Body.String(), "error.settings.provider_unreachable")
	}
}

func TestModels_OpenAIChatFilterNewestFirst(t *testing.T) {
	ts := fakeProvider(t, 200, `{"data":[
		{"id":"text-embedding-3-small","created":5},
		{"id":"gpt-4o-audio-preview","created":6},
		{"id":"gpt-4o","created":10},
		{"id":"gpt-5","created":30},
		{"id":"o3-mini","created":20},
		{"id":"gpt-image-1","created":40},
		{"id":"whisper-1","created":1},
		{"id":"gpt-4o-realtime-preview","created":7}]}`, nil)
	w := getModels(modelsRouter(t, "", ts.URL), "provider=openai&api_key=sk-x")
	require.Equal(t, 200, w.Code)
	var got struct {
		Models []struct {
			ID string `json:"id"`
		} `json:"models"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	var ids []string
	for _, m := range got.Models {
		ids = append(ids, m.ID)
	}
	assert.Equal(t, "gpt-5,o3-mini,gpt-4o", strings.Join(ids, ","))
}

// A failed key check is logged with the provider status and error type, never
// the key or the provider body.
func TestModels_FailedKeyCheckIsLogged(t *testing.T) {
	var logs bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	ts := fakeProvider(t, 429, `{"type":"error","error":{"type":"rate_limit_error","message":"body-text-not-logged"}}`, nil)
	w := getModels(modelsRouter(t, ts.URL, ts.URL), "provider=anthropic&api_key=sk-secret-key")
	assert.Equal(t, 502, w.Code)

	var line map[string]any
	require.NoError(t, json.Unmarshal(bytes.Split(logs.Bytes(), []byte("\n"))[0], &line))
	assert.Equal(t, "settings.models.failed", line["msg"])
	assert.Equal(t, "WARN", line["level"])
	assert.Equal(t, "anthropic", line["provider"])
	assert.EqualValues(t, 429, line["provider_status"])
	assert.Equal(t, "rate_limit_error", line["provider_error_type"])
	assert.NotContains(t, logs.String(), "sk-secret-key")
	assert.NotContains(t, logs.String(), "body-text-not-logged")

	logs.Reset()
	closed := httptest.NewServer(http.NotFoundHandler())
	closed.Close()
	w = getModels(modelsRouter(t, closed.URL, closed.URL), "provider=openai&api_key=sk-secret-key")
	assert.Equal(t, 502, w.Code)
	assert.Contains(t, logs.String(), `"provider_status":0`)
	assert.Contains(t, logs.String(), `"error":`)
	assert.NotContains(t, logs.String(), "sk-secret-key")
}
