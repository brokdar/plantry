package planning_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/nutrition"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
)

// batchSides maps each main session key of p to the sides its plates eat,
// the cook plate's first.
func batchSides(s planning.State, p planning.Proposal) map[string][]int64 {
	out := map[string][]int64{}
	for _, ss := range p.Sessions {
		if s.Foods[ss.FoodID].Role != food.RoleMain {
			continue
		}
		for _, pl := range p.Plates {
			main, side := false, int64(0)
			for _, c := range pl.Components {
				switch {
				case c.SessionKey == ss.Key:
					main = true
				case s.Foods[c.FoodID].Role != food.RoleMain:
					side = c.FoodID
				}
			}
			if !main {
				continue
			}
			if pl.Date.Equal(ss.CookDate) && pl.SlotID == ss.CookSlotID {
				out[ss.Key] = append([]int64{side}, out[ss.Key]...)
			} else {
				out[ss.Key] = append(out[ss.Key], side)
			}
		}
	}
	return out
}

// distinct counts the different values of ids.
func distinct(ids []int64) int {
	seen := map[int64]bool{}
	for _, id := range ids {
		seen[id] = true
	}
	return len(seen)
}

// cookCells are the distinct cells p's sessions cook on.
func cookCells(p planning.Proposal) map[string]bool {
	out := map[string]bool{}
	for _, ss := range p.Sessions {
		out[fmt.Sprintf("%s/%d", ss.CookDate.Format("2006-01-02"), ss.CookSlotID)] = true
	}
	return out
}

// varyLeaves are the no-cook leaf sides a varied plate may eat: a leftover
// plate's varied side never adds a cook session, so only these (and an
// existing batch) are variants.
const varyLeaves = 3

func varyCooker(on bool) planning.State {
	s := batchCooker()
	for i := range varyLeaves {
		id := int64(300 + i)
		s.Foods[id] = planning.FoodFacts{
			ID: id, Name: fmt.Sprintf("Leaf side %d", i+1), Kind: food.KindLeaf, LeafRole: food.RoleSideVeg, DietClass: "vegan",
			NoCook: true, NutritionComplete: true, Macros: nutrition.Macros{Kcal: 150}, ServingG: 100,
		}
		s.NoCook = append(s.NoCook, id)
	}
	s.Settings.BatchVariation = on
	return s
}

// PLATE-6: with chef.batch_variation on, the plates of a batch vary their
// side (the cook plate keeps the batch's side); off, every plate of a batch
// eats the side cooked with it.
func TestHeuristic_BatchVariation(t *testing.T) {
	tests := []struct {
		name string
		on   bool
	}{
		{"variation on", true},
		{"variation off", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := varyCooker(tt.on)
			p := choose(t, planning.ChooseInput{State: s, Target: windowTarget(s)})
			batches, varied := 0, 0
			for key, sides := range batchSides(s, p) {
				if len(sides) < 2 {
					continue
				}
				batches++
				if tt.on {
					// The cook plate keeps the batch side, the leftovers vary
					// over the leaves: a plate beyond them repeats one. A
					// batch whose days are full has no variant (verified
					// like an alternative) and keeps its side.
					if distinct(sides) > 1 {
						varied++
						assert.Equal(t, min(len(sides), 1+varyLeaves), distinct(sides), "batch %s: every plate a different side: %v", key, sides)
					}
				} else {
					assert.Equal(t, 1, distinct(sides), "batch %s: one side for the batch: %v", key, sides)
				}
			}
			require.Positive(t, batches, "the fixture cooks batches")
			if tt.on {
				t.Logf("batch variation coverage: %d of %d batches", varied, batches)
				assert.Greater(t, varied*2, batches, "most batches vary")
			}
		})
	}
}

// Variation stays within nutrition: it adds no issue the unvaried week does
// not have, and a side far from the batch side's kcal is never a variant.
func TestHeuristic_BatchVariationKeepsNutrition(t *testing.T) {
	on, off := varyCooker(true), varyCooker(false)
	pOn := choose(t, planning.ChooseInput{State: on, Target: windowTarget(on)})
	pOff := choose(t, planning.ChooseInput{State: off, Target: windowTarget(off)})
	issues := map[string]bool{}
	for _, is := range planning.Evaluate(planning.Merge(off, windowTarget(off), pOff), windowTarget(off)).Issues {
		issues[is.Code] = true
	}
	for _, is := range planning.Evaluate(planning.Merge(on, windowTarget(on), pOn), windowTarget(on)).Issues {
		assert.True(t, issues[is.Code], "variation adds %s", is.Code)
	}
	assert.Equal(t, cookCells(pOff), cookCells(pOn), "variation cooks a side at an existing cook, never on a new cell")

	// Only one other side, 400 kcal heavier than the batch side: no variant.
	s := varyCooker(true)
	for id, f := range s.Foods {
		if (f.Role == food.RoleSideVeg || f.Kind == food.KindLeaf) && id != 201 {
			delete(s.Foods, id)
		}
	}
	s.Foods[205] = recipe(205, "Heavy side", food.RoleSideVeg, "vegan", 550)
	p := choose(t, planning.ChooseInput{State: s, Target: windowTarget(s)})
	for key, sides := range batchSides(s, p) {
		assert.Equal(t, 1, distinct(sides), "batch %s: a side 400 kcal off is no variant: %v", key, sides)
	}
}

// PLATE-6: the repeated main of a batch is never judged as a repeat.
func TestHeuristic_BatchMainIsNoRepeat(t *testing.T) {
	s := varyCooker(true)
	p := choose(t, planning.ChooseInput{State: s, Target: windowTarget(s)})
	for _, is := range planning.Evaluate(planning.Merge(s, windowTarget(s), p), windowTarget(s)).Issues {
		assert.NotContains(t, []string{planning.CodeVarietyRepeat, planning.CodeVarietyRepeatWeek, planning.CodeDishRepeats}, is.Code)
	}
}
