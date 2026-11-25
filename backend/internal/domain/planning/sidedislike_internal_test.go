package planning

import (
	"testing"

	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const sdRice = int64(16)

// sideAddPlanner is bandsPlanner's day short of its target (2200; it eats
// 1466 kcal) with plate fit on: the lunch holds the curry alone, the dinner
// the bake. Rice and couscous are the same side dish for the planner (250
// kcal a portion, cooked fresh beside the curry), so either holds the day's
// kcal and the tie goes to the lower id, couscous.
func sideAddPlanner(ratherNot ...int64) *planner {
	h, _ := bandsPlanner(nil, bfPlates(
		[]ProposedComponent{{FoodID: bfCurry, Portions: 1, SessionKey: "a"}},
		[]ProposedComponent{{FoodID: bfBake, Portions: 1, SessionKey: "b"}},
	))
	rice := h.s.Foods[bfCouscous]
	rice.ID, rice.Name = sdRice, "Reis"
	h.s.Foods[sdRice] = rice
	h.prop.Sessions = h.prop.Sessions[:3] // no couscous batch
	h.s.RatherNot = map[int64]bool{}
	for _, id := range ratherNot {
		h.s.RatherNot[id] = true
	}
	h.s.Settings.PlateFit = true
	h.in.State = h.s
	h.sided, h.topUpBase, h.adds = map[int]bool{}, map[[2]int]float64{}, map[int]*dayAdds{}
	return h
}

// sideOnLunch is the side addSide put beside the lunch's curry (0: none).
func sideOnLunch(h *planner) int64 {
	for _, c := range plateOf(h, bfLunch).Components {
		if c.FoodID != bfCurry {
			return c.FoodID
		}
	}
	return 0
}

// addSide never adds a side the household would rather not get, except for
// the floor lift, and there only after every other side that holds the day.
func TestAddSide_RatherNot(t *testing.T) {
	tests := []struct {
		name      string
		ratherNot []int64
		floorLift bool
		want      int64
	}{
		{name: "no refusal: the tie goes to couscous", want: bfCouscous},
		{name: "refused couscous: rice", ratherNot: []int64{bfCouscous}, want: sdRice},
		{name: "both refused: no side", ratherNot: []int64{bfCouscous, sdRice}},
		{name: "floor lift: the refused side ranks after the other", ratherNot: []int64{bfCouscous}, floorLift: true, want: sdRice},
		{name: "floor lift, nothing else: the refused side", ratherNot: []int64{bfCouscous, sdRice}, floorLift: true, want: bfCouscous},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := sideAddPlanner(tt.ratherNot...)
			h.floorLift = tt.floorLift
			dk := h.dayNow(bfMon)
			require.NotNil(t, dk)
			require.InDelta(t, 1466, dk.Kcal, 1)
			h.addSide(bfMon, 2200)
			assert.Equal(t, tt.want, sideOnLunch(h))
		})
	}
}

// The filler passes (culinary and nutrition) never add a rather-not food;
// the floor lift may, when no other food is left.
func TestAddNoCook_RatherNot(t *testing.T) {
	t.Run("culinary pass: the next filler", func(t *testing.T) {
		h, _ := fillerPlanner(true, 55, true)
		h.s.RatherNot = map[int64]bool{pfCheese: true}
		h.in.State = h.s
		got := added(t, h, 2200)
		assert.NotEqual(t, pfCheese, got)
		assert.NotZero(t, got)
	})
	t.Run("culinary pass, only a refused filler: none", func(t *testing.T) {
		h, _ := fillerPlanner(true, 55, true)
		h.s.NoCook, h.s.RatherNot = []int64{pfCheese}, map[int64]bool{pfCheese: true}
		h.in.State = h.s
		assert.False(t, h.addNoCook(bfMon, false, 2200))
	})
	t.Run("floor lift, only a refused filler: added", func(t *testing.T) {
		h, _ := fillerPlanner(true, 55, true)
		h.s.NoCook, h.s.RatherNot = []int64{pfCheese}, map[int64]bool{pfCheese: true}
		h.in.State = h.s
		h.floorLift = true
		assert.Equal(t, pfCheese, added(t, h, 2200))
	})
	t.Run("floor lift: a refused filler after the others", func(t *testing.T) {
		h, _ := fillerPlanner(true, 55, true)
		h.s.RatherNot = map[int64]bool{pfCheese: true, pfChickpeas: true}
		h.in.State = h.s
		h.floorLift = true
		assert.Equal(t, pfBread, added(t, h, 2200))
	})
	t.Run("nutrition pass, day short: no refused filler", func(t *testing.T) {
		h := nutritionFillPlanner(nfApple, false, 2850)
		h.s.RatherNot = map[int64]bool{nfApple: true}
		h.in.State = h.s
		h.nutritionFill()
		assert.False(t, holds(h, nfApple))
		assert.Empty(t, fillChoices(h))
	})
}

// A side pick never doubles a food the plate already holds: sideFit does not
// count the candidate's own twin as a clash, so pickSide must skip it (the
// guard's re-pick put a second Steamed broccoli beside the chili).
func TestPickSide_NeverDoublesAPlateFood(t *testing.T) {
	h := sideAddPlanner()
	c := plate.CellKey{Date: bfMon, SlotID: bfLunch}
	first, ok := h.pickSide(c, bfCurry, nil, 0, "")
	require.True(t, ok)
	if second, ok := h.pickSide(c, bfCurry, []int64{first}, 0, ""); ok {
		assert.NotEqual(t, first, second, "a second %d beside the first", first)
	}
	if got, ok := h.pickSide(c, bfCurry, []int64{bfCurry}, 0, ""); ok {
		assert.NotEqual(t, bfCurry, got)
	}
}

func TestBatchHolds(t *testing.T) {
	p := Proposal{Plates: []ProposedPlate{{Components: []ProposedComponent{
		{FoodID: 1, SessionKey: "a"}, {FoodID: 9, SessionKey: "b"},
	}}, {Components: []ProposedComponent{{FoodID: 1, SessionKey: "a"}, {FoodID: 2, SessionKey: "s"}}}}}
	assert.True(t, batchHolds(p, "a", 9), "plate 1 already eats food 9 from another session")
	assert.True(t, batchHolds(p, "a", 2), "plate 2 eats food 2 from another session")
	assert.False(t, batchHolds(p, "a", 3))
}
