package agent_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper/testsvc"
)

// find_alternatives gives the food's and the plate's kcal apart, the food
// being replaced and, for a light ask, only candidates under the cap (audit B6).
func TestFindAlternatives_FoodAndPlateKcalCurrentAndLight(t *testing.T) {
	t.Parallel()
	e := newPlanToolEnv(t)
	e.withMacros()
	require.NoError(t, testsvc.Settings(e.db).Set(context.Background(), "plan.max_cook_occasions_per_day", "2"))
	p, _ := e.twoDinners()
	var main plate.PlateComponent
	for _, c := range p.Components {
		if c.Position == plate.PositionMain {
			main = c
		}
	}
	require.NotZero(t, main.ID)

	res := e.mustExec("find_alternatives", fmt.Sprintf(`{"plate_id":%d,"what":"main","limit":10}`, p.ID))
	cur, ok := res["current"].(map[string]any)
	require.True(t, ok, "the replaced food comes as current: %v", res)
	assert.EqualValues(t, main.FoodID, cur["food_id"])
	assert.Contains(t, cur, "food_kcal")
	assert.Contains(t, cur, "plate_kcal")
	alts := res["alternatives"].([]any)
	require.NotEmpty(t, alts)
	for _, a := range alts {
		m := a.(map[string]any)
		assert.Contains(t, m, "food_kcal")
		assert.Contains(t, m, "plate_kcal")
		assert.Equal(t, "main", m["role"])
		assert.NotContains(t, m, "kcal", "no ambiguous kcal")
		assert.NotContains(t, m, "protein_g")
	}

	// A light ask lists only plates within the cap.
	light := e.mustExec("find_alternatives", fmt.Sprintf(`{"plate_id":%d,"what":"main","light":"lighter","limit":10}`, p.ID))
	lc := light["current"].(map[string]any)
	limit, _ := lc["max_kcal"].(float64)
	require.Positive(t, limit, "the cap of the marked meal: %v", lc)
	for _, a := range light["alternatives"].([]any) {
		assert.LessOrEqual(t, a.(map[string]any)["plate_kcal"], limit)
	}

	meal := e.mustExec("find_alternatives", fmt.Sprintf(`{"plate_id":%d,"what":"meal","limit":3}`, p.ID))
	if len(p.Components) > 1 {
		assert.Contains(t, meal["hint"], "what=main")
	}
}
