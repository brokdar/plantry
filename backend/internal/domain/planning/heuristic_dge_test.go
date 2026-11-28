package planning_test

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
)

// cooked reports whether the proposal cooks food id.
func cooked(p planning.Proposal, id int64) bool {
	for _, s := range p.Sessions {
		if s.FoodID == id {
			return true
		}
	}
	return false
}

func withGroups(s planning.State, id int64, g planning.FoodGroups) {
	f := s.Foods[id]
	f.Groups = g
	s.Foods[id] = f
}

// The DGE rules rank below everything else: among mains that are otherwise
// equal (batchCooker's Main 4..10 differ only by id), a fish or a legume main
// is preferred once while the week has none, and a meat main that would take
// the week past 300 g comes after the others.
func TestHeuristic_DGETieBreak(t *testing.T) {
	const (
		meaty  = int64(104)
		legume = int64(109)
		fish   = int64(110)
	)
	t.Run("prefers one fish and one legume main when none is planned", func(t *testing.T) {
		s := batchCooker()
		base := choose(t, planning.ChooseInput{State: s, Target: windowTarget(s)})
		assert.False(t, cooked(base, fish) && cooked(base, legume), "without groups the late ids are not both picked")

		withGroups(s, fish, planning.FoodGroups{FishG: 120})
		withGroups(s, legume, planning.FoodGroups{LegumeG: 80})
		p := choose(t, planning.ChooseInput{State: s, Target: windowTarget(s)})
		assert.True(t, cooked(p, fish), "fish main")
		assert.True(t, cooked(p, legume), "legume main")
	})
	t.Run("a dash of fish or legumes (under 50 g) is no pick", func(t *testing.T) {
		s := batchCooker()
		base := choose(t, planning.ChooseInput{State: s, Target: windowTarget(s)})
		withGroups(s, fish, planning.FoodGroups{FishG: 49})
		withGroups(s, legume, planning.FoodGroups{LegumeG: 49})
		p := choose(t, planning.ChooseInput{State: s, Target: windowTarget(s)})
		assert.Equal(t, cooked(base, fish), cooked(p, fish))
		assert.Equal(t, cooked(base, legume), cooked(p, legume))
	})
	t.Run("no fish pick for a vegetarian, legumes still", func(t *testing.T) {
		s := batchCooker()
		s.Profile.DietType = profile.DietVegetarian
		for id := int64(101); id <= 110; id++ {
			f := s.Foods[id]
			f.DietClass = "vegetarian"
			s.Foods[id] = f
		}
		withGroups(s, fish, planning.FoodGroups{FishG: 120})
		withGroups(s, legume, planning.FoodGroups{LegumeG: 80})
		p := choose(t, planning.ChooseInput{State: s, Target: windowTarget(s)})
		assert.True(t, cooked(p, legume))
	})
	t.Run("a meat main past 300 g a week comes last", func(t *testing.T) {
		s := batchCooker()
		withGroups(s, meaty, planning.FoodGroups{MeatG: 100}) // x 4 reference portions = 400 g
		p := choose(t, planning.ChooseInput{State: s, Target: windowTarget(s)})
		assert.False(t, cooked(p, meaty))
	})
}

// A fish main past two fish meals in the week comes last (R5: the CP4b
// explorer week held three fish meals).
func TestHeuristic_FishMealsPastTwoRankLast(t *testing.T) {
	s := batchCooker()
	s.Settings.BatchCooking = false
	for id := int64(101); id <= 110; id++ { // none eaten lately
		f := s.Foods[id]
		f.LastEaten = nil
		s.Foods[id] = f
	}
	for id := int64(104); id <= 108; id++ {
		withGroups(s, id, planning.FoodGroups{FishG: 120})
		f := s.Foods[id]
		f.Favorite = true // favorites rank above the DGE keys: only the cap holds the fish back
		s.Foods[id] = f
	}
	p := choose(t, planning.ChooseInput{State: s, Target: windowTarget(s)})
	fish := 0
	for _, ss := range p.Sessions {
		if s.Foods[ss.FoodID].Groups.FishG >= 50 {
			fish++
		}
	}
	assert.Equal(t, 2, fish, "the favorite fish mains fill two meals")
}

func TestEvaluate_FishAboveTwoMealsIsASoftIssue(t *testing.T) {
	s := batchCooker()
	s.Profile.KcalTarget = f64(1800)
	for id := int64(101); id <= 110; id++ {
		withGroups(s, id, planning.FoodGroups{FishG: 120})
	}
	target := windowTarget(s)
	p := choose(t, planning.ChooseInput{State: s, Target: target})
	ev := planning.Evaluate(planning.Merge(s, target, p), target)
	require.NotNil(t, ev.Week)
	has := slices.ContainsFunc(ev.Issues, func(is planning.Issue) bool { return is.Code == planning.CodeWeekFishAboveDGE })
	assert.Equal(t, ev.Week.FishMeals > 2, has)
	assert.Greater(t, ev.Week.FishMeals, 2, "a fish-only library goes past two")
}

// A batch counts as the meals it feeds: with batch cooking on, two fish
// batches of two meals each are already the limit (final trial: a family
// week planned three fish batches, six meals).
func TestHeuristic_FishBatchesCountTheirMeals(t *testing.T) {
	s := batchCooker()
	s.Profile.KcalTarget = f64(1800)
	s.Settings.MaxCookOccasionsPerDay = 3
	for id := int64(101); id <= 110; id++ {
		f := s.Foods[id]
		f.LastEaten = nil
		s.Foods[id] = f
	}
	for id := int64(104); id <= 106; id++ {
		withGroups(s, id, planning.FoodGroups{FishG: 120})
		f := s.Foods[id]
		f.Favorite = true
		s.Foods[id] = f
	}
	target := windowTarget(s)
	p := choose(t, planning.ChooseInput{State: s, Target: target})
	ev := planning.Evaluate(planning.Merge(s, target, p), target)
	require.NotNil(t, ev.Week)
	assert.LessOrEqual(t, ev.Week.FishMeals, planning.DGEFishMaxMeals)
}
