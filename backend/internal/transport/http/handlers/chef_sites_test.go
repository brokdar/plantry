package handlers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/crypto"
	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain/settings"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
	"github.com/jaltszeimer/plantry/backend/internal/transport/http/handlers"
)

type siteItem struct {
	Domain       string     `json:"domain"`
	Lang         string     `json:"lang"`
	Enabled      bool       `json:"enabled"`
	Default      bool       `json:"default"`
	BlockedUntil *time.Time `json:"blocked_until"`
}

// The sites routes list the allow-list with each site's default flag and
// search block, toggle a site, add a normalised site (bad domain, bad
// language and duplicates refused), refuse removing a default site and
// clear a block.
func TestSitesHandler(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := testhelper.NewTestDB(t)
	svc := settings.NewService(sqlite.NewAppSettingsRepo(db), settings.NewEnvSnapshot(nil), crypto.NilCipher{})
	repo := sqlite.NewFindRepo(db)
	h := handlers.NewSitesHandler(svc, repo)
	r := chi.NewRouter()
	r.Get("/api/chef/sites", h.List)
	r.Post("/api/chef/sites", h.Add)
	r.Put("/api/chef/sites/{domain}", h.SetEnabled)
	r.Delete("/api/chef/sites/{domain}", h.Remove)
	r.Post("/api/chef/sites/{domain}/retry", h.Retry)
	do := func(method, path, body string) (*httptest.ResponseRecorder, []siteItem) {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(method, path, strings.NewReader(body)))
		var out struct {
			Items []siteItem `json:"items"`
		}
		if w.Code == http.StatusOK {
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
		}
		return w, out.Items
	}
	find := func(items []siteItem, d string) *siteItem {
		for i := range items {
			if items[i].Domain == d {
				return &items[i]
			}
		}
		return nil
	}

	w, items := do(http.MethodGet, "/api/chef/sites", "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Len(t, items, len(settings.DefaultChefSites()))
	require.NotNil(t, find(items, "bbcgoodfood.com"))
	assert.True(t, find(items, "bbcgoodfood.com").Default)
	assert.Nil(t, find(items, "bbcgoodfood.com").BlockedUntil)

	t.Run("toggle", func(t *testing.T) {
		w, items := do(http.MethodPut, "/api/chef/sites/bbcgoodfood.com", `{"enabled":false}`)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		assert.False(t, find(items, "bbcgoodfood.com").Enabled)
		stored, err := svc.ChefSites(ctx)
		require.NoError(t, err)
		assert.False(t, stored[0].Enabled)
		w, _ = do(http.MethodPut, "/api/chef/sites/bbcgoodfood.com", `{}`)
		assert.Equal(t, http.StatusBadRequest, w.Code)
		w, _ = do(http.MethodPut, "/api/chef/sites/unknown.example.com", `{"enabled":true}`)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("add normalises and validates", func(t *testing.T) {
		w, items := do(http.MethodPost, "/api/chef/sites", `{"domain":" HTTPS://www.Example-Food.com/recipes/1 ","lang":"de"}`)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		added := find(items, "example-food.com")
		require.NotNil(t, added)
		assert.Equal(t, siteItem{Domain: "example-food.com", Lang: "de", Enabled: true}, *added)

		for name, body := range map[string]string{
			"no dot":       `{"domain":"localhost","lang":"en"}`,
			"empty":        `{"domain":"  ","lang":"en"}`,
			"spaces":       `{"domain":"not a host.com","lang":"en"}`,
			"bad language": `{"domain":"other-food.com","lang":"fr"}`,
			"no body":      `nope`,
		} {
			w, _ := do(http.MethodPost, "/api/chef/sites", body)
			assert.Equal(t, http.StatusBadRequest, w.Code, name)
		}
		w, _ = do(http.MethodPost, "/api/chef/sites", `{"domain":"www.example-food.com","lang":"en"}`)
		assert.Equal(t, http.StatusConflict, w.Code, "duplicate of an added site")
		w, _ = do(http.MethodPost, "/api/chef/sites", `{"domain":"chefkoch.de","lang":"de"}`)
		assert.Equal(t, http.StatusConflict, w.Code, "duplicate of a default site")
	})

	t.Run("remove", func(t *testing.T) {
		w, _ := do(http.MethodDelete, "/api/chef/sites/chefkoch.de", "")
		require.Equal(t, http.StatusConflict, w.Code, "a default site can only be disabled")
		stored, err := svc.ChefSites(ctx)
		require.NoError(t, err)
		assert.True(t, find2(stored, "chefkoch.de"))

		w, items := do(http.MethodDelete, "/api/chef/sites/example-food.com", "")
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		assert.Nil(t, find(items, "example-food.com"))
		w, _ = do(http.MethodDelete, "/api/chef/sites/example-food.com", "")
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("blocked site shows its date and retry clears it", func(t *testing.T) {
		until := time.Now().Add(20 * 24 * time.Hour).UTC().Truncate(time.Second)
		require.NoError(t, repo.BlockSearch(ctx, []string{"budgetbytes.com"}, until))
		require.NoError(t, repo.BlockSearch(ctx, []string{"minimalistbaker.com"}, time.Now().Add(-time.Hour)))
		_, items := do(http.MethodGet, "/api/chef/sites", "")
		require.NotNil(t, find(items, "budgetbytes.com").BlockedUntil)
		assert.True(t, find(items, "budgetbytes.com").BlockedUntil.Equal(until))
		assert.Nil(t, find(items, "minimalistbaker.com").BlockedUntil, "an expired block is not shown")

		w, items := do(http.MethodPost, "/api/chef/sites/budgetbytes.com/retry", "")
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		assert.Nil(t, find(items, "budgetbytes.com").BlockedUntil)
		blocked, err := repo.SearchBlocked(ctx, time.Now())
		require.NoError(t, err)
		assert.Empty(t, blocked)
	})
}

func find2(sites []settings.ChefSite, d string) bool {
	for _, s := range sites {
		if s.Domain == d {
			return true
		}
	}
	return false
}
