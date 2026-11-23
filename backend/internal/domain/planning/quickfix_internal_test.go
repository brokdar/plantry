package planning

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

// V73: the quick wish is judged by the cooking time at the meal. A slow dish
// cooked on Sunday and eaten on Monday's limited dinner as a leftover costs
// nothing there: only a batch cooked in the limited cell counts as slow.
func TestSlowBatch_OnlyTheCookCellCounts(t *testing.T) {
	t.Parallel()
	sun := time.Date(2026, 11, 29, 0, 0, 0, 0, time.UTC)
	mon := sun.AddDate(0, 0, 1)
	const dinner, stew int64 = 3, 7
	s := State{
		Foods:      map[int64]FoodFacts{stew: {ID: stew, Kind: food.KindComposed, Role: food.RoleMain, Minutes: 90}},
		MaxMinutes: map[cellID]int{idOf(plate.CellKey{Date: mon, SlotID: dinner}): 30},
	}
	prop := Proposal{
		Sessions: []ProposedSession{{Key: "s1", FoodID: stew, CookDate: sun, CookSlotID: dinner, Portions: 2}},
		Plates: []ProposedPlate{
			{Date: sun, SlotID: dinner, Components: []ProposedComponent{{FoodID: stew, Portions: 1, SessionKey: "s1"}}},
			{Date: mon, SlotID: dinner, Components: []ProposedComponent{{FoodID: stew, Portions: 1, SessionKey: "s1"}}},
		},
	}
	key, _ := slowBatch(s, prop, map[string]bool{})
	assert.Empty(t, key, "a leftover at the limited dinner is no slow batch")

	prop.Sessions[0].CookDate = mon // the stew now cooks at the limited dinner
	key, limit := slowBatch(s, prop, map[string]bool{})
	assert.Equal(t, "s1", key)
	assert.Equal(t, 30, limit)
}
