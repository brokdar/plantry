package planning

import (
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/nutrition"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

// R5 fix: a wish's maximum minutes is the whole cooking time of the plate's
// cell, main and every side cooked beside it, not the main alone.
func sidesFixture() (State, plate.CellKey) {
	mon := time.Date(2026, 11, 30, 0, 0, 0, 0, time.UTC)
	const dinner, curry, roast, salad, couscous int64 = 3, 7, 8, 9, 10
	cell := plate.CellKey{Date: mon, SlotID: dinner}
	comp := func(id int64, name string, role food.Role, mins int) FoodFacts {
		return FoodFacts{ID: id, Name: name, Kind: food.KindComposed, Role: role, Minutes: mins, NutritionComplete: true, Macros: nutrition.Macros{Kcal: 300}}
	}
	return State{
		Foods: map[int64]FoodFacts{
			curry:    comp(curry, "Curry", food.RoleMain, 25),
			roast:    comp(roast, "Roasted vegetables", food.RoleSideVeg, 30),
			salad:    comp(salad, "Green salad", food.RoleSideVeg, 5),
			couscous: comp(couscous, "Couscous", food.RoleSideStarch, 10),
		},
		MaxMinutes: map[cellID]int{idOf(cell): 30},
	}, cell
}

func TestPickSide_WishCountsTheWholePlate(t *testing.T) {
	t.Parallel()
	s, cell := sidesFixture()
	s.Cells = []Cell{{Date: cell.Date, SlotID: cell.SlotID}}
	in := ChooseInput{State: s, Target: Target{Cells: []plate.CellKey{cell}}}
	h := newPlanner(in)
	got, ok := h.pickSide(cell, 7, nil, 0, "")
	require.True(t, ok)
	assert.Contains(t, []int64{9}, got, "25 min main + side: only the 5 minute salad fits 30 minutes")

	in.State.MaxMinutes = nil
	h = newPlanner(in)
	_, ok = h.pickSide(cell, 7, nil, 0, "")
	require.True(t, ok)
	cands := 0
	for id, f := range in.State.Foods {
		if id != 7 && !h.sideTooLong(25+f.Minutes, 7, f, cell) {
			cands++
		}
	}
	assert.Equal(t, 3, cands, "without the wish every side is open")
}

func TestPickSide_ChefAllowanceCountsTheWholePlate(t *testing.T) {
	t.Parallel()
	s, cell := sidesFixture()
	s.MaxMinutes = nil
	s.Settings.ChefOn = true
	s.Profile.MinutesWeekday, s.Profile.MinutesWeekend = 30, 90
	h := newPlanner(ChooseInput{State: s, Target: Target{Cells: []plate.CellKey{cell}}})
	assert.True(t, h.sideTooLong(25+30, 7, s.Foods[8], cell), "main + roasted vegetables is 55 min on a 30 min weekday")
	assert.False(t, h.sideTooLong(25+5, 7, s.Foods[9], cell))
}

func TestFixQuickSides_ReplacesOrDropsASideOverTheLimit(t *testing.T) {
	t.Parallel()
	s, cell := sidesFixture()
	in := ChooseInput{State: s, Target: Target{Cells: []plate.CellKey{cell}}}
	prop := Proposal{
		Sessions: []ProposedSession{
			{Key: "s1", FoodID: 7, CookDate: cell.Date, CookSlotID: cell.SlotID, Portions: 1},
			{Key: "s2", FoodID: 8, CookDate: cell.Date, CookSlotID: cell.SlotID, Portions: 1},
		},
		Plates: []ProposedPlate{{Date: cell.Date, SlotID: cell.SlotID, Components: []ProposedComponent{
			{FoodID: 7, Portions: 1, SessionKey: "s1"}, {FoodID: 8, Portions: 1, SessionKey: "s2"},
		}}},
	}
	got, notes := fixQuickSides(in, prop, map[int64]bool{})
	require.Len(t, notes, 1)
	require.Len(t, got.Sessions, 2)
	assert.Equal(t, int64(9), got.Sessions[1].FoodID, "the 30 min side gives way to the 5 minute salad (closest in energy, ties by id)")
	assert.LessOrEqual(t, Merge(in.State, in.Target, got).cellMinutes(cell, 0), 30)

	// No side fits the main's leftover time: the side is left out.
	in.State.Foods = map[int64]FoodFacts{7: s.Foods[7], 8: s.Foods[8]}
	got, _ = fixQuickSides(in, prop, map[int64]bool{})
	require.Len(t, got.Sessions, 1)
	assert.Len(t, got.Plates[0].Components, 1)

	// Within the limit nothing changes.
	in.State.MaxMinutes = map[cellID]int{idOf(cell): 60}
	got, notes = fixQuickSides(in, prop, map[int64]bool{})
	assert.Empty(t, notes)
	assert.Len(t, got.Sessions, 2)
}

func TestCookCellOver_CountsSidesAlreadyCookedThere(t *testing.T) {
	t.Parallel()
	s, cell := sidesFixture()
	s.Sessions = []Session{{CookSession: plate.CookSession{FoodID: 7, CookDate: cell.Date, CookSlotID: cell.SlotID}}}
	assert.True(t, s.cookCellOver(cell, s.Foods[8], 0), "25 + 30 min")
	assert.False(t, s.cookCellOver(cell, s.Foods[9], 0), "25 + 5 min")
	assert.False(t, s.cookCellOver(cell, s.Foods[8], 7), "replacing the main frees its minutes")
}

// A cook of a leftover main (cooked earlier) adds nothing to the cell's
// cooking time: the wish's 30 minutes stay free for a side.
func TestPickSide_LeftoverMainCostsNoMinutes(t *testing.T) {
	t.Parallel()
	s, cell := sidesFixture()
	s.Foods[7] = func() FoodFacts { f := s.Foods[7]; f.Minutes = 60; return f }()
	s.Foods = map[int64]FoodFacts{7: s.Foods[7], 8: s.Foods[8]}
	sun := plate.CookSession{FoodID: 7, CookDate: cell.Date.AddDate(0, 0, -1), CookSlotID: cell.SlotID}
	s.Sessions = []Session{{CookSession: sun}}
	h := newPlanner(ChooseInput{State: s, Target: Target{Cells: []plate.CellKey{cell}}})
	assert.Zero(t, h.cellCookMinutes(cell, 7, 0))
	got, ok := h.pickSide(cell, 7, nil, 0, "")
	require.True(t, ok, "the 30 min side fits: the 60 min main was cooked on Sunday")
	assert.Equal(t, int64(8), got)

	// Cooked in the cell itself, the main counts and shuts the side out.
	s.Sessions = []Session{{CookSession: plate.CookSession{FoodID: 7, CookDate: cell.Date, CookSlotID: cell.SlotID}}}
	h = newPlanner(ChooseInput{State: s, Target: Target{Cells: []plate.CellKey{cell}}})
	assert.Equal(t, 60, h.cellCookMinutes(cell, 7, 0))
	_, ok = h.pickSide(cell, 7, nil, 0, "")
	assert.False(t, ok)
}

// A day already under its kcal floor takes no side that lowers its kcal.
func TestFixQuickSides_UnderFloorDayKeepsItsKcal(t *testing.T) {
	t.Parallel()
	s, cell := sidesFixture()
	s.Cells = []Cell{{Date: cell.Date, SlotID: cell.SlotID}}
	salad := s.Foods[9]
	salad.Macros.Kcal = 100
	s.Foods[9] = salad
	s.Profile.KcalTarget = func() *float64 { v := 3000.0; return &v }()
	in := ChooseInput{State: s, Target: Target{Cells: []plate.CellKey{cell}}}
	prop := Proposal{
		Sessions: []ProposedSession{
			{Key: "s1", FoodID: 7, CookDate: cell.Date, CookSlotID: cell.SlotID, Portions: 1},
			{Key: "s2", FoodID: 8, CookDate: cell.Date, CookSlotID: cell.SlotID, Portions: 1},
		},
		Plates: []ProposedPlate{{Date: cell.Date, SlotID: cell.SlotID, Components: []ProposedComponent{
			{FoodID: 7, Portions: 1, SessionKey: "s1"}, {FoodID: 8, Portions: 1, SessionKey: "s2"},
		}}},
	}
	before := evaluateLean(Merge(in.State, in.Target, prop), in.Target)
	require.True(t, slices.ContainsFunc(before.Issues, func(is Issue) bool { return is.Code == CodeKcalBelowFloor }), "fixture: the day is under its floor")
	got, notes := fixQuickSides(in, prop, map[int64]bool{})
	assert.Empty(t, notes, "the lighter salad and a drop would both deepen the deficit")
	assert.Len(t, got.Sessions, 2)
}
