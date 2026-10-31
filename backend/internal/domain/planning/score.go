package planning

import (
	"math"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain/availability"
)

// Score holds the quality components of a plan (spec §6.5, D23). There is
// no weighted total.
type Score struct {
	// Coverage is filled plan cells / plan cells of the target (1 without
	// plan cells).
	Coverage float64 `json:"coverage"`
	// HardViolations counts violations by rule.
	HardViolations map[string]int `json:"hard_violations"`
	MacroDeviation MacroDeviation `json:"macro_deviation"`
	CookOccasions  CookOccasions  `json:"cook_occasions"`
	Variety        Variety        `json:"variety"`
	// PreferenceFit sums, over the target's cooks, 1 per favorite plus the
	// recipe's like score.
	PreferenceFit float64 `json:"preference_fit"`
	// WastePortions are the target sessions' portions neither frozen nor
	// eaten fresh.
	WastePortions float64 `json:"waste_portions"`
	// UnknownDataShare is the share of foods on target plates with an
	// unknown diet class or incomplete nutrition.
	UnknownDataShare float64 `json:"unknown_data_share"`
}

// MacroDeviation is the mean absolute % deviation of kcal and of the P/F/C
// targets that are set, per eater: averaged over the days (DailyPct) and of
// the days' average (WeeklyPct). Nil without a kcal target or without
// planned days.
type MacroDeviation struct {
	DailyPct  *float64 `json:"daily_pct"`
	WeeklyPct *float64 `json:"weekly_pct"`
}

// CookOccasions are the cooking occasions per target day (YYYY-MM-DD) and
// the number of target days with at least one leftover or frozen plate.
type CookOccasions struct {
	PerDay           map[string]int `json:"per_day"`
	DaysWithPrepared int            `json:"days_with_prepared"`
}

// Variety counts the distinct mains cooked in the target, the H8 repeats
// among them and the never-eaten ones (D15).
type Variety struct {
	Distinct   int `json:"distinct"`
	Repeats    int `json:"repeats"`
	NewRecipes int `json:"new_recipes"`
}

// ComputeScore scores the target of s (usually after Merge) given the
// issues Check returned for it.
func ComputeScore(s State, t Target, issues []Issue) Score {
	x := newIndex(s, t)
	sc := Score{
		Coverage:       x.coverage(),
		HardViolations: map[string]int{},
		MacroDeviation: x.macroDeviation(),
		CookOccasions:  x.cookOccasions(),
	}
	for _, is := range issues {
		if is.Severity == SeverityViolation {
			sc.HardViolations[is.Rule]++
		}
		if is.Rule == "H8" {
			sc.Variety.Repeats++
		}
	}
	mains := map[int64]bool{}
	for _, c := range x.cooks() {
		f := x.s.Foods[c.foodID]
		if f.Favorite {
			sc.PreferenceFit++
		}
		sc.PreferenceFit += f.LikeScore
		if varietyRole(f.Role) && !mains[f.ID] {
			mains[f.ID] = true
			if f.LastEaten == nil {
				sc.Variety.NewRecipes++
			}
		}
		sc.WastePortions += c.waste
	}
	sc.Variety.Distinct = len(mains)
	sc.UnknownDataShare = x.unknownDataShare()
	return sc
}

func (x *index) coverage() float64 {
	cells, filled := 0, 0
	for _, c := range x.s.Cells {
		if c.Mode != availability.ModePlan || !x.inTarget(c.Key()) {
			continue
		}
		cells++
		if len(x.byCell[idOf(c.Key())]) > 0 {
			filled++
		}
	}
	if cells == 0 {
		return 1
	}
	return float64(filled) / float64(cells)
}

// cook is one recipe cooked on a target cell: a session, or a composed
// component without one (cooked fresh for its plate, no waste).
type cook struct {
	foodID int64
	waste  float64
}

func (x *index) cooks() []cook {
	var out []cook
	for _, ss := range x.s.Sessions {
		if _, ok := x.food(ss.FoodID); !ok || !x.inTarget(ss.CookCell()) {
			continue
		}
		use := x.usage(ss)
		waste := (ss.Portions - ss.FrozenPortions) - (use.Total - use.Frozen)
		out = append(out, cook{foodID: ss.FoodID, waste: math.Max(0, waste)})
	}
	for _, p := range x.s.Plates {
		if !x.inTarget(p.Cell()) {
			continue
		}
		for _, c := range p.Components {
			if _, linked := refOf(c); linked || c.Portions == nil {
				continue
			}
			if _, ok := x.food(c.FoodID); ok {
				out = append(out, cook{foodID: c.FoodID})
			}
		}
	}
	return out
}

func (x *index) macroDeviation() MacroDeviation {
	p := x.s.Profile
	if p.KcalTarget == nil || *p.KcalTarget <= 0 {
		return MacroDeviation{}
	}
	kcal := *p.KcalTarget
	targets := []float64{kcal}
	for _, m := range []struct {
		pct         *float64
		kcalPerGram float64
	}{{p.ProteinPct, 4}, {p.FatPct, 9}, {p.CarbsPct, 4}} {
		if m.pct != nil && *m.pct > 0 {
			targets = append(targets, kcal*(*m.pct)/100/m.kcalPerGram)
		}
	}
	// Per-eater intake per target day with a plate, in targets' order.
	var days [][]float64
	for _, d := range x.days {
		intake := make([]float64, 4)
		planned := false
		for _, pl := range x.s.Plates {
			if dayOf(pl.Date) != dayOf(d) {
				continue
			}
			planned = true
			for _, c := range pl.Components {
				f, ok := x.food(c.FoodID)
				if !ok {
					continue
				}
				m := c.Multiplier()
				intake[0] += f.Macros.Kcal * m
				intake[1] += f.Macros.Protein * m
				intake[2] += f.Macros.Fat * m
				intake[3] += f.Macros.Carbs * m
			}
		}
		if planned {
			days = append(days, pick(intake, p))
		}
	}
	if len(days) == 0 {
		return MacroDeviation{}
	}
	avg := make([]float64, len(targets))
	daily := 0.0
	for _, day := range days {
		daily += meanDeviation(day, targets)
		for i, v := range day {
			avg[i] += v / float64(len(days))
		}
	}
	daily /= float64(len(days))
	weekly := meanDeviation(avg, targets)
	return MacroDeviation{DailyPct: &daily, WeeklyPct: &weekly}
}

// pick keeps kcal plus the macros whose target percentage is set, in the
// order macroDeviation builds its targets.
func pick(intake []float64, p Profile) []float64 {
	out := []float64{intake[0]}
	for i, pct := range []*float64{p.ProteinPct, p.FatPct, p.CarbsPct} {
		if pct != nil && *pct > 0 {
			out = append(out, intake[i+1])
		}
	}
	return out
}

func meanDeviation(actual, targets []float64) float64 {
	sum := 0.0
	for i, t := range targets {
		sum += math.Abs(actual[i]-t) / t * 100
	}
	return sum / float64(len(targets))
}

func (x *index) cookOccasions() CookOccasions {
	co := CookOccasions{PerDay: make(map[string]int, len(x.days))}
	prepared := map[int]bool{}
	for _, ss := range x.s.Sessions {
		for _, l := range x.links[ss.ref()] {
			if !l.Cell.Equal(ss.CookCell()) {
				prepared[dayOf(l.Cell.Date)] = true
			}
		}
	}
	for _, d := range x.days {
		co.PerDay[d.Format(time.DateOnly)] = len(x.occasions[dayOf(d)])
		if prepared[dayOf(d)] {
			co.DaysWithPrepared++
		}
	}
	return co
}

func (x *index) unknownDataShare() float64 {
	seen := map[int64]bool{}
	unknown := 0
	for _, p := range x.s.Plates {
		if !x.inTarget(p.Cell()) {
			continue
		}
		for _, c := range p.Components {
			f, ok := x.food(c.FoodID)
			if !ok || seen[f.ID] {
				continue
			}
			seen[f.ID] = true
			if !dietKnown(f.DietClass) || !f.NutritionComplete {
				unknown++
			}
		}
	}
	if len(seen) == 0 {
		return 0
	}
	return float64(unknown) / float64(len(seen))
}
