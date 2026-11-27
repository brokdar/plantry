package planning_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaltszeimer/plantry/backend/internal/domain/flavor"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
)

func withFacts(s planning.State, id int64, edit func(*planning.FoodFacts)) planning.State {
	f := s.Foods[id]
	edit(&f)
	s.Foods[id] = f
	return s
}

// THEME-1: a staple is the strong boost: among mains that qualify it comes
// first, ahead of the taste score.
func TestHeuristic_StapleComesFirst(t *testing.T) {
	assert.Equal(t, int64(104), firstMain(t, batchCooker()))
	s := withFacts(batchCooker(), 109, func(f *planning.FoodFacts) { f.Staple = true })
	s = withFacts(s, 106, func(f *planning.FoodFacts) { f.Taste = 1 })
	assert.Equal(t, int64(109), firstMain(t, s))
}

// THEME-1: no soft rule holds a staple back: a monotony cap it passes does
// not rank it down.
func TestHeuristic_StapleIgnoresMonotonyCap(t *testing.T) {
	s := batchCooker()
	thai := flavor.Attributes{Cuisine: "thai"}
	for _, id := range []int64{101, 104, 105} {
		s = withFacts(s, id, func(f *planning.FoodFacts) { f.Attrs = thai })
	}
	start := s.Window.Start
	for i := range 8 {
		s.History = append(s.History, planning.HistoryPlate{Date: start.AddDate(0, 0, -1-i), SlotID: dinner, FoodIDs: []int64{101}})
	}
	assert.Equal(t, int64(106), firstMain(t, s), "Thai past its cap ranks down")
	s = withFacts(s, 105, func(f *planning.FoodFacts) { f.Staple = true })
	assert.Equal(t, int64(105), firstMain(t, s), "a staple past the cap still comes first")
}

// THEME-1: a staple stays bound by the hard rules: one eaten within the
// variety weeks (H8) does not come before the others.
func TestHeuristic_StapleBoundByVariety(t *testing.T) {
	s := withFacts(batchCooker(), 101, func(f *planning.FoodFacts) { f.Staple = true }) // eaten 3 days ago
	assert.Equal(t, int64(104), firstMain(t, s))
}

// A recipe holding a never item is no candidate: not as a main, not as a
// side, not as an alternative.
func TestHeuristic_NeverItemExcluded(t *testing.T) {
	s := withFacts(batchCooker(), 104, func(f *planning.FoodFacts) { f.Never = true })
	assert.Equal(t, int64(105), firstMain(t, s))
	s = plateFitCooker(true)
	assert.Equal(t, sideSalad, firstSide(t, s))
	s = withFacts(s, sideSalad, func(f *planning.FoodFacts) { f.Never = true })
	assert.NotEqual(t, sideSalad, firstSide(t, s))

	alt := altState()
	for _, a := range altResult(t, alt, planning.AltRequest{PlateID: altSunLunch, What: planning.AltMain}).Alternatives {
		if a.FoodID == lasagne {
			alt = withFacts(alt, lasagne, func(f *planning.FoodFacts) { f.Never = true })
		}
	}
	for _, a := range altResult(t, alt, planning.AltRequest{PlateID: altSunLunch, What: planning.AltMain}).Alternatives {
		assert.NotEqual(t, lasagne, a.FoodID)
	}
}
