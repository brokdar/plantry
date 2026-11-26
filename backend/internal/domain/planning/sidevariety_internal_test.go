package planning

import (
	"testing"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// With plate fit on, a side another plate of the day serves comes after every
// other side (CP1-variety: no side twice a day); off, the pick stays as it was.
func TestPickSide_NotTwiceADay(t *testing.T) {
	lunch := plate.CellKey{Date: bfMon, SlotID: bfLunch}
	h := sideAddPlanner()
	first, ok := h.pickSide(lunch, bfCurry, nil, 0, "")
	require.True(t, ok)
	other := sdRice
	if first == sdRice {
		other = bfCouscous
	}

	h = sideAddPlanner()
	h.prop.Plates[2].Components = append(h.prop.Plates[2].Components, ProposedComponent{FoodID: first, Portions: 1, SessionKey: "c"})
	got, ok := h.pickSide(lunch, bfCurry, nil, 0, "")
	require.True(t, ok)
	assert.Equal(t, other, got, "the dinner serves %d already", first)

	h.s.Settings.PlateFit = false
	got, _ = h.pickSide(lunch, bfCurry, nil, 0, "")
	assert.Equal(t, first, got, "plate fit off: no change")
}

// The guard's side picker counts its own re-picks in the week's rotation, so
// two re-picks with equal sides do not both take the same one (the explorer
// week got Carrot salad on seven plates).
func TestSidePickerFor_RotatesItsPicks(t *testing.T) {
	h := sideAddPlanner()
	in := h.in
	in.State.Plates = nil
	pick := sidePickerFor(in, Proposal{})
	require.NotNil(t, pick)
	a, ok := pick(plate.CellKey{Date: bfMon, SlotID: bfLunch}, bfCurry, nil)
	require.True(t, ok)
	b, ok := pick(plate.CellKey{Date: bfMon, SlotID: bfDinner}, bfCurry, nil)
	require.True(t, ok)
	assert.NotEqual(t, a, b)
}

// Between sides that hold the day alike, addSide takes the one eaten longest
// ago (the rotation across weeks): couscous was eaten last week, rice never.
func TestAddSide_RotatesByLastEaten(t *testing.T) {
	h := sideAddPlanner()
	last := bfMon.AddDate(0, 0, -7)
	f := h.s.Foods[bfCouscous]
	f.LastEaten = &last
	h.s.Foods[bfCouscous] = f
	h.addSide(bfMon, 2200)
	assert.Equal(t, sdRice, sideOnLunch(h))
}

// A meal at its share of the day does not grow while another meal of the day
// is under its own: the lift goes to the light meal first (CP1-variety: no
// 1,300 kcal lunch beside a 650 kcal dinner), and the heavy meal only carries
// what the light one cannot.
func TestTopUpDay_GrowsTheLightMealFirst(t *testing.T) {
	plates := func() []ProposedPlate {
		return bfPlates(
			[]ProposedComponent{{FoodID: bfBake, Portions: 1.25, SessionKey: "b"}},
			[]ProposedComponent{{FoodID: bfCurry, Portions: 1, SessionKey: "a"}},
		)
	}
	setup := func(fit bool) *planner {
		h, _ := bandsPlanner(nil, plates())
		h.prop.Sessions[1].CookSlotID, h.prop.Sessions[2].CookSlotID = bfDinner, bfLunch
		h.s.Settings.PlateFit = fit
		h.in.State = h.s
		h.topUpBase = map[[2]int]float64{}
		return h
	}
	portions := func(h *planner, slot int64) float64 { return plateOf(h, slot).Components[0].Portions }

	h := setup(true)
	require.Equal(t, map[int64]bool{bfLunch: true}, h.fullMeals(bfMon), "the 875 kcal bake is past the lunch's share")
	h.topUpDay(bfMon, 150, true, false)
	assert.Equal(t, 1.25, portions(h, bfLunch), "the full lunch keeps its portions")
	assert.Equal(t, 1.5, portions(h, bfDinner), "the curry dinner takes the lift")

	h = setup(true)
	h.topUpDay(bfMon, 400, true, false)
	assert.Equal(t, 1.5, portions(h, bfDinner))
	assert.Greater(t, portions(h, bfLunch), 1.25, "what the dinner cannot carry goes to the lunch")

	h = setup(true)
	w := 150.0
	h.s.Profile.WeightKg = &w
	h.in.State = h.s
	assert.Nil(t, h.fullMeals(bfMon), "a day under its protein target: the protein meal may grow")

	h = setup(false)
	assert.Nil(t, h.fullMeals(bfMon), "plate fit off: no change")
}

// Batch variation never gives a leftover plate a side another plate of the
// day already serves: dayFoods are the foods of the day's other plates.
func TestDayFoods(t *testing.T) {
	merged := State{Plates: []Plate{
		{ID: -1, Date: bfMon, SlotID: bfLunch, Components: []Component{{FoodID: bfCurry}, {FoodID: sdRice}}},
		{ID: -2, Date: bfMon, SlotID: bfDinner, Components: []Component{{FoodID: bfBake}}},
		{ID: -3, Date: bfMon.Add(24 * time.Hour), SlotID: bfLunch, Components: []Component{{FoodID: bfCouscous}}},
	}}
	assert.Equal(t, map[int64]bool{bfCurry: true, sdRice: true}, merged.dayFoods(merged.Plates[1]))
}
