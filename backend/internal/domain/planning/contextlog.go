package planning

import (
	"cmp"
	"context"
	"encoding/json"
	"log/slog"
	"slices"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain/airun"
	"github.com/jaltszeimer/plantry/backend/internal/domain/availability"
)

// Run-log step names of the code's decisions.
const (
	ContextStep          = "plan.context"
	HeuristicChoicesStep = "heuristic.choices"
)

// Why a recipe is not a candidate.
const (
	excludeReplaced    = "replaced_by_target"
	excludeDiet        = "diet_incompatible"
	excludeSideRole    = "side_swap_other_role"
	excludeTokenBudget = "token_budget"
)

// excludedFood is a recipe the chooser never saw, and why.
type excludedFood struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

// shortDay is a day with a No-plate meal; TargetKcal is its scaled kcal
// target, nil without a kcal target.
type shortDay struct {
	Date       string   `json:"date"`
	Planned    int      `json:"planned"`
	Meals      int      `json:"meals"`
	TargetKcal *float64 `json:"target_kcal"`
}

// sideRecord is the side a side-swap run replaces.
type sideRecord struct {
	FoodID         int64       `json:"food_id"`
	Role           string      `json:"role"`
	CookDate       string      `json:"cook_date"`
	CookSlotID     int64       `json:"cook_slot_id"`
	Portions       float64     `json:"portions"`
	FrozenPortions float64     `json:"frozen_portions"`
	Plates         []sidePlate `json:"plates"`
}

type sidePlate struct {
	PlateID  int64   `json:"plate_id"`
	Date     string  `json:"date"`
	SlotID   int64   `json:"slot_id"`
	Eaters   float64 `json:"eaters"`
	PerEater float64 `json:"per_eater"`
}

// contextRecord is the result of the run's `plan.context` step: what the
// code handed the chooser (the settings and targets it works under, the
// short days, how much of the window is fixed, the candidate recipes and the
// ones left out with their reasons).
type contextRecord struct {
	Chooser string `json:"chooser"`
	Window  struct {
		Start string `json:"start"`
		End   string `json:"end"`
		Today string `json:"today"`
	} `json:"window"`
	Settings struct {
		MaxCookOccasionsPerDay int  `json:"max_cook_occasions_per_day"`
		VarietyWeeks           int  `json:"variety_weeks"`
		BatchCooking           bool `json:"batch_cooking"`
		DefaultKeepDays        int  `json:"default_keep_days"`
		FreezingEnabled        bool `json:"freezing_enabled"`
		FreezerKeepDays        int  `json:"freezer_keep_days"`
		PlateFit               bool `json:"plate_fit"`
		BatchVariation         bool `json:"batch_variation"`
	} `json:"settings"`
	Profile struct {
		KcalTarget         *float64 `json:"kcal_target"`
		ProteinPct         *float64 `json:"protein_pct"`
		FatPct             *float64 `json:"fat_pct"`
		CarbsPct           *float64 `json:"carbs_pct"`
		DietType           string   `json:"diet_type"`
		MeatDaysPerWeekMax *int     `json:"meat_days_per_week_max"`
		HouseholdPortions  float64  `json:"household_portions"`
	} `json:"profile"`
	ExploreQuota int        `json:"explore_quota"`
	TargetCells  int        `json:"target_cells"`
	OpenCells    int        `json:"open_cells"`
	ShortDays    []shortDay `json:"short_days"`
	// What the run's scope handed the chooser: the replaced recipes, the
	// batches the target must not eat from, the per-day eaters override and
	// the side a side swap replaces.
	Avoid        []int64            `json:"avoid"`
	AvoidBatches []int64            `json:"avoid_batches"`
	Exclude      []int64            `json:"exclude"`
	EatersByDate map[string]float64 `json:"eaters_by_date"`
	Side         *sideRecord        `json:"side"`
	// Light are the light requests the run started with (the chooser may add
	// its own; every plan.evaluation step carries the ones it was made under).
	Light []LightIntent `json:"light,omitempty"`
	// Fixed rows: plates and sessions outside the target or confirmed (the
	// chooser may not change them), and the totals they are counted from.
	Plates struct {
		Total int `json:"total"`
		Fixed int `json:"fixed"`
	} `json:"plates"`
	Sessions struct {
		Total int `json:"total"`
		Fixed int `json:"fixed"`
	} `json:"sessions"`
	CandidateIDs []int64        `json:"candidate_ids"`
	Excluded     []excludedFood `json:"excluded"`
}

// candidateSplit returns the recipes candidates() offers and, for every
// other composed recipe, why it is out.
func candidateSplit(in ChooseInput) (included []FoodFacts, excluded []excludedFood) {
	elig, excluded := eligibleSplit(in)
	included = candidates(in)
	for _, f := range elig {
		if !slices.ContainsFunc(included, func(c FoodFacts) bool { return c.ID == f.ID }) {
			excluded = append(excluded, excludedFood{f.ID, f.Name, excludeSideRole})
		}
	}
	return included, excluded
}

// recordContext stores the run's `plan.context` step (no-op without a run).
// trimmed are the candidates the LLM prompt's token budget dropped.
func recordContext(ctx context.Context, in ChooseInput, chooser string, trimmed []FoodFacts) {
	run := airun.FromContext(ctx)
	if run == nil {
		return
	}
	s := in.State
	x := newIndex(s, in.Target)
	var rec contextRecord
	rec.Chooser = chooser
	rec.Light = s.Light
	rec.Window.Start, rec.Window.End = s.Window.Start.Format(time.DateOnly), s.Window.End.Format(time.DateOnly)
	rec.Window.Today = s.Today.Format(time.DateOnly)
	rec.Settings.MaxCookOccasionsPerDay, rec.Settings.VarietyWeeks, rec.Settings.BatchCooking = s.Settings.MaxCookOccasionsPerDay, s.Settings.VarietyWeeks, s.Settings.BatchCooking
	rec.Settings.DefaultKeepDays, rec.Settings.FreezingEnabled, rec.Settings.FreezerKeepDays = s.Settings.DefaultKeepDays, s.Settings.FreezingEnabled, s.Settings.FreezerKeepDays
	rec.Settings.PlateFit, rec.Settings.BatchVariation = s.Settings.PlateFit, s.Settings.BatchVariation
	p := s.Profile
	rec.Profile.KcalTarget, rec.Profile.ProteinPct, rec.Profile.FatPct, rec.Profile.CarbsPct = p.KcalTarget, p.ProteinPct, p.FatPct, p.CarbsPct
	rec.Profile.DietType, rec.Profile.MeatDaysPerWeekMax, rec.Profile.HouseholdPortions = string(p.DietType), p.MeatDaysPerWeekMax, p.HouseholdPortions
	rec.ExploreQuota = in.ExploreQuota
	rec.TargetCells = len(in.Target.Cells)
	rec.ShortDays = []shortDay{}
	for _, dk := range x.dayAccounts(x.days) {
		if dk.Active() < dk.Meals {
			sd := shortDay{Date: dk.Date.Format(time.DateOnly), Planned: dk.Active(), Meals: dk.Meals}
			if dk.Target > 0 {
				scaled := dk.Scaled
				sd.TargetKcal = &scaled
			}
			rec.ShortDays = append(rec.ShortDays, sd)
		}
	}
	rec.Avoid, rec.AvoidBatches, rec.Exclude, rec.EatersByDate = nonNil(in.Avoid), nonNil(in.AvoidBatches), nonNil(in.Exclude), in.EatersByDate
	if sw := in.Side; sw != nil {
		sr := &sideRecord{
			FoodID: sw.FoodID, Role: string(sw.Role), CookDate: sw.CookDate.Format(time.DateOnly), CookSlotID: sw.CookSlotID,
			Portions: sw.Portions, FrozenPortions: sw.FrozenPortions, Plates: []sidePlate{},
		}
		for _, sp := range sw.Plates {
			sr.Plates = append(sr.Plates, sidePlate{sp.PlateID, sp.Date.Format(time.DateOnly), sp.SlotID, sp.Eaters, sp.PerEater})
		}
		rec.Side = sr
	}
	for _, c := range s.Cells {
		if c.Mode == availability.ModePlan && x.inTarget(c.Key()) && len(x.byCell[idOf(c.Key())]) == 0 {
			rec.OpenCells++
		}
	}
	rec.Plates.Total = len(s.Plates)
	for _, pl := range s.Plates {
		if pl.Confirmed || !x.inTarget(pl.Cell()) {
			rec.Plates.Fixed++
		}
	}
	rec.Sessions.Total = len(s.Sessions)
	for _, ss := range s.Sessions {
		if ss.Confirmed || !x.checked(ss) {
			rec.Sessions.Fixed++
		}
	}
	included, excluded := candidateSplit(in)
	rec.CandidateIDs = []int64{}
	dropped := map[int64]bool{}
	for _, f := range trimmed {
		dropped[f.ID] = true
		excluded = append(excluded, excludedFood{f.ID, f.Name, excludeTokenBudget})
	}
	for _, f := range included {
		if !dropped[f.ID] {
			rec.CandidateIDs = append(rec.CandidateIDs, f.ID)
		}
	}
	slices.SortFunc(excluded, func(a, b excludedFood) int { return cmp.Compare(a.ID, b.ID) })
	if excluded == nil {
		excluded = []excludedFood{}
	}
	rec.Excluded = excluded
	args, _ := json.Marshal(map[string]any{"chooser": chooser})
	result, err := json.Marshal(rec)
	if err != nil {
		slog.WarnContext(ctx, "plan.context not recorded", "error", err)
		return
	}
	run.ToolStep(context.WithoutCancel(ctx), ContextStep, args, result, false, 0)
}

// heuristicChoice is what the heuristic did for one cell: the recipes it
// put there and a reason code (cook_new, leftover_fixed_batch,
// leftover_proposed_batch, leftover_grown_batch, leftover_of_new_batch,
// frozen_portion, side_swap) or why it left the cell empty (left_empty_no_main,
// left_empty_cook_cap).
type heuristicChoice struct {
	Date   string  `json:"date"`
	SlotID int64   `json:"slot_id"`
	Foods  []int64 `json:"foods,omitempty"`
	Reason string  `json:"reason"`
}

func (h *planner) logChoice(pp ProposedPlate, code string) {
	c := heuristicChoice{Date: pp.Date.Format(time.DateOnly), SlotID: pp.SlotID, Reason: code}
	for _, pc := range pp.Components {
		c.Foods = append(c.Foods, pc.FoodID)
	}
	h.choices = append(h.choices, c)
}

// recordChoices stores the run's `heuristic.choices` step.
func (h *planner) recordChoices(ctx context.Context) {
	run := airun.FromContext(ctx)
	if run == nil {
		return
	}
	choices := h.choices
	if choices == nil {
		choices = []heuristicChoice{}
	}
	result, err := json.Marshal(choices)
	if err != nil {
		slog.WarnContext(ctx, "heuristic.choices not recorded", "error", err)
		return
	}
	run.ToolStep(context.WithoutCancel(ctx), HeuristicChoicesStep, []byte(`{}`), result, false, 0)
}
