package planning

import (
	"context"
	"encoding/json"
	"math"
	"slices"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain/airun"
)

// EvaluationStep is the name of the run-log tool step that stores an
// Evaluation; its args are {"phase": ..., "attempt"?: n}.
const EvaluationStep = "plan.evaluation"

// Evaluation phases: chat runs record `before` and `after`; generation runs
// `attempt` (each chooser attempt, the merged proposal) and `after`.
// PhaseFinalProposal closes a generation run that failed: the last
// proposal (or the unchanged plan) with the run's status.
const (
	PhaseBefore        = "before"
	PhaseAttempt       = "attempt"
	PhaseAfter         = "after"
	PhaseFinalProposal = "final_proposal"
	// PhaseDryRun is a planner's evaluate_plan call or a chat
	// apply_changes dry run (args carry `n`).
	PhaseDryRun = "dry_run"
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
	// Issues are all of Check's findings, warnings included, unless the
	// evaluation was ViewedAs a narrower list; AllIssues then keeps the
	// raw Check result.
	Issues    []Issue    `json:"issues"`
	AllIssues []Issue    `json:"all_issues,omitempty"`
	Totals    EvalTotals `json:"totals"`
	// Week is the weekly kcal ledger; nil without a kcal target or a
	// complete day.
	Week *WeekEval `json:"week,omitempty"`
	// Light are the light requests the evaluation was made under.
	Light []LightIntent `json:"light,omitempty"`
}

// WeekEval is the weekly kcal ledger per eater over the window's complete
// days with nutrition data (the days S2 judges): their average against the
// average of their scaled targets. The week matters most: the policy's week
// band is narrower than its day band.
type WeekEval struct {
	Days    int     `json:"days"`
	AvgKcal float64 `json:"avg_kcal"`
	// TargetKcal is the average of the days' targets after the light
	// requests; NormalTargetKcal the average of the normal ones (only when
	// they differ).
	TargetKcal       float64 `json:"target_kcal"`
	NormalTargetKcal float64 `json:"normal_target_kcal,omitempty"`
	// Pct is AvgKcal as a share of TargetKcal, in percent.
	Pct float64 `json:"pct"`
	// AvgProteinG is the average protein per day; TargetProteinG the average
	// scaled g/kg target and ProteinPct their share in percent (both 0
	// without a weight).
	AvgProteinG    float64 `json:"avg_protein_g"`
	TargetProteinG float64 `json:"target_protein_g,omitempty"`
	ProteinPct     float64 `json:"protein_pct,omitempty"`
	// CarbsPct and FatPct are the weekly shares of energy, in percent.
	CarbsPct float64 `json:"carbs_pct"`
	FatPct   float64 `json:"fat_pct"`
	// FiberG is the average fibre per day against the DGE target of 30 g, for
	// display only. 0 (absent) when no food of the week has fibre data or any
	// food lacks it (a partial average would mislead).
	FiberG float64 `json:"fiber_g,omitempty"`
	// DGE food groups per eater (see groups.go). MeatG is the sum over the
	// counted days (Days), compared to the limit prorated to them. FishMeals
	// and LegumeMeals count the plates of the counted days with at least
	// minMealG of fish or legumes. ProduceGPerDay is the average fruit and
	// vegetable grams per counted day, display only.
	MeatG          float64 `json:"meat_g,omitempty"`
	FishMeals      int     `json:"fish_meals,omitempty"`
	LegumeMeals    int     `json:"legume_meals,omitempty"`
	ProduceGPerDay float64 `json:"produce_g_per_day,omitempty"`
}

// MinWeekDays is how many complete days the weekly average needs before it
// is judged: the average of one or two days is no week.
const MinWeekDays = 3

// EvalTotals counts the issues by severity.
type EvalTotals struct {
	Violations int `json:"violations"`
	Warnings   int `json:"warnings"`
	// Soft counts the S2/S3 issues.
	Soft int `json:"soft"`
}

// DayEval is one day of the window with at least one planned meal.
// TargetKcal is the day's kcal target scaled to its planned meals (a short
// day asks less); the percentages are the shares of the day's kcal.
type DayEval struct {
	Date       string   `json:"date"`
	Kcal       float64  `json:"kcal"`
	TargetKcal *float64 `json:"target_kcal"`
	// NormalTargetKcal is the target without the light request; only on a
	// light day. Light is "day" or "meal" when the day was asked for light
	// (or hearty: LightLevel is then "hearty").
	NormalTargetKcal float64 `json:"normal_target_kcal,omitempty"`
	Light            string  `json:"light,omitempty"`
	LightLevel       string  `json:"light_level,omitempty"`
	// LightMeals names the marked meals with their slot ("light dinner",
	// "hearty lunch"), so a "meal" mark is not anonymous.
	LightMeals []string `json:"light_meals,omitempty"`
	// BandLow and BandHigh are the S2 band around TargetKcal, FloorKcal the
	// H9 floor (it stays on the normal target) and RoomKcal what the day
	// still holds before the band top (0 over it); all kcal per eater, only
	// with a kcal target.
	BandLow        float64  `json:"band_low,omitempty"`
	BandHigh       float64  `json:"band_high,omitempty"`
	FloorKcal      float64  `json:"floor_kcal,omitempty"`
	RoomKcal       float64  `json:"room_kcal,omitempty"`
	ProteinG       float64  `json:"protein_g"`
	ProteinTargetG float64  `json:"protein_target_g,omitempty"`
	ProteinPct     float64  `json:"protein_pct"`
	FatPct         float64  `json:"fat_pct"`
	CarbsPct       float64  `json:"carbs_pct"`
	FiberG         float64  `json:"-"`
	FiberMissing   bool     `json:"-"`
	Meat           bool     `json:"meat"`
	MeatFoods      []string `json:"meat_foods"`
	CookOccasions  int      `json:"cook_occasions"`
	// CookSlots names the slots that cook that day (the chat prompt's day
	// table); not part of the stored or model-facing evaluation.
	CookSlots []string `json:"-"`
	// Complete: every planned, non-routine meal of the day holds a plate
	// (not to be confused with NutritionMissing).
	Complete bool `json:"complete"`
	// NutritionMissing: a food of the day lacks nutrition data, so kcal
	// and the shares understate the day (the board warns the same).
	NutritionMissing bool `json:"nutrition_missing"`
	// Away: the day's lunch and dinner are No plate (DayKcal.Away); H9, S2,
	// the day S3 and the week do not judge it.
	Away bool `json:"away,omitempty"`
	// AwayKcal is the energy the slots that are not planned are assumed to hold
	// (the day's full target x their expected shares, SB3) and PlannedTargetKcal
	// the day's target less it; both only on a day with a slot not planned.
	AwayKcal          float64 `json:"away_kcal,omitempty"`
	PlannedTargetKcal float64 `json:"planned_target_kcal,omitempty"`
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
	// Intent is "light" or "hearty" on a meal marked so; MaxKcal (light) or
	// MinKcal (hearty) is the meal's cap or minimum per eater (S6/S6b).
	Intent  string  `json:"intent,omitempty"`
	MaxKcal float64 `json:"max_kcal,omitempty"`
	MinKcal float64 `json:"min_kcal,omitempty"`
	// Kind is "cold_plate" for a meal of no-cook foods and no recipe.
	Kind string `json:"kind,omitempty"`
}

// ComponentEval is one food on a plate: its portions (composed) or grams
// (leaf) per eater and the kcal that adds per eater. SessionID names a
// persisted cook session, SessionKey a proposed one.
type ComponentEval struct {
	// ID is the persisted plate component (apply_changes plate_component_id); 0 when proposed.
	ID       int64    `json:"plate_component_id,omitempty"`
	FoodID   int64    `json:"food_id"`
	Name     string   `json:"name"`
	Role     string   `json:"role"`
	Portions *float64 `json:"portions,omitempty"`
	Grams    *float64 `json:"grams,omitempty"`
	Kcal     float64  `json:"kcal"`
	ProteinG float64  `json:"protein_g"`
	// NutritionComplete is false when the food lacks nutrition data.
	NutritionComplete bool   `json:"nutrition_complete"`
	SessionID         int64  `json:"session_id,omitempty"`
	SessionKey        string `json:"session_key,omitempty"`
}

// Evaluate describes the plan in s for the window's days: every day with a
// planned meal, whatever t covers (t only scopes Check). It is the single
// source of the day accounts (index.dayAccounts, which DayKcals shares), the
// H5 meat days (index.meatDays, which h5 shares), the cook occasions and the
// foods' macros; pass the state after Merge to evaluate a proposal.
func Evaluate(s State, t Target) Evaluation { return evaluate(s, t, false) }

// evaluateLean is Evaluate without the options of the energy and protein
// issues: for the planner's own re-checks, which read the open issues and the
// figures, never the options (each option costs a re-evaluation).
func evaluateLean(s State, t Target) Evaluation { return evaluateWith(s, t, false, true) }

// EvaluateWeek is Evaluate with verified swap options on the week's meat and
// fish issues (S7); the planner's evaluate_plan, its repair feedback and its
// guard read it. It costs a re-evaluation per candidate swap.
func EvaluateWeek(s State, t Target) Evaluation { return evaluate(s, t, true) }

func evaluate(s State, t Target, weekOpts bool) Evaluation {
	return evaluateWith(s, t, weekOpts, false)
}

func evaluateWith(s State, t Target, weekOpts, noOptions bool) Evaluation {
	x := newIndex(s, t)
	x.weekOpts, x.noOptions = weekOpts, noOptions
	ev := Evaluation{
		Days: []DayEval{}, Plates: []PlateEval{}, MeatDaysMax: s.Profile.MeatDaysPerWeekMax,
		Issues: check(x), Light: s.Light,
	}
	ev.Totals = totalsOf(ev.Issues)

	for _, p := range s.Plates {
		if !s.Window.Contains(p.Date) {
			continue
		}
		pe := PlateEval{
			Date: p.Date.Format(time.DateOnly), SlotID: p.SlotID, PlateID: p.ID, Eaters: p.Eaters,
			Components: []ComponentEval{},
		}
		for _, c := range p.Components {
			f, ok := x.food(c.FoodID)
			ce := ComponentEval{
				ID: c.ID, FoodID: c.FoodID, Name: f.Name, Role: string(f.Role), Portions: c.Portions, Grams: c.Grams,
				NutritionComplete: ok && f.NutritionComplete, SessionID: c.SessionID, SessionKey: c.SessionKey,
			}
			if ok {
				m := c.Multiplier()
				ce.Kcal, ce.ProteinG = f.Macros.Kcal*m, f.Macros.Protein*m
				pe.Kcal += ce.Kcal
				pe.ProteinG += f.Macros.Protein * m
			}
			pe.Components = append(pe.Components, ce)
		}
		x.markIntent(&pe, p)
		if x.s.isColdPlate(p) {
			pe.Kind = ColdPlateKind
		}
		ev.Plates = append(ev.Plates, pe)
	}

	meat := x.meatDays()
	ev.MeatDays = len(meat)

	var days []time.Time // the window's days with a cell, ascending
	seen := map[int]bool{}
	for _, c := range s.Cells {
		if d := dayOf(c.Date); s.Window.Contains(c.Date) && !seen[d] {
			seen[d] = true
			days = append(days, c.Date)
		}
	}
	slices.SortFunc(days, time.Time.Compare)
	hasTarget := s.Profile.KcalTarget != nil && *s.Profile.KcalTarget > 0
	for _, dk := range x.dayAccounts(days) {
		de := DayEval{
			Date: dk.Date.Format(time.DateOnly), Kcal: dk.Kcal, ProteinG: dk.ProteinG,
			MeatFoods: []string{}, Complete: dk.Complete, NutritionMissing: dk.NutritionMissing, Away: dk.Away,
			CookOccasions: len(x.occasions[dayOf(dk.Date)]),
		}
		for _, c := range s.Cells {
			if dayOf(c.Date) == dayOf(dk.Date) && x.occasions[dayOf(dk.Date)][idOf(c.Key())] {
				de.CookSlots = append(de.CookSlots, slotName(s, c.SlotID))
			}
		}
		if hasTarget {
			sc := dk.Adjusted
			de.TargetKcal = &sc
			if dk.Adjusted != dk.Scaled {
				de.NormalTargetKcal = dk.Scaled
			}
			de.FloorKcal = math.Round(dk.Floor(x.pol))
			// The floor is hard and never lowered: a range never starts under it
			// (a day target the floor lifted has a band whose low end would).
			de.BandLow, de.BandHigh = max(math.Round(sc*x.pol.DayLow), de.FloorKcal), math.Round(sc*x.pol.DayHigh)
			if dk.AwayShare > 1e-9 {
				de.AwayKcal = math.Round(dk.Target * dk.AwayShare)
				de.PlannedTargetKcal = math.Round(dk.Target) - de.AwayKcal
			}
			de.RoomKcal = math.Max(0, de.BandHigh-math.Round(dk.Kcal))
		}
		// A day away is not judged and is never called light.
		if !dk.Away {
			de.Light, de.LightLevel = x.lightOf(dk.Date)
			de.LightMeals = x.lightMeals(dk.Date)
		}
		if t := x.pol.ProteinTargetG(dk.Share()); t > 0 {
			de.ProteinTargetG = t
		}
		de.FiberG, de.FiberMissing = dk.FiberG, dk.FiberMissing
		if dk.Kcal > 0 {
			de.ProteinPct, de.FatPct, de.CarbsPct = dk.ProteinG*4/dk.Kcal*100, dk.FatG*9/dk.Kcal*100, dk.CarbsG*4/dk.Kcal*100
		}
		for _, id := range meat[dayOf(dk.Date)] {
			de.Meat = true
			de.MeatFoods = append(de.MeatFoods, s.Foods[id].Name)
		}
		ev.Days = append(ev.Days, de)
	}
	ev.Week = x.week(ev)
	ev.Issues = append(ev.Issues, x.softIssues(ev, t)...)
	x.floorOptions(&ev, t)
	if weekOpts {
		x.weekOptionsFor(&ev, t)
	}
	ev.Totals = totalsOf(ev.Issues)
	return ev
}

func totalsOf(issues []Issue) EvalTotals {
	var t EvalTotals
	for _, is := range issues {
		switch is.Severity {
		case SeverityViolation:
			t.Violations++
		case SeveritySoft:
			t.Soft++
		default:
			t.Warnings++
		}
	}
	return t
}

// ViewedAs returns e with Issues (and Totals) narrowed to issues, the list
// a user or the chat prompt saw; the raw Check result moves to AllIssues.
func (e Evaluation) ViewedAs(issues []Issue) Evaluation {
	e.AllIssues = e.Issues
	e.Issues = issues
	e.Totals = totalsOf(issues)
	return e
}

// RecordEvaluation stores build() as a `plan.evaluation` step of phase for
// the run in ctx; build is not called without a run. It uses a detached ctx
// so a cancelled run still records its closing evaluation.
func RecordEvaluation(ctx context.Context, phase string, build func() Evaluation) {
	recordEvaluation(ctx, map[string]any{"phase": phase}, build)
}

// RecordEvaluationWith is RecordEvaluation with more fields in the step's args
// (the closing evaluation also carries what the code changed after the chooser).
func RecordEvaluationWith(ctx context.Context, phase string, extra map[string]any, build func() Evaluation) {
	a := map[string]any{"phase": phase}
	for k, v := range extra {
		a[k] = v
	}
	recordEvaluation(ctx, a, build)
}

// RecordAttemptEvaluation stores the merged proposal of chooser attempt n.
func RecordAttemptEvaluation(ctx context.Context, n int, build func() Evaluation) {
	recordEvaluation(ctx, map[string]any{"phase": PhaseAttempt, "attempt": n}, build)
}

// RecordDryRun stores the evaluation of the n-th dry run of a run.
func RecordDryRun(ctx context.Context, n int, build func() Evaluation) {
	recordEvaluation(ctx, map[string]any{"phase": PhaseDryRun, "n": n}, build)
}

// RecordFinalProposal closes a failed generation run: the evaluation of its
// last proposal, with the run's status ("error", "timeout", "panic") and the
// stage that failed.
func RecordFinalProposal(ctx context.Context, status, stage string, build func() Evaluation) {
	recordEvaluation(ctx, map[string]any{"phase": PhaseFinalProposal, "status": status, "stage": stage}, build)
}

func recordEvaluation(ctx context.Context, a map[string]any, build func() Evaluation) {
	run := airun.FromContext(ctx)
	if run == nil {
		return
	}
	args, _ := json.Marshal(a)
	result, err := json.Marshal(build())
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

// floorOptions gives every H9 `kcal_below_floor` issue the same verified
// options as a day under its band (S2): no-cook adds at serving size and
// raises of components already on the day, ranked by whether they reach the
// floor.
func (x *index) floorOptions(ev *Evaluation, t Target) {
	for i, is := range ev.Issues {
		if is.Code != CodeKcalBelowFloor || is.Date == "" {
			continue
		}
		day, err := time.Parse(time.DateOnly, is.Date)
		if err != nil {
			continue
		}
		for _, d := range ev.Days {
			if d.Date == is.Date && d.TargetKcal != nil && *d.TargetKcal > 0 {
				ev.Issues[i].Options = x.optionsFor(*ev, t, d, day, false, *d.TargetKcal, d.FloorKcal)
			}
		}
	}
}
