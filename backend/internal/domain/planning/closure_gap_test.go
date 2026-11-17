package planning_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
)

// A day regenerate of Sunday expands to the batch closure: Monday lunch ate
// the replaced batch. Monday's cooking cap is used by a confirmed cook at
// dinner, so Monday lunch can only eat a leftover. With a 1 day keep window
// the greedy walk cooked on Saturday, let that batch feed Sunday and left
// Sunday's own cooking occasion unused, so nothing reached Monday lunch.
// Every cell that was filled must be filled again.
func TestHeuristic_ClosureCellOnCappedDayIsRefilled(t *testing.T) {
	for _, tc := range []struct {
		name   string
		keep   int
		target []plateCellKey
	}{
		{"sunday cook", 1, []plateCellKey{cell(sun, dinner), cell(mon, lunch)}},
		{"saturday cook", 1, []plateCellKey{cell(sat, dinner), cell(sun, lunch), cell(sun, dinner), cell(mon, lunch)}},
		{"saturday cook with sunday breakfast", 1, []plateCellKey{cell(sat, dinner), cell(sun, breakfast), cell(sun, lunch), cell(sun, dinner), cell(mon, lunch)}},
		{"saturday cook, long keep", 3, []plateCellKey{cell(sat, dinner), cell(sun, lunch), cell(sun, dinner), cell(mon, lunch)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := batchCooker()
			s.Settings.DefaultKeepDays = tc.keep
			s.Foods[curry] = recipe(curry, "Curry", food.RoleMain, "vegan", 600)
			// Confirmed cook on Monday dinner, eaten there only.
			s.Sessions = []planning.Session{persistedSession(7, curry, mon, dinner, 1, true)}
			s.Plates = append(s.Plates, persisted(70, mon, dinner, true, curry, 7))
			target := targetCells(tc.target...)

			p := choose(t, planning.ChooseInput{State: s, Target: target, Avoid: []int64{101}})

			issues := check(s, target, p)
			assert.Empty(t, only(issues, "H1"), "%+v", issues)
			assert.Empty(t, only(issues, "H2"), "%+v", issues)
			require.Len(t, p.Plates, len(tc.target))
			var monLunch bool
			for _, pp := range p.Plates {
				monLunch = monLunch || (pp.Date.Equal(day(mon)) && pp.SlotID == lunch)
			}
			assert.True(t, monLunch, "monday lunch is filled")
		})
	}
}

// A forced cook must reach the stranded cell with the keep days of the recipe
// it really cooks, not the default. Tuesday lunch can only eat a leftover
// (Tuesday dinner is a confirmed cook); Monday has no open cell. Sunday lunch
// is two days away: only the recipe that keeps 3 days may be cooked there.
func TestHeuristic_ForcedCookReachesWithRecipeKeepDays(t *testing.T) {
	s := batchCooker()
	s.Settings.DefaultKeepDays = 3
	one, three := 1, 3
	for id, f := range s.Foods {
		f.KeepDays = &one
		s.Foods[id] = f
	}
	long := s.Foods[105]
	long.KeepDays = &three
	s.Foods[105] = long
	side := s.Foods[201]
	side.KeepDays = &three
	s.Foods[201] = side
	s.Foods[curry] = recipe(curry, "Curry", food.RoleMain, "vegan", 600)
	s.Sessions = []planning.Session{persistedSession(7, curry, mon, dinner, 1, true), persistedSession(8, curry, tue, dinner, 1, true)}
	s.Plates = append(s.Plates, persisted(70, mon, dinner, true, curry, 7), persisted(80, tue, dinner, true, curry, 8))
	target := targetCells(cell(sat, dinner), cell(sun, lunch), cell(tue, lunch))

	p := choose(t, planning.ChooseInput{State: s, Target: target})

	issues := check(s, target, p)
	assert.Empty(t, only(issues, "H1"), "%+v", issues)
	assert.Empty(t, only(issues, "H2"), "%+v", issues)
	assert.Empty(t, only(issues, "H4"), "%+v", issues)
	assert.Len(t, p.Sessions, 4, "two cooks with a side each, no more: %+v", p.Sessions)
}

// Two stranded cells are both refilled and the cooks stay within the cap.
func TestHeuristic_TwoStrandedCellsAreRefilled(t *testing.T) {
	s := batchCooker()
	s.Settings.DefaultKeepDays = 1
	s.Foods[curry] = recipe(curry, "Curry", food.RoleMain, "vegan", 600)
	s.Sessions = []planning.Session{persistedSession(7, curry, mon, dinner, 1, true), persistedSession(8, curry, wed, dinner, 1, true)}
	s.Plates = append(s.Plates, persisted(70, mon, dinner, true, curry, 7), persisted(80, wed, dinner, true, curry, 8))
	target := targetCells(cell(sun, dinner), cell(mon, lunch), cell(tue, dinner), cell(wed, lunch))

	p := choose(t, planning.ChooseInput{State: s, Target: target, Avoid: []int64{101}})

	issues := check(s, target, p)
	assert.Empty(t, only(issues, "H1"), "%+v", issues)
	assert.Empty(t, only(issues, "H2"), "%+v", issues)
}
