package planning_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaltszeimer/plantry/backend/internal/domain/diet"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/nutrition"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
)

const (
	bread = int64(904)
	nuts  = int64(905)
	pear  = int64(906)
)

// fillerState is proteinState (its candy is a sweet) with more ordinary no-cook
// foods, so only the caps and the sweets rule bound the adds.
func fillerState(kcal float64) planning.State {
	s := proteinState(profile.DietOmnivore)
	s.Profile.KcalTarget = f64(kcal)
	for _, f := range []struct {
		id            int64
		name          string
		kcal, protein float64
	}{{bread, "Bread", 250, 8}, {nuts, "Nuts", 600, 15}, {pear, "Pear", 50, 0}} {
		s.Foods[f.id] = planning.FoodFacts{
			ID: f.id, Name: f.name, Kind: food.KindLeaf, DietClass: diet.Vegan, NoCook: true, NutritionComplete: true,
			Macros: nutrition.Macros{Kcal: f.kcal, Protein: f.protein},
		}
		s.NoCook = append(s.NoCook, f.id)
	}
	return s
}

// Per day, over every pass: at most 2 plain fillers and 1 protein add (3
// together; a protein-rich food may serve as a filler), each at most 200 g and 20 % of the day target, on separate
// plates before any stacks; never a sweet. Past the cap the day stays short
// and the notes say so.
func TestHeuristic_FillerCapsPerDay(t *testing.T) {
	for _, kcal := range []float64{3000, 5000} {
		s := fillerState(kcal)
		tgt := windowTarget(s)
		p := choose(t, planning.ChooseInput{State: s, Target: tgt})

		perDay := map[string]int{}
		plates := map[string]map[int64]int{}
		for _, pl := range p.Plates {
			date := pl.Date.Format("2006-01-02")
			for _, c := range pl.Components {
				f := s.Foods[c.FoodID]
				if !f.NoCook || c.SessionKey != "" || f.Kind != food.KindLeaf {
					continue
				}
				perDay[date]++
				if plates[date] == nil {
					plates[date] = map[int64]int{}
				}
				plates[date][pl.SlotID]++
				assert.NotEqual(t, candy, c.FoodID, "%.0f %s: a sweet is never filler", kcal, date)
				assert.LessOrEqual(t, c.Grams, 200.0+1e-9, "%.0f %s: %s", kcal, date, f.Name)
				assert.LessOrEqual(t, f.Macros.Kcal*c.Grams/100, kcal*0.2+1e-6, "%.0f %s: %s", kcal, date, f.Name)
			}
		}
		for date, n := range perDay {
			assert.LessOrEqual(t, n, 3, "%.0f %s: adds", kcal, date)
			if n >= 2 {
				assert.GreaterOrEqual(t, len(plates[date]), 2, "%.0f %s: spread over plates", kcal, date)
			}
		}
		if kcal == 5000 {
			assert.Contains(t, p.Notes, "Below target")
		}
	}
}

// The day-level note skips days without nutrition data (the planner does too)
// and light days (their target is meant to be lower).
func TestHeuristic_KcalNoteSkipsMissingNutritionAndLightDays(t *testing.T) {
	s := fillerState(5000)
	s.Light = []planning.LightIntent{{Date: mon, Scope: "day"}}
	p := choose(t, planning.ChooseInput{State: s, Target: windowTarget(s)})
	assert.Contains(t, p.Notes, "Below target")
	assert.NotContains(t, p.Notes, "Mon −", "a light day is meant to be lower")

	m := fillerState(5000)
	for id, f := range m.Foods {
		if f.Kind == food.KindComposed {
			f.NutritionComplete = false
			m.Foods[id] = f
		}
	}
	p = choose(t, planning.ChooseInput{State: m, Target: windowTarget(m)})
	assert.NotContains(t, p.Notes, "Below target", "days without nutrition data have no gap to report")
}
