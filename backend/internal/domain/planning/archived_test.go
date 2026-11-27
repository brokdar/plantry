package planning_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
)

// An archived recipe (LIB-4) is never auto-planned: the heuristic offers
// neither its mains nor its sides, whatever it picked before the archive.
func TestHeuristic_NeverPlansArchivedRecipes(t *testing.T) {
	s := batchCooker()
	tgt := windowTarget(s)
	before := choose(t, planning.ChooseInput{State: s, Target: tgt})
	picked := map[int64]bool{}
	for _, ps := range before.Sessions {
		picked[ps.FoodID] = true
	}
	for _, pp := range before.Plates {
		for _, c := range pp.Components {
			picked[c.FoodID] = true
		}
	}
	archived := 0
	for id := range picked {
		if f := s.Foods[id]; f.Composed() {
			f.Archived = true
			s.Foods[id] = f
			archived++
		}
	}
	assert.Positive(t, archived)

	after := choose(t, planning.ChooseInput{State: s, Target: tgt})

	for _, ps := range after.Sessions {
		assert.False(t, s.Foods[ps.FoodID].Archived, "session of archived %d", ps.FoodID)
	}
	for _, pp := range after.Plates {
		for _, c := range pp.Components {
			f := s.Foods[c.FoodID]
			assert.False(t, f.Composed() && f.Archived, "plate holds archived %d", c.FoodID)
		}
	}
}
