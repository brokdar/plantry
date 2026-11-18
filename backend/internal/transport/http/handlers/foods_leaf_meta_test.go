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

func doJSON(t *testing.T, r http.Handler, method, url, body string) (int, map[string]any) {
	t.Helper()
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest(method, url, bytes.NewBufferString(body)))
	var got map[string]any
	_ = json.Unmarshal(resp.Body.Bytes(), &got)
	return resp.Code, got
}

func TestFoodHandler_SetNoCook(t *testing.T) {
	r := setupFoodRouter(t)
	id := createLeaf(t, r, "Reis")
	url := fmt.Sprintf("/api/foods/%d/no-cook", id)

	_, got := doJSON(t, r, http.MethodGet, fmt.Sprintf("/api/foods/%d", id), "")
	assert.Equal(t, false, got["no_cook"])
	assert.Equal(t, "derived", got["no_cook_source"])

	code, got := doJSON(t, r, http.MethodPut, url, `{"value":true}`)
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, true, got["no_cook"])
	assert.Equal(t, "user", got["no_cook_source"])

	code, got = doJSON(t, r, http.MethodPut, url, `{"value":false}`)
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, false, got["no_cook"])
	assert.Equal(t, "user", got["no_cook_source"])

	code, got = doJSON(t, r, http.MethodPut, url, `{"value":null}`)
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, "derived", got["no_cook_source"])

	code, _ = doJSON(t, r, http.MethodPut, "/api/foods/99999/no-cook", `{"value":true}`)
	assert.Equal(t, http.StatusNotFound, code)
	code, _ = doJSON(t, r, http.MethodPut, "/api/foods/x/no-cook", `{"value":true}`)
	assert.Equal(t, http.StatusBadRequest, code)
	code, _ = doJSON(t, r, http.MethodPut, url, `{bad`)
	assert.Equal(t, http.StatusBadRequest, code)
}

func TestFoodHandler_ServingG(t *testing.T) {
	r := setupFoodRouter(t)
	id := createLeaf(t, r, "Apfel")
	url := fmt.Sprintf("/api/foods/%d/no-cook", id)

	code, got := doJSON(t, r, http.MethodPut, url, `{"value":true,"serving_g":150}`)
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, true, got["no_cook"])
	assert.Equal(t, 150.0, got["serving_g"])

	_, got = doJSON(t, r, http.MethodGet, fmt.Sprintf("/api/foods/%d", id), "")
	assert.Equal(t, 150.0, got["serving_g"])

	code, got = doJSON(t, r, http.MethodPut, url, `{"value":null,"serving_g":null}`)
	require.Equal(t, http.StatusOK, code)
	assert.Nil(t, got["serving_g"])

	for _, body := range []string{`{"serving_g":0}`, `{"serving_g":-5}`} {
		code, got = doJSON(t, r, http.MethodPut, url, body)
		assert.Equal(t, http.StatusBadRequest, code, body)
		assert.Contains(t, fmt.Sprint(got), "error.food.invalid_serving")
	}

	code, got = doJSON(t, r, http.MethodPost, "/api/foods",
		fmt.Sprintf(`{"name":"Bowl2","kind":"composed","role":"main","children":[{"child_id":%d,"amount":100,"unit":"g"}]}`, id))
	require.Equal(t, http.StatusCreated, code, got)
	rid := int64(got["id"].(float64))
	code, got = doJSON(t, r, http.MethodPut, fmt.Sprintf("/api/foods/%d/no-cook", rid), `{"value":true,"serving_g":100}`)
	assert.Equal(t, http.StatusBadRequest, code)
	assert.Contains(t, fmt.Sprint(got), "error.food.serving_leaf_only")
}

func TestFoodHandler_NoCookOverridesRecipeToo(t *testing.T) {
	r := setupFoodRouter(t)
	leaf := createLeaf(t, r, "Brot")
	code, got := doJSON(t, r, http.MethodPost, "/api/foods",
		fmt.Sprintf(`{"name":"Bowl","kind":"composed","role":"main","children":[{"child_id":%d,"amount":100,"unit":"g"}]}`, leaf))
	require.Equal(t, http.StatusCreated, code, got)
	id := int64(got["id"].(float64))
	code, got = doJSON(t, r, http.MethodPut, fmt.Sprintf("/api/foods/%d/no-cook", id), `{"value":true}`)
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, true, got["no_cook"])
}

func TestFoodHandler_LeafRoles(t *testing.T) {
	r := setupFoodRouter(t)

	// Derived: produce is side_veg.
	gurke := createLeaf(t, r, "Gurke")
	code, got := doJSON(t, r, http.MethodPut, fmt.Sprintf("/api/foods/%d/shop-section", gurke), `{"section":"produce"}`)
	require.Equal(t, http.StatusOK, code, got)
	assert.Equal(t, "side_veg", got["role"])
	assert.Equal(t, "derived", got["role_source"])

	// Override: a leaf may carry a side or sauce role.
	code, got = doJSON(t, r, http.MethodPost, "/api/foods", `{"name":"Ketchup","kind":"leaf","role":"sauce"}`)
	require.Equal(t, http.StatusCreated, code, got)
	assert.Equal(t, "sauce", got["role"])
	assert.Equal(t, "user", got["role_source"])

	// No role, nothing derived.
	code, got = doJSON(t, r, http.MethodPost, "/api/foods", `{"name":"Zucker","kind":"leaf"}`)
	require.Equal(t, http.StatusCreated, code, got)
	assert.NotContains(t, got, "role")
	assert.Nil(t, got["role_source"])

	// main and standalone stay recipe-only.
	for _, role := range []string{"main", "standalone"} {
		code, _ = doJSON(t, r, http.MethodPost, "/api/foods", fmt.Sprintf(`{"name":"X%s","kind":"leaf","role":"%s"}`, role, role))
		assert.Equal(t, http.StatusBadRequest, code, role)
	}
}
