package eval

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
)

// ExpectEvaluation is the expectation on the saved plan's Evaluation.
const ExpectEvaluation = "evaluation"

// EvalExpect asserts on the closing `plan.evaluation` step of the measured
// run: the plan as saved, judged by the policy (bands, floors, weekly
// ledger). A zero field asserts nothing.
type EvalExpect struct {
	// WeekPct bounds Week.Pct: the weekly average as a percent of the
	// intent-adjusted target ([min, max]).
	WeekPct []float64 `json:"week_pct"`
	// WeekProteinPctMin is the least Week.ProteinPct.
	WeekProteinPctMin float64 `json:"week_protein_pct_min"`
	// FishOrLegumeMealsMin is the least Week.FishMeals + Week.LegumeMeals
	// (the DGE weekly food rules, a soft preference).
	FishOrLegumeMealsMin int `json:"fish_or_legume_meals_min"`
	// MinDayShare is the least share of its NORMAL target every complete day
	// holds (0.75: the floor held).
	MinDayShare float64 `json:"min_day_share"`
	// LightAdjusted: Week.NormalTargetKcal is set and above Week.TargetKcal
	// (a light day lowered the week's target).
	LightAdjusted bool `json:"light_adjusted"`
	// Present and Absent are issue codes the evaluation must and must not
	// report.
	Present []string `json:"issues_present"`
	Absent  []string `json:"issues_absent"`
	// NotesContain are substrings of the run notes.
	NotesContain []string `json:"notes_contain"`
}

// afterEvaluation reads the Evaluation the run stored when it was saved (its
// `after` step); nil when it stored none.
func (e *env) afterEvaluation(ctx context.Context, runID int64) (*planning.Evaluation, error) {
	var raw string
	err := e.db.QueryRowContext(ctx, `SELECT result_json FROM ai_run_steps
		WHERE run_id = ? AND name = ? AND args_json LIKE '%"after"%' ORDER BY seq DESC LIMIT 1`,
		runID, planning.EvaluationStep).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var ev planning.Evaluation
	if err := json.Unmarshal([]byte(raw), &ev); err != nil {
		return nil, fmt.Errorf("run %d evaluation: %w", runID, err)
	}
	return &ev, nil
}

// evaluationExpect checks x against the measured run's closing evaluation.
func (e *env) evaluationExpect(x EvalExpect) Expectation {
	res := Expectation{Name: ExpectEvaluation}
	fail := func(f string, a ...any) Expectation { res.Detail = fmt.Sprintf(f, a...); return res }
	ev := e.after
	if ev == nil {
		return fail("the run stored no closing evaluation")
	}
	w := ev.Week
	if len(x.WeekPct) == 2 || x.WeekProteinPctMin > 0 || x.LightAdjusted {
		if w == nil {
			return fail("no weekly ledger")
		}
		if len(x.WeekPct) == 2 && (w.Pct < x.WeekPct[0] || w.Pct > x.WeekPct[1]) {
			return fail("week %.1f%% of target, want %.0f-%.0f%%", w.Pct, x.WeekPct[0], x.WeekPct[1])
		}
		if w.ProteinPct < x.WeekProteinPctMin {
			return fail("week protein %.1f%% of target, want >= %.0f%%", w.ProteinPct, x.WeekProteinPctMin)
		}
		if x.LightAdjusted && w.NormalTargetKcal <= w.TargetKcal {
			return fail("week target %.0f is not below the normal %.0f", w.TargetKcal, w.NormalTargetKcal)
		}
	}
	if x.FishOrLegumeMealsMin > 0 && (w == nil || w.FishMeals+w.LegumeMeals < x.FishOrLegumeMealsMin) {
		return fail("fish and legume meals below %d", x.FishOrLegumeMealsMin)
	}
	if x.MinDayShare > 0 {
		for _, d := range ev.Days {
			normal := d.NormalTargetKcal
			if d.TargetKcal != nil && normal == 0 {
				normal = *d.TargetKcal
			}
			if d.Complete && normal > 0 && d.Kcal < x.MinDayShare*normal-1e-9 {
				return fail("%s holds %.0f of %.0f kcal (%.0f%%), want >= %.0f%%", d.Date, d.Kcal, normal, d.Kcal/normal*100, x.MinDayShare*100)
			}
		}
	}
	has := func(code string) bool {
		return slices.ContainsFunc(ev.Issues, func(is planning.Issue) bool { return is.Code == code })
	}
	for _, c := range x.Present {
		if !has(c) {
			return fail("no %s issue, want one", c)
		}
	}
	for _, c := range x.Absent {
		if has(c) {
			return fail("%s issue reported", c)
		}
	}
	for _, s := range x.NotesContain {
		if !strings.Contains(e.notes, s) {
			return fail("run notes lack %q: %q", s, e.notes)
		}
	}
	res.OK = true
	return res
}
