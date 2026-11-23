package planning

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// T1c (ledger V73, tune-v2 g2-planner-04): with light lunches the heuristic
// left days under their band (1669-1770 kcal against 1782) although each light
// lunch sat well under its cap (482-570 against 737): topUpDay never grows a
// light meal, and the dinner was at its largest portion. The day's top-up now
// grows the light meal's batch within its cap, with no new cooking time.
func TestTopUpFloors_GrowsALightLunchWithinItsCap(t *testing.T) {
	t.Parallel()
	light := []LightIntent{{Date: bfMon.Format(time.DateOnly), Scope: LightScopeMeal, Slot: "lunch"}}
	h, tgt := bandsPlanner(light, bfPlates(
		[]ProposedComponent{{FoodID: bfCurry, Portions: 1, SessionKey: "a"}},
		[]ProposedComponent{{FoodID: bfBake, Portions: MaxDishPortions, SessionKey: "b"}},
	))
	h.prop.Sessions[2].Portions = MaxDishPortions
	k := 2600.0
	h.s.Profile.KcalTarget = &k
	day := func() (DayEval, PlateEval) {
		ev := evaluateLean(Merge(h.s, tgt, h.prop), tgt)
		require.Len(t, ev.Days, 1)
		for _, pe := range ev.Plates {
			if pe.SlotID == bfLunch {
				return ev.Days[0], pe
			}
		}
		t.Fatal("no lunch")
		return DayEval{}, PlateEval{}
	}
	d0, lunch0 := day()
	require.Less(t, d0.Kcal, d0.BandLow, "the fixture starts under the band")
	require.Positive(t, lunch0.MaxKcal, "the lunch is marked light")
	require.Less(t, lunch0.Kcal+316*roundPortions, lunch0.MaxKcal, "the light lunch has room")

	h.topUpFloors()

	d1, lunch1 := day()
	assert.Greater(t, lunch1.Kcal, lunch0.Kcal, "the light lunch grew")
	assert.LessOrEqual(t, lunch1.Kcal, lunch1.MaxKcal+0.5, "and stays light")
	assert.Greater(t, d1.Kcal, d0.Kcal)
	assert.Equal(t, d0.CookOccasions, d1.CookOccasions, "no new cooking time")
}
