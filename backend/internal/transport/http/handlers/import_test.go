package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/httpfetch"
	"github.com/jaltszeimer/plantry/backend/internal/adapters/jsonld"
	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/importer"
	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
	"github.com/jaltszeimer/plantry/backend/internal/transport/http/handlers"
)

// noAI is the resolver main wires when no provider is configured.
type noAI struct{}

func (noAI) Current(context.Context) (llm.Client, string, error) {
	return nil, "", llm.ErrProviderMissing
}

func newImportRouter(t *testing.T) http.Handler {
	t.Helper()
	db := testhelper.NewTestDB(t)
	repo := sqlite.NewFoodRepo(db)
	resolver := food.NewResolver(repo, nil, offlineFDC{}, nil)
	svc := importer.NewService(httpfetch.New(), jsonld.Extractor{}, noAI{}, resolver, food.NewService(repo))
	h := handlers.NewImportHandler(svc)
	r := chi.NewRouter()
	r.Post("/api/import/extract", h.Extract)
	r.Get("/api/import/lookup", h.LookupLine)
	return r
}

// offlineFDC is an FDC provider without a key: configured reports false.
type offlineFDC struct{}

func (offlineFDC) SearchByName(context.Context, string, int) ([]food.Candidate, error) {
	return nil, nil
}
func (offlineFDC) Configured(context.Context) bool { return false }

type errBody struct {
	MessageKey string            `json:"message_key"`
	Status     int               `json:"status"`
	Params     map[string]string `json:"params"`
}

func postExtract(t *testing.T, h http.Handler, ctx context.Context, body string) (*httptest.ResponseRecorder, errBody) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/import/extract", bytes.NewBufferString(body)).WithContext(ctx)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var eb errBody
	_ = json.Unmarshal(rec.Body.Bytes(), &eb)
	return rec, eb
}

func TestImportExtract_DistinctErrorsCarryTheHost(t *testing.T) {
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/missing":
			http.NotFound(w, r)
		case "/blocked":
			w.WriteHeader(http.StatusForbidden)
		case "/boom":
			w.WriteHeader(http.StatusInternalServerError)
		case "/slow":
			time.Sleep(300 * time.Millisecond)
		default: // an HTML page with no recipe data
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte("<html><body>hi</body></html>"))
		}
	}))
	defer site.Close()
	host := mustHost(t, site.URL)

	closed := httptest.NewServer(http.NotFoundHandler())
	closedURL := closed.URL
	closed.Close()

	h := newImportRouter(t)
	cases := []struct {
		name    string
		url     string
		timeout time.Duration
		status  int
		key     string
		host    string
	}{
		{"404", site.URL + "/missing", 0, 502, "error.import.not_found", host},
		{"403", site.URL + "/blocked", 0, 502, "error.import.blocked", host},
		{"500", site.URL + "/boom", 0, 502, "error.import.fetch_failed", host},
		{"timeout", site.URL + "/slow", 50 * time.Millisecond, 504, "error.import.timeout", host},
		{"connection refused", closedURL + "/x", 0, 502, "error.import.unreachable", mustHost(t, closedURL)},
		{"no recipe, no assistant", site.URL + "/page", 0, 422, "error.import.no_recipe_no_ai", host},
		{"not a url", "hello", 0, 400, "error.import.invalid_url", ""},
		{"ftp", "ftp://x.y/z", 0, 400, "error.import.invalid_url", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			if tc.timeout > 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, tc.timeout)
				defer cancel()
			}
			body, _ := json.Marshal(map[string]string{"url": tc.url})
			rec, eb := postExtract(t, h, ctx, string(body))
			assert.Equal(t, tc.status, rec.Code)
			assert.Equal(t, tc.key, eb.MessageKey)
			assert.Equal(t, tc.host, eb.Params["host"])
		})
	}
}

func TestImportExtract_PastedPageWithoutRecipe(t *testing.T) {
	rec, eb := postExtract(t, newImportRouter(t), context.Background(), `{"html":"<html><body>hi</body></html>"}`)
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	assert.Equal(t, "error.import.no_recipe_no_ai", eb.MessageKey)
}

// F6: no FDC key and nothing local is "not configured", not "nothing found".
func TestImportLookup_EmptyResultSaysWhy(t *testing.T) {
	h := newImportRouter(t)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/import/lookup?query=onion", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	var out struct {
		Results           []any  `json:"results"`
		RecommendedIndex  int    `json:"recommended_index"`
		UnavailableReason string `json:"unavailable_reason"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	assert.Empty(t, out.Results)
	assert.Equal(t, -1, out.RecommendedIndex)
	assert.Equal(t, "not_configured", out.UnavailableReason)
}

func mustHost(t *testing.T, raw string) string {
	t.Helper()
	u, err := url.Parse(raw)
	require.NoError(t, err)
	return strings.TrimSpace(u.Hostname())
}
