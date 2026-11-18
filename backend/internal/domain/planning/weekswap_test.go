package planning_test

import (
	"math"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/availability"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
)

// meatyWeek is a batch cooker with a kcal target whose mains 101-107 hold 150 g
// of meat a portion, 108 nothing, 109 fish and 110 no meat either.
func meatyWeek() planning.State {
	s := batchCooker()
	s.Profile.KcalTarget = f64(1800)
	for id := int64(101); id <= 107; id++ {
		withGroups(s, id, planning.FoodGroups{MeatG: 150})
	}
	withGroups(s, 109, planning.FoodGroups{FishG: 150})
	return s
}

func meatOf(s planning.State, p planning.Proposal) float64 {
	m := planning.Merge(s, windowTarget(s), p)
	total := 0.0
	for _, pl := range m.Plates {
		for _, c := range pl.Components {
			total += s.Foods[c.FoodID].Groups.MeatG * c.Multiplier()
		}
	}
	return total
}

func TestMeatBudgetOf(t *testing.T) {
	away := func(s *planning.State, dates ...string) {
		for _, d := range dates {
			for _, slot := range []int64{lunch, dinner} {
				setMode(s, cell(d, slot), availability.ModeNone)
			}
			setMode(s, cell(d, breakfast), availability.ModeNone)
		}
	}
	tests := []struct {
		name     string
		prep     func(*planning.State)
		ok       bool
		days     int
		limit    float64
		fixed    float64
		budget   float64
		approxOK bool
	}{
		{name: "full week", prep: func(*planning.State) {}, ok: true, days: 7, limit: 300, budget: 300},
		{name: "short week is prorated", prep: func(s *planning.State) { away(s, sat, "2026-11-08") }, ok: true, days: 5, limit: 300 * 5 / 7.0, budget: 300 * 5 / 7.0},
		{name: "too short a week is not judged", prep: func(s *planning.State) { away(s, sat, "2026-11-08", tue) }, ok: false},
		{name: "confirmed plates use the budget", prep: func(s *planning.State) {
			s.Plates = append(s.Plates, planning.Plate{ID: 900, Date: day(tue), SlotID: dinner, Eaters: 1, Confirmed: true, Components: []planning.Component{{FoodID: 101, Portions: f64(1)}}})
		}, ok: true, days: 7, limit: 300, fixed: 150, budget: 150},
		{name: "vegetarian has none", prep: func(s *planning.State) { s.Profile.DietType = profile.DietVegetarian }, ok: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := meatyWeek()
			tt.prep(&s)
			target := windowTarget(s)
			b, ok := planning.MeatBudgetOf(s, target)
			require.Equal(t, tt.ok, ok)
			if !ok {
				return
			}
			assert.Equal(t, tt.days, b.Days)
			assert.InDelta(t, tt.limit, b.LimitG, 1e-6)
			assert.InDelta(t, tt.fixed, b.FixedG, 1e-6)
			assert.InDelta(t, tt.budget, b.BudgetG, 1e-6)
		})
	}
}

// A plan the picks left over the budget is brought within it by verified swaps
// (largest meat plate first), one fish meal is placed, and every change has a
// run note.
func TestGuardWeek_SwapsMeatAndPlacesFish(t *testing.T) {
	over := meatyWeek()
	for id := int64(108); id <= 110; id++ { // the picks see only meat mains
		withGroups(over, id, planning.FoodGroups{MeatG: 150})
	}
	prop := choose(t, planning.ChooseInput{State: over, Target: windowTarget(over)})
	before := meatOf(over, prop)
	require.Greater(t, before, 300.0, "the picks overshoot")

	s := meatyWeek()
	target := windowTarget(s)
	got, notes := planning.GuardWeek(s, target, prop, nil, "")
	assert.LessOrEqual(t, meatOf(s, got), 300.0+1e-9)
	assert.NotEmpty(t, notes)
	for _, n := range notes {
		assert.True(t, strings.HasPrefix(n, "Weekly meat") || strings.HasPrefix(n, "Weekly fish"), n)
	}
	ev := planning.Evaluate(planning.Merge(s, target, got), target)
	require.NotNil(t, ev.Week)
	assert.GreaterOrEqual(t, ev.Week.FishMeals, 1, "a fish meal")
	assert.Empty(t, violations(ev.Issues))
	for _, p := range ev.Plates {
		for _, c := range p.Components {
			if c.Portions != nil {
				assert.LessOrEqual(t, *c.Portions, planning.MaxDishPortions+1e-9)
			}
		}
	}
	for _, is := range ev.Issues {
		assert.NotEqual(t, planning.CodeWeekMeatAboveDGE, is.Code)
	}
}

func TestGuardWeek_Limits(t *testing.T) {
	over := meatyWeek()
	for id := int64(108); id <= 110; id++ {
		withGroups(over, id, planning.FoodGroups{MeatG: 150})
	}
	prop := choose(t, planning.ChooseInput{State: over, Target: windowTarget(over)})

	t.Run("nothing is forced without a suitable main, and the issue says so", func(t *testing.T) {
		s := over // every main holds meat: no swap lowers it
		got, notes := planning.GuardWeek(s, windowTarget(s), prop, nil, "")
		assert.Empty(t, notes)
		assert.InDelta(t, meatOf(s, prop), meatOf(s, got), 1e-9)
		ev := planning.EvaluateWeek(planning.Merge(s, windowTarget(s), got), windowTarget(s))
		found := false
		for _, is := range ev.Issues {
			if is.Code == planning.CodeWeekMeatAboveDGE {
				found = true
				assert.True(t, is.NoFixFound)
				assert.NotEmpty(t, is.NoFixReason)
				assert.Empty(t, is.Options)
			}
		}
		assert.True(t, found)
	})
	t.Run("a vegetarian diet is untouched", func(t *testing.T) {
		s := meatyWeek()
		s.Profile.DietType = profile.DietVegetarian
		got, notes := planning.GuardWeek(s, windowTarget(s), prop, nil, "")
		assert.Empty(t, notes)
		assert.Equal(t, prop, got)
	})
	t.Run("a vegetarian instruction places no fish", func(t *testing.T) {
		s := meatyWeek()
		_, notes := planning.GuardWeek(s, windowTarget(s), prop, nil, "vegetarisch diese Woche, high protein")
		for _, n := range notes {
			assert.False(t, strings.HasPrefix(n, "Weekly fish"), n)
		}
	})
	t.Run("excluded foods are not placed", func(t *testing.T) {
		s := meatyWeek()
		skip := map[int64]bool{108: true, 109: true, 110: true}
		got, _ := planning.GuardWeek(s, windowTarget(s), prop, skip, "")
		assert.InDelta(t, meatOf(s, prop), meatOf(s, got), 1e-9)
	})
}

// The evaluation lists the meat plates by grams and verified swaps; each swap
// names the cell, lowers the meat and passes its own dry run.
func TestEvaluateWeek_OffersVerifiedSwaps(t *testing.T) {
	over := meatyWeek()
	for id := int64(108); id <= 110; id++ {
		withGroups(over, id, planning.FoodGroups{MeatG: 150})
	}
	prop := choose(t, planning.ChooseInput{State: over, Target: windowTarget(over)})
	s := meatyWeek()
	target := windowTarget(s)
	merged := planning.Merge(s, target, prop)
	ev := planning.EvaluateWeek(merged, target)
	var meat *planning.Issue
	for i := range ev.Issues {
		if ev.Issues[i].Code == planning.CodeWeekMeatAboveDGE {
			meat = &ev.Issues[i]
		}
	}
	require.NotNil(t, meat)
	assert.NotEmpty(t, meat.Params["plates"])
	require.NotEmpty(t, meat.Options)
	assert.False(t, meat.NoFixFound)
	for _, o := range meat.Options {
		assert.Equal(t, "swap", o.Action)
		assert.Positive(t, o.MeatSavedG)
		assert.NotEmpty(t, o.Why)
		assert.Equal(t, "replace", o.Op.Op)
	}
	assert.Empty(t, planning.Evaluate(merged, target).Issues[len(planning.Evaluate(merged, target).Issues)-1].Options, "plain Evaluate carries no week options")
	assert.False(t, math.IsNaN(meat.Options[0].MeatSavedG))
}

// The heuristic takes the week's meat down as far as verified swaps reach and
// plans a fish meal where the library has a fish main; a library of meat mains
// only stays as it is.
func TestHeuristic_WeekMeatBudgetAndFish(t *testing.T) {
	all := meatyWeek()
	for id := int64(108); id <= 110; id++ {
		withGroups(all, id, planning.FoodGroups{MeatG: 150})
	}
	stuck := choose(t, planning.ChooseInput{State: all, Target: windowTarget(all)})
	s := meatyWeek()
	prop := choose(t, planning.ChooseInput{State: s, Target: windowTarget(s)})
	assert.Less(t, meatOf(s, prop), meatOf(all, stuck))
	assert.True(t, fishPlanned(s, prop), "a fish meal")
}

func fishPlanned(s planning.State, p planning.Proposal) bool {
	m := planning.Merge(s, windowTarget(s), p)
	for _, pl := range m.Plates {
		for _, c := range pl.Components {
			if s.Foods[c.FoodID].Groups.FishG*c.Multiplier() >= 50 {
				return true
			}
		}
	}
	return false
}
