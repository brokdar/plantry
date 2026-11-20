package planning_test

import (
	"context"
	"encoding/json"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/airun"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
)

type fakeExtractor struct {
	c        planning.Constraints
	err      error
	readyErr error
	calls    int
}

func (f *fakeExtractor) Ready(context.Context) error { return f.readyErr }
func (f *fakeExtractor) Extract(context.Context, planning.State, string) (planning.Constraints, error) {
	f.calls++
	return f.c, f.err
}

// modelChooser is a model that places meals: with an Extractor it must never
// run (PL2).
type modelChooser struct{ t *testing.T }

func (f modelChooser) Choose(context.Context, planning.ChooseInput) (planning.Proposal, error) {
	f.t.Fatal("a model placed meals: the heuristic plans (PL2)")
	return planning.Proposal{}, nil
}

// PL2: with an Extractor the "llm" chooser reads the wish and the heuristic
// plans: the constraints land in the saved plan (light stored, guests), and
// the model chooser is never called.
func TestService_WishIsExtractedAndTheHeuristicPlans(t *testing.T) {
	e := newSvcEnv(t)
	fx := &fakeExtractor{c: planning.Constraints{
		Light:  []planning.LightIntent{{Date: wed, Scope: planning.LightScopeMeal, Slot: "dinner"}},
		Eaters: map[string]float64{sun: 3},
	}}
	e.deps.Extractor = fx
	e.deps.Choosers = map[airun.Chooser]planning.Chooser{airun.ChooserHeuristic: planning.Heuristic{}, airun.ChooserLLM: modelChooser{t}}
	e.svc = planning.NewService(e.deps)
	req := e.req()
	req.Chooser, req.Instruction = airun.ChooserLLM, "Wednesday dinner light, guests on Sunday"

	id := e.generate(req)

	end := e.runs.end(id)
	assert.Equal(t, 1, fx.calls)
	assert.NotEqual(t, airun.StatusError, end.Status, end.ErrorKey)
	assert.Equal(t, "x1+"+planning.HeuristicVersion, e.runs.start(id).PromptVersion)
	for k, p := range e.window() {
		if p.DateString() == sun {
			assert.InDelta(t, 3, p.Eaters, 1e-9, k)
		}
	}
	var scope, scopeLight string
	require.NoError(t, e.db.QueryRow(`SELECT date||scope, slot FROM plan_light WHERE date = ?`, wed).Scan(&scope, &scopeLight))
	assert.Equal(t, "dinner", scopeLight, "the extracted light request is stored with the plan")
}

// A wish that cannot be read does not fail the run and is not ignored
// silently: the note says so.
func TestService_UnreadableWishIsSaidInTheNotes(t *testing.T) {
	e := newSvcEnv(t)
	e.deps.Extractor = &fakeExtractor{err: domain.ErrAIProviderFailed}
	e.deps.Choosers = map[airun.Chooser]planning.Chooser{airun.ChooserHeuristic: planning.Heuristic{}}
	e.svc = planning.NewService(e.deps)
	req := e.req()
	req.Chooser, req.Instruction = airun.ChooserLLM, "whatever"

	end := e.runs.end(e.generate(req))

	assert.Equal(t, airun.StatusPartial, end.Status, "an unread wish is not a silent success")
	assert.Contains(t, end.Notes, "could not be read")
	assert.NotContains(t, end.Notes, domain.ErrAIProviderFailed.Error(), "the reason stays in the log")
}

// The note of an unread wish is in the profile's language.
func TestService_UnreadableWishNoteFollowsTheLocale(t *testing.T) {
	e := newSvcEnv(t)
	e.exec(`UPDATE user_profile SET locale = 'de' WHERE id = 1`)
	e.deps.Extractor = &fakeExtractor{err: domain.ErrAIProviderFailed}
	e.deps.Choosers = map[airun.Chooser]planning.Chooser{airun.ChooserHeuristic: planning.Heuristic{}}
	e.svc = planning.NewService(e.deps)
	req := e.req()
	req.Chooser, req.Instruction = airun.ChooserLLM, "egal"

	end := e.runs.end(e.generate(req))

	assert.Equal(t, airun.StatusPartial, end.Status)
	assert.Contains(t, end.Notes, "Dein Wunsch konnte nicht gelesen werden")
}

// The cooking-day limit of the wish: the heuristic keeps to it while every meal
// can still be filled and says so when it needed another day.
func TestService_CookDayLimitIsKeptOrSaid(t *testing.T) {
	cookDays := func(e *svcEnv) int {
		days := map[string]bool{}
		for _, p := range e.window() {
			for _, c := range p.Components {
				v, err := e.plates.Session(e.ctx, *c.CookSessionID)
				require.NoError(t, err)
				if v.CookDate.Equal(p.Date) && v.CookSlotID == p.SlotID && p.SlotID != e.breakfast {
					days[p.DateString()] = true
				}
			}
		}
		return len(days)
	}
	run := func(limit int) (*svcEnv, planning.RunEnd) {
		e := newSvcEnv(t)
		e.deps.Extractor = &fakeExtractor{c: planning.Constraints{MaxCookDays: limit}}
		e.deps.Choosers = map[airun.Chooser]planning.Chooser{airun.ChooserHeuristic: planning.Heuristic{}}
		e.svc = planning.NewService(e.deps)
		req := e.req()
		req.Chooser, req.Instruction = airun.ChooserLLM, "I can only cook on a few days"
		return e, e.runs.end(e.generate(req))
	}
	free, _ := run(0)
	limit := cookDays(free) - 1
	require.Positive(t, limit)
	e, end := run(limit)
	assert.NotEqual(t, airun.StatusError, end.Status, end.ErrorKey)
	assert.LessOrEqual(t, cookDays(e), cookDays(free), "never more cooking days than without the limit")
	if cookDays(e) > limit {
		assert.Contains(t, end.Notes, "cooking day beyond your limit")
	}
}

// ParseConstraints never fails: what the window, the slots or the library
// reject is left out and named.
func TestParseConstraints_DropsWhatCannotBeUsed(t *testing.T) {
	s := baseState()
	manyMains(&s, 3)
	f := s.Foods[100]
	f.Name = "Lentil curry"
	s.Foods[100] = f
	s.SlotNames = map[int64]string{dinner: "slot.dinner"}
	name := f.Name
	raw := json.RawMessage(`{"light":[{"date":"2031-01-01","scope":"day"},{"date":"` + sat + `","scope":"meal","slot":"dinner"}],
		"max_cook_days":99,"avoid_foods":["` + name + `","No such dish"],"eaters":[{"date":"` + sun + `","count":4},{"date":"2031-01-01","count":4}]}`)
	c := planning.ParseConstraints(s, raw)
	assert.Equal(t, []planning.LightIntent{{Date: sat, Scope: "meal", Slot: "dinner"}}, c.Light)
	assert.Zero(t, c.MaxCookDays)
	assert.Equal(t, []int64{100}, c.Avoid)
	assert.Equal(t, map[string]float64{sun: 4}, c.Eaters)
	assert.Len(t, c.Ignored, 3)
	assert.Equal(t, planning.Constraints{Ignored: []string{"the wish could not be read"}}, planning.ParseConstraints(s, json.RawMessage(`nope`)))
}

// brokenChooser plans like the heuristic and then breaks a hard rule (a
// component with no portions): the plan a model might hand in.
type brokenChooser struct{}

func (brokenChooser) Choose(ctx context.Context, in planning.ChooseInput) (planning.Proposal, error) {
	p, err := planning.Heuristic{}.Choose(ctx, in)
	for i := range p.Plates {
		if len(p.Plates[i].Components) > 0 && p.Plates[i].Components[0].SessionKey != "" {
			p.Plates[i].Components[0].Portions = -1
			break
		}
	}
	return p, err
}

// PL1b: a run whose plan still holds a hard issue never saves it: the
// heuristic plan is saved instead and the notes say so.
func TestService_OpenHardIssueFallsBackToTheHeuristicPlan(t *testing.T) {
	e := newSvcEnv(t)
	e.deps.Choosers = map[airun.Chooser]planning.Chooser{airun.ChooserHeuristic: planning.Heuristic{}, airun.ChooserLLM: brokenChooser{}}
	e.svc = planning.NewService(e.deps)
	req := e.req()
	req.Chooser = airun.ChooserLLM

	id := e.generate(req)

	end := e.runs.end(id)
	assert.Equal(t, airun.StatusOK, end.Status, "%s %s", end.ErrorKey, end.ViolationsJSON)
	assert.Contains(t, end.Notes, "heuristic plan instead")
	assert.JSONEq(t, `[]`, end.ViolationsJSON)
}

// KR7: a replanned day does not cook a main that a neighbouring day cooks or
// that its other meal holds. (An extra portion of an earlier dish, a leftover,
// is a different thing: only the day's own new cookings are judged.)
func TestService_ReplannedDayCooksADifferentMainThanItsNeighbours(t *testing.T) {
	days := []string{"2026-11-08", "2026-11-09", "2026-11-10", "2026-11-11", "2026-11-12"}
	e := newSvcEnv(t)
	e.ok(e.generate(e.req()))
	for i, d := range days {
		finished(t, e, e.regenerate(planning.Scope{Kind: planning.ScopeDay, Date: day(d)}))
		cooked := func(date string) map[int64]bool { // the mains cooked on a date
			out := map[int64]bool{}
			for _, p := range e.window() {
				if p.DateString() != date || p.SlotID == e.breakfast {
					continue
				}
				for _, c := range p.Components {
					if c.CookSessionID == nil { // a cold plate's leaf: nothing cooked
						continue
					}
					v, err := e.plates.Session(e.ctx, *c.CookSessionID)
					require.NoError(t, err)
					if v.CookDate.Format(time.DateOnly) == date {
						out[c.FoodID] = true
					}
				}
			}
			return out
		}
		today := cooked(d)
		for _, n := range []string{day(d).AddDate(0, 0, -1).Format(time.DateOnly), day(d).AddDate(0, 0, 1).Format(time.DateOnly)} {
			for f := range cooked(n) {
				if slices.Contains(e.mains, f) {
					assert.False(t, today[f], "day %d (%s) cooks food %d that %s cooks too", i, d, f, n)
				}
			}
		}
	}
}

// PL1b: when the heuristic does no better, nothing is saved: the run ends in
// error naming the rule and the plan is as it was.
func TestService_NoBetterFallbackSavesNothing(t *testing.T) {
	e := newSvcEnv(t)
	e.deps.Choosers = map[airun.Chooser]planning.Chooser{airun.ChooserHeuristic: brokenChooser{}, airun.ChooserLLM: brokenChooser{}}
	e.svc = planning.NewService(e.deps)
	before := len(e.window())
	req := e.req()
	req.Chooser = airun.ChooserLLM

	end := e.runs.end(e.generate(req))

	assert.Equal(t, airun.StatusError, end.Status)
	assert.Equal(t, "error.plan.hard_rule_open", end.ErrorKey)
	assert.Contains(t, end.Notes, "nothing was saved")
	assert.Len(t, e.window(), before, "the drafts are as they were")
}
