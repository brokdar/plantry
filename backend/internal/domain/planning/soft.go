package planning

import (
	"fmt"
	"math"
	"slices"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain/diet"
)

// Option is one computed way to close a soft rule's gap on a day: add a
// no-cook food to a plate, or raise the amount of a component already on
// it. Figures are per eater; the day figures are the day after the option.
type Option struct {
	// Action is "add" or "raise".
	Action string `json:"action"`
	FoodID int64  `json:"food_id"`
	Name   string `json:"name"`
	Date   string `json:"date"`
	SlotID int64  `json:"slot_id"`
	// PlateID is nil for a proposed plate (address it by date and slot).
	PlateID *int64 `json:"plate_id"`
	// Portions or Grams: the amount to add, or the new amount of a raise
	// (From is the current one).
	Portions      *float64 `json:"portions,omitempty"`
	Grams         *float64 `json:"grams,omitempty"`
	From          *float64 `json:"from,omitempty"`
	KcalDelta     float64  `json:"kcal_delta"`
	ProteinDeltaG float64  `json:"protein_delta_g"`
	DayKcal       float64  `json:"day_kcal"`
	DayProteinPct float64  `json:"day_protein_pct"`
	// CookOccasions is the day's count after the option (never above the
	// H2 limit); NewMeatDay is true when the option makes a meat-free day
	// a meat day.
	CookOccasions int  `json:"cook_occasions"`
	NewMeatDay    bool `json:"new_meat_day"`
}

const (
	maxOptions      = 3
	maxVerified     = 16
	minAddGrams     = 50
	maxAddGrams     = 300
	raiseFactor     = 1.25
	roundGrams      = 10
	roundPortions   = 0.25
	minAddPortions  = 0.5
	maxAddPortions  = 2.0
	proteinKcalPerG = 4
)

// softIssues reports the soft rules S2 (day kcal outside the band of its
// scaled target) and S3 (protein share below the profile's minus the
// margin) for every complete day of ev with a plate. Each carries its
// options.
func (x *index) softIssues(ev Evaluation, t Target) []Issue {
	var out []Issue
	proteinTarget := x.s.Profile.ProteinPct
	withPlate := map[string]bool{}
	for _, p := range ev.Plates {
		withPlate[p.Date] = true
	}
	for _, d := range ev.Days {
		if !d.Complete || !withPlate[d.Date] || d.NutritionMissing || d.Kcal <= 0 {
			continue
		}
		day, _ := time.Parse(time.DateOnly, d.Date)
		if d.TargetKcal != nil && *d.TargetKcal > 0 {
			target := *d.TargetKcal
			code := ""
			switch {
			case d.Kcal < target*KcalBandLow:
				code = CodeKcalBelowBand
			case d.Kcal > target*KcalBandHigh:
				code = CodeKcalAboveBand
			}
			if code != "" {
				is := issue("S2", code, SeveritySoft).onDate(day).
					with("kcal", math.Round(d.Kcal)).with("target_kcal", math.Round(target)).
					with("gap_kcal", math.Round(target-d.Kcal)).
					with("band_low_kcal", math.Round(target*KcalBandLow)).with("band_high_kcal", math.Round(target*KcalBandHigh))
				if code == CodeKcalBelowBand {
					is.Options = x.options(ev, t, d, day)
				}
				out = append(out, is)
			}
		}
		if proteinTarget != nil && *proteinTarget > 0 && d.ProteinPct < *proteinTarget-ProteinMarginPts {
			is := issue("S3", CodeProteinLow, SeveritySoft).onDate(day).
				with("protein_pct", math.Round(d.ProteinPct*10)/10).with("target_pct", *proteinTarget).
				with("min_pct", *proteinTarget-ProteinMarginPts)
			is.Options = x.options(ev, t, d, day)
			out = append(out, is)
		}
	}
	return out
}

// options ranks the verified ways to close day d's gap: by protein added,
// then by how well the kcal fit the gap, at most maxOptions. An option is
// verified by re-checking the plan with it applied: it must add no hard
// issue (so H2 and H3 never get worse), keep the day within the kcal band
// and, for a protein gap, not lower the protein share.
func (x *index) options(ev Evaluation, t Target, d DayEval, day time.Time) []Option {
	target := 0.0
	if d.TargetKcal != nil {
		target = *d.TargetKcal
	}
	want := target - d.Kcal
	if want <= 0 {
		want = target*KcalBandHigh - d.Kcal
	}
	if want <= 0 {
		return []Option{}
	}
	base := hardKeys(ev.Issues)
	cands := x.candidates(day, want)
	slices.SortStableFunc(cands, func(a, b optCand) int {
		if a.protein != b.protein {
			if a.protein > b.protein {
				return -1
			}
			return 1
		}
		return int(math.Abs(want-a.kcal) - math.Abs(want-b.kcal))
	})
	out := []Option{}
	for i, c := range cands {
		if i >= maxVerified || len(out) >= maxOptions {
			break
		}
		if o, ok := x.verify(c, t, base, d, target); ok {
			out = append(out, o)
		}
	}
	return out
}

// optCand is an unverified option: the plate edit and its figures.
type optCand struct {
	opt     Option
	plate   int // index in s.Plates
	comp    int // component index for a raise, -1 for an add
	add     Component
	kcal    float64
	protein float64
}

func (x *index) editablePlates(day time.Time) []int {
	var out []int
	for i, p := range x.s.Plates {
		if dayOf(p.Date) == dayOf(day) && !p.Confirmed && x.s.Window.Contains(p.Date) && !x.routine(p.Cell()) {
			out = append(out, i)
		}
	}
	return out
}

func (x *index) candidates(day time.Time, want float64) []optCand {
	plates := x.editablePlates(day)
	if len(plates) == 0 {
		return nil
	}
	// A new food goes on the day's last plate (dinner), where a side or
	// snack fits.
	last := plates[0]
	for _, i := range plates {
		if x.s.SlotOrder[x.s.Plates[i].SlotID] >= x.s.SlotOrder[x.s.Plates[last].SlotID] {
			last = i
		}
	}
	var out []optCand
	dietType := diet.Class(x.s.Profile.DietType)
	for _, id := range x.s.NoCook {
		f, ok := x.s.Foods[id]
		if !ok || !f.NutritionComplete || f.Macros.Kcal <= 0 {
			continue
		}
		if dietType != "" && (!diet.Valid(f.DietClass) || !diet.Compatible(f.DietClass, dietType)) {
			continue
		}
		onPlate := false
		for _, c := range x.s.Plates[last].Components {
			onPlate = onPlate || c.FoodID == id
		}
		if onPlate {
			continue
		}
		c := optCand{plate: last, comp: -1}
		if f.Composed() {
			p := clamp(math.Round(want/f.Macros.Kcal/roundPortions)*roundPortions, minAddPortions, maxAddPortions)
			c.add = Component{FoodID: id, Portions: &p, Proposed: true}
			c.opt.Portions = &p
			c.kcal, c.protein = f.Macros.Kcal*p, f.Macros.Protein*p
		} else {
			g := clamp(math.Round(want/f.Macros.Kcal*100/roundGrams)*roundGrams, minAddGrams, maxAddGrams)
			c.add = Component{FoodID: id, Grams: &g, Proposed: true}
			c.opt.Grams = &g
			c.kcal, c.protein = f.Macros.Kcal*g/100, f.Macros.Protein*g/100
		}
		c.opt.Action, c.opt.FoodID, c.opt.Name = "add", id, f.Name
		out = append(out, c)
	}
	for _, pi := range plates {
		for ci, comp := range x.s.Plates[pi].Components {
			f, ok := x.s.Foods[comp.FoodID]
			if !ok || !f.NutritionComplete {
				continue
			}
			c := optCand{plate: pi, comp: ci}
			switch {
			case comp.Portions != nil && *comp.Portions > 0:
				n := math.Round(*comp.Portions*raiseFactor/roundPortions) * roundPortions
				if n <= *comp.Portions {
					n = *comp.Portions + roundPortions
				}
				from := *comp.Portions
				c.opt.Portions, c.opt.From = &n, &from
				c.kcal, c.protein = f.Macros.Kcal*(n-from), f.Macros.Protein*(n-from)
			case comp.Grams != nil && *comp.Grams > 0:
				n := math.Round(*comp.Grams*raiseFactor/roundGrams) * roundGrams
				if n <= *comp.Grams {
					n = *comp.Grams + roundGrams
				}
				from := *comp.Grams
				c.opt.Grams, c.opt.From = &n, &from
				c.kcal, c.protein = f.Macros.Kcal*(n-from)/100, f.Macros.Protein*(n-from)/100
			default:
				continue
			}
			c.opt.Action, c.opt.FoodID, c.opt.Name = "raise", comp.FoodID, f.Name
			out = append(out, c)
		}
	}
	return out
}

// verify applies c to a copy of the plan and keeps it when the re-check
// holds; the returned option carries the day's figures after it.
func (x *index) verify(c optCand, t Target, base map[string]bool, d DayEval, target float64) (Option, bool) {
	s := x.s
	s.Plates = slices.Clone(s.Plates)
	p := s.Plates[c.plate]
	p.Components = slices.Clone(p.Components)
	if c.comp < 0 {
		p.Components = append(p.Components, c.add)
	} else {
		comp := p.Components[c.comp]
		comp.Portions, comp.Grams = c.opt.Portions, c.opt.Grams
		p.Components[c.comp] = comp
	}
	s.Plates[c.plate] = p
	for k := range hardKeys(Check(s, t)) {
		if !base[k] {
			return Option{}, false
		}
	}
	y := newIndex(s, t)
	var after *DayKcal
	for _, dk := range y.dayAccounts([]time.Time{p.Date}) {
		after = &dk
	}
	if after == nil || (target > 0 && after.Kcal > target*KcalBandHigh) {
		return Option{}, false
	}
	pct := 0.0
	if after.Kcal > 0 {
		pct = after.ProteinG * proteinKcalPerG / after.Kcal * 100
	}
	if pct < d.ProteinPct && c.protein <= 0 {
		return Option{}, false
	}
	o := c.opt
	o.Date, o.SlotID = p.Date.Format(time.DateOnly), p.SlotID
	if !p.Proposed() {
		id := p.ID
		o.PlateID = &id
	}
	o.KcalDelta, o.ProteinDeltaG = math.Round(c.kcal), math.Round(c.protein*10)/10
	o.DayKcal, o.DayProteinPct = math.Round(after.Kcal), math.Round(pct*10)/10
	o.CookOccasions = len(y.occasions[dayOf(p.Date)])
	f := x.s.Foods[c.opt.FoodID]
	o.NewMeatDay = !d.Meat && f.DietClass == diet.Omnivore
	if o.NewMeatDay && x.s.Profile.MeatDaysPerWeekMax != nil && len(y.meatDays()) > *x.s.Profile.MeatDaysPerWeekMax {
		return Option{}, false
	}
	return o, true
}

func hardKeys(issues []Issue) map[string]bool {
	m := map[string]bool{}
	for _, is := range issues {
		if is.Severity == SeverityViolation {
			m[issueID(is)] = true
		}
	}
	return m
}

func issueID(is Issue) string {
	var slot, plate, sess int64
	if is.SlotID != nil {
		slot = *is.SlotID
	}
	if is.PlateID != nil {
		plate = *is.PlateID
	}
	if is.SessionID != nil {
		sess = *is.SessionID
	}
	return fmt.Sprintf("%s|%s|%s|%d|%d|%d", is.Rule, is.Code, is.Date, slot, plate, sess)
}

func clamp(v, lo, hi float64) float64 { return math.Max(lo, math.Min(hi, v)) }
