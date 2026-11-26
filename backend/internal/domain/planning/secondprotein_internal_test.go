package planning

import (
	"testing"

	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// proteinSidePlanner is sideAddPlanner with the rice side turned into
// marinated tofu (a protein side) and, with meat, the curry into a chicken
// curry that carries its own protein.
func proteinSidePlanner(meat bool, ratherNot ...int64) *planner {
	h := sideAddPlanner(ratherNot...)
	tofu := h.s.Foods[sdRice]
	tofu.Name, tofu.Role = "Marinated tofu", food.RoleSideProtein
	h.s.Foods[sdRice] = tofu
	if meat {
		curry := h.s.Foods[bfCurry]
		curry.Name, curry.Groups.MeatG = "Chicken curry", 120
		h.s.Foods[bfCurry] = curry
	}
	h.in.State = h.s
	return h
}

// A protein side beside a main that carries its own protein clashes (judges:
// "beef ragù with marinated tofu"); beside a main without one it does not, and
// a starch side never does.
func TestSideFit_SecondProtein(t *testing.T) {
	h := proteinSidePlanner(true)
	clash, _ := h.s.sideFit(bfCurry, nil, h.s.Foods[sdRice])
	assert.True(t, clash, "tofu beside a chicken curry")
	clash, _ = h.s.sideFit(bfCurry, nil, h.s.Foods[bfCouscous])
	assert.False(t, clash, "couscous beside a chicken curry")
	clash, _ = h.s.sideFit(bfBake, []int64{bfCurry}, h.s.Foods[sdRice])
	assert.True(t, clash, "another component carries the protein")
	beans := h.s.Foods[bfCouscous]
	beans.Name, beans.Role, beans.Groups.LegumeG = "Bean salad", food.RoleSideVeg, 120
	clash, _ = h.s.sideFit(bfCurry, nil, beans)
	assert.False(t, clash, "a bean side is a vegetable beside the chicken curry")

	h = proteinSidePlanner(false)
	clash, _ = h.s.sideFit(bfCurry, nil, h.s.Foods[sdRice])
	assert.False(t, clash, "tofu beside a lentil curry without its own protein")
}

// pickSide passes over a second protein even when it would rank first.
func TestPickSide_NoSecondProtein(t *testing.T) {
	lunch := plate.CellKey{Date: bfMon, SlotID: bfLunch}
	h := proteinSidePlanner(false)
	h.used[bfCouscous] = 3 // the rotation prefers the tofu
	got, ok := h.pickSide(lunch, bfCurry, nil, 0, "")
	require.True(t, ok)
	require.Equal(t, sdRice, got, "no protein on the plate: the tofu may join")

	h = proteinSidePlanner(true)
	h.used[bfCouscous] = 3
	got, ok = h.pickSide(lunch, bfCurry, nil, 0, "")
	require.True(t, ok)
	assert.Equal(t, bfCouscous, got)
}

// addSide adds no second protein; the floor lift may, after every other side.
func TestAddSide_NoSecondProtein(t *testing.T) {
	tests := []struct {
		name      string
		ratherNot []int64
		floorLift bool
		want      int64
	}{
		{name: "only the tofu is left: no side", ratherNot: []int64{bfCouscous}},
		{name: "floor lift: couscous before the tofu", floorLift: true, want: bfCouscous},
		{name: "floor lift, nothing else lifts: the tofu", ratherNot: []int64{bfCouscous}, floorLift: true, want: sdRice},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := proteinSidePlanner(true, tt.ratherNot...)
			h.floorLift = tt.floorLift
			// Couscous was eaten last week: by rotation alone the tofu would win.
			last := bfMon.AddDate(0, 0, -7)
			f := h.s.Foods[bfCouscous]
			f.LastEaten = &last
			h.s.Foods[bfCouscous] = f
			h.addSide(bfMon, 2200)
			assert.Equal(t, tt.want, sideOnLunch(h))
		})
	}
}

// CP1 family: a lentil curry got a tofu add. Legumes are the main's protein,
// so with plate fit on no tofu leaf joins it; off, the pairing is unchanged,
// and a main without its own protein still takes tofu.
func TestAddOnFits_NoTofuBesideLegumeMain(t *testing.T) {
	h := sideAddPlanner()
	s := h.s
	s.PairAddOns = true
	curry := s.Foods[bfCurry]
	curry.Name, curry.Groups.LegumeG = "Speedy lentil coconut curry", 150
	s.Foods[bfCurry] = curry
	const tofuID int64 = 9901
	tofu := FoodFacts{ID: tofuID, Name: "Tofu (firm)", Kind: food.KindLeaf, DietClass: "vegan", NoCook: true, NutritionComplete: true}
	s.Foods[tofuID] = tofu
	p := Plate{Date: bfMon, SlotID: bfLunch, Components: []Component{{FoodID: bfCurry, Position: plate.PositionMain}}}

	s.Settings.PlateFit = true
	assert.False(t, s.addOnFits(p, tofu, "lunch"), "tofu beside a lentil curry")
	s.Settings.PlateFit = false
	assert.True(t, s.addOnFits(p, tofu, "lunch"), "plate fit off: unchanged")

	s.Settings.PlateFit = true
	curry.Name, curry.Groups.LegumeG, curry.Macros.Protein = "Vegetable curry", 0, 1
	s.Foods[bfCurry] = curry
	assert.True(t, s.addOnFits(p, tofu, "lunch"), "a main without its own protein takes tofu")
}
