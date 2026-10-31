package handlers_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func putDietClass(t *testing.T, r http.Handler, id any, body string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/foods/%v/diet-class", id), bytes.NewBufferString(body)))
	return w
}

// createRecipe creates a main recipe of 100 g of each leaf.
func createRecipe(t *testing.T, r http.Handler, name string, leaves ...int64) int64 {
	t.Helper()
	children := make([]string, len(leaves))
	for i, id := range leaves {
		children[i] = fmt.Sprintf(`{"child_id":%d,"amount":100,"unit":"g"}`, id)
	}
	body := fmt.Sprintf(`{"name":%q,"kind":"composed","role":"main","reference_portions":2,"children":[%s]}`,
		name, strings.Join(children, ","))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/foods", bytes.NewBufferString(body)))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var got map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	return int64(got["id"].(float64))
}

// updateRecipeChildren replaces a recipe's children with 100 g of each leaf.
func updateRecipeChildren(t *testing.T, r http.Handler, id int64, leaves ...int64) {
	t.Helper()
	children := make([]string, len(leaves))
	for i, l := range leaves {
		children[i] = fmt.Sprintf(`{"child_id":%d,"amount":100,"unit":"g"}`, l)
	}
	body := fmt.Sprintf(`{"name":"Salat","kind":"composed","role":"main","reference_portions":2,"children":[%s]}`,
		strings.Join(children, ","))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/foods/%d", id), bytes.NewBufferString(body)))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
}

func listFoods(t *testing.T, r http.Handler, query string) (int, []map[string]any) {
	t.Helper()
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/foods?"+query, nil))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var got struct {
		Items []map[string]any `json:"items"`
		Total int              `json:"total"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	return got.Total, got.Items
}

func names(items []map[string]any) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it["name"].(string)
	}
	return out
}

func TestFoodHandler_Diet_LeavesCarryTheirClass(t *testing.T) {
	r := setupFoodRouter(t)
	for name, want := range map[string]string{
		"Hähnchenbrust": "omnivore", "Lachs": "pescetarian", "Feta": "vegetarian", "Kichererbsen": "vegan", "Xyz": "unknown",
	} {
		got := getFood(t, r, createLeaf(t, r, name))
		assert.Equal(t, want, got["diet_class"], name)
		assert.Equal(t, "auto", got["diet_class_source"], name)
	}
	_, items := listFoods(t, r, "kind=leaf")
	require.Len(t, items, 5)
	for _, it := range items {
		assert.Contains(t, it, "diet_class", "list items carry the class")
	}
}

// B3-AC2
func TestFoodHandler_Diet_RecipeIsTheMaxOfItsChildren(t *testing.T) {
	r := setupFoodRouter(t)
	chickpeas, feta := createLeaf(t, r, "Kichererbsen"), createLeaf(t, r, "Feta")
	salad := createRecipe(t, r, "Salat", chickpeas, feta)

	got := getFood(t, r, salad)
	assert.Equal(t, "vegetarian", got["diet_class"])
	assert.Equal(t, "derived", got["diet_class_source"])

	speck := createLeaf(t, r, "Speck")
	updateRecipeChildren(t, r, salad, chickpeas, feta, speck)
	got = getFood(t, r, salad)
	assert.Equal(t, "omnivore", got["diet_class"], "adding Speck")

	w := putDietClass(t, r, salad, `{"class":"vegan"}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	got = getFood(t, r, salad)
	assert.Equal(t, "vegan", got["diet_class"], "the override wins")
	assert.Equal(t, "user", got["diet_class_source"])

	updateRecipeChildren(t, r, salad, chickpeas, feta)
	assert.Equal(t, "vegan", getFood(t, r, salad)["diet_class"], "editing the recipe keeps the override")

	unknown := createRecipe(t, r, "Mystery", chickpeas, createLeaf(t, r, "Xyz"))
	got = getFood(t, r, unknown)
	assert.Equal(t, "unknown", got["diet_class"])
	assert.Equal(t, "derived", got["diet_class_source"])

	// A sub-recipe passes its effective class (with its override) up.
	outer := createRecipe(t, r, "Bowl", salad, chickpeas)
	assert.Equal(t, "vegan", getFood(t, r, outer)["diet_class"])
}

// B3-AC3
func TestFoodHandler_Diet_ListFilter(t *testing.T) {
	r := setupFoodRouter(t)
	chickpeas, feta := createLeaf(t, r, "Kichererbsen"), createLeaf(t, r, "Feta")
	salmon, speck, xyz := createLeaf(t, r, "Lachs"), createLeaf(t, r, "Speck"), createLeaf(t, r, "Xyz")
	createRecipe(t, r, "A Hummus", chickpeas)
	createRecipe(t, r, "B Salat", chickpeas, feta)
	createRecipe(t, r, "C Lachsbowl", chickpeas, salmon)
	createRecipe(t, r, "D Carbonara", feta, speck)
	createRecipe(t, r, "E Mystery", chickpeas, xyz)

	total, items := listFoods(t, r, "kind=composed&diet=vegetarian")
	assert.Equal(t, []string{"A Hummus", "B Salat"}, names(items))
	assert.Equal(t, 2, total)

	_, items = listFoods(t, r, "kind=composed&diet=vegan")
	assert.Equal(t, []string{"A Hummus"}, names(items))

	total, items = listFoods(t, r, "kind=composed&diet=omnivore")
	assert.Equal(t, []string{"A Hummus", "B Salat", "C Lachsbowl", "D Carbonara"}, names(items), "unknown is excluded")
	assert.Equal(t, 4, total)

	total, items = listFoods(t, r, "kind=composed&diet=omnivore&limit=2&offset=1")
	assert.Equal(t, []string{"B Salat", "C Lachsbowl"}, names(items), "paged after filtering")
	assert.Equal(t, 4, total)

	_, items = listFoods(t, r, "kind=composed&diet=omnivore&offset=10")
	assert.Empty(t, items)

	_, items = listFoods(t, r, "kind=leaf&diet=vegan")
	assert.Equal(t, []string{"Kichererbsen"}, names(items), "leaves filter too")

	for _, d := range []string{"unknown", "paleo"} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/foods?diet="+d, nil))
		assert.Equal(t, http.StatusBadRequest, w.Code, d)
		assert.Contains(t, w.Body.String(), "error.food.invalid_diet_class", d)
	}
}

// B3-AC4
func TestFoodHandler_Diet_ClearOverride(t *testing.T) {
	r := setupFoodRouter(t)
	feta := createLeaf(t, r, "Feta")

	w := putDietClass(t, r, feta, `{"class":"vegan"}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var put map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &put))
	assert.Equal(t, "vegan", put["diet_class"])
	assert.Equal(t, "user", put["diet_class_source"])
	assert.Contains(t, put, "used_in_count", "a full leaf food")

	w = putDietClass(t, r, feta, `{"class":null}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &put))
	assert.Equal(t, "vegetarian", put["diet_class"], "the auto class is back")
	assert.Equal(t, "auto", put["diet_class_source"])
	assert.Equal(t, "auto", getFood(t, r, feta)["diet_class_source"])
}

func TestFoodHandler_Diet_SetErrors(t *testing.T) {
	r := setupFoodRouter(t)
	leaf := createLeaf(t, r, "Feta")
	tests := []struct {
		name   string
		id     any
		body   string
		status int
		key    string
	}{
		{"unknown is not settable", leaf, `{"class":"unknown"}`, http.StatusBadRequest, "error.food.invalid_diet_class"},
		{"invalid class", leaf, `{"class":"paleo"}`, http.StatusBadRequest, "error.food.invalid_diet_class"},
		{"empty class", leaf, `{"class":""}`, http.StatusBadRequest, "error.food.invalid_diet_class"},
		{"malformed body", leaf, `{`, http.StatusBadRequest, "error.invalid_body"},
		{"class not a string", leaf, `{"class":3}`, http.StatusBadRequest, "error.invalid_body"},
		{"non-numeric id", "abc", `{"class":null}`, http.StatusBadRequest, "error.invalid_id"},
		{"unknown food", 9999, `{"class":null}`, http.StatusNotFound, "error.not_found"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := putDietClass(t, r, tt.id, tt.body)
			assert.Equal(t, tt.status, w.Code, w.Body.String())
			assert.Contains(t, w.Body.String(), tt.key)
		})
	}
	assert.Equal(t, "auto", getFood(t, r, leaf)["diet_class_source"], "nothing was written")
}

func TestFoodHandler_Diet_ScaledIngredientsCarryTheClass(t *testing.T) {
	r := setupFoodRouter(t)
	feta, speck := createLeaf(t, r, "Feta"), createLeaf(t, r, "Speck")
	inner := createRecipe(t, r, "Salat", feta)
	outer := createRecipe(t, r, "Teller", inner, speck)

	w := getScaled(t, r, fmt.Sprintf("/api/foods/%d/scaled?portions=2", outer))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var got struct {
		Ingredients []struct {
			Name      string `json:"name"`
			DietClass string `json:"diet_class"`
		} `json:"ingredients"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	require.Len(t, got.Ingredients, 2)
	assert.Equal(t, "vegetarian", got.Ingredients[0].DietClass, "a composed child carries its derived class")
	assert.Equal(t, "omnivore", got.Ingredients[1].DietClass)
}
