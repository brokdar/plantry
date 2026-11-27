package planning_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

// trialCooker is batchCooker with two never-eaten mains and no variety
// span, so the heuristic cooks each of them in several sessions.
func trialCooker() planning.State {
	s := batchCooker()
	for id := int64(101); id <= 110; id++ {
		if id != 104 && id != 105 {
			delete(s.Foods, id)
		}
	}
	s.Settings.VarietyWeeks = 0
	s.Settings.ChefOn = true
	return s
}

func sessionsOf(t *testing.T, s planning.State, id int64) int {
	t.Helper()
	n := 0
	for _, ss := range choose(t, planning.ChooseInput{State: s, Target: windowTarget(s)}).Sessions {
		if ss.FoodID == id {
			n++
		}
	}
	return n
}

// AC-15 (NEW-1): a trial recipe is cooked in at most one session per window
// until its trial ends; one already cooked in the window takes none; an
// archived recipe is never auto-planned.
func TestHeuristic_TrialOncePerWindow(t *testing.T) {
	assert.Greater(t, sessionsOf(t, trialCooker(), 104), 1, "precondition: repeats without a trial")

	s := withFacts(trialCooker(), 104, func(f *planning.FoodFacts) { f.Trial = true })
	assert.Equal(t, 1, sessionsOf(t, s, 104))

	s.Sessions = append(s.Sessions, planning.Session{CookSession: plate.CookSession{ID: 900, FoodID: 104, CookDate: s.Window.Start}, Confirmed: true})
	assert.Equal(t, 0, sessionsOf(t, s, 104), "already cooked in the window")

	s = withFacts(trialCooker(), 104, func(f *planning.FoodFacts) { f.Trial = true })
	s.Settings.ChefOn = false
	assert.Greater(t, sessionsOf(t, s, 104), 1, "Chef off: no trial rule")

	s = withFacts(trialCooker(), 104, func(f *planning.FoodFacts) { f.Archived = true })
	assert.Equal(t, 0, sessionsOf(t, s, 104))
}
