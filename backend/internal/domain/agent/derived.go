package agent

import (
	"encoding/json"
	"fmt"
	"math"
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
		return fmt.Sprintf("%.0f kcal over the most a light meal holds", math.Round(kcal-max))
	case max > 0:
		return "within the most a light meal holds"
	case min > 0 && kcal < min-0.5:
		return fmt.Sprintf("%.0f kcal under the least a filling meal holds", math.Round(min-kcal))
	case min > 0:
		return "reaches the least a filling meal holds"
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
		if k, ok := after[dayKey].(float64); ok && (low > 0 || high > 0) {
			m["day_vs_range"], changed = vsRange(k, low, high), true
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
		if addOfferTell(obj) {
			changed = true
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
	news, _ := issues["new"].([]any)
	for _, n := range news {
		is, _ := n.(map[string]any)
		opts, _ := is["options"].([]any)
		if len(opts) == 0 {
			continue
		}
		o, _ := opts[0].(map[string]any)
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
		}
		s := "I can also " + what
		if k := num(o["day_kcal"]); k > 0 {
			s += fmt.Sprintf(" (the day would then hold %.0f kcal)", math.Round(k))
		}
		return s + ": say so if you want it."
	}
	return ""
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
