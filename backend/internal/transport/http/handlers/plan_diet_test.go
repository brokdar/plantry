package handlers_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// B2-AC2: a vegetarian profile and a Chili with chicken breast (omnivore by
// its ingredient's name, derived through the recipe) gives one H5
// diet_incompatible issue per Chili plate.
func TestPlanHTTP_DietIncompatible(t *testing.T) {
	a := newReadAPI(t)
	chicken := a.insert(`INSERT INTO foods (name, kind, source) VALUES ('Hähnchenbrust', 'leaf', 'manual')`)
	a.insert(fmt.Sprintf(`INSERT INTO food_components (parent_id, child_id, amount, unit, grams, sort_order) VALUES (%d, %d, 300, 'g', 300, 1)`, a.chili, chicken))
	a.ac1()

	issuesOf := func() []map[string]any {
		code, body := a.do(http.MethodGet, "/api/plan?start=2026-10-31&today=2026-10-30", "")
		require.Equal(t, http.StatusOK, code, body)
		out := []map[string]any{}
		for _, it := range body["issues"].([]any) {
			out = append(out, it.(map[string]any))
		}
		return out
	}
	assert.Empty(t, issuesOf(), "an omnivore profile eats the Chili")

	_, err := a.db.Exec(`UPDATE user_profile SET diet_type = 'vegetarian' WHERE id = 1`)
	require.NoError(t, err)
	issues := issuesOf()
	require.Len(t, issues, 3, "one per Chili plate")
	for _, is := range issues {
		assert.Equal(t, "H5", is["rule"])
		assert.Equal(t, "diet_incompatible", is["code"])
		assert.Equal(t, "violation", is["severity"])
		assert.Equal(t, map[string]any{"food_id": float64(a.chili), "diet_class": "omnivore", "diet_type": "vegetarian"}, is["params"])
	}
}
