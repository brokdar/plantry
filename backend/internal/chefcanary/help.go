package chefcanary

import (
	"context"
	"errors"
	"fmt"
	"time"
	"unicode/utf8"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/chefsets"
	"github.com/jaltszeimer/plantry/backend/internal/domain/airun"
	"github.com/jaltszeimer/plantry/backend/internal/domain/chef"
	"github.com/jaltszeimer/plantry/backend/internal/domain/chefhelp"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
)

// HelpConfig is the help mode: explain-step and went-wrong questions on a few
// recipes through chefhelp.Service. The recipes and their cook sessions are
// made on a scratch copy of the database; the run log is written to Env.DB.
type HelpConfig struct {
	Env
	Set chefsets.HelpSet
}

type helpStubs struct{}

func (helpStubs) ChefEnabled(context.Context) (bool, error) { return true, nil }
func (helpStubs) Check(context.Context, airun.Kind, bool) (chef.Decision, error) {
	return chef.Decision{Run: true}, nil
}

func (helpStubs) Get(context.Context) (*profile.Profile, error) {
	return &profile.Profile{Kitchen: profile.Kitchen{Equipment: []string{"oven", "stovetop"}}}, nil
}

// RunHelp asks every question and reports the latency, cost and the shape
// rules the service applies: at most three lines of bounded length, and how
// often the amount check made it ask the model again.
func RunHelp(ctx context.Context, cfg HelpConfig) (ModeReport, error) {
	l, err := newLedger(ctx, cfg.Env)
	if err != nil {
		return ModeReport{}, err
	}
	cp, cleanup, err := scratchCopy(ctx, cfg.DB)
	if err != nil {
		return ModeReport{}, err
	}
	defer cleanup()
	foods := sqlite.NewFoodRepo(cp)
	sessions := sqlite.NewCookSessionRepo(cp)
	slotID, err := scratchSlot(ctx, cp)
	if err != nil {
		return ModeReport{}, err
	}
	leaves, err := leafIDs(ctx, foods)
	if err != nil {
		return ModeReport{}, err
	}
	sessionOf := map[string]int64{}
	for _, r := range cfg.Set.Recipes {
		recipe, _, err := makeRecipe(ctx, foods, leaves, "help "+r.ID, r.Portions, r.Ingredients)
		if err != nil {
			return ModeReport{}, err
		}
		for i, s := range r.Steps {
			recipe.Instructions = append(recipe.Instructions, food.Instruction{StepNumber: i + 1, Text: s})
		}
		if err := foods.Update(ctx, recipe); err != nil {
			return ModeReport{}, err
		}
		sess := &plate.CookSession{FoodID: recipe.ID, CookDate: time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC), CookSlotID: slotID, Portions: r.Portions, State: plate.StateConfirmed}
		if err := sessions.CreateSession(ctx, sess); err != nil {
			return ModeReport{}, err
		}
		sessionOf[r.ID] = sess.ID
	}
	resolver := llm.StaticResolver(l.rec.Meter(cfg.Client, cfg.Provider), cfg.Model)
	svc := chefhelp.NewService(sessions, foods, helpStubs{}, helpStubs{}, helpStubs{}, resolver, l.rec)

	rep := ModeReport{Mode: "help", Cases: len(cfg.Set.Questions)}
	var latencies, costs []float64
	var failed, tooLong, retried int
	for i, q := range cfg.Set.Questions {
		if l.capReached() {
			rep.StoppedBySpendCap = true
			break
		}
		start := time.Now()
		ans, err := svc.Help(ctx, chefhelp.Request{SessionID: sessionOf[q.Recipe], StepIndex: q.Step, Kind: q.Kind, Problem: q.Problem})
		elapsed := time.Since(start)
		u, serr := l.settle(ctx)
		if serr != nil {
			return ModeReport{}, serr
		}
		id := fmt.Sprintf("%s-%d-%s", q.Recipe, q.Step, q.Kind)
		if q.Problem != "" {
			id += "-" + q.Problem
		}
		res := CaseResult{ID: fmt.Sprintf("%02d-%s", i+1, id), Groups: []string{q.Kind}, CostUSD: u.CostUSD, LatencyMS: elapsed.Milliseconds()}
		rep.Ran++
		latencies, costs = append(latencies, elapsed.Seconds()), append(costs, u.CostUSD)
		switch {
		case err != nil:
			failed++
			res.Outcome, res.Detail = "error", err.Error()
			if errors.Is(err, chefhelp.ErrModel) {
				res.Outcome = "model_error"
			}
		default:
			calls := 0
			steps, serr := l.runs.ListSteps(ctx, ans.RunID)
			if serr != nil {
				return ModeReport{}, serr
			}
			for _, s := range steps {
				if s.Type == airun.StepLLMCall {
					calls++
				}
			}
			if calls > 1 {
				retried++
			}
			shapeOK := len(ans.Lines) > 0 && len(ans.Lines) <= chefhelp.MaxLines
			for _, ln := range ans.Lines {
				shapeOK = shapeOK && utf8.RuneCountInString(ln) <= chefhelp.MaxLineRunes
			}
			if !shapeOK {
				tooLong++
			}
			res.OK, res.Outcome = shapeOK, map[bool]string{true: "answered", false: "bad_shape"}[shapeOK]
			res.Detail = fmt.Sprintf("lines=%d model_calls=%d", len(ans.Lines), calls)
		}
		rep.Results = append(rep.Results, res)
	}
	l.finish(&rep)

	rep.add(Metric{Name: "latency.p50", Value: percentile(latencies, 50), Unit: "s"})
	rep.add(Metric{Name: "latency.p90", Value: percentile(latencies, 90), Unit: "s"}.target("<= 10 s", false, 10))
	rep.add(Metric{Name: "cost_per_help.p50", Value: percentile(costs, 50), Unit: "usd"})
	rep.add(Metric{Name: "cost_per_help.p90", Value: percentile(costs, 90), Unit: "usd"})
	rep.add(Metric{Name: "failed_answers", Value: float64(failed), Note: "error or model_error"}.target("0", false, 0))
	rep.add(Metric{Name: "answers_breaking_the_line_rule", Value: float64(tooLong), Note: fmt.Sprintf("more than %d lines or a line over %d characters", chefhelp.MaxLines, chefhelp.MaxLineRunes)}.target("0", false, 0))
	rep.add(Metric{Name: "answers_asked_twice", Value: float64(retried), Note: "the amount check made the service ask the model again"})
	return rep, nil
}
