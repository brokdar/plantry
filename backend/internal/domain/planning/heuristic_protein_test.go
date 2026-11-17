package planning_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/diet"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/nutrition"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
)

const (
	skyr  = int64(901)
	tofu  = int64(902)
	candy = int64(903)
)

// proteinState is batchCooker with a weight (a 130 g protein target, 117 g
// minimum) against mains holding 30 g per portion, plus no-cook leaves: skyr
// (vegetarian, 11 g / 100 g), tofu (vegan, 8 g / 100 g) and a protein-free
// sweet.
func proteinState(dt profile.DietType) planning.State {
	s := batchCooker()
	s.Profile.KcalTarget = f64(2400)
	s.Profile.WeightKg = f64(130)
	s.Profile.DietType = dt
	leaf := func(id int64, name string, class diet.Class, kcal, protein float64) {
		s.Foods[id] = planning.FoodFacts{
			ID: id, Name: name, Kind: food.KindLeaf, DietClass: class, NoCook: true, NutritionComplete: true,
			Macros: nutrition.Macros{Kcal: kcal, Protein: protein},
		}
		s.NoCook = append(s.NoCook, id)
	}
	leaf(skyr, "Skyr", diet.Vegetarian, 60, 11)
	leaf(tofu, "Tofu", diet.Vegan, 120, 8)
	leaf(candy, "Candy", diet.Vegan, 400, 0)
	return s
}

func dayProtein(s planning.State, p planning.Proposal) map[string]planning.DayEval {
	tgt := windowTarget(s)
	out := map[string]planning.DayEval{}
	for _, d := range planning.Evaluate(planning.Merge(s, tgt, p), tgt).Days {
		out[d.Date] = d
	}
	return out
}

func foodsAdded(p planning.Proposal, id int64) int {
	n := 0
	for _, pl := range p.Plates {
		for _, c := range pl.Components {
			if c.FoodID == id {
				n++
			}
		}
	}
	return n
}

func TestHeuristic_ProteinGapClosing(t *testing.T) {
	t.Run("lifts protein with the most protein-rich food, within the band and the hard rules", func(t *testing.T) {
		s := proteinState(profile.DietOmnivore)
		tgt := windowTarget(s)
		p := choose(t, planning.ChooseInput{State: s, Target: tgt})

		without := proteinState(profile.DietOmnivore)
		without.NoCook, without.Profile.WeightKg = nil, nil
		base := dayProtein(without, choose(t, planning.ChooseInput{State: without, Target: windowTarget(without)}))

		assert.Positive(t, foodsAdded(p, skyr), "skyr closes the gap")
		assert.Zero(t, foodsAdded(p, candy), "a protein-free food never helps")

		m := planning.Merge(s, tgt, p)
		assert.Empty(t, violations(planning.Check(m, tgt)))
		after := dayProtein(s, p)
		lifted := 0
		for date, d := range after {
			assert.LessOrEqual(t, d.Kcal, *d.TargetKcal*1.1+1e-6, "%s stays under the band top", date)
			if d.ProteinG > base[date].ProteinG+1e-6 {
				lifted++
			}
		}
		assert.Positive(t, lifted)
		for _, pl := range p.Plates {
			n := 0
			for _, c := range pl.Components {
				if c.FoodID == skyr || c.FoodID == tofu {
					n++
				}
			}
			assert.LessOrEqual(t, n, 2, "at most 2 adds per day on one plate")
		}
	})

	t.Run("respects the diet type", func(t *testing.T) {
		s := proteinState(profile.DietVegan)
		for id, f := range s.Foods {
			if f.Kind == food.KindComposed {
				f.DietClass = diet.Vegan
				s.Foods[id] = f
			}
		}
		p := choose(t, planning.ChooseInput{State: s, Target: windowTarget(s)})
		assert.Zero(t, foodsAdded(p, skyr), "skyr is vegetarian")
		assert.Positive(t, foodsAdded(p, tofu))
		tgt := windowTarget(s)
		assert.Empty(t, violations(planning.Check(planning.Merge(s, tgt, p), tgt)))
	})

	t.Run("never breaks the kcal day band", func(t *testing.T) {
		s := proteinState(profile.DietOmnivore)
		s.Profile.KcalTarget = f64(2000) // days already at the band top
		tgt := windowTarget(s)
		p := choose(t, planning.ChooseInput{State: s, Target: tgt})
		before := proteinState(profile.DietOmnivore)
		before.Profile.KcalTarget = f64(2000)
		before.Profile.WeightKg = nil
		base := dayProtein(before, choose(t, planning.ChooseInput{State: before, Target: windowTarget(before)}))
		for date, d := range dayProtein(s, p) {
			assert.LessOrEqual(t, d.Kcal, max(*d.TargetKcal*1.1, base[date].Kcal)+1e-6, date)
		}
	})

	t.Run("unreachable: no protein food to add, the checker reports S3", func(t *testing.T) {
		s := proteinState(profile.DietOmnivore)
		s.NoCook = []int64{candy} // the kcal pass may still use it; it holds no protein
		tgt := windowTarget(s)
		p := choose(t, planning.ChooseInput{State: s, Target: tgt})
		assert.Contains(t, p.Notes, "Protein below target")
		ev := planning.Evaluate(planning.Merge(s, tgt, p), tgt)
		found := false
		for _, is := range ev.Issues {
			found = found || is.Code == planning.CodeProteinLow
		}
		require.True(t, found)
	})

	t.Run("week with room: every day reaches its target, no weekly protein issue, one add per plate", func(t *testing.T) {
		s := proteinState(profile.DietOmnivore)
		s.Profile.KcalTarget = f64(2400)
		tgt := windowTarget(s)
		p := choose(t, planning.ChooseInput{State: s, Target: tgt})
		ev := planning.Evaluate(planning.Merge(s, tgt, p), tgt)
		for _, is := range ev.Issues {
			assert.NotEqual(t, planning.CodeWeekProteinLow, is.Code)
			assert.NotEqual(t, planning.CodeProteinLow, is.Code)
		}
		for _, d := range ev.Days {
			assert.GreaterOrEqual(t, d.ProteinG, d.ProteinTargetG-1e-6, "%s closes toward 100 %%", d.Date)
		}
		perDay := map[string]int{}
		for _, pl := range p.Plates {
			n := 0
			for _, c := range pl.Components {
				if c.FoodID == skyr || c.FoodID == tofu {
					n++
				}
			}
			assert.LessOrEqual(t, n, 1, "at most one protein add per plate")
			perDay[pl.Date.Format("2006-01-02")] += n
		}
		for date, n := range perDay {
			assert.LessOrEqual(t, n, 2, date)
		}
	})

	t.Run("seven at-target days: the adds keep the week inside its band", func(t *testing.T) {
		s := proteinState(profile.DietOmnivore)
		s.Profile.KcalTarget = f64(2000) // the week starts at 98 %; day adds alone could take it past 105 %
		tgt := windowTarget(s)
		p := choose(t, planning.ChooseInput{State: s, Target: tgt})
		ev := planning.Evaluate(planning.Merge(s, tgt, p), tgt)
		assert.LessOrEqual(t, ev.Week.Pct, 105+1e-6)
		for _, is := range ev.Issues {
			assert.NotEqual(t, planning.CodeWeekKcalAboveBand, is.Code)
		}
		assert.Positive(t, foodsAdded(p, skyr)+foodsAdded(p, tofu), "there was room for some adds")
	})

	t.Run("a week already above its band takes no protein adds", func(t *testing.T) {
		s := proteinState(profile.DietOmnivore)
		s.Profile.KcalTarget = f64(1800) // the week starts at 109 %
		p := choose(t, planning.ChooseInput{State: s, Target: windowTarget(s)})
		assert.Zero(t, foodsAdded(p, skyr)+foodsAdded(p, tofu))
	})

	t.Run("without a weight the heuristic does not chase protein", func(t *testing.T) {
		s := proteinState(profile.DietOmnivore)
		s.Profile.WeightKg = nil
		s.Profile.KcalTarget = f64(1800) // days above the band: the kcal pass adds nothing either
		p := choose(t, planning.ChooseInput{State: s, Target: windowTarget(s)})
		assert.Zero(t, foodsAdded(p, skyr))
	})
}
