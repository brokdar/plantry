//go:build spectest

package spectest

import (
	"context"
	"fmt"
	"sync"
	"testing"

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
					type meal struct {
						kcal   float64
						out    bool
						lo, hi float64
						word   string
					}
					perDay := map[string][]meal{}
					other := map[string]float64{}
					for _, d := range ev.Days {
						other[d.Date] = d.Kcal
					}
					for _, p := range ev.Plates {
						w := slotWord(s, p.SlotID)
						if w != "lunch" && w != "dinner" {
							continue
						}
						b := bandOfSlot(s, p.SlotID)
						// the meal at its expected share (SB2: the planner's budget), inside the library's reach
						exp := min(max(shares(s)[p.SlotID]*T, 0.5*mainMin), 1.5*mainMax)
						m := meal{kcal: p.Kcal, word: w, out: p.Kcal < b.Min/100*T || p.Kcal > b.Max/100*T, lo: exp, hi: exp}
						other[p.Date] -= p.Kcal
						perDay[p.Date] = append(perDay[p.Date], m)
						if m.out {
							c.OutBand++ // the band oracle of SB1, not the product's S10
						}
					}
					for _, d := range ev.Days {
						ms := perDay[d.Date]
						var lo, hi float64
						for _, m := range ms {
							lo, hi = lo+m.lo, hi+m.hi
						}
						// feasible: with every lunch and dinner at its expected share the day lands in its day band
						feasible := len(ms) > 0 && other[d.Date]+lo <= d.BandHigh && other[d.Date]+hi >= d.BandLow
						if !feasible {
							c.InfeasDays++
							continue
						}
						for _, m := range ms {
							c.FeasMeals++
							if m.out {
								c.FeasOutBand++
							}
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
				id, err := e.planner.Regenerate(context.Background(), planning.Request{
					Start: chatWeek, Today: today,
					Scope: planning.Scope{Kind: planning.ScopeDay, Date: date}, Chooser: airun.ChooserHeuristic,
				})
				require.NoError(t, err)
				_, err = e.planner.Run(context.Background(), id)
				require.NoError(t, err)
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

// PL7: at least 80 % of the planned lunches and dinners lie inside their slot
// band, counted on the days the PL6 pre-check marks feasible: with the day's other
// meals as planned (a routine breakfast is fixed) and every lunch and dinner at its
// expected share (SB2), the day lands in its day band. Where a fixed meal is far
// below its own expected share, the others must exceed theirs and the day is not
// counted.
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
