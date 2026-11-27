package planning_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/flavor"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
)

func firstMain(t *testing.T, s planning.State) int64 {
	t.Helper()
	p := choose(t, planning.ChooseInput{State: s, Target: windowTarget(s)})
	require.NotEmpty(t, p.Sessions)
	return p.Sessions[0].FoodID
}

// TASTE: the taste score is a soft key. Among mains it ranks before the
// least recently eaten (batchCooker's never-eaten Mains 4-10 tie up to the
// food id) but never above an earlier key: a loved recipe eaten within the
// variety weeks still comes after the others.
func TestHeuristic_TasteRanksMains(t *testing.T) {
	withTaste := func(id int64, score float64) planning.State {
		s := batchCooker()
		m := s.Foods[id]
		m.Taste = score
		s.Foods[id] = m
		return s
	}
	assert.Equal(t, int64(104), firstMain(t, batchCooker()))
	assert.Equal(t, int64(106), firstMain(t, withTaste(106, 0.3)), "the better taste score first")
	assert.Equal(t, int64(104), firstMain(t, withTaste(101, 1)), "eaten within the variety weeks: taste does not outrank it")
	assert.Equal(t, int64(105), firstMain(t, withTaste(104, -0.15)), "rather not: ranked down, not excluded")
	assert.Equal(t, int64(104), firstMain(t, withTaste(106, 0.04)), "below a tenth counts as equal")
}

// TASTE: among sides the taste score breaks ties after plate fit: a better
// liked side never beats the one plate fit wants, but between two that
// plate fit judges equal it decides.
func TestHeuristic_TasteBreaksSideTiesAfterPlateFit(t *testing.T) {
	s := plateFitCooker(true)
	roast := s.Foods[sideRoast]
	roast.Taste = 1
	s.Foods[sideRoast] = roast
	assert.Equal(t, sideSalad, firstSide(t, s), "plate fit (the fresh gap) outranks taste")

	twin := s.Foods[sideSalad]
	twin.ID, twin.Name, twin.Taste = 204, "Tomatensalat", 0.4
	s.Foods[204] = twin
	assert.Equal(t, int64(204), firstSide(t, s), "an equal fit: the better taste score wins over the lower id")
}

// TASTE-9: a main whose cuisine the last 14 days served past the soft cap
// ranks after one that does not, apart from any dislike.
func TestHeuristic_MonotonySoftCap(t *testing.T) {
	s := batchCooker()
	thai := flavor.Attributes{Cuisine: "thai"}
	for _, id := range []int64{101, 104} {
		f := s.Foods[id]
		f.Attrs = thai
		s.Foods[id] = f
	}
	start := s.Window.Start
	for i := range 8 { // eight Thai plates in the 14 days before the window
		s.History = append(s.History, planning.HistoryPlate{Date: start.AddDate(0, 0, -1-i), SlotID: dinner, FoodIDs: []int64{101}})
	}
	assert.Equal(t, int64(105), firstMain(t, s), "Thai is past its cap: the next main")

	s.History = s.History[:7]
	assert.Equal(t, int64(104), firstMain(t, s), "within the cap nothing changes")
}
