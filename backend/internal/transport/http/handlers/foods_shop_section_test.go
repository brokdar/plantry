package handlers_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func putShopSection(t *testing.T, r http.Handler, id any, body string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/foods/%v/shop-section", id), bytes.NewBufferString(body)))
	return w
}

func getFood(t *testing.T, r http.Handler, id int64) map[string]any {
	t.Helper()
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/foods/%d", id), nil))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var got map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	return got
}

func TestFoodHandler_ShopSection_AC5(t *testing.T) {
	r := setupFoodRouter(t)
	id := createLeaf(t, r, "Kidneybohnen (Dose)")

	got := getFood(t, r, id)
	assert.Equal(t, "cans_jars", got["shop_section"])
	assert.Equal(t, "auto", got["shop_section_source"])

	w := putShopSection(t, r, id, `{"section":"frozen"}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var put map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &put))
	assert.Equal(t, "frozen", put["shop_section"])
	assert.Equal(t, "user", put["shop_section_source"])
	assert.Contains(t, put, "used_in_count", "a full leaf food")

	got = getFood(t, r, id)
	assert.Equal(t, "frozen", got["shop_section"])
	assert.Equal(t, "user", got["shop_section_source"])

	w = putShopSection(t, r, id, `{"section":null}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	got = getFood(t, r, id)
	assert.Equal(t, "cans_jars", got["shop_section"])
	assert.Equal(t, "auto", got["shop_section_source"])
}

func TestFoodHandler_ShopSection_ListCarriesSection(t *testing.T) {
	r := setupFoodRouter(t)
	createLeaf(t, r, "Milch")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/foods?kind=leaf", nil))
	require.Equal(t, http.StatusOK, w.Code)
	var got struct {
		Items []map[string]any `json:"items"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	require.Len(t, got.Items, 1)
	assert.Equal(t, "dairy_eggs", got.Items[0]["shop_section"])
	assert.Equal(t, "auto", got.Items[0]["shop_section_source"])
}

func TestFoodHandler_ShopSection_Errors(t *testing.T) {
	r := setupFoodRouter(t)
	leaf := createLeaf(t, r, "Zwiebel")
	body, _ := json.Marshal(map[string]any{
		"name": "Chili", "kind": "composed", "role": "main", "reference_portions": 4,
		"children": []map[string]any{{"child_id": leaf, "amount": 1, "unit": "g", "grams": 1}},
	})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/foods", bytes.NewReader(body)))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var recipe map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &recipe))
	recipeID := int64(recipe["id"].(float64))
	assert.NotContains(t, recipe, "shop_section", "recipes have no section")

	tests := []struct {
		name   string
		id     any
		body   string
		status int
		key    string
	}{
		{"composed food", recipeID, `{"section":"produce"}`, http.StatusBadRequest, "error.food.shop_section_leaf_only"},
		{"unknown section", leaf, `{"section":"garden"}`, http.StatusBadRequest, "error.food.invalid_shop_section"},
		{"empty section", leaf, `{"section":""}`, http.StatusBadRequest, "error.food.invalid_shop_section"},
		{"malformed body", leaf, `{`, http.StatusBadRequest, "error.invalid_body"},
		{"section not a string", leaf, `{"section":3}`, http.StatusBadRequest, "error.invalid_body"},
		{"non-numeric id", "abc", `{"section":null}`, http.StatusBadRequest, "error.invalid_id"},
		{"unknown food", 9999, `{"section":null}`, http.StatusNotFound, "error.not_found"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := putShopSection(t, r, tt.id, tt.body)
			assert.Equal(t, tt.status, w.Code, w.Body.String())
			assert.Contains(t, w.Body.String(), tt.key)
		})
	}
	assert.Equal(t, "auto", getFood(t, r, leaf)["shop_section_source"], "nothing was written")
}
