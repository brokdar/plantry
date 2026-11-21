//go:build spectest

package spectest

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"sync"
	"testing"

	"github.com/jaltszeimer/plantry/backend/internal/domain/diet"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/airun"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/domain/window"
)

type planCase struct {
	HH      string
	Start   string
	Status  string
	Viol    []string
	Days    int
	InBand  int
	Err     error
	Chooser string
	Notes   string
	Meals   int
	OutBand int
	// the PL6 pre-check: lunches and dinners on days where an in-band lunch and
	// dinner can bring the day into its band, given the plan's other meals
	FeasMeals   int
	FeasOutBand int
	InfeasDays  int
}

var (
	planOnce sync.Once
	planRuns []planCase
)

func generate(t *testing.T, e *env, start window.Window, chooser airun.Chooser) *planning.RunView {
	t.Helper()
	id, err := e.planner.Generate(context.Background(), planning.Request{Start: start.Start, Today: today, Chooser: chooser})
	require.NoError(t, err)
	v, err := e.planner.Run(context.Background(), id)
	require.NoError(t, err)
	return v
}

// allPlans generates every household's weeks with the heuristic (no model:
// the constraint extraction of a free-text wish is faked by an empty wish).
func allPlans(t *testing.T) []planCase {
	planOnce.Do(func() {
		var wg sync.WaitGroup
		out := make([]planCase, len(households)*len(plannerWks))
		sem := make(chan struct{}, 8)
		for hi, h := range households {
			for wi, wk := range plannerWks {
				wg.Add(1)
				go func() {
					defer wg.Done()
					sem <- struct{}{}
					defer func() { <-sem }()
					e := newEnv(t, "gen-"+h, wk)
					v := generate(t, e, e.win, airun.ChooserHeuristic)
					c := planCase{HH: h, Start: wk.Format("01-02"), Status: string(v.Status), Chooser: string(v.Chooser), Notes: v.Notes}
					for k := range e.violations() {
						c.Viol = append(c.Viol, k)
					}
					s := e.st()
					ev := planning.Evaluate(s, planning.WindowTarget(s))
					for _, d := range ev.Days {
						c.Days++
						if d.TargetKcal != nil && d.Kcal >= d.BandLow-0.5 && d.Kcal <= d.BandHigh+0.5 {
							c.InBand++
						}
					}
					for _, p := range ev.Plates {
						if w := slotWord(s, p.SlotID); w == "lunch" || w == "dinner" {
							c.Meals++
						}
					}
					T := fullTarget(s)
					mainMin, mainMax := 1e9, 0.0
					for _, f := range s.Foods {
						if f.Kind == food.KindComposed && f.Role == food.RoleMain && f.NutritionComplete && f.Macros.Kcal > 0 {
							mainMin, mainMax = min(mainMin, f.Macros.Kcal), max(mainMax, f.Macros.Kcal)
						}
					}
					for _, d := range ev.Days {
						exp, shift := slotExpected(s, ev, d.Date)
						var meals []planning.PlateEval
						for _, p := range ev.Plates {
							if w := slotWord(s, p.SlotID); p.Date == d.Date && (w == "lunch" || w == "dinner") && p.PlateID > 0 {
								meals = append(meals, p)
							}
						}
						// PL6 (v1.2): a day is infeasible only when a shifted expected kcal lies outside the
						// library's reach (0.5-1.5 portions of a main); a fixed routine meal no longer matters
						feasible := len(meals) > 0
						for _, p := range meals {
							if e := exp[p.SlotID]; e < 0.5*mainMin || e > 1.5*mainMax {
								feasible = false
							}
						}
						c.Meals += len(meals)
						for _, p := range meals {
							b := bandOfSlot(s, p.SlotID)
							out := p.Kcal < b.Min/100*T+shift[p.SlotID] || p.Kcal > b.Max/100*T+shift[p.SlotID]
							if out {
								c.OutBand++ // the band oracle of SB1/SB5, not the product's S10
							}
							if feasible {
								c.FeasMeals++
								if out {
									c.FeasOutBand++
								}
							}
						}
						if !feasible {
							c.InfeasDays++
						}
					}
					out[hi*len(plannerWks)+wi] = c
				}()
			}
		}
		wg.Wait()
		planRuns = out
	})
	return planRuns
}

// PL1: a generated week is saved with 0 hard violations.
func TestPL1_PlannerMatrixNoHardViolations(t *testing.T) {
	for _, c := range allPlans(t) {
		t.Run(c.HH+"/"+c.Start, func(t *testing.T) {
			assert.Empty(t, c.Viol, "hard violations saved (status %s)", c.Status)
		})
	}
}

// PL3: at least 6 of 7 days inside the day band (gated on feasible weeks; the
// feasibility pre-check PL6 is not implemented, so every week is counted).
func TestPL3_PlannerMatrixDaysInBand(t *testing.T) {
	for _, c := range allPlans(t) {
		t.Run(c.HH+"/"+c.Start, func(t *testing.T) {
			assert.GreaterOrEqual(t, c.InBand, 6, "%d of %d days in band", c.InBand, c.Days)
		})
	}
}

// stubLLM is a chooser registered as the LLM chooser, ready, that records
// whether it was asked to place meals.
type stubLLM struct {
	mu    sync.Mutex
	calls int
	empty bool
}

func (s *stubLLM) Ready(context.Context) error { return nil }

func (s *stubLLM) Choose(ctx context.Context, in planning.ChooseInput) (planning.Proposal, error) {
	s.mu.Lock()
	s.calls++
	s.mu.Unlock()
	if s.empty {
		return planning.Proposal{}, nil
	}
	return planning.Heuristic{}.Choose(ctx, in)
}

// PL2: plans come from the heuristic even when a provider is ready; the LLM is
// only an extraction step and never places meals.
func TestPL2_HeuristicPlansEvenWhenProviderReady(t *testing.T) {
	for _, h := range households {
		t.Run(h, func(t *testing.T) {
			e := newEnv(t, "gen-"+h, chatWeek)
			stub := &stubLLM{}
			e.deps.Choosers[airun.ChooserLLM] = stub
			e.planner = planning.NewService(e.deps)
			v := generate(t, e, e.win, "")
			assert.Equal(t, string(airun.ChooserHeuristic), string(v.Chooser), "the plan was made by %s", v.Chooser)
			assert.Zero(t, stub.calls, "the LLM chooser was asked to place meals")
		})
	}
}

// PL1b: a run that ends with an open hard issue is not saved as the result:
// the heuristic plan is saved instead and the notes say so.
func TestPL1b_NeverSaveWithOpenHardIssue(t *testing.T) {
	for _, h := range households {
		t.Run(h, func(t *testing.T) {
			e := newEnv(t, "gen-"+h, chatWeek)
			e.deps.Choosers[airun.ChooserLLM] = &stubLLM{empty: true} // leaves every cell empty: H1 open
			e.planner = planning.NewService(e.deps)
			v := generate(t, e, e.win, airun.ChooserLLM)
			viol := e.violations()
			assert.Empty(t, viol, "saved with open hard issues %v (status %s, notes %q)", viol, v.Status, v.Notes)
			assert.Regexp(t, `(?i)heuristic|fallback|instead`, v.Notes, "the notes do not say the plan was replaced")
		})
	}
}

// KR7: a replanned day does not repeat the main of a neighbouring day or the
// other meal of the same day.
func TestKR7_ReplanDoesNotRepeatNeighbours(t *testing.T) {
	for _, h := range households {
		for _, dayIdx := range []int{2, 4} {
			date := chatWeek.AddDate(0, 0, dayIdx)
			t.Run(fmt.Sprintf("%s/%s", h, date.Format("01-02")), func(t *testing.T) {
				e := newEnv(t, "chat-"+h, chatWeek)
				before := e.st()
				id, err := e.planner.Regenerate(context.Background(), planning.Request{
					Start: chatWeek, Today: today,
					Scope: planning.Scope{Kind: planning.ScopeDay, Date: date}, Chooser: airun.ChooserHeuristic,
				})
				require.NoError(t, err)
				v, err := e.planner.Run(context.Background(), id)
				require.NoError(t, err)
				if v.ErrorKey == "error.plan.day_same_dish" && snapshot(e.st()) == snapshot(before) {
					// "nothing saved, told" is KR7's last step. It passes only when the test
					// itself shows that no fallback exists: no cold plate can be made from
					// the library (bread, hummus or cheese, a raw vegetable, fitting the diet).
					ok, why := coldPlatePossible(before)
					t.Logf("KR7 path: nothing saved; cold plate possible=%v (%s)", ok, why)
					assert.False(t, ok, "nothing was saved although a cold plate can be made (%s): a fallback existed", why)
					return
				}
				t.Logf("KR7 path: replan saved")
				s := e.st()
				mains := map[string]map[int64]bool{} // date -> main foods
				perMeal := map[string][]int64{}
				sessOf := map[string][]int64{}
				for _, p := range s.Plates {
					for _, c := range p.Components {
						if f := s.Foods[c.FoodID]; f.Kind == food.KindComposed && f.Role == food.RoleMain {
							d := p.Date.Format("2006-01-02")
							if mains[d] == nil {
								mains[d] = map[int64]bool{}
							}
							mains[d][c.FoodID] = true
							perMeal[d] = append(perMeal[d], c.FoodID)
							sessOf[d] = append(sessOf[d], c.SessionID)
						}
					}
				}
				d := date.Format("2006-01-02")
				for _, nb := range []string{date.AddDate(0, 0, -1).Format("2006-01-02"), date.AddDate(0, 0, 1).Format("2006-01-02")} {
					for f := range mains[d] {
						assert.False(t, mains[nb][f], "%s repeats the main %q of %s", d, s.Foods[f].Name, nb)
					}
				}
				// the same dish at both meals is one cooking eaten twice (KR7 step 3, allowed
				// where H8 is not triggered; the ladder order itself has no oracle) or two cookings (a repeat)
				sess := map[int64]int64{}
				for i, f := range perMeal[d] {
					if prev, dup := sess[f]; dup && prev != sessOf[d][i] {
						assert.Fail(t, "cooked twice", "%s has %q cooked at both meals", d, s.Foods[f].Name)
					}
					sess[f] = sessOf[d][i]
				}
			})
		}
	}
}

// PL7 (v1.2): at least 80 % of the planned lunches and dinners lie inside their
// shifted slot band (SB5: a routine meal counts with its real kcal and the others
// absorb its gap). A day is left out only when a shifted expected kcal is beyond
// the library's reach.
func TestPL7_PlannerMealsInsideSlotBands(t *testing.T) {
	for _, c := range allPlans(t) {
		t.Run(c.HH+"/"+c.Start, func(t *testing.T) {
			if c.FeasMeals == 0 {
				t.Skipf("no feasible day (%d infeasible days)", c.InfeasDays)
			}
			assert.GreaterOrEqual(t, float64(c.FeasMeals-c.FeasOutBand)/float64(c.FeasMeals), 0.8,
				"%d of %d lunches and dinners on feasible days outside their band (%d infeasible days)", c.FeasOutBand, c.FeasMeals, c.InfeasDays)
		})
	}
}

var (
	breadWords  = regexp.MustCompile(`(?i)bread|brot|ciabatta|baguette|toast|br[oö]tchen|roll`)
	spreadWords = regexp.MustCompile(`(?i)hummus|cheese|k[aä]se|mozzarella|feta|parmesan|gouda|cheddar|halloumi`)
)

// coldPlatePossible is the spec's KR7 cold plate computed from the library: a
// no-cook bread, hummus or a cheese, and a raw vegetable, all fitting the diet.
func coldPlatePossible(s planning.State) (bool, string) {
	var bread, spread, veg string
	vegKcal := 1e9
	ids := make([]int64, 0, len(s.Foods))
	for id := range s.Foods {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	for _, id := range ids {
		f := s.Foods[id]
		if f.Kind != food.KindLeaf || !f.NoCook || !f.NutritionComplete || f.Macros.Kcal <= 0 || f.Sweet {
			continue
		}
		if planning.DietRestricts(s.Profile.DietType) && !(diet.Valid(f.DietClass) && diet.Compatible(f.DietClass, diet.Class(s.Profile.DietType))) {
			continue
		}
		switch {
		case f.LeafRole == food.RoleSideStarch && breadWords.MatchString(f.Name):
			bread = f.Name
		case spreadWords.MatchString(f.Name):
			spread = f.Name
		case f.LeafRole == food.RoleSideVeg && f.Macros.Kcal <= 60 && f.Macros.Kcal < vegKcal: // a raw vegetable (the leanest, so not a fruit)
			veg, vegKcal = f.Name, f.Macros.Kcal
		}
	}
	return bread != "" && spread != "" && veg != "", fmt.Sprintf("bread %q, spread %q, vegetable %q", bread, spread, veg)
}
