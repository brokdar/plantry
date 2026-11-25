package planning_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
)

// KIT-1: the time allowance is a soft preference. Of two mains, the favourite
// takes 45 minutes and the other 20; the favourite would win every tie.
// Weekdays allow 30 minutes, weekends and batch cook days (a recipe for at
// least 3 plates) 60.
func TestHeuristic_TimeAllowanceIsASoftPreference(t *testing.T) {
	const slowID, fastID int64 = 101, 102
	build := func(slowMin, weekday, weekend int, batch bool, refPortions float64) planning.State {
		s := baseState()
		s.Settings.BatchCooking = batch
		s.Profile.MinutesWeekday, s.Profile.MinutesWeekend = weekday, weekend
		slow := recipe(slowID, "Slow", food.RoleMain, "omnivore", 600)
		slow.Favorite, slow.Minutes, slow.ReferencePortions = true, slowMin, refPortions
		fast := recipe(fastID, "Fast", food.RoleMain, "omnivore", 600)
		fast.Minutes, fast.ReferencePortions = 20, refPortions
		s.Foods = map[int64]planning.FoodFacts{slowID: slow, fastID: fast}
		return s
	}
	cases := []struct {
		name  string
		date  string
		s     planning.State
		wants int64
	}{
		{"weekday over the allowance yields to the faster dish", tue, build(45, 30, 60, false, 1), fastID},
		{"weekday within the raised allowance keeps the favourite", tue, build(45, 50, 60, false, 1), slowID},
		{"weekend uses the weekend allowance", sat, build(45, 30, 60, false, 1), slowID},
		{"weekend over the weekend allowance yields", sat, build(45, 30, 40, false, 1), fastID},
		{"a batch for 3 plates on a weekday gets the weekend allowance", tue, build(45, 30, 60, true, 3), slowID},
		{"a batch for 2 plates does not", tue, build(45, 30, 60, true, 2), fastID},
		{"unknown minutes are never over", tue, build(0, 30, 60, false, 1), slowID},
		{"no allowance known (a hand-built state) changes nothing", tue, build(45, 0, 0, false, 1), slowID},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := choose(t, planning.ChooseInput{State: tc.s, Target: targetCells(cell(tc.date, dinner))})
			if assert.Len(t, p.Sessions, 1) {
				assert.Equal(t, tc.wants, p.Sessions[0].FoodID)
			}
		})
	}
}
