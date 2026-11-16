package planning

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain/airun"
	"github.com/jaltszeimer/plantry/backend/internal/domain/availability"
	"github.com/jaltszeimer/plantry/backend/internal/domain/diet"
)

// EvaluationStep is the name of the run-log tool step that stores an
// Evaluation; its args are {"phase": ..., "attempt"?: n}.
const EvaluationStep = "plan.evaluation"

// Evaluation phases: chat runs record `before` and `after`; generation runs
// `attempt` (each chooser attempt, the merged proposal) and `after`.
const (
	PhaseBefore  = "before"
	PhaseAttempt = "attempt"
	PhaseAfter   = "after"
)

// Evaluation is a plan's quality at one moment, from the state alone: what
// each day and plate holds and every checker finding. It is the single
// source for the run log's `plan.evaluation` steps (and later the agent's
// evaluate tools). Per-person figures are per eater.
type Evaluation struct {
	Days []DayEval `json:"days"`
	// MeatDays counts the window's days with an omnivore food on a plate
	// (the H5 meat rule); MeatDaysMax is the profile's limit, nil without.
	MeatDays    int         `json:"meat_days"`
	MeatDaysMax *int        `json:"meat_days_max"`
	Plates      []PlateEval `json:"plates"`
	Issues      []Issue     `json:"issues"`
	Totals      EvalTotals  `json:"totals"`
}

// EvalTotals counts the issues by severity.
type EvalTotals struct {
	Violations int `json:"violations"`
	Warnings   int `json:"warnings"`
}

// DayEval is one day with a target cell and at least one planned meal.
// TargetKcal is the day's kcal target scaled to its planned meals (a short
// day asks less); the percentages are the shares of the day's kcal.
type DayEval struct {
	Date          string   `json:"date"`
	Kcal          float64  `json:"kcal"`
	TargetKcal    *float64 `json:"target_kcal"`
	ProteinG      float64  `json:"protein_g"`
	ProteinPct    float64  `json:"protein_pct"`
	FatPct        float64  `json:"fat_pct"`
	CarbsPct      float64  `json:"carbs_pct"`
	Meat          bool     `json:"meat"`
	MeatFoods     []string `json:"meat_foods"`
	CookOccasions int      `json:"cook_occasions"`
	// Complete: every planned, non-routine meal of the day holds a plate.
	Complete bool `json:"complete"`
}

// PlateEval is one plate of the window with its per-eater macros.
type PlateEval struct {
	Date       string          `json:"date"`
	SlotID     int64           `json:"slot_id"`
	PlateID    int64           `json:"plate_id"` // negative: proposed
	Eaters     float64         `json:"eaters"`
	Kcal       float64         `json:"kcal"`
	ProteinG   float64         `json:"protein_g"`
	Components []ComponentEval `json:"components"`
}

// ComponentEval is one food on a plate: its portions (composed) or grams
// (leaf) per eater and the kcal that adds per eater. SessionID names a
// persisted cook session, SessionKey a proposed one.
type ComponentEval struct {
	FoodID     int64    `json:"food_id"`
	Name       string   `json:"name"`
	Role       string   `json:"role"`
	Portions   *float64 `json:"portions,omitempty"`
	Grams      *float64 `json:"grams,omitempty"`
	Kcal       float64  `json:"kcal"`
	SessionID  int64    `json:"session_id,omitempty"`
	SessionKey string   `json:"session_key,omitempty"`
}

// Evaluate describes the plan in s for the days of t. It composes Check,
// DayKcals, the H5 meat-day rule, the cook occasions of the index and the
// foods' macros; pass the state after Merge to evaluate a proposal.
func Evaluate(s State, t Target) Evaluation {
	x := newIndex(s, t)
	ev := Evaluation{
		Days: []DayEval{}, Plates: []PlateEval{}, MeatDaysMax: s.Profile.MeatDaysPerWeekMax,
		Issues: Check(s, t),
	}
	for _, is := range ev.Issues {
		if is.Severity == SeverityViolation {
			ev.Totals.Violations++
		} else {
			ev.Totals.Warnings++
		}
	}

	meat := map[int][]string{} // day -> omnivore foods on its plates
	for _, p := range s.Plates {
		pe := PlateEval{
			Date: p.Date.Format(time.DateOnly), SlotID: p.SlotID, PlateID: p.ID, Eaters: p.Eaters,
			Components: []ComponentEval{},
		}
		for _, c := range p.Components {
			f, ok := x.food(c.FoodID)
			ce := ComponentEval{
				FoodID: c.FoodID, Name: f.Name, Role: string(f.Role), Portions: c.Portions, Grams: c.Grams,
				SessionID: c.SessionID, SessionKey: c.SessionKey,
			}
			if ok {
				m := c.Multiplier()
				ce.Kcal = f.Macros.Kcal * m
				pe.Kcal += ce.Kcal
				pe.ProteinG += f.Macros.Protein * m
				if f.DietClass == diet.Omnivore && s.Window.Contains(p.Date) {
					d := dayOf(p.Date)
					if !containsStr(meat[d], f.Name) {
						meat[d] = append(meat[d], f.Name)
					}
				}
			}
			pe.Components = append(pe.Components, ce)
		}
		if s.Window.Contains(p.Date) {
			ev.Plates = append(ev.Plates, pe)
		}
	}
	ev.MeatDays = len(meat)

	kcal := map[int]DayKcal{}
	for _, d := range DayKcals(s, t) {
		kcal[dayOf(d.Date)] = d
	}
	for _, d := range x.days {
		de := DayEval{Date: d.Format(time.DateOnly), MeatFoods: []string{}, Complete: true}
		dk, hasKcal := kcal[dayOf(d)]
		planned := false
		for _, c := range s.Cells {
			if dayOf(c.Date) == dayOf(d) && c.Mode != availability.ModeNone {
				planned = true
			}
		}
		if !planned {
			continue // a day of No-plate meals has nothing to evaluate
		}
		if hasKcal {
			sc := dk.Scaled
			de.TargetKcal = &sc
			de.Complete = dk.Complete
		}
		var protein, fat, carbs float64
		for _, p := range s.Plates {
			if dayOf(p.Date) != dayOf(d) {
				continue
			}
			for _, c := range p.Components {
				if f, ok := x.food(c.FoodID); ok {
					m := c.Multiplier()
					de.Kcal += f.Macros.Kcal * m
					protein += f.Macros.Protein * m
					fat += f.Macros.Fat * m
					carbs += f.Macros.Carbs * m
				}
			}
		}
		de.ProteinG = protein
		if de.Kcal > 0 {
			de.ProteinPct, de.FatPct, de.CarbsPct = protein*4/de.Kcal*100, fat*9/de.Kcal*100, carbs*4/de.Kcal*100
		}
		if foods := meat[dayOf(d)]; len(foods) > 0 {
			de.Meat, de.MeatFoods = true, foods
		}
		de.CookOccasions = len(x.occasions[dayOf(d)])
		ev.Days = append(ev.Days, de)
	}
	return ev
}

func containsStr(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}

// RecordEvaluation stores ev as a `plan.evaluation` step of the run in ctx
// (no-op without one). attempt is the chooser attempt of an `attempt`
// phase and omitted otherwise (pass -1). It uses a detached ctx so a
// cancelled run still records its closing evaluation.
func RecordEvaluation(ctx context.Context, phase string, attempt int, ev Evaluation) {
	run := airun.FromContext(ctx)
	if run == nil {
		return
	}
	a := map[string]any{"phase": phase}
	if attempt >= 0 {
		a["attempt"] = attempt
	}
	args, _ := json.Marshal(a)
	result, err := json.Marshal(ev)
	isErr := false
	if err != nil {
		result, isErr = []byte(`{"error":"encode"}`), true
	}
	run.ToolStep(context.WithoutCancel(ctx), EvaluationStep, args, result, isErr, 0)
}

// RecordEvaluationError stores a failed `plan.evaluation` step for phase:
// the state could not be built, so the evaluation is missing from the run.
func RecordEvaluationError(ctx context.Context, phase string, cause error) {
	run := airun.FromContext(ctx)
	if run == nil {
		return
	}
	args, _ := json.Marshal(map[string]any{"phase": phase})
	result, _ := json.Marshal(map[string]any{"error": cause.Error()})
	run.ToolStep(context.WithoutCancel(ctx), EvaluationStep, args, result, true, 0)
}
