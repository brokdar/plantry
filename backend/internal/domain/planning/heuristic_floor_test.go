package planning_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaltszeimer/plantry/backend/internal/domain/availability"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
)

func floorCodes(issues []planning.Issue) []string {
	var out []string
	for _, is := range issues {
		if is.Rule == "H9" {
			out = append(out, is.Date)
		}
	}
	return out
}

// A day under the floor is lifted to it: only mains and carb sides grow,
// never by more than 1.5x, and batch sessions stay consistent (no hard
// violation, every session covers its plates).
func TestHeuristic_FloorTopUpLiftsADay(t *testing.T) {
	s := batchCooker()
	s.Profile.KcalTarget = f64(3000) // floor 2250; a weekday holds ~1850

	tgt := windowTarget(s)
	p := choose(t, planning.ChooseInput{State: s, Target: tgt})
	m := planning.Merge(s, tgt, p)

	assert.Empty(t, floorCodes(planning.Check(m, tgt)), "every day reaches the floor")
	assert.Empty(t, violations(planning.Check(m, tgt)), "sessions still cover their plates")

	eaten := map[string]float64{}
	for _, pl := range p.Plates {
		for _, c := range pl.Components {
			assert.LessOrEqual(t, c.Portions, 1.5+1e-9, "capped at 1.5x")
			f := s.Foods[c.FoodID]
			if f.Role == food.RoleSideVeg {
				assert.InDelta(t, 1, c.Portions, 1e-9, "sides do not grow")
			}
			if c.SessionKey != "" {
				eaten[c.SessionKey] += c.Portions
			}
		}
	}
	for _, ss := range p.Sessions {
		assert.GreaterOrEqual(t, ss.Portions+1e-9, eaten[ss.Key], "session %s covers its plates (shopping buys the session)", ss.Key)
	}
}

// A floor the plan cannot reach within the cap ends as an explicit H9 and a
// note to the user, not an endless loop.
func TestHeuristic_UnreachableFloorStaysAnH9WithANote(t *testing.T) {
	s := batchCooker()
	s.Profile.KcalTarget = f64(5000) // floor 3750, far above 1.5x of a weekday

	tgt := windowTarget(s)
	p := choose(t, planning.ChooseInput{State: s, Target: tgt})
	m := planning.Merge(s, tgt, p)

	assert.NotEmpty(t, floorCodes(planning.Check(m, tgt)))
	assert.Contains(t, p.Notes, "daily minimum")
}

// A routine cell with no plate has no kcal to count: it does not scale the
// floor, so the day is not a false H9.
func TestCheck_FloorIgnoresRoutineCellWithoutPlate(t *testing.T) {
	s := baseState()
	s.Profile.KcalTarget = f64(1600) // 75 % = 1200; two of three meals: 800
	setMode(&s, cell(mon, breakfast), availability.ModeRoutine)

	_, issues := weekOf(s)

	assert.NotContains(t, codeDates(issues, "H9", planning.CodeKcalBelowFloor), mon)
}

func TestBoardIssues_FloorOnlyForDaysThatCanChange(t *testing.T) {
	s := baseState()
	s.Today = day(mon)
	s.Profile.KcalTarget = f64(2400) // floor 1800, a day of three curries holds 1500
	id := int64(1)
	for _, d := range []struct {
		date      string
		confirmed bool
	}{{sat, true}, {tue, false}, {mon, true}} {
		for _, slot := range []int64{breakfast, lunch, dinner} {
			s.Plates = append(s.Plates, persisted(id, d.date, slot, d.confirmed, curry, 0))
			id++
		}
	}

	got := map[string]planning.Severity{}
	for _, is := range planning.BoardIssues(s) {
		if is.Rule == "H9" {
			got[is.Date] = is.Severity
		}
	}

	assert.NotContains(t, got, sat, "past days are history")
	assert.Equal(t, planning.SeverityViolation, got[tue], "a draft day can still be fixed")
	assert.Equal(t, planning.SeverityWarning, got[mon], "a confirmed day is at most a warning")
}
