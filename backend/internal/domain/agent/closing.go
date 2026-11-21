package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/window"
)

// The closing option of a result: one worked alternative, checked on a copy of
// the plan, or none. The builders below are the only places that make one for
// a refused write or a day under its minimum, and next names an option only
// when it is there.

// refusalOffer is, for an op the guard left out, the nearest verified
// alternative that does what the user asked as far as the rules allow: a
// replacement for a removed add-on, another main or side for a refused
// replace, a smaller step for a refused quantity, a swap of the main dishes
// for a refused swap of two meals. Nil when none passes the check.
func refusalOffer(ctx context.Context, svc Services, w window.Window, op changeOp, first bool) map[string]any {
	switch op.Op {
	case "remove":
		return replacementOffer(ctx, svc, w, op, first)
	case "replace", "set_quantity", "move":
	default:
		return nil
	}
	if svc.State == nil {
		return nil
	}
	st, err := svc.State(ctx, w, todayFrom(ctx))
	if err != nil {
		return nil
	}
	st.Avoid, st.PairAddOns = svc.Removed.avoid(ctx), true
	var o map[string]any
	switch op.Op {
	case "replace":
		o = replaceAlternative(ctx, svc, w, st, op)
	case "set_quantity":
		o = smallerStep(ctx, svc, w, st, op)
	case "move":
		o = mainSwap(ctx, svc, w, st, op)
	}
	if o != nil {
		o["recommended"] = first
		o["recommended_because"] = "checked on a copy of the plan: no new problem, no new warning"
	}
	return o
}

// componentOf finds the plate and component an op names: by component id, or
// by plate and position (main by default).
func componentOf(st planning.State, op changeOp) (planning.Plate, planning.Component, bool) {
	for _, p := range st.Plates {
		if op.PlateComponentID == 0 && p.ID != op.PlateID {
			continue
		}
		for _, c := range p.Components {
			switch {
			case op.PlateComponentID != 0 && c.ID == op.PlateComponentID:
				return p, c, true
			case op.PlateComponentID == 0 && c.Position == plate.Position(cmpOr(op.Position, "main")):
				return p, c, true
			}
		}
	}
	return planning.Plate{}, planning.Component{}, false
}

func cmpOr(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// replaceAlternative is the best clean other dish for the place a refused
// replace named.
func replaceAlternative(ctx context.Context, svc Services, w window.Window, st planning.State, op changeOp) map[string]any {
	p, c, ok := componentOf(st, op)
	if !ok {
		return nil
	}
	req := planning.AltRequest{PlateID: p.ID, What: planning.AltMain, Limit: 6}
	if c.Position != plate.PositionMain {
		req.What, req.ComponentID = planning.AltSide, c.ID
	}
	res, err := planning.Alternatives(st, planning.WindowTarget(st), req)
	if err != nil || res.Current == nil {
		return nil
	}
	a, ok := firstClean(res.Alternatives, func(ops []planning.OptionOp) bool { return offerIsClean(ctx, svc, w, ops) })
	if !ok {
		return nil
	}
	return map[string]any{
		"what":    fmt.Sprintf("use %s instead of %s on %s: the dish you asked for cannot go there", food.Quote(a.Name), food.Quote(res.Current.Name), mealName(st, p)),
		"what_de": fmt.Sprintf("%s statt %s bei %s verwenden: das gewünschte Gericht kann dort nicht hin", deQuote(a.Name), deQuote(res.Current.Name), deCell(mealName(st, p))),
		"ops":     a.Ops,
		"effect":  map[string]any{"day": dayLabel(p.Date), "day_kcal_after": a.DayKcal},
	}
}

// smallerStep is the biggest part of a refused quantity change that passes
// the check: halfway, then a quarter of the way, in quarter portions.
func smallerStep(ctx context.Context, svc Services, w window.Window, st planning.State, op changeOp) map[string]any {
	p, c, ok := componentOf(st, op)
	if !ok || op.PlateComponentID == 0 || c.Portions == nil || op.Portions == nil {
		return nil
	}
	cur, req := *c.Portions, *op.Portions
	for _, f := range []float64{0.5, 0.25} {
		q := math.Round((cur+(req-cur)*f)*4) / 4
		if q == cur || q == req || q <= 0 {
			continue
		}
		id := c.ID
		ops := []planning.OptionOp{{Op: "set_quantity", PlateComponentID: &id, Portions: &q}}
		if offerIsClean(ctx, svc, w, ops) {
			return map[string]any{
				"what":    fmt.Sprintf("set %s on %s to %s portions instead of %s: the whole step cannot go through", food.Quote(foodNameOf(st, c.FoodID)), mealName(st, p), fmtNum(q), fmtNum(req)),
				"what_de": fmt.Sprintf("%s bei %s auf %s Portionen statt %s setzen: der ganze Schritt geht nicht durch", deQuote(foodNameOf(st, c.FoodID)), deCell(mealName(st, p)), fmtComma(q), fmtComma(req)),
				"ops":     ops,
			}
		}
	}
	return nil
}

func foodNameOf(st planning.State, id int64) string { return st.Foods[id].Name }

// mainSwap is, for a refused swap of two meals (the cooking and the leftovers
// that follow a moved plate broke a rule), the swap of their main dishes with
// the sides where they are, when that passes the check.
func mainSwap(ctx context.Context, svc Services, w window.Window, st planning.State, op changeOp) map[string]any {
	if string(op.Occupant) != `"swap"` {
		return nil
	}
	var to struct {
		Date   string `json:"date"`
		SlotID int64  `json:"slot_id"`
	}
	if json.Unmarshal(op.To, &to) != nil {
		return nil
	}
	ai := indexPlate(st, op.PlateID)
	bi := slices.IndexFunc(st.Plates, func(p planning.Plate) bool {
		return p.Date.Format("2006-01-02") == to.Date && p.SlotID == to.SlotID
	})
	if ai < 0 || bi < 0 {
		return nil
	}
	a, b := st.Plates[ai], st.Plates[bi]
	mainOf := func(p planning.Plate) (planning.Component, bool) {
		for _, c := range p.Components {
			if c.Position == plate.PositionMain && c.ID != 0 {
				return c, true
			}
		}
		return planning.Component{}, false
	}
	ca, ok1 := mainOf(a)
	cb, ok2 := mainOf(b)
	if !ok1 || !ok2 || ca.FoodID == cb.FoodID {
		return nil
	}
	ida, idb := ca.ID, cb.ID
	ops := []planning.OptionOp{
		{Op: "replace", PlateComponentID: &ida, FoodID: cb.FoodID},
		{Op: "replace", PlateComponentID: &idb, FoodID: ca.FoodID},
	}
	if !offerIsClean(ctx, svc, w, ops) {
		return nil
	}
	return map[string]any{
		"what_de": fmt.Sprintf("die Hauptgerichte von %s und %s (%s und %s) tauschen und die Beilagen lassen, wo sie sind: die ganzen Mahlzeiten zu verschieben geht nicht", deCell(mealName(st, a)), deCell(mealName(st, b)), deQuote(foodNameOf(st, ca.FoodID)), deQuote(foodNameOf(st, cb.FoodID))),
		"what":    fmt.Sprintf("swap the main dishes of %s and %s (%s and %s) and leave the sides where they are: moving the whole meals cannot go through", mealName(st, a), mealName(st, b), food.Quote(foodNameOf(st, ca.FoodID)), food.Quote(foodNameOf(st, cb.FoodID))),
		"ops":     ops,
	}
}

// floorFixOffer is the verified fix for a day a saved change left under its
// minimum: the first add that reaches it, else the adds that reach it together
// (at most three, on different meals), each from the issue's own options.
// Nil when no such fix passes the check: the reply then says so.
func floorFixOffer(ctx context.Context, svc Services, w window.Window, is planning.Issue, all []planning.Issue) map[string]any {
	kcal, floor := paramNum(is, "kcal"), paramNum(is, "floor_kcal")
	// The options of the day's own issue and of its energy band issue (a day
	// under its minimum is under its band too, and that issue may be older).
	opts := slices.Clone(is.Options)
	for _, x := range all {
		if x.Date == is.Date && x.Code == planning.CodeKcalBelowBand {
			opts = append(opts, x.Options...)
		}
	}
	if kcal <= 0 || floor <= 0 || len(opts) == 0 {
		return nil
	}
	slices.SortStableFunc(opts, func(a, b planning.Option) int {
		switch {
		case a.KcalDelta > b.KcalDelta:
			return -1
		case a.KcalDelta < b.KcalDelta:
			return 1
		}
		return 0
	})
	opOf := func(o planning.Option) []planning.OptionOp {
		if len(o.Ops) > 0 {
			return o.Ops
		}
		return []planning.OptionOp{o.Op}
	}
	var ops []planning.OptionOp
	var names, namesDE []string
	var used []string
	gain := 0.0
	for _, o := range opts {
		key := fmt.Sprintf("%s|%d", o.Date, o.SlotID)
		if o.KcalDelta <= 0 || len(opOf(o)) == 0 || slices.Contains(used, key) {
			continue
		}
		used = append(used, key)
		ops = append(ops, opOf(o)...)
		names = append(names, optionWords(o))
		namesDE = append(namesDE, optionWordsDE(o))
		if gain += o.KcalDelta; kcal+gain >= floor || len(names) == 3 {
			break
		}
	}
	if kcal+gain < floor || !offerIsClean(ctx, svc, w, ops) {
		return nil
	}
	return map[string]any{
		"what":                strings.Join(names, " and ") + fmt.Sprintf(" so %s reaches its minimum of %.0f kcal", dayLabelStr(is.Date), floor),
		"what_de":             strings.Join(namesDE, " und ") + fmt.Sprintf(", damit %s seinen Mindestwert von %.0f kcal erreicht", deDay(dayLabelStr(is.Date)), floor),
		"ops":                 ops,
		"recommended":         true,
		"recommended_because": "brings the day above its minimum, no new warning",
		"effect":              map[string]any{"day": dayLabelStr(is.Date), "day_kcal_after": math.Round(kcal + gain)},
	}
}

// optionWords says an add or raise option in kitchen words: "add 120 g of
// "Ciabatta" to Tue 24 Nov" or "raise "Curry" to 1.5 portions on Tue 24 Nov".
func optionWords(o planning.Option) string {
	amount := ""
	switch {
	case o.Grams != nil:
		amount = fmt.Sprintf("%.0f g", math.Round(*o.Grams))
	case o.Portions != nil:
		amount = fmtNum(*o.Portions) + " portions"
	}
	where := strings.TrimSpace(dayLabelStr(o.Date) + " " + o.Slot)
	if o.Action == "raise" {
		return fmt.Sprintf("raise %s to %s on %s", food.Quote(o.Name), amount, where)
	}
	return fmt.Sprintf("add %s of %s to %s", amount, food.Quote(o.Name), where)
}

// optionWordsDE is optionWords in German.
func optionWordsDE(o planning.Option) string {
	amount := ""
	switch {
	case o.Grams != nil:
		amount = fmt.Sprintf("%.0f g", math.Round(*o.Grams))
	case o.Portions != nil:
		amount = dePortions(*o.Portions)
	}
	where := strings.TrimSpace(dayLabelStr(o.Date) + " " + o.Slot)
	if len(strings.Fields(where)) == 3 {
		where = deDay(where)
	} else {
		where = deCell(where)
	}
	if o.Action == "raise" {
		return fmt.Sprintf("%s am %s auf %s erhöhen", deQuote(o.Name), where, amount)
	}
	return fmt.Sprintf("%s %s zu %s hinzufügen", amount, deQuote(o.Name), where)
}

// unLightOffer is RL9: when no unmarked planned meal of a day left under its
// minimum can take a fix, the one offer removes the light mark of the marked
// lunch or dinner that has the most room, applied only on a yes (the offer
// carries the mark's removal and nothing else). nil when the day has no
// marked meal: the reply then says plainly that nothing can be offered.
func unLightOffer(ctx context.Context, svc Services, w window.Window, is planning.Issue) map[string]any {
	if svc.State == nil {
		return nil
	}
	st, err := svc.State(ctx, w, todayFrom(ctx))
	if err != nil {
		return nil
	}
	var best *planning.Plate
	var mark planning.LightIntent
	bestKcal := math.Inf(1)
	ev := planning.Evaluate(st, planning.WindowTarget(st))
	kcal := map[int64]float64{}
	for _, pe := range ev.Plates {
		kcal[pe.PlateID] = pe.Kcal
	}
	for i, p := range st.Plates {
		if p.Date.Format(time.DateOnly) != is.Date {
			continue
		}
		light := st.LightMealAt(p.Date, p.SlotID)
		if !light || kcal[p.ID] >= bestKcal {
			continue
		}
		best, bestKcal = &st.Plates[i], kcal[p.ID]
		for _, n := range []string{"lunch", "dinner"} {
			if strings.TrimPrefix(st.SlotNames[p.SlotID], "slot.") == n {
				mark = planning.LightIntent{Date: is.Date, Scope: planning.LightScopeNone, Slot: n}
			}
		}
	}
	if best == nil || mark.Slot == "" {
		return nil
	}
	meal := mealName(st, *best)
	return map[string]any{
		"what":                fmt.Sprintf("remove the light mark of %s, so it can take the food the day is missing; I add it only after you say yes", meal),
		"what_de":             fmt.Sprintf("die Leicht-Markierung von %s entfernen, damit es das fehlende Essen des Tages aufnehmen kann; ich füge es erst nach deinem Ja hinzu", deCell(meal)),
		"light":               []planning.LightIntent{mark},
		"recommended":         true,
		"recommended_because": "no other planned meal of the day can take more food while it is marked light",
		"effect":              map[string]any{"day": dayLabelStr(is.Date), "day_kcal_now": math.Round(paramNum(is, "kcal")), "day_minimum": math.Round(paramNum(is, "floor_kcal"))},
	}
}

// offerLine is the one offer of a reply, rendered by code in both languages
// with a fixed marker ("I can also ...: say so if you want it."). It is never a
// question and is the last line of the reply.
type offerLine struct {
	EN string `json:"en"`
	DE string `json:"de"`
}

func makeOffer(whatEN, whatDE string) *offerLine {
	return &offerLine{
		EN: "I can also " + whatEN + ": say so if you want it.",
		DE: "Ich kann auch " + whatDE + ": sag Bescheid, wenn du das möchtest.",
	}
}

var (
	deDateIn   = regexp.MustCompile(`\b(Mon|Tue|Wed|Thu|Fri|Sat|Sun) (\d{1,2}) (Jan|Feb|Mar|Apr|May|Jun|Jul|Aug|Sep|Oct|Nov|Dec)\b`)
	deAddTo    = regexp.MustCompile(`^add (\d+) g of (.+) (?:to|on) (.+)$`)
	deLeaveOut = regexp.MustCompile(`^leave out (.+)$`)
	deSwapFor  = regexp.MustCompile(`^swap (.+?)'s (".+?") for (".+?")(?: so the day stays above its minimum)?$`)
	deSwapMain = regexp.MustCompile(`^swap the main for (.+)$`)
)

// deDates writes the day labels of a sentence in German.
func deDates(s string) string {
	return deDateIn.ReplaceAllStringFunc(s, deDay)
}

// deWhat says an offer's action in German: the common shapes translated, any
// other as a plain statement that a checked change is ready.
func deWhat(en string) string {
	t := strings.TrimSpace(en)
	if m := deAddTo.FindStringSubmatch(t); m != nil {
		return fmt.Sprintf("%s g %s zu %s hinzufügen", m[1], deQuoted(m[2]), deCell(deDates(m[3])))
	}
	if m := deLeaveOut.FindStringSubmatch(t); m != nil {
		return deQuoted(m[1]) + " weglassen"
	}
	if m := deSwapFor.FindStringSubmatch(t); m != nil {
		return fmt.Sprintf("bei %s %s durch %s ersetzen", deCell(m[1]), deQuoted(m[2]), deQuoted(m[3]))
	}
	if m := deSwapMain.FindStringSubmatch(t); m != nil {
		return "das Hauptgericht durch " + deQuoted(m[1]) + " ersetzen"
	}
	return "eine geprüfte Änderung anwenden"
}

// offerOf is the one offer of a saved change, or nil: the first of the result's
// offers, else the fix of a day under its range, else the first option of an
// issue the change made new.
func offerOf(obj map[string]any) *offerLine {
	if list, _ := obj["offers"].([]any); len(list) > 0 {
		if o, ok := list[0].(map[string]any); ok {
			if what, _ := o["what"].(string); what != "" {
				en, de := what, deWhat(what)
				if w, _ := o["what_de"].(string); w != "" {
					de = w
				}
				if eff, _ := o["effect"].(map[string]any); eff != nil {
					day, _ := eff["day"].(string)
					if k := num(eff["day_kcal_after"]); day != "" && k > 0 {
						en += fmt.Sprintf(" (%s would then hold %.0f kcal)", day, math.Round(k))
						de += fmt.Sprintf(" (%s hätte dann %.0f kcal)", deDay(day), math.Round(k))
					}
				}
				return makeOffer(en, de)
			}
		}
	}
	after, _ := obj["after"].(map[string]any)
	issues, _ := after["issues"].(map[string]any)
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
			if off := dayFixOffer(o, m); off != nil {
				return off
			}
		}
	}
	news, _ := issues["new"].([]any)
	for _, is := range news {
		o := first(is)
		if o == nil {
			continue
		}
		name, _ := o["name"].(string)
		date, _ := o["date"].(string)
		var amountEN, amountDE string
		switch {
		case num(o["grams"]) > 0:
			amountEN, amountDE = fmt.Sprintf("%.0f g of ", math.Round(num(o["grams"]))), fmt.Sprintf("%.0f g ", math.Round(num(o["grams"])))
		case num(o["portions"]) > 0:
			amountEN, amountDE = fmtNum(num(o["portions"]))+" portions of ", fmtComma(num(o["portions"]))+" Portionen "
		}
		var en, de string
		where, whereDE := "", ""
		if date != "" {
			where, whereDE = " on "+dayLabelStr(date), " am "+deDay(dayLabelStr(date))
			if slotName, _ := o["slot"].(string); slotName != "" {
				where += " " + slotName
				whereDE += " " + deSlot(slotName)
			}
		}
		switch o["action"] {
		case "add":
			en, de = "add "+amountEN+name, amountDE+deQuote(name)+" hinzufügen"
		case "raise":
			en, de = "raise "+name+" to "+strings.TrimSuffix(amountEN, " of "), deQuote(name)+" auf "+strings.TrimSpace(amountDE)+" erhöhen"
		case "remove":
			en, de = "leave out "+name, deQuote(name)+" weglassen"
		case "swap":
			en, de = "swap in "+name, deQuote(name)+" einsetzen"
		case "portions":
			en, de = "change the portions of "+name+" to "+strings.TrimSuffix(amountEN, " of "), "die Portionen von "+deQuote(name)+" auf "+strings.TrimSpace(amountDE)+" ändern"
		case "move_cook":
			en, de = "cook it on another day", "es an einem anderen Tag kochen"
		}
		if en == "" || (name == "" && o["action"] != "move_cook") {
			continue
		}
		en, de = en+where, de+whereDE
		if k := num(o["day_kcal"]); k > 0 {
			en += fmt.Sprintf(" (the day would then hold %.0f kcal)", math.Round(k))
			de += fmt.Sprintf(" (der Tag hätte dann %.0f kcal)", math.Round(k))
		}
		return makeOffer(en, de)
	}
	return nil
}

// dayFixOffer is the one fix of a day under its range: the meal, the day, the
// amount and the day's kcal afterwards.
func dayFixOffer(o, issue map[string]any) *offerLine {
	name, _ := o["name"].(string)
	date, _ := issue["date"].(string)
	slotName, _ := o["slot"].(string)
	op, _ := o["op"].(map[string]any)
	var amountEN, amountDE string
	switch {
	case num(op["amount"]) > 0:
		amountEN, amountDE = fmt.Sprintf("%.0f g of ", math.Round(num(op["amount"]))), fmt.Sprintf("%.0f g ", math.Round(num(op["amount"])))
	case num(op["portions"]) > 0:
		amountEN, amountDE = fmtNum(num(op["portions"]))+" portions of ", fmtComma(num(op["portions"]))+" Portionen "
	}
	where := strings.TrimSpace(dayLabelStr(date) + " " + slotName)
	var en, de string
	switch o["action"] {
	case "add":
		en, de = "add "+amountEN+name, amountDE+deQuote(name)+" hinzufügen"
	case "raise":
		en, de = "raise "+name+" to "+strings.TrimSuffix(amountEN, " of "), deQuote(name)+" auf "+strings.TrimSpace(amountDE)+" erhöhen"
	}
	k := num(o["day_kcal"])
	if en == "" || name == "" || where == "" || k <= 0 {
		return nil
	}
	whereDE := deCell(where)
	if slotName == "" {
		whereDE = deDay(where)
	}
	if was, _ := issue["was_under_range"].(bool); was {
		return &offerLine{
			EN: fmt.Sprintf("%s was already under its range before this change. I can also %s on %s (the day would then hold %.0f kcal, inside its range): say so if you want it.", dayLabelStr(date), en, where, math.Round(k)),
			DE: fmt.Sprintf("%s lag schon vor dieser Änderung unter dem Bereich. Ich kann auch %s bei %s (der Tag hätte dann %.0f kcal, im Bereich): sag Bescheid, wenn du das möchtest.", deDay(dayLabelStr(date)), de, whereDE, math.Round(k)),
		}
	}
	return makeOffer(fmt.Sprintf("%s on %s (the day would then hold %.0f kcal, inside its range)", en, where, math.Round(k)),
		fmt.Sprintf("%s bei %s (der Tag hätte dann %.0f kcal, im Bereich)", de, whereDE, math.Round(k)))
}

// setOffer puts the offer of a saved change into its report; false when there
// is none (a check, or no verified offer).
func setOffer(obj map[string]any) bool {
	if obj["dry_run"] == true {
		return false
	}
	rep, _ := obj["report"].(map[string]any)
	if rep == nil {
		return false
	}
	o := offerOf(obj)
	if o == nil {
		return false
	}
	rep["offer"] = map[string]any{"en": o.EN, "de": o.DE}
	return true
}
