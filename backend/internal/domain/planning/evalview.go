package planning

import (
	"bytes"
	"encoding/json"
	"math"
	"slices"
	"strings"
)

// keepEmpty names the keys a model-facing payload never drops, even when
// zero or empty: an explicit "issues": [] or "meat_days": 0 reads as a
// result, an absent key as a gap.
var keepEmpty = []string{"issues", "totals", "violations", "warnings", "soft", "meat_days"}

// trueByDefault names the flags whose normal value is true: the payload
// carries them only when false (a plate or day that is not complete).
var trueByDefault = []string{"complete", "nutrition_complete"}

// View scopes an Evaluation for the model: the days a call touched and the
// plates whose components it should see.
type View struct {
	// Dates are the days (and their plates) to keep; nil keeps every day.
	Dates map[string]bool
	// Detail reports whether a plate keeps its component list; nil: none.
	Detail func(PlateEval) bool
	// Slim keeps, on the plates Detail does not select, a slim component
	// list (SlimComponents) instead of none.
	Slim bool
}

// SlimComponents keeps what judging a plate needs: the component id, name,
// quantity, kcal and protein (and nutrition_complete, shown only when false).
// The cells and the proposal already carry food, role and session.
func SlimComponents(cs []ComponentEval) []ComponentEval {
	out := make([]ComponentEval, len(cs))
	for i, c := range cs {
		out[i] = ComponentEval{
			ID: c.ID, Name: c.Name, Portions: c.Portions, Grams: c.Grams,
			Kcal: c.Kcal, ProteinG: c.ProteinG, NutritionComplete: c.NutritionComplete,
		}
	}
	return out
}

// ProposedView scopes e to what a proposal touched: the days that hold a
// proposed plate (negative id) and, with their components, those plates.
// With no proposed plate it keeps everything, without components.
func (e Evaluation) ProposedView() View {
	v := View{}
	for _, p := range e.Plates {
		if p.PlateID < 0 {
			if v.Dates == nil {
				v.Dates = map[string]bool{}
				v.Detail = func(p PlateEval) bool { return p.PlateID < 0 }
			}
			v.Dates[p.Date] = true
		}
	}
	return v
}

// Compact is the model-facing form of e: only v's days, their plates and soft
// issues,
// components only where v asks, numbers rounded (kcal to integers, other
// figures to one decimal) and every zero, false, null and empty field
// omitted except the keys in keepEmpty; the flags in trueByDefault are shown
// only when false. The run log keeps the full e.
func (e Evaluation) Compact(v View) map[string]any {
	e.AllIssues = nil
	if v.Dates != nil {
		e.Days = slices.DeleteFunc(slices.Clone(e.Days), func(d DayEval) bool { return !v.Dates[d.Date] })
		e.Plates = slices.DeleteFunc(slices.Clone(e.Plates), func(p PlateEval) bool { return !v.Dates[p.Date] })
		// Soft issues (and their options) of other days are noise; hard
		// rules and warnings always stay, a proposal can break them anywhere.
		e.Issues = slices.DeleteFunc(slices.Clone(e.Issues), func(is Issue) bool {
			return is.Severity == SeveritySoft && is.Date != "" && !v.Dates[is.Date]
		})
		e.Totals = totalsOf(e.Issues)
	}
	plates := make([]PlateEval, len(e.Plates))
	for i, p := range e.Plates {
		switch {
		case v.Detail != nil && v.Detail(p):
		case v.Slim:
			p.Components = SlimComponents(p.Components)
		default:
			p.Components = nil
		}
		plates[i] = p
	}
	e.Plates = plates
	m, _ := Compact(e).(map[string]any)
	return m
}

// Compact rounds and prunes v as Evaluation.Compact does: v is marshalled to
// JSON and returned as generic values (nil when it cannot be marshalled).
func Compact(v any) any {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var g any
	if err := dec.Decode(&g); err != nil {
		return nil
	}
	out, _ := prune("", g)
	return out
}

// prune returns g rounded and without empty fields; the bool is false when
// the value itself is empty and key does not keep it.
func prune(key string, g any) (any, bool) {
	keep := slices.Contains(keepEmpty, key)
	switch x := g.(type) {
	case nil:
		return nil, false
	case bool:
		if slices.Contains(trueByDefault, key) {
			return x, !x
		}
		return x, x
	case string:
		return x, x != ""
	case json.Number:
		s := x.String()
		if !strings.ContainsAny(s, ".eE") {
			return x, s != "0" || keep
		}
		f, err := x.Float64()
		if err != nil {
			return x, true
		}
		if strings.Contains(key, "kcal") {
			f = math.Round(f)
		} else {
			f = math.Round(f*10) / 10
		}
		return f, f != 0 || keep
	case []any:
		out := make([]any, 0, len(x))
		for _, e := range x {
			if p, ok := prune(key, e); ok {
				out = append(out, p)
			}
		}
		return out, len(out) > 0 || keep
	case map[string]any:
		trimOption(x)
		out := make(map[string]any, len(x))
		for k, e := range x {
			if p, ok := prune(k, e); ok {
				out[k] = p
			}
		}
		return out, len(out) > 0 || keep
	}
	return g, true
}

// trimOption drops the fields of an issue option (a map with `action` and
// `op`) that its `op` already carries: the op is what the model applies, the
// rest is name, deltas and the resulting day figures.
func trimOption(m map[string]any) {
	if _, ok := m["action"]; !ok {
		return
	}
	if _, ok := m["op"].(map[string]any); !ok {
		return
	}
	for _, k := range []string{"food_id", "date", "slot_id", "plate_id", "portions", "grams"} {
		delete(m, k)
	}
}
