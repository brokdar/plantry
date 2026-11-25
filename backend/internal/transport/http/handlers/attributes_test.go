package handlers_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain/flavor"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/foodevent"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
	transport "github.com/jaltszeimer/plantry/backend/internal/transport/http"
	"github.com/jaltszeimer/plantry/backend/internal/transport/http/handlers"
)

func TestAttributesHTTP(t *testing.T) {
	db := testhelper.NewTestDB(t)
	ctx := context.Background()
	foods := sqlite.NewFoodRepo(db)
	resolver := food.NewNutritionResolver(foods)
	restorers := map[foodevent.Kind]foodevent.Restorer{}
	events := foodevent.NewService(sqlite.NewTxRunner(db), sqlite.NewFoodEventRepo(db), restorers, nil)
	svc := flavor.NewService(sqlite.NewFoodAttributeRepo(db), foods, resolver, events)
	restorers[foodevent.KindAttribute] = svc
	router := transport.NewRouter(slog.New(slog.NewTextHandler(io.Discard, nil)), http.NotFoundHandler(), transport.Handlers{
		Foods:      handlers.NewFoodHandler(food.NewService(foods), resolver, nil),
		FoodEvents: handlers.NewFoodEventHandler(events),
		Attributes: handlers.NewAttributeHandler(svc),
	})

	src := food.SourceManual
	kcal, fat := 18.0, 0.2
	leaf := &food.Food{Name: "Tomaten", Kind: food.KindLeaf, Source: &src, Kcal100g: &kcal, Fat100g: &fat}
	require.NoError(t, foods.Create(ctx, leaf))
	role, portions := food.RoleMain, 2.0
	recipe := &food.Food{
		Name: "Tomatensuppe", Kind: food.KindComposed, Role: &role, ReferencePortions: &portions,
		Tags: []string{"Italienisch"}, Children: []food.FoodComponent{{ChildID: leaf.ID, Amount: 600, Unit: "g", Grams: 600}},
	}
	require.NoError(t, foods.Create(ctx, recipe))
	path := fmt.Sprintf("/api/foods/%d/attributes", recipe.ID)

	do := func(method, path, body string) (int, map[string]any) {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader(body)))
		var out map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		return rec.Code, out
	}

	code, body := do(http.MethodGet, path, "")
	require.Equal(t, http.StatusOK, code, body)
	assert.Equal(t, map[string]any{"cuisine": "italian", "heroes": []any{"Tomaten"}, "richness": float64(0), "saucy": true, "colour": "red"}, body["attributes"])
	assert.Equal(t, "rule", body["sources"].(map[string]any)["cuisine"])
	assert.NotContains(t, body, "undo")

	code, body = do(http.MethodPut, path, `{"kind":"soup","taste":{"acid":2,"umami":1},"cuisine":null}`)
	require.Equal(t, http.StatusOK, code, body)
	attrs := body["attributes"].(map[string]any)
	assert.Equal(t, "soup", attrs["kind"])
	assert.Equal(t, map[string]any{"salt": float64(0), "acid": float64(2), "sweet": float64(0), "bitter": float64(0), "umami": float64(1), "heat": float64(0)}, attrs["taste"])
	assert.Equal(t, "user", body["sources"].(map[string]any)["kind"])
	assert.Equal(t, float64(flavor.Version), body["version"])
	undo := body["undo"].(map[string]any)

	// Undo through the food change log.
	tok, err := json.Marshal(undo)
	require.NoError(t, err)
	code, _ = do(http.MethodPost, "/api/food-events/revert", string(tok))
	require.Equal(t, http.StatusOK, code)
	_, body = do(http.MethodGet, path, "")
	assert.NotContains(t, body["attributes"], "kind")

	for _, tc := range []struct {
		name, method, path, body string
		status                   int
		key                      string
	}{
		{"malformed json", http.MethodPut, path, `{`, http.StatusBadRequest, "error.invalid_body"},
		{"not an object", http.MethodPut, path, `[]`, http.StatusBadRequest, "error.invalid_body"},
		{"unknown field", http.MethodPut, path, `{"spiciness":3}`, http.StatusBadRequest, "error.invalid_body"},
		{"wrong type", http.MethodPut, path, `{"richness":"high"}`, http.StatusBadRequest, "error.invalid_body"},
		{"outside vocabulary", http.MethodPut, path, `{"colour":"turquoise"}`, http.StatusBadRequest, "error.invalid_body"},
		{"non-numeric id", http.MethodGet, "/api/foods/abc/attributes", "", http.StatusBadRequest, "error.invalid_id"},
		{"missing food", http.MethodGet, "/api/foods/9999/attributes", "", http.StatusNotFound, ""},
		{"missing food put", http.MethodPut, "/api/foods/9999/attributes", `{"kind":"soup"}`, http.StatusNotFound, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code, body := do(tc.method, tc.path, tc.body)
			assert.Equal(t, tc.status, code, body)
			if tc.key != "" {
				assert.Equal(t, tc.key, body["message_key"])
			}
		})
	}
}
