package eval

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/airun"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
)

// The model-facing evaluation is a fraction of the stored one. This measures
// both on the family_daily week (the eval library): the full JSON the run log
// keeps, the planner's evaluate_plan result (every plate proposed, so every
// day and component), a get_plan result (no components) and an apply_changes
// style result (one day, one plate with components). Run with -v for the
// byte counts.
func TestCompactEvaluationPayload(t *testing.T) {
	ctx := context.Background()
	lib, err := LoadLibrary()
	require.NoError(t, err)
	scs, err := LoadScenarios("family_daily")
	require.NoError(t, err)
	sc := scs[0]
	d, err := newDBs("")
	require.NoError(t, err)
	t.Cleanup(func() { _ = d.close() })
	conn, err := d.open("payload")
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	e := newEnv(conn, Config{Chooser: airun.ChooserHeuristic})
	require.NoError(t, e.seed(ctx, sc, lib))
	for _, st := range sc.Steps {
		_, err := e.step(ctx, sc, st, airun.ChooserHeuristic)
		require.NoError(t, err)
	}

	var raw string
	err = conn.QueryRowContext(ctx,
		`SELECT result_json FROM ai_run_steps WHERE name = ? AND args_json LIKE '%"after"%' ORDER BY id DESC LIMIT 1`,
		planning.EvaluationStep).Scan(&raw)
	require.NoError(t, err)
	var ev planning.Evaluation
	require.NoError(t, json.Unmarshal([]byte(raw), &ev))
	require.NotEmpty(t, ev.Plates)

	size := func(v any) int {
		b, err := json.Marshal(v)
		require.NoError(t, err)
		return len(b)
	}
	full := len(raw)
	planner := size(ev.Compact(planning.View{Detail: func(planning.PlateEval) bool { return true }}))
	getPlan := size(ev.Compact(planning.View{Slim: true}))
	first := ev.Plates[0]
	one := size(ev.Compact(planning.View{
		Dates:  map[string]bool{first.Date: true},
		Detail: func(p planning.PlateEval) bool { return p.PlateID == first.PlateID },
	}))
	t.Logf("evaluation bytes: full %d, planner evaluate_plan %d, get_plan %d, one-day apply_changes %d (%d plates, %d days)",
		full, planner, getPlan, one, len(ev.Plates), len(ev.Days))
	assert.Less(t, planner, full*4/5)
	assert.Less(t, getPlan, full*4/5) // slim components on every plate
	assert.Less(t, one, full/4)
}
