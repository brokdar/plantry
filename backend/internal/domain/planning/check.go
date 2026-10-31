package planning

import (
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain/availability"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
)

// Severity tells whether an issue breaks a hard rule or only warns.
type Severity string

const (
	SeverityViolation Severity = "violation"
	SeverityWarning   Severity = "warning"
)

// Issue codes (D22). H3/H4 reuse the session warning codes of plate.
const (
	CodeMissingPlate          = "missing_plate"
	CodeExtraPlate            = "extra_plate"
	CodePlateInNoneCell       = "plate_in_none_cell"
	CodeTooManyCookOccasions  = "too_many_cook_occasions"
	CodeDietIncompatible      = "diet_incompatible"
	CodeMeatDaysExceeded      = "meat_days_exceeded"
	CodeDietUnknown           = "diet_unknown"
	CodeConfirmedInTarget     = "confirmed_in_target"
	CodeSessionOutsideTarget  = "session_outside_target"
	CodeUnknownFood           = "unknown_food"
	CodeUnknownSession        = "unknown_session"
	CodeNotComposed           = "not_composed"
	CodeSessionFoodMismatch   = "session_food_mismatch"
	CodeDuplicateSessionKey   = "duplicate_session_key"
	CodeInvalidPortions       = "invalid_portions"
	CodePlateWithoutComponent = "plate_without_components"
	CodePlateOutsideTarget    = "plate_outside_target"
	CodePlateOnConfirmedCell  = "plate_on_confirmed_cell"
	CodeVarietyRepeat         = "variety_repeat"
	CodeVarietyRelaxed        = "variety_relaxed"
)

// Issue is one finding of the checker (D22). Plates and sessions of a
// proposal have no id yet: their issues carry the cell, and a proposed
// session's key is in Params["session_key"].
type Issue struct {
	Rule      string         `json:"rule"`
	Code      string         `json:"code"`
	Severity  Severity       `json:"severity"`
	Date      string         `json:"date,omitempty"`
	SlotID    *int64         `json:"slot_id,omitempty"`
	PlateID   *int64         `json:"plate_id,omitempty"`
	SessionID *int64         `json:"session_id,omitempty"`
	Params    map[string]any `json:"params"`
}

// Check applies the hard rules H1–H8 (spec §6.4, D21) to the target of s and
// returns the issues in rule order. Pass the state after Merge to check a
// proposal, or the persisted state to check a window as it is.
func Check(s State, t Target) []Issue {
	x := newIndex(s, t)
	out := []Issue{}
	out = append(out, x.h1()...)
	out = append(out, x.h2()...)
	out = append(out, x.h3h4()...)
	out = append(out, x.h5()...)
	out = append(out, x.h6()...)
	out = append(out, x.h7()...)
	out = append(out, x.h8()...)
	return out
}

func issue(rule, code string, sev Severity) Issue {
	return Issue{Rule: rule, Code: code, Severity: sev, Params: map[string]any{}}
}

func (i Issue) atCell(c plate.CellKey) Issue {
	slot := c.SlotID
	i.Date = c.Date.Format(time.DateOnly)
	i.SlotID = &slot
	return i
}

func (i Issue) onDate(d time.Time) Issue {
	i.Date = d.Format(time.DateOnly)
	return i
}

func (i Issue) onPlate(p Plate) Issue {
	i = i.atCell(p.Cell())
	if !p.Proposed() {
		id := p.ID
		i.PlateID = &id
	}
	return i
}

func (i Issue) onSession(s Session) Issue {
	i = i.atCell(s.CookCell())
	if s.Proposed() {
		i.Params["session_key"] = s.Key
	} else {
		id := s.ID
		i.SessionID = &id
	}
	return i
}

func (i Issue) with(k string, v any) Issue {
	i.Params[k] = v
	return i
}

// h1: every plan or routine cell in the target holds exactly one plate; no
// plate sits in a none cell.
func (x *index) h1() []Issue {
	var out []Issue
	for _, c := range x.s.Cells {
		key := c.Key()
		if !x.inTarget(key) {
			continue
		}
		plates := x.byCell[idOf(key)]
		switch {
		case c.Mode == availability.ModeNone:
			for _, pi := range plates {
				out = append(out, issue("H1", CodePlateInNoneCell, SeverityViolation).onPlate(x.s.Plates[pi]))
			}
		case len(plates) == 0:
			out = append(out, issue("H1", CodeMissingPlate, SeverityViolation).atCell(key).with("mode", string(c.Mode)))
		case len(plates) > 1:
			out = append(out, issue("H1", CodeExtraPlate, SeverityViolation).atCell(key).with("count", len(plates)))
		}
	}
	return out
}

// h2: cooking occasions per target day stay within the setting.
func (x *index) h2() []Issue {
	limit := x.s.Settings.MaxCookOccasionsPerDay
	if limit <= 0 {
		return nil
	}
	var out []Issue
	for _, d := range x.days {
		if n := len(x.occasions[dayOf(d)]); n > limit {
			out = append(out, issue("H2", CodeTooManyCookOccasions, SeverityViolation).onDate(d).with("count", n).with("max", limit))
		}
	}
	return out
}

// h3h4: the target's sessions are not over-consumed (H3) and their leftover
// and frozen plates sit after the cook cell and within keep days (H4), as
// plate.Warnings reports them.
func (x *index) h3h4() []Issue {
	var out []Issue
	for _, ss := range x.s.Sessions {
		if !x.checked(ss) {
			continue
		}
		links := x.links[ss.ref()]
		for _, w := range plate.Warnings(ss.CookSession, links, x.keepDays(ss.FoodID), x.s.Settings.BatchSettings, x.s.SlotOrder) {
			rule := "H3"
			if w.Code == plate.WarnLeftoverBeforeCook || w.Code == plate.WarnPastKeepDays {
				rule = "H4"
			}
			is := issue(rule, w.Code, SeverityViolation).onSession(ss)
			if w.PlateID != nil {
				p := x.s.Plates[x.byPlate[*w.PlateID]]
				sessionID := is.SessionID
				is = is.onPlate(p)
				is.SessionID = sessionID
			}
			out = append(out, is)
		}
	}
	return out
}

// dietRank orders the diet classes from most to least restrictive.
var dietRank = map[string]int{"vegan": 0, "vegetarian": 1, "pescetarian": 2, "omnivore": 3}

// dietKnown reports whether class is one of the four classes.
func dietKnown(class string) bool {
	_, ok := dietRank[class]
	return ok
}

// dietCompatible reports whether a food of a known class fits the diet
// type. An empty diet type allows everything.
func dietCompatible(class string, t profile.DietType) bool {
	limit, ok := dietRank[string(t)]
	return !ok || dietRank[class] <= limit
}

// h5: every food on a target plate fits the diet type (unknown is a
// warning) and the window's days with an omnivore food stay within
// meat_days_per_week_max.
func (x *index) h5() []Issue {
	var out []Issue
	meatDays := map[int]bool{}
	for _, p := range x.s.Plates {
		inWindow := x.s.Window.Contains(p.Date)
		inTarget := x.inTarget(p.Cell())
		seen := map[int64]bool{}
		for _, c := range p.Components {
			f, ok := x.food(c.FoodID)
			if !ok {
				continue
			}
			if inWindow && f.DietClass == "omnivore" {
				meatDays[dayOf(p.Date)] = true
			}
			if !inTarget || seen[c.FoodID] {
				continue
			}
			seen[c.FoodID] = true
			switch {
			case !dietKnown(f.DietClass):
				out = append(out, issue("H5", CodeDietUnknown, SeverityWarning).onPlate(p).with("food_id", f.ID))
			case !dietCompatible(f.DietClass, x.s.Profile.DietType):
				out = append(out, issue("H5", CodeDietIncompatible, SeverityViolation).onPlate(p).
					with("food_id", f.ID).with("diet_class", f.DietClass).with("diet_type", string(x.s.Profile.DietType)))
			}
		}
	}
	if limit := x.s.Profile.MeatDaysPerWeekMax; limit != nil && len(meatDays) > *limit {
		out = append(out, issue("H5", CodeMeatDaysExceeded, SeverityViolation).
			onDate(x.s.Window.Start).with("days", len(meatDays)).with("max", *limit))
	}
	return out
}

// h6: the target names no confirmed row, and proposed sessions cook on a
// target cell.
func (x *index) h6() []Issue {
	var out []Issue
	for _, id := range x.t.PlateIDs {
		if pi, ok := x.byPlate[id]; ok && x.s.Plates[pi].Confirmed {
			out = append(out, issue("H6", CodeConfirmedInTarget, SeverityViolation).onPlate(x.s.Plates[pi]))
		}
	}
	for _, id := range x.t.SessionIDs {
		if si, ok := x.byRef[sessionRef{id: id}]; ok && x.s.Sessions[si].Confirmed {
			out = append(out, issue("H6", CodeConfirmedInTarget, SeverityViolation).onSession(x.s.Sessions[si]))
		}
	}
	for _, ss := range x.s.Sessions {
		if ss.Proposed() && !x.inTarget(ss.CookCell()) {
			out = append(out, issue("H6", CodeSessionOutsideTarget, SeverityViolation).onSession(ss))
		}
	}
	return out
}

// h7: every id exists, sessions and proposed components are recipes, links
// match their session's food, portions are positive, every plate has a
// component, and proposed plates sit on target cells that hold no confirmed
// plate.
func (x *index) h7() []Issue {
	var out []Issue
	for _, key := range x.dupKeys {
		out = append(out, issue("H7", CodeDuplicateSessionKey, SeverityViolation).with("session_key", key))
	}
	for _, ss := range x.s.Sessions {
		if !ss.Proposed() {
			continue
		}
		f, ok := x.food(ss.FoodID)
		switch {
		case !ok:
			out = append(out, issue("H7", CodeUnknownFood, SeverityViolation).onSession(ss).with("food_id", ss.FoodID))
		case f.Kind != food.KindComposed:
			out = append(out, issue("H7", CodeNotComposed, SeverityViolation).onSession(ss).with("food_id", ss.FoodID))
		}
		if !(ss.Portions > 0) || ss.FrozenPortions < 0 || ss.FrozenPortions > ss.Portions {
			out = append(out, issue("H7", CodeInvalidPortions, SeverityViolation).onSession(ss))
		}
	}
	for _, p := range x.s.Plates {
		if len(p.Components) == 0 {
			out = append(out, issue("H7", CodePlateWithoutComponent, SeverityViolation).onPlate(p))
		}
		if p.Proposed() {
			out = append(out, x.placement(p)...)
			if !(p.Eaters > 0) {
				out = append(out, issue("H7", CodeInvalidPortions, SeverityViolation).onPlate(p).with("eaters", p.Eaters))
			}
		}
		for _, c := range p.Components {
			out = append(out, x.component(p, c)...)
		}
	}
	return out
}

// placement checks where a proposed plate sits.
func (x *index) placement(p Plate) []Issue {
	for _, pi := range x.byCell[idOf(p.Cell())] {
		if x.s.Plates[pi].Confirmed {
			return []Issue{issue("H7", CodePlateOnConfirmedCell, SeverityViolation).onPlate(p)}
		}
	}
	if !x.inTarget(p.Cell()) {
		return []Issue{issue("H7", CodePlateOutsideTarget, SeverityViolation).onPlate(p)}
	}
	return nil
}

// component checks one component's food and session link.
func (x *index) component(p Plate, c Component) []Issue {
	f, ok := x.food(c.FoodID)
	if !ok {
		return []Issue{issue("H7", CodeUnknownFood, SeverityViolation).onPlate(p).with("food_id", c.FoodID)}
	}
	var out []Issue
	if p.Proposed() {
		if f.Kind != food.KindComposed {
			out = append(out, issue("H7", CodeNotComposed, SeverityViolation).onPlate(p).with("food_id", c.FoodID))
		}
		if c.Portions == nil || !(*c.Portions > 0) {
			out = append(out, issue("H7", CodeInvalidPortions, SeverityViolation).onPlate(p).with("food_id", c.FoodID))
		}
	}
	r, linked := refOf(c)
	if !linked {
		return out
	}
	si, ok := x.byRef[r]
	switch {
	case !ok:
		is := issue("H7", CodeUnknownSession, SeverityViolation).onPlate(p)
		if r.key != "" {
			is = is.with("session_key", r.key)
		} else {
			is = is.with("session_id", r.id)
		}
		out = append(out, is)
	case x.s.Sessions[si].FoodID != c.FoodID:
		out = append(out, issue("H7", CodeSessionFoodMismatch, SeverityViolation).onPlate(p).
			with("food_id", c.FoodID).with("session_food_id", x.s.Sessions[si].FoodID))
	}
	return out
}

// h8: no unconfirmed cook in the target (a session, or a composed
// component cooked fresh without one) cooks a main eaten, or cooked earlier,
// within variety_weeks. Routine cells repeat by design and are skipped. When fewer eligible recipes exist than such cooks,
// repeats are warnings (variety_relaxed).
func (x *index) h8() []Issue {
	weeks := x.s.Settings.VarietyWeeks
	if weeks <= 0 {
		return nil
	}
	span := 7 * weeks
	var subject []int
	for i, ev := range x.cooks {
		f, ok := x.food(ev.foodID)
		if ev.confirmed || !ok || !f.Composed() || !varietyRole(f.Role) || !x.inTarget(ev.cell) || x.routine(ev.cell) {
			continue
		}
		subject = append(subject, i)
	}
	if len(subject) == 0 {
		return nil
	}
	code, sev := CodeVarietyRepeat, SeverityViolation
	if x.eligibleRecipes(span) < len(subject) {
		code, sev = CodeVarietyRelaxed, SeverityWarning
	}
	var out []Issue
	for _, i := range subject {
		ev := x.cooks[i]
		since := ev.cell.Date.AddDate(0, 0, -span)
		var prev time.Time
		var repeat bool
		if le := x.s.Foods[ev.foodID].LastEaten; le != nil && !le.Before(since) && le.Before(ev.cell.Date) {
			prev, repeat = *le, true
		} else {
			prev, repeat = x.earlierCook(i, since)
		}
		if !repeat {
			continue
		}
		is := issue("H8", code, sev)
		if ev.session >= 0 {
			is = is.onSession(x.s.Sessions[ev.session])
		} else {
			is = is.onPlate(x.s.Plates[ev.plate])
		}
		out = append(out, is.with("food_id", ev.foodID).with("last_eaten", prev.Format(time.DateOnly)))
	}
	return out
}

// earlierCook finds another cook of cook i's food at an earlier cell, on or
// after since.
func (x *index) earlierCook(i int, since time.Time) (time.Time, bool) {
	ev := x.cooks[i]
	for j, o := range x.cooks {
		if j == i || o.foodID != ev.foodID || o.cell.Date.Before(since) || x.routine(o.cell) {
			continue
		}
		if plate.Before(o.cell, ev.cell, x.s.SlotOrder) {
			return o.cell.Date, true
		}
	}
	return time.Time{}, false
}

// eligibleRecipes counts the recipes H8 could pick for the window: mains
// that fit the diet and were not eaten within span days before its start.
func (x *index) eligibleRecipes(span int) int {
	since := x.s.Window.Start.AddDate(0, 0, -span)
	n := 0
	for _, f := range x.s.Foods {
		if !f.Composed() || !varietyRole(f.Role) || !dietCompatible(f.DietClass, x.s.Profile.DietType) {
			continue
		}
		if f.LastEaten == nil || f.LastEaten.Before(since) {
			n++
		}
	}
	return n
}
