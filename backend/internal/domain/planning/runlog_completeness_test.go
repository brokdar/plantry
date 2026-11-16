package planning_test

import (
	"encoding/json"
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
