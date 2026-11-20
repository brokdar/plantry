package agent

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
)

// The model must never need arithmetic: it subtracted a limit from a meal
// ("662 over the limit") and a day's range from a total. addDerived puts those
// differences into the results that carry both figures, as short phrases.

// vsRange says where a day's kcal lie against its range.
func vsRange(kcal, low, high float64) string {
	switch {
	case low > 0 && kcal < low-0.5:
		return fmt.Sprintf("%.0f kcal below its range", math.Round(low-kcal))
	case high > 0 && kcal > high+0.5:
		return fmt.Sprintf("%.0f kcal above its range", math.Round(kcal-high))
	}
	return "inside its range"
}

// vsLimit says where a meal's kcal lie against the most a light meal holds
// (max) or the least a filling meal holds (min); only one of them is set.
func vsLimit(kcal, max, min float64) string {
	switch {
	case max > 0 && kcal > max+0.5:
		return fmt.Sprintf("%.0f kcal more than a light meal should hold", math.Round(kcal-max))
	case max > 0:
		return "within what a light meal should hold"
	case min > 0 && kcal < min-0.5:
		return fmt.Sprintf("%.0f kcal less than a filling meal needs", math.Round(min-kcal))
	case min > 0:
		return "enough for a filling meal"
	}
	return ""
}

func num(v any) float64 {
	f, _ := v.(float64)
	return f
}

// addDerived adds the differences to a find_ways, find_alternatives or
// apply_changes result (kitchen keys, after plainRules). Anything else, or a
// result that does not parse, is returned as it is.
func addDerived(name string, raw json.RawMessage) json.RawMessage {
	if name != "find_ways" && name != "find_alternatives" && name != "apply_changes" {
		return raw
	}
	var obj map[string]any
	if json.Unmarshal(raw, &obj) != nil || obj == nil {
		return raw
	}
	low, high := num(obj["range_low"]), num(obj["range_high"])
	maxK, minK := num(obj["max_kcal"]), num(obj["min_kcal"])
	changed := false
	entry := func(m map[string]any, mealKey, dayKey string, max, min float64) {
		after, _ := m["after"].(map[string]any)
		if after == nil {
			after = m
		}
		if k, ok := after[mealKey].(float64); ok {
			if s := vsLimit(k, max, min); s != "" {
				m["meal_vs_limit"], changed = s, true
			}
		}
		// A way that moves the day's range (it ends a light mark) carries its own.
		l, h := low, high
		if own := num(m["range_low"]); own > 0 {
			l, h = own, num(m["range_high"])
		}
		if k, ok := after[dayKey].(float64); ok && (l > 0 || h > 0) {
			m["day_vs_range"], changed = vsRange(k, l, h), true
		}
	}
	switch name {
	case "find_ways":
		for _, key := range []string{"ways", "offer"} {
			list, _ := obj[key].([]any)
			for _, e := range list {
				if m, ok := e.(map[string]any); ok {
					entry(m, "meal_kcal", "day_kcal", maxK, minK)
				}
			}
		}
	case "find_alternatives":
		cur, _ := obj["current"].(map[string]any)
		cmax, cmin := 0.0, 0.0
		if cur != nil {
			cmax, cmin = num(cur["max_kcal"]), num(cur["min_kcal"])
		}
		list, _ := obj["alternatives"].([]any)
		for _, e := range list {
			if m, ok := e.(map[string]any); ok {
				entry(m, "plate_kcal", "day_kcal", cmax, cmin)
			}
		}
	case "apply_changes":
		if req, ok := obj["request"].(map[string]any); ok {
			entry(req, "meal_kcal", "day_kcal", num(req["max_kcal"]), num(req["min_kcal"]))
		}
		if cookingMovedTell(obj) {
			changed = true
		}
		if addOfferTell(obj) {
			changed = true
		}
	}
	if name == "find_ways" {
		if s := waysClosing(obj); s != "" {
			obj["closing"], changed = s, true
		}
	}
	if !changed {
		return raw
	}
	out, err := json.Marshal(obj)
	if err != nil {
		return raw
	}
	return out
}

// dryRunView is an apply_changes result as the model reads it: the first key
// is saved (true or false), the second a status sentence, and a check names
// what it computed for what it is (would_apply, plan_if_applied,
// report_if_applied), so it never looks like a write. dry_run is dropped:
// saved says it. A result without both fields is returned as it is.
func dryRunView(raw json.RawMessage) json.RawMessage {
	var obj map[string]json.RawMessage
	if json.Unmarshal(raw, &obj) != nil || obj["saved"] == nil || obj["status"] == nil {
		return raw
	}
	var dry bool
	if d, ok := obj["dry_run"]; ok {
		_ = json.Unmarshal(d, &dry)
	}
	rename := map[string]string{}
	if dry {
		rename = map[string]string{"applied": "would_apply", "after": "plan_if_applied", "report": "report_if_applied"}
	}
	out := []byte(`{"saved":` + string(obj["saved"]) + `,"status":` + string(obj["status"]))
	delete(obj, "saved")
	delete(obj, "status")
	delete(obj, "dry_run")
	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		name := k
		if to, ok := rename[k]; ok {
			name = to
		}
		kb, _ := json.Marshal(name)
		out = append(out, ',')
		out = append(out, kb...)
		out = append(out, ':')
		out = append(out, obj[k]...)
	}
	out = append(out, '}')
	if !json.Valid(out) {
		return raw
	}
	return out
}

// modelView is a tool result as the model reads it in the conversation.
func modelView(raw json.RawMessage) json.RawMessage { return dryRunView(modelDates(raw)) }

// offerSentence says the verified offer of a saved change as the closing
// statement of the reply, or "" when there is none: the first of the result's
// offers, else the first option of an issue the change made new.
func offerSentence(obj map[string]any) string {
	if list, _ := obj["offers"].([]any); len(list) > 0 {
		if o, ok := list[0].(map[string]any); ok {
			if what, _ := o["what"].(string); what != "" {
				s := "I can also " + what
				if eff, _ := o["effect"].(map[string]any); eff != nil {
					day, _ := eff["day"].(string)
					if k := num(eff["day_kcal_after"]); day != "" && k > 0 {
						s += fmt.Sprintf(" (%s would then hold %.0f kcal)", day, math.Round(k))
					}
				}
				return s + ": say so if you want it."
			}
		}
	}
	after, _ := obj["after"].(map[string]any)
	issues, _ := after["issues"].(map[string]any)
	// The fix of a day under its range comes first; else the first option of an
	// issue the change made new.
	var picks []map[string]any
	first := func(is any) map[string]any {
		m, _ := is.(map[string]any)
		opts, _ := m["options"].([]any)
		if len(opts) == 0 {
			return nil
		}
		o, _ := opts[0].(map[string]any)
		return o
	}
	fixes, _ := issues["day_fix"].([]any)
	for _, is := range fixes {
		m, _ := is.(map[string]any)
		if o := first(is); o != nil {
			if s := dayFixSentence(o, m); s != "" {
				if m["was_under_range"] == true {
					s = dayLabelStr(fmt.Sprint(m["date"])) + " was already under its range before this change. " + s
				}
				return s
			}
		}
	}
	news, _ := issues["new"].([]any)
	for _, is := range news {
		if o := first(is); o != nil {
			picks = append(picks, o)
		}
	}
	for _, o := range picks {
		name, _ := o["name"].(string)
		date, _ := o["date"].(string)
		what := ""
		amount := ""
		switch {
		case num(o["grams"]) > 0:
			amount = fmt.Sprintf("%.0f g of ", math.Round(num(o["grams"])))
		case num(o["portions"]) > 0:
			amount = fmt.Sprintf("%s portions of ", fmtNum(num(o["portions"])))
		}
		switch o["action"] {
		case "add":
			what = "add " + amount + name
		case "raise":
			what = "raise " + name + " to " + strings.TrimSuffix(amount, " of ")
		case "remove":
			what = "leave out " + name
		case "swap":
			what = "swap in " + name
		case "portions":
			what = "change the portions of " + name + " to " + strings.TrimSuffix(amount, " of ")
		case "move_cook":
			what = "cook it on another day"
		}
		if what == "" || (name == "" && o["action"] != "move_cook") {
			continue
		}
		if date != "" {
			what += " on " + dayLabelStr(date)
			if slot, _ := o["slot"].(string); slot != "" {
				what += " " + slot
			}
		}
		s := "I can also " + what
		if k := num(o["day_kcal"]); k > 0 {
			s += fmt.Sprintf(" (the day would then hold %.0f kcal)", math.Round(k))
		}
		return s + ": say so if you want it."
	}
	return ""
}

// dayFixSentence says the one fix of a day under its range with the meal, the
// day, the amount and the day's kcal afterwards ("I can also add 120 g of
// Ciabatta to Wed 25 Nov lunch (the day would then hold 2023 kcal): say so if
// you want it."); "" when the option cannot be said.
func dayFixSentence(o, issue map[string]any) string {
	name, _ := o["name"].(string)
	date, _ := issue["date"].(string)
	slot, _ := o["slot"].(string)
	op, _ := o["op"].(map[string]any)
	amount := ""
	switch {
	case num(op["amount"]) > 0:
		amount = fmt.Sprintf("%.0f g of ", math.Round(num(op["amount"])))
	case num(op["portions"]) > 0:
		amount = fmtNum(num(op["portions"])) + " portions of "
	}
	where := strings.TrimSpace(dayLabelStr(date) + " " + slot)
	var what string
	switch o["action"] {
	case "add":
		what = "add " + amount + name
	case "raise":
		what = "raise " + name + " to " + strings.TrimSuffix(amount, " of ")
	}
	k := num(o["day_kcal"])
	if what == "" || name == "" || where == "" || k <= 0 {
		return ""
	}
	return fmt.Sprintf("I can also %s on %s (the day would then hold %.0f kcal, inside its range): say so if you want it.", what, where, math.Round(k))
}

// addOfferTell makes the offer the last item of report.tell.
func addOfferTell(obj map[string]any) bool {
	if obj["dry_run"] == true {
		return false
	}
	rep, _ := obj["report"].(map[string]any)
	if rep == nil {
		return false
	}
	s := offerSentence(obj)
	if s == "" {
		return false
	}
	tell, _ := rep["tell"].([]any)
	rep["tell"] = append(tell, s)
	return true
}

// waysClosing is the ready closing sentence for the first offer of a find_ways
// result (a step beyond the asked meal, never applied unasked), in the style of
// the last report.tell item; "" without an offer.
func waysClosing(obj map[string]any) string {
	list, _ := obj["offer"].([]any)
	for _, e := range list {
		o, _ := e.(map[string]any)
		what, _ := o["what"].(string)
		if what == "" {
			continue
		}
		s := "I can also " + what
		var notes []string
		if o["changes_other_meal"] == true {
			notes = append(notes, "it also changes another meal")
		}
		if after, _ := o["after"].(map[string]any); after != nil {
			if k := num(after["day_kcal"]); k > 0 {
				notes = append(notes, fmt.Sprintf("the day would then hold %.0f kcal", math.Round(k)))
			}
		}
		if f, _ := o["fix"].(map[string]any); f != nil {
			if w, _ := f["what"].(string); w != "" {
				notes = append(notes, fmt.Sprintf("that leaves the day under its range, so I would also %s (the day would then hold %.0f kcal)", w, math.Round(num(f["day_kcal_after"]))))
			}
		}
		if len(notes) > 0 {
			s += " (" + strings.Join(notes, "; ") + ")"
		}
		return s + ": say so if you want it."
	}
	return ""
}

// cookingMovedTell turns the cooking_moved facts of an away call into at most
// three tell items: where everything is now cooked and what each dish feeds,
// and that nothing else changed. The per-dish cooking lines of the same moves
// (the cooking day lists, "now cooked", "no longer cooked", the automatic fix)
// are dropped so nothing repeats or contradicts it.
func cookingMovedTell(obj map[string]any) bool {
	cm, _ := obj["cooking_moved"].(map[string]any)
	rep, _ := obj["report"].(map[string]any)
	if cm == nil || rep == nil {
		return false
	}
	groups, _ := cm["cooked_now"].([]any)
	var items []string
	var days []string
	for _, g := range groups {
		m, _ := g.(map[string]any)
		day, _ := m["day"].(string)
		if day == "" {
			continue
		}
		day = dayLabelStr(day)
		days = append(days, day)
		at, _ := m["cooking_time"].(string)
		if at != "" {
			at = " at " + at
		}
		var dishes []string
		ds, _ := m["dishes"].([]any)
		for _, d := range ds {
			dm, _ := d.(map[string]any)
			dish, _ := dm["dish"].(string)
			var feeds []string
			switch f := dm["feeds"].(type) {
			case []any:
				for _, x := range f {
					if s, ok := x.(string); ok {
						feeds = append(feeds, s)
					}
				}
			case string:
				feeds = append(feeds, f)
			}
			if dish != "" {
				dishes = append(dishes, dish+" for "+strings.Join(feeds, ", "))
			}
		}
		if len(dishes) == 0 {
			continue
		}
		items = append(items, fmt.Sprintf("Everything that moved is now cooked together on %s%s, one cooking time: %s.", day, at, strings.Join(dishes, "; ")))
	}
	if len(items) == 0 {
		return false
	}
	// Order: the away lines, where everything is cooked now, what else changed
	// (or that nothing did), the rest.
	var awayItems, rest, resizes []string
	tell, _ := rep["tell"].([]any)
	otherMeal := false
	for _, it := range tell {
		t, _ := it.(string)
		drop := strings.Contains(t, "is now cooked") && !strings.Contains(t, "is now cooked for") || strings.Contains(t, "no longer cooked") ||
			strings.HasPrefix(t, "To keep one cooking time")
		for _, d := range days {
			drop = drop || strings.HasPrefix(t, d+" you cook") || strings.HasPrefix(t, d+" cooking changes")
		}
		switch {
		case drop:
		case strings.Contains(t, " is away: "):
			awayItems = append(awayItems, t)
		case strings.Contains(t, "is now cooked for") && strings.Contains(t, "nothing is left over"):
			resizes = append(resizes, t)
		default:
			if mealLine.MatchString(t) {
				otherMeal = true
			}
			rest = append(rest, t)
		}
	}
	var out []any
	for _, t := range awayItems {
		out = append(out, t)
	}
	for _, t := range items {
		out = append(out, t)
	}
	switch {
	case len(resizes) > 0:
		short := make([]string, len(resizes))
		for i, t := range resizes {
			short[i] = strings.TrimSuffix(strings.Replace(t, "; nothing is left over.", "", 1), ".")
			short[i] = strings.Replace(short[i], "The ", "the ", 1)
		}
		out = append(out, "Apart from that only this changed: "+strings.Join(short, "; ")+".")
	case !otherMeal && cm["nothing_else_changed"] == true:
		out = append(out, "No other meal changed.")
	}
	for _, t := range rest {
		out = append(out, t)
	}
	rep["tell"] = out
	return true
}

// mealLine matches a tell item about a changed meal ("Tue 24 Nov dinner: ...").
var mealLine = regexp.MustCompile(`^\w{3} \d+ \w{3} (breakfast|lunch|dinner)[^:]*:`)
