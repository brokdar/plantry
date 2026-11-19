package planning_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/airun"
	"github.com/jaltszeimer/plantry/backend/internal/domain/diet"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/nutrition"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
)

// A model that keeps a dish at 2.5 portions is sent back with the value, the
// cap and the fix.
func TestLLMChooser_PortionsOverCapAreSentBack(t *testing.T) {
	in := run4Input()
	doubleKcal(&in.State)
	bad := validWeek(t, in)
	bad.Plates[0]["components"].([]map[string]any)[0]["portions"] = 2.5

	prop, reqs := chooseWith(t, in, submit(t, "tu_1", bad), submit(t, "tu_2", validWeek(t, in)))

	require.Len(t, reqs, 2)
	fb := feedbackOf(t, reqs[1])
	assert.Contains(t, fb, "2.5 portions per person, the most allowed is 1.5")
	assert.Contains(t, fb, "close the energy gap with a side or a no-cook add-on")
	assert.Empty(t, violationsOfProposal(in, prop))
}

// overChooser lets the heuristic plan and then, like a model that ignored
// every repair, raises one plate's dish to 2.5 portions (and its session
// with it, so H3 holds).
type overChooser struct{}

func (overChooser) Choose(ctx context.Context, in planning.ChooseInput) (planning.Proposal, error) {
	p, err := planning.Heuristic{}.Choose(ctx, in)
	if err != nil {
		return p, err
	}
	pl := &p.Plates[0]
	c := &pl.Components[0]
	for i := range p.Sessions {
		if p.Sessions[i].Key == c.SessionKey {
			p.Sessions[i].Portions += 1.5 * func() float64 {
				if pl.Eaters > 0 {
					return pl.Eaters
				}
				return in.State.Profile.HouseholdPortions
			}()
		}
	}
	c.Portions = 2.5
	return p, nil
}

// Whatever the chooser returns, the saved plan holds no dish above the cap,
// and the run notes say what was lowered.
func TestService_SavedPlanNeverHoldsPortionsOverTheCap(t *testing.T) {
	e := newSvcEnv(t)
	e.deps.Choosers = map[airun.Chooser]planning.Chooser{airun.ChooserHeuristic: overChooser{}}
	e.svc = planning.NewService(e.deps)

	id := e.generate(e.req())
	end := e.ok(id)

	assert.Contains(t, end.Notes, "portions lowered from 2.5 to 1.5")
	seen := 0
	for _, p := range e.window() {
		for _, c := range p.Components {
			if c.Portions != nil {
				seen++
				assert.LessOrEqual(t, *c.Portions, planning.MaxDishPortions)
			}
		}
	}
	assert.NotZero(t, seen)
}

// The heuristic's no-cook add of a composed dish (a salad) used to reach 2
// portions on a day far under its target; it stops at the cap and leaves the
// rest to the other adds and the notes.
func TestHeuristic_ComposedAddStopsAtTheCap(t *testing.T) {
	s := fillerState(3500)
	const salad = int64(907)
	s.Foods[salad] = planning.FoodFacts{
		ID: salad, Name: "Carrot salad", Kind: food.KindComposed, Role: food.RoleSideVeg, DietClass: diet.Vegan, NoCook: true,
		NutritionComplete: true, Macros: nutrition.Macros{Kcal: 250, Protein: 2},
	}
	s.NoCook = []int64{salad}
	p := choose(t, planning.ChooseInput{State: s, Target: windowTarget(s)})

	added := 0
	for _, pl := range p.Plates {
		for _, c := range pl.Components {
			assert.LessOrEqual(t, c.Portions, planning.MaxDishPortions, "food %d on %s", c.FoodID, pl.Date.Format("2006-01-02"))
			if c.FoodID == salad {
				added++
			}
		}
	}
	assert.NotZero(t, added, "the fixture must make the heuristic add the salad")
}

// The computed options never propose more than the cap: a composed no-cook add
// (a salad) is offered at 1.5 portions, not 2, and a dish already at 1.5 is not
// offered a raise.
func TestOptions_NeverProposeAboveTheCap(t *testing.T) {
	s := softState(2600) // the day holds 2100 kcal with one dish at 1.5, under its band
	s.Profile.MeatDaysPerWeekMax = intp(7)
	const salad = int64(60)
	s.Foods[salad] = planning.FoodFacts{
		ID: salad, Name: "Carrot salad", Kind: food.KindComposed, Role: food.RoleSideVeg, DietClass: diet.Vegan, NoCook: true,
		NutritionComplete: true, Macros: nutrition.Macros{Kcal: 60, Protein: 1},
	}
	s.NoCook = []int64{salad}
	s.Plates[0].Components[0].Portions = f64(1.5)
	s.Sessions[0].Portions = 1.5
	ev := planning.Evaluate(s, windowTarget(s))
	s2 := softOf(ev, "S2")
	require.Len(t, s2, 1)
	adds := 0
	for _, o := range s2[0].Options {
		if o.Portions != nil {
			assert.LessOrEqual(t, *o.Portions, planning.MaxDishPortions, "%s %s", o.Action, o.Name)
		}
		if o.Action == "raise" && o.Portions != nil {
			assert.Less(t, *o.From, planning.MaxDishPortions, "a dish at the cap gets no raise: %s", o.Name)
		}
		if o.Action == "add" && o.FoodID == salad {
			adds++
			assert.Equal(t, planning.MaxDishPortions, *o.Portions)
		}
	}
	assert.Positive(t, adds)
}
