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

type scaledBody struct {
	FoodID            int64   `json:"food_id"`
	Portions          float64 `json:"portions"`
	ReferencePortions float64 `json:"reference_portions"`
	Factor            float64 `json:"factor"`
	Ingredients       []struct {
		ChildID int64   `json:"child_id"`
		Name    string  `json:"name"`
		Kind    string  `json:"kind"`
		Amount  float64 `json:"amount"`
		Unit    string  `json:"unit"`
		Grams   float64 `json:"grams"`
		Display struct {
			Amount float64 `json:"amount"`
			Unit   string  `json:"unit"`
		} `json:"display"`
		Buy *struct {
			Amount float64 `json:"amount"`
			Unit   string  `json:"unit"`
		} `json:"buy"`
	} `json:"ingredients"`
}

// createChili creates a Chili for 4 with 400 g beans and 2 onions (1 piece = 120 g).
func createChili(t *testing.T, r http.Handler) int64 {
	t.Helper()
	beans := createLeaf(t, r, "Kidney beans")
	onion := createLeaf(t, r, "Onion")
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/foods/%d/portions", onion),
		bytes.NewBufferString(`{"unit":"piece","grams":120}`)))
	require.Less(t, resp.Code, 300, resp.Body.String())

	body := fmt.Sprintf(`{"name":"Chili","kind":"composed","role":"main","reference_portions":4,`+
		`"children":[{"child_id":%d,"amount":400,"unit":"g"},{"child_id":%d,"amount":2,"unit":"piece"}]}`, beans, onion)
	resp = httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest(http.MethodPost, "/api/foods", bytes.NewBufferString(body)))
	require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())
	var got map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	return int64(got["id"].(float64))
}

func getScaled(t *testing.T, r http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, path, nil))
	return resp
}

// B5-AC2
func TestFoodHandler_Scaled(t *testing.T) {
	r := setupFoodRouter(t)
	id := createChili(t, r)

	resp := getScaled(t, r, fmt.Sprintf("/api/foods/%d/scaled?portions=6", id))
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	var got scaledBody
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	assert.Equal(t, id, got.FoodID)
	assert.InDelta(t, 6, got.Portions, 1e-9)
	assert.InDelta(t, 4, got.ReferencePortions, 1e-9)
	assert.InDelta(t, 1.5, got.Factor, 1e-9)
	require.Len(t, got.Ingredients, 2)

	beans, onions := got.Ingredients[0], got.Ingredients[1]
	assert.Equal(t, "Kidney beans", beans.Name)
	assert.Equal(t, "leaf", beans.Kind)
	assert.InDelta(t, 600, beans.Amount, 1e-9)
	assert.InDelta(t, 600, beans.Grams, 1e-9)
	assert.InDelta(t, 600, beans.Display.Amount, 1e-9)
	assert.Equal(t, "g", beans.Display.Unit)
	assert.Nil(t, beans.Buy)

	assert.Equal(t, "piece", onions.Unit)
	assert.InDelta(t, 3, onions.Amount, 1e-9)
	assert.InDelta(t, 360, onions.Grams, 1e-9)
	require.NotNil(t, onions.Buy)
	assert.InDelta(t, 3, onions.Buy.Amount, 1e-9)
	assert.Equal(t, "piece", onions.Buy.Unit)

	resp = getScaled(t, r, fmt.Sprintf("/api/foods/%d/scaled?portions=3", id))
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	onions = got.Ingredients[1]
	assert.InDelta(t, 1.5, onions.Amount, 1e-9)
	require.NotNil(t, onions.Buy)
	assert.InDelta(t, 2, onions.Buy.Amount, 1e-9)
}

// B5-AC5
func TestFoodHandler_Scaled_Errors(t *testing.T) {
	r := setupFoodRouter(t)
	id := createChili(t, r)
	leaf := createLeaf(t, r, "Rice")

	tests := []struct {
		name       string
		path       string
		wantStatus int
		wantKey    string
	}{
		{"zero portions", fmt.Sprintf("/api/foods/%d/scaled?portions=0", id), http.StatusBadRequest, "error.food.invalid_portions"},
		{"negative portions", fmt.Sprintf("/api/foods/%d/scaled?portions=-2", id), http.StatusBadRequest, "error.food.invalid_portions"},
		{"above 100", fmt.Sprintf("/api/foods/%d/scaled?portions=101", id), http.StatusBadRequest, "error.food.invalid_portions"},
		{"missing portions", fmt.Sprintf("/api/foods/%d/scaled", id), http.StatusBadRequest, "error.food.invalid_portions"},
		{"non-numeric portions", fmt.Sprintf("/api/foods/%d/scaled?portions=abc", id), http.StatusBadRequest, "error.food.invalid_portions"},
		{"NaN portions", fmt.Sprintf("/api/foods/%d/scaled?portions=NaN", id), http.StatusBadRequest, "error.food.invalid_portions"},
		{"unknown id", "/api/foods/999999/scaled?portions=2", http.StatusNotFound, "error.not_found"},
		{"non-numeric id", "/api/foods/abc/scaled?portions=2", http.StatusBadRequest, "error.invalid_id"},
		{"leaf food", fmt.Sprintf("/api/foods/%d/scaled?portions=2", leaf), http.StatusBadRequest, "error.invalid_body"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := getScaled(t, r, tt.path)
			require.Equal(t, tt.wantStatus, resp.Code, resp.Body.String())
			var body map[string]any
			require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
			assert.Equal(t, tt.wantKey, body["message_key"])
		})
	}
}
