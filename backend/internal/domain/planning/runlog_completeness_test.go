package planning_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/airun"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
)

// loggedStep is a tool step of a run: its name, args and result in seq order.
type loggedStep struct {
	Name, Args, Result string
}

// toolSteps are the run's tool_call and llm_call steps in seq order (an
// llm_call has the name "llm_call").
func (e *llmEnv) toolSteps(id int64) []loggedStep {
	e.t.Helper()
	rows, err := e.db.Query(`SELECT CASE type WHEN 'llm_call' THEN 'llm_call' ELSE name END,
		COALESCE(args_json, ''), COALESCE(result_json, '') FROM ai_run_steps WHERE run_id = ? ORDER BY seq`, id)
	require.NoError(e.t, err)
	defer func() { _ = rows.Close() }()
	var out []loggedStep
	for rows.Next() {
		var s loggedStep
		require.NoError(e.t, rows.Scan(&s.Name, &s.Args, &s.Result))
		out = append(out, s)
	}
	require.NoError(e.t, rows.Err())
	return out
}

func names(steps []loggedStep) []string {
	out := make([]string, len(steps))
	for i, s := range steps {
		out[i] = s.Name
	}
	return out
}

func phaseOf(t *testing.T, s loggedStep) (phase string, attempt *int) {
	t.Helper()
	var a struct {
		Phase   string `json:"phase"`
		Attempt *int   `json:"attempt"`
	}
	require.NoError(t, json.Unmarshal([]byte(s.Args), &a))
	return a.Phase, a.Attempt
}

// A scripted LLM generation records the context the code built, the plan
// after each attempt and the saved plan.
func TestRunLog_LLMGenerationRecordsContextAttemptsAndAfter(t *testing.T) {
	e := newLLMEnv(t)
	id := e.gen(submit(t, "tu_1", e.h2Plan()), submit(t, "tu_2", e.validPlan()))
	steps := e.toolSteps(id)

	assert.Equal(t, []string{
		planning.ContextStep,
		"llm_call", "plan.attempt", planning.EvaluationStep,
		"llm_call", "plan.attempt", planning.EvaluationStep,
		planning.EvaluationStep,
	}, names(steps))

	var ctxRec struct {
		Chooser      string  `json:"chooser"`
		CandidateIDs []int64 `json:"candidate_ids"`
		OpenCells    int     `json:"open_cells"`
		Excluded     []any   `json:"excluded"`
		ShortDays    []any   `json:"short_days"`
		Plates       struct{ Total, Fixed int }
		Settings     struct {
			MaxCook int `json:"max_cook_occasions_per_day"`
		}
	}
	require.NoError(t, json.Unmarshal([]byte(steps[0].Result), &ctxRec))
	assert.Equal(t, "llm", ctxRec.Chooser)
	assert.NotEmpty(t, ctxRec.CandidateIDs)
	assert.Positive(t, ctxRec.OpenCells)
	assert.NotNil(t, ctxRec.Excluded)
	assert.NotNil(t, ctxRec.ShortDays)

	for i, want := range []struct {
		step    int
		phase   string
		attempt int
	}{{3, "attempt", 0}, {6, "attempt", 1}} {
		phase, attempt := phaseOf(t, steps[want.step])
		assert.Equal(t, want.phase, phase, "step %d", i)
		require.NotNil(t, attempt)
		assert.Equal(t, want.attempt, *attempt)
	}
	phase, attempt := phaseOf(t, steps[7])
	assert.Equal(t, "after", phase)
	assert.Nil(t, attempt)

	var first, last planning.Evaluation
	require.NoError(t, json.Unmarshal([]byte(steps[3].Result), &first))
	require.NoError(t, json.Unmarshal([]byte(steps[7].Result), &last))
	assert.Positive(t, first.Totals.Violations, "the first attempt breaks H2")
	assert.Zero(t, last.Totals.Violations)
	assert.NotEmpty(t, last.Plates)
	assert.NotEmpty(t, last.Days)
}

// The heuristic records its context and per-cell choices, then the plan.
func TestRunLog_HeuristicRecordsChoicesAndAfter(t *testing.T) {
	e := newLLMEnv(t)
	req := e.req()
	req.Chooser = airun.ChooserHeuristic
	id := e.generate(req)
	steps := e.toolSteps(id)

	require.Equal(t, []string{planning.ContextStep, planning.HeuristicChoicesStep, planning.EvaluationStep}, names(steps))
	var ctxRec struct {
		Chooser string `json:"chooser"`
	}
	require.NoError(t, json.Unmarshal([]byte(steps[0].Result), &ctxRec))
	assert.Equal(t, "heuristic", ctxRec.Chooser)

	var choices []struct {
		Date   string  `json:"date"`
		SlotID int64   `json:"slot_id"`
		Foods  []int64 `json:"foods"`
		Reason string  `json:"reason"`
	}
	require.NoError(t, json.Unmarshal([]byte(steps[1].Result), &choices))
	require.NotEmpty(t, choices)
	reasons := map[string]bool{}
	for _, c := range choices {
		assert.NotEmpty(t, c.Reason)
		assert.NotEmpty(t, c.Date)
		reasons[c.Reason] = true
	}
	assert.True(t, reasons["cook_new"], "the first cell of a batch is cooked: %v", reasons)

	phase, _ := phaseOf(t, steps[2])
	assert.Equal(t, "after", phase)
}

// The excluded list names every recipe the chooser never saw and why.
func TestRunLog_ContextExcludesReplacedRecipes(t *testing.T) {
	e := newLLMEnv(t)
	first := e.gen(submit(t, "tu_1", e.validPlan()))
	require.Equal(t, "ok", e.run(first).Status)

	// Regenerating a day replaces its drafts: their recipes are avoided.
	e.script.Turns = nil
	req := e.req()
	req.Chooser = airun.ChooserHeuristic
	req.Scope = planning.Scope{Kind: planning.ScopeDay, Date: e.start}
	id, err := e.svc.Regenerate(e.ctx, req)
	require.NoError(t, err)
	steps := e.toolSteps(id)
	require.NotEmpty(t, steps)
	require.Equal(t, planning.ContextStep, steps[0].Name)
	var rec struct {
		Excluded []struct {
			ID     int64  `json:"id"`
			Name   string `json:"name"`
			Reason string `json:"reason"`
		} `json:"excluded"`
	}
	require.NoError(t, json.Unmarshal([]byte(steps[0].Result), &rec))
	require.NotEmpty(t, rec.Excluded)
	assert.Equal(t, "replaced_by_target", rec.Excluded[0].Reason)
	assert.NotEmpty(t, rec.Excluded[0].Name)
}

type closingArgs struct {
	Phase  string `json:"phase"`
	Status string `json:"status"`
	Stage  string `json:"stage"`
}

func closing(t *testing.T, steps []loggedStep) (closingArgs, planning.Evaluation) {
	t.Helper()
	require.NotEmpty(t, steps)
	last := steps[len(steps)-1]
	require.Equal(t, planning.EvaluationStep, last.Name)
	var a closingArgs
	require.NoError(t, json.Unmarshal([]byte(last.Args), &a))
	var ev planning.Evaluation
	require.NoError(t, json.Unmarshal([]byte(last.Result), &ev))
	return a, ev
}

// `after` is the saved plan read back: its plates carry persisted ids, the
// attempts' proposals do not.
func TestRunLog_AfterIsTheSavedPlan(t *testing.T) {
	e := newLLMEnv(t)
	id := e.gen(submit(t, "tu_1", e.validPlan()))
	steps := e.toolSteps(id)
	a, after := closing(t, steps)
	assert.Equal(t, "after", a.Phase)
	require.NotEmpty(t, after.Plates)
	for _, p := range after.Plates {
		assert.Positive(t, p.PlateID, "a persisted plate")
	}
	var attempt planning.Evaluation
	require.NoError(t, json.Unmarshal([]byte(steps[3].Result), &attempt))
	require.NotEmpty(t, attempt.Plates)
	assert.Negative(t, attempt.Plates[0].PlateID, "a proposed plate")
}

// A failed chooser still closes the run with the last proposal's evaluation.
func TestRunLog_ChooserErrorClosesWithFinalProposal(t *testing.T) {
	t.Run("no proposal at all", func(t *testing.T) {
		e := newLLMEnv(t)
		id := e.gen()
		a, ev := closing(t, e.toolSteps(id))
		assert.Equal(t, planning.PhaseFinalProposal, a.Phase)
		assert.Equal(t, "choosing", a.Stage)
		for _, p := range ev.Plates {
			for _, c := range p.Components {
				assert.Equal(t, "standalone", c.Role, "only the routine plates of the base plan")
			}
		}
		assert.NotEmpty(t, ev.Days)
	})
}

// A day regenerate evaluates the whole window: days and meat_days agree.
func TestRunLog_DayRegenerateEvaluatesTheWindow(t *testing.T) {
	e := newLLMEnv(t)
	require.Equal(t, "ok", e.run(e.gen(submit(t, "tu_1", e.validPlan()))).Status)
	req := e.req()
	req.Chooser = airun.ChooserHeuristic
	req.Scope = planning.Scope{Kind: planning.ScopeDay, Date: e.start}
	id, err := e.svc.Regenerate(e.ctx, req)
	require.NoError(t, err)
	steps := e.toolSteps(id)
	_, ev := closing(t, steps)
	assert.Len(t, ev.Days, 7)
	var meat int
	for _, d := range ev.Days {
		if d.Meat {
			meat++
		}
	}
	assert.Equal(t, ev.MeatDays, meat)

	var rec struct {
		Avoid        []int64            `json:"avoid"`
		AvoidBatches []int64            `json:"avoid_batches"`
		EatersByDate map[string]float64 `json:"eaters_by_date"`
		Side         any                `json:"side"`
	}
	require.NoError(t, json.Unmarshal([]byte(steps[0].Result), &rec))
	assert.NotEmpty(t, rec.Avoid)
	assert.NotNil(t, rec.AvoidBatches)
	assert.Equal(t, map[string]float64{e.start.Format("2006-01-02"): 1}, rec.EatersByDate, "a regenerate keeps the replaced plates' eaters")
	assert.Nil(t, rec.Side)
}

// failingChooser proposes with the heuristic, then fails with err: a chooser
// that errors after it made a proposal.
type failingChooser struct{ err error }

func (f failingChooser) Choose(ctx context.Context, in planning.ChooseInput) (planning.Proposal, error) {
	prop, _ := planning.Heuristic{}.Choose(ctx, in)
	return prop, f.err
}

func TestRunLog_FailedChooserKeepsItsLastProposal(t *testing.T) {
	for name, tc := range map[string]struct {
		err    error
		status string
	}{
		"error":   {errors.New("boom"), "error"},
		"timeout": {context.DeadlineExceeded, "timeout"},
	} {
		t.Run(name, func(t *testing.T) {
			e := newLLMEnv(t)
			e.deps.Choosers[airun.ChooserHeuristic] = failingChooser{tc.err}
			e.svc = planning.NewService(e.deps)
			req := e.req()
			req.Chooser = airun.ChooserHeuristic
			id := e.generate(req)
			a, ev := closing(t, e.toolSteps(id))
			assert.Equal(t, planning.PhaseFinalProposal, a.Phase)
			assert.Equal(t, tc.status, a.Status)
			assert.Equal(t, "choosing", a.Stage)
			var proposed int
			for _, p := range ev.Plates {
				if p.PlateID < 0 {
					proposed++
				}
			}
			assert.Greater(t, proposed, 5, "the chooser's proposal, not only the routine plates")
		})
	}
}

// A side swap records its choice and its context.
func TestRunLog_SideSwapRecordsChoiceAndSide(t *testing.T) {
	e := newLLMEnv(t)
	first := e.req()
	first.Chooser = airun.ChooserHeuristic
	require.Equal(t, "ok", e.run(e.generate(first)).Status)
	var comp int64
	require.NoError(t, e.db.QueryRow(`SELECT pc.id FROM plate_components pc JOIN foods f ON f.id = pc.food_id
		WHERE f.role = 'side_veg' LIMIT 1`).Scan(&comp))
	req := e.req()
	req.Chooser = airun.ChooserHeuristic
	req.Scope = planning.Scope{Kind: planning.ScopeSide, ComponentID: comp}
	id, err := e.svc.Regenerate(e.ctx, req)
	require.NoError(t, err)
	steps := e.toolSteps(id)
	require.Equal(t, []string{planning.ContextStep, planning.HeuristicChoicesStep, planning.EvaluationStep}, names(steps))
	var choices []struct {
		Reason string  `json:"reason"`
		Foods  []int64 `json:"foods"`
	}
	require.NoError(t, json.Unmarshal([]byte(steps[1].Result), &choices))
	require.Len(t, choices, 1)
	assert.Equal(t, "side_swap", choices[0].Reason)
	assert.Len(t, choices[0].Foods, 1)
	var rec struct {
		Side *struct {
			FoodID int64 `json:"food_id"`
			Role   string
			Plates []any `json:"plates"`
		} `json:"side"`
	}
	require.NoError(t, json.Unmarshal([]byte(steps[0].Result), &rec))
	require.NotNil(t, rec.Side)
	assert.Equal(t, "side_veg", rec.Side.Role)
	assert.NotEmpty(t, rec.Side.Plates)
}
