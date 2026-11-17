package planning

import (
	"cmp"
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
	// Op is the apply_changes op that applies the option as it is.
	Op OptionOp `json:"op"`

	inBand   bool
	shareGap float64
}

// OptionOp is an option written as a valid apply_changes op: add (an
// existing plate by plate_id, else the cell by date + slot_id; a leaf takes
// amount + unit, a recipe portions), set_quantity (plate_component_id), and
// for Alternatives swap and remove.
type OptionOp struct {
	Op               string   `json:"op"`
	PlateID          *int64   `json:"plate_id,omitempty"`
	PlateComponentID *int64   `json:"plate_component_id,omitempty"`
	FoodID           int64    `json:"food_id,omitempty"`
	Date             string   `json:"date,omitempty"`
	SlotID           int64    `json:"slot_id,omitempty"`
	Portions         *float64 `json:"portions,omitempty"`
	Amount           *float64 `json:"amount,omitempty"`
	Unit             string   `json:"unit,omitempty"`
	// SessionID makes an add eat from that existing batch (Alternatives).
	SessionID *int64 `json:"session_id,omitempty"`
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

// week is the weekly ledger of ev: the average kcal of the days S2 judges
// (complete, with a plate and nutrition data) against the average of their
// scaled targets. Nil without a kcal target or such a day.
func (x *index) week(ev Evaluation) *WeekEval {
	withPlate := map[string]bool{}
	for _, p := range ev.Plates {
		withPlate[p.Date] = true
	}
	var w WeekEval
	fiberMissing := false
	counted := map[string]bool{}
	for _, d := range ev.Days {
		if !d.Complete || d.Away || !withPlate[d.Date] || d.NutritionMissing || d.Kcal <= 0 || d.TargetKcal == nil || *d.TargetKcal <= 0 {
			continue
		}
		w.Days++
		counted[d.Date] = true
		w.AvgKcal += d.Kcal
		w.TargetKcal += *d.TargetKcal
		w.NormalTargetKcal += cmp.Or(d.NormalTargetKcal, *d.TargetKcal)
		w.AvgProteinG += d.ProteinG
		w.TargetProteinG += d.ProteinTargetG
		w.CarbsPct += d.CarbsPct * d.Kcal
		w.FatPct += d.FatPct * d.Kcal
		w.FiberG += d.FiberG
		fiberMissing = fiberMissing || d.FiberMissing
	}
	if w.Days == 0 {
		return nil
	}
	n := float64(w.Days)
	w.Pct = w.AvgKcal / w.TargetKcal * 100
	if w.TargetProteinG > 0 {
		w.ProteinPct = w.AvgProteinG / w.TargetProteinG * 100
	}
	// The macro shares are weighted by the days' kcal.
	w.CarbsPct /= w.AvgKcal
	w.FatPct /= w.AvgKcal
	w.AvgKcal /= n
	w.TargetKcal /= n
	if w.NormalTargetKcal /= n; math.Abs(w.NormalTargetKcal-w.TargetKcal) < 1e-9 {
		w.NormalTargetKcal = 0
	}
	w.AvgProteinG /= n
	w.TargetProteinG /= n
	w.FiberG /= n
	if fiberMissing {
		// A partial average would mislead: show none.
		w.FiberG = 0
	}
	x.weekGroups(ev, counted, &w)
	return &w
}

// weekGroups fills the DGE food-group figures of w from the plates of the
// counted days. Grams are per eater (plate components are per eater).
func (x *index) weekGroups(ev Evaluation, counted map[string]bool, w *WeekEval) {
	var total FoodGroups
	for _, p := range ev.Plates {
		if !counted[p.Date] {
			continue
		}
		var g FoodGroups
		for _, c := range p.Components {
			f, ok := x.food(c.FoodID)
			if !ok {
				continue
			}
			m := 0.0
			switch {
			case c.Portions != nil:
				m = *c.Portions
			case c.Grams != nil:
				m = *c.Grams / 100
			}
			g = g.add(f.Groups.times(m))
		}
		total = total.add(g)
		if g.FishG >= minMealG {
			w.FishMeals++
		}
		if g.LegumeG >= minMealG {
			w.LegumeMeals++
		}
	}
	n := float64(w.Days)
	w.MeatG = total.MeatG
	w.ProduceGPerDay = total.ProduceG / n
}

// dgeIssues are the weekly DGE food-group soft issues (S7): meat above the
// limit prorated to the counted days, no fish meal, no legume meal. They need
// minDGEDays complete days, so a half-planned week raises none. No date and
// no options. Meat and fish are skipped when the diet type rules them
// out. Fruit and vegetables are display only (FoodGroups.ProduceG).
func (x *index) dgeIssues(w *WeekEval) []Issue {
	var out []Issue
	if w.Days < minDGEDays {
		return nil
	}
	dt := diet.Class(x.s.Profile.DietType)
	meatOK := dt == "" || diet.Compatible(diet.Omnivore, dt)
	fishOK := dt == "" || diet.Compatible(diet.Pescetarian, dt)
	if meatOK && w.MeatG > DGEMeatMaxG*float64(w.Days)/weekDays {
		out = append(out, issue("S7", CodeWeekMeatAboveDGE, SeveritySoft).
			with("days", w.Days).with("meat_g", math.Round(w.MeatG)).with("max_g", math.Round(DGEMeatMaxG*float64(w.Days)/weekDays)))
	}
	if fishOK && w.FishMeals == 0 {
		out = append(out, issue("S7", CodeWeekFishBelowDGE, SeveritySoft).with("days", w.Days).with("fish_meals", 0).with("min_meals", 1))
	}
	if w.LegumeMeals == 0 {
		out = append(out, issue("S7", CodeWeekLegumesBelow, SeveritySoft).with("days", w.Days).with("legume_meals", 0).with("min_meals", 1))
	}
	return out
}

// softIssues reports the soft rules S2 (day kcal outside the band of its
// scaled target) and S3 (protein share below the profile's minus the
// margin) for every complete day of ev with a plate. Each carries its
// options.
func (x *index) softIssues(ev Evaluation, t Target) []Issue {
	var out []Issue
	proteinTarget := x.s.Profile.ProteinPct
	perKg := x.pol.ProteinGPerKg > 0
	withPlate := map[string]bool{}
	for _, p := range ev.Plates {
		withPlate[p.Date] = true
	}
	for _, d := range ev.Days {
		if !d.Complete || d.Away || !withPlate[d.Date] || d.NutritionMissing || d.Kcal <= 0 {
			continue
		}
		day, _ := time.Parse(time.DateOnly, d.Date)
		if d.TargetKcal != nil && *d.TargetKcal > 0 {
			target := *d.TargetKcal
			code := ""
			switch {
			case d.Kcal < target*x.pol.DayLow:
				code = CodeKcalBelowBand
			case d.Kcal > target*x.pol.DayHigh:
				code = CodeKcalAboveBand
			}
			if code != "" {
				is := issue("S2", code, SeveritySoft).onDate(day).
					with("kcal", math.Round(d.Kcal)).with("target_kcal", math.Round(target)).
					with("gap_kcal", math.Round(target-d.Kcal)).
					with("band_low_kcal", math.Round(target*x.pol.DayLow)).with("band_high_kcal", math.Round(target*x.pol.DayHigh))
				if code == CodeKcalBelowBand {
					is.Options = x.options(ev, t, d, day, false)
				}
				out = append(out, is)
			}
		}
		if perKg {
			// The g/kg target replaces the share margin.
			if lo := d.ProteinTargetG * ProteinDayMin; d.ProteinG < lo {
				is := issue("S3", CodeProteinLow, SeveritySoft).onDate(day).
					with("protein_g", math.Round(d.ProteinG*10)/10).with("target_g", math.Round(d.ProteinTargetG*10)/10).
					with("min_g", math.Round(lo*10)/10).with("protein_pct", math.Round(d.ProteinPct*10)/10)
				is.Options = x.options(ev, t, d, day, true)
				out = append(out, is)
			}
		} else if proteinTarget != nil && *proteinTarget > 0 && d.ProteinPct < *proteinTarget-ProteinMarginPts {
			is := issue("S3", CodeProteinLow, SeveritySoft).onDate(day).
				with("protein_pct", math.Round(d.ProteinPct*10)/10).with("target_pct", *proteinTarget).
				with("min_pct", *proteinTarget-ProteinMarginPts)
			is.Options = x.options(ev, t, d, day, true)
			out = append(out, is)
		}
	}
	out = append(out, x.lightIssues(ev)...)
	out = append(out, x.dishIssues()...)
	// S4: the weekly average outside the policy's week band. No date and no
	// options: the model decides which days to move.
	if w := ev.Week; w != nil && w.Days >= MinWeekDays {
		if code := x.pol.WeekCode(w.Pct / 100); code != "" {
			out = append(out, issue("S4", code, SeveritySoft).
				with("days", w.Days).with("avg_kcal", math.Round(w.AvgKcal)).with("target_kcal", math.Round(w.TargetKcal)).
				with("pct", math.Round(w.Pct*10)/10).
				with("band_low_pct", math.Round(x.pol.WeekLow*100)).with("band_high_pct", math.Round(x.pol.WeekHigh*100)))
		}
		out = append(out, x.weekMacroIssues(w)...)
		out = append(out, x.dgeIssues(w)...)
	}
	return out
}

// weekMacroIssues are the weekly soft issues of the protein average (against
// the g/kg target) and of the carb and fat shares (against the policy's
// ranges). No date and no options: the model decides which days to change.
func (x *index) weekMacroIssues(w *WeekEval) []Issue {
	var out []Issue
	if x.pol.ProteinGPerKg > 0 && w.TargetProteinG > 0 && w.ProteinPct < ProteinWeekMin*100-1e-9 {
		out = append(out, issue("S3", CodeWeekProteinLow, SeveritySoft).
			with("days", w.Days).with("avg_protein_g", math.Round(w.AvgProteinG*10)/10).
			with("target_protein_g", math.Round(w.TargetProteinG*10)/10).with("pct", math.Round(w.ProteinPct*10)/10))
	}
	for _, m := range []struct {
		code      string
		pct       float64
		low, high float64
	}{{CodeWeekCarbsRange, w.CarbsPct, x.pol.CarbsLow, x.pol.CarbsHigh}, {CodeWeekFatRange, w.FatPct, x.pol.FatLow, x.pol.FatHigh}} {
		if m.pct < m.low-1e-9 || m.pct > m.high+1e-9 {
			out = append(out, issue("S5", m.code, SeveritySoft).
				with("days", w.Days).with("pct", math.Round(m.pct*10)/10).
				with("range_low_pct", math.Round(m.low)).with("range_high_pct", math.Round(m.high)))
		}
	}
	return out
}

// options returns the verified ways to close day d's gap, at most
// maxOptions. An option is verified by re-checking the plan with it
// applied: it must add no hard issue (so H2 and H3 never get worse), keep
// the day within the kcal band, and move the day the right way: an S3
// option (protein) must raise the day's protein share, an S2 option must
// bring the day's kcal closer to its target. Options that bring the day into
// the band come first, then the larger protein share gain, then the better
// fit.
func (x *index) options(ev Evaluation, t Target, d DayEval, day time.Time, protein bool) []Option {
	target := 0.0
	if d.TargetKcal != nil {
		target = *d.TargetKcal
	}
	want := target - d.Kcal
	if want <= 0 {
		want = target*x.pol.DayHigh - d.Kcal
	}
	if want <= 0 {
		return []Option{}
	}
	base := hardKeys(ev.Issues)
	cands := x.candidates(day, want)
	for i := range cands {
		capLeafAdd(&cands[i], x.s.Foods[cands[i].add.FoodID], target)
	}
	slices.SortStableFunc(cands, func(a, b optCand) int {
		return cmp.Or(cmp.Compare(b.protein, a.protein), cmp.Compare(math.Abs(want-a.kcal), math.Abs(want-b.kcal)))
	})
	var ok []Option
	for i, c := range cands {
		if i >= maxVerified {
			break
		}
		if o, good := x.verify(c, t, base, d, target, protein); good {
			ok = append(ok, o)
		}
	}
	slices.SortStableFunc(ok, func(a, b Option) int {
		if a.inBand != b.inBand {
			if a.inBand {
				return -1
			}
			return 1
		}
		return cmp.Or(cmp.Compare(b.shareGap, a.shareGap), cmp.Compare(math.Abs(target-a.DayKcal), math.Abs(target-b.DayKcal)))
	})
	if len(ok) > maxOptions {
		ok = ok[:maxOptions]
	}
	if ok == nil {
		ok = []Option{}
	}
	return ok
}

// optCand is an unverified option: the plate edit and its figures.
type optCand struct {
	opt     Option
	plate   int // index in s.Plates
	comp    int // component index for a raise, -1 for an add
	compID  int64
	add     Component
	kcal    float64
	protein float64
}

// op writes c as an apply_changes op.
func (c optCand) op(plateID *int64, date string, slot int64) OptionOp {
	o := c.opt
	if c.comp >= 0 {
		id := c.compID
		op := OptionOp{Op: "set_quantity", PlateComponentID: &id, Portions: o.Portions}
		if o.Grams != nil {
			op.Amount, op.Unit = o.Grams, "g"
		}
		return op
	}
	op := OptionOp{Op: "add", FoodID: o.FoodID, PlateID: plateID, Portions: o.Portions}
	if plateID == nil {
		op.Date, op.SlotID = date, slot
	}
	if o.Grams != nil {
		op.Amount, op.Unit = o.Grams, "g"
	}
	return op
}

// capLeafAdd caps a leaf add at the heuristic's per-add cap (dayAdds):
// maxAddFillerGrams and fillerDayShare of the day's target. An add that
// cannot close the gap alone is still offered at the cap; its day figures
// show the partial effect.
func capLeafAdd(c *optCand, f FoodFacts, target float64) {
	if c.comp >= 0 || c.add.Grams == nil || f.Macros.Kcal <= 0 {
		return
	}
	g := min(*c.add.Grams, maxAddFillerGrams)
	if target > 0 {
		g = min(g, math.Floor(target*fillerDayShare/f.Macros.Kcal*100/roundGrams)*roundGrams)
	}
	g = max(g, roundGrams)
	*c.add.Grams = g
	c.kcal, c.protein = f.Macros.Kcal*g/100, f.Macros.Protein*g/100
}

func (x *index) editablePlates(day time.Time) []int {
	var out []int
	for i, p := range x.s.Plates {
		// A light meal stays light: no option or top-up adds to its plate.
		if dayOf(p.Date) == dayOf(day) && !p.Confirmed && x.s.Window.Contains(p.Date) && !x.routine(p.Cell()) && !x.s.lightMeal(p.Date, p.SlotID) {
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
			c := optCand{plate: pi, comp: ci, compID: comp.ID}
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
func (x *index) verify(c optCand, t Target, base map[string]bool, d DayEval, target float64, protein bool) (Option, bool) {
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
	if after == nil || (target > 0 && after.Kcal > target*x.pol.DayHigh) {
		return Option{}, false
	}
	pct := 0.0
	if after.Kcal > 0 {
		pct = after.ProteinG * proteinKcalPerG / after.Kcal * 100
	}
	perKg := x.pol.ProteinGPerKg > 0
	if protein && ((perKg && after.ProteinG <= d.ProteinG) || (!perKg && pct <= d.ProteinPct)) {
		return Option{}, false
	}
	if !protein && target > 0 && math.Abs(target-after.Kcal) >= math.Abs(target-d.Kcal) {
		return Option{}, false
	}
	o := c.opt
	o.inBand, o.shareGap = target <= 0 || after.Kcal >= target*x.pol.DayLow, pct-d.ProteinPct
	if perKg {
		o.shareGap = after.ProteinG - d.ProteinG
	}
	o.Date, o.SlotID = p.Date.Format(time.DateOnly), p.SlotID
	if !p.Proposed() {
		id := p.ID
		o.PlateID = &id
	}
	o.Op = c.op(o.PlateID, o.Date, o.SlotID)
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
