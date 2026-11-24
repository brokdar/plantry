package agent

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/availability"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"

	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/window"
)

// exemptOps are explicit requests whose hard consequences the user asked for:
// an away day is not judged, a deleted plate is the point of the call.
var exemptOps = map[string]bool{"away": true, "set_availability": true, "delete_plate": true}

// newHard are the hard issues the evaluation after holds and before did not.
// An empty cell is a plan still being built, not a consequence of the edit.
func newHard(before, after planning.Evaluation) []planning.Issue {
	was := issueMap(before.Issues)
	var out []planning.Issue
	for _, is := range after.Issues {
		if _, old := was[issueKey(is)]; !old && is.Severity == planning.SeverityViolation && is.Code != planning.CodeMissingPlate {
			out = append(out, is)
		}
	}
	return out
}

// hardWhyL is hardWhy in English and German.
func hardWhyL(is planning.Issue) (en, de string) {
	if is.Code == planning.CodeKcalBelowFloor {
		kcal, _ := is.Params["kcal"].(float64)
		floor, _ := is.Params["floor_kcal"].(float64)
		return fmt.Sprintf("%s would fall to %.0f kcal, under its minimum of %.0f kcal", dayLabelStr(is.Date), kcal, floor),
			fmt.Sprintf("%s würde auf %.0f kcal fallen, unter den Mindestwert von %.0f kcal", deDay(dayLabelStr(is.Date)), kcal, floor)
	}
	labelDE, ok := hardLabelDE[is.Code]
	if !ok {
		labelDE = "eine Regel wäre verletzt"
	}
	figEN, figDE := hardFigures(is)
	on, onDE := "", ""
	if is.Date != "" {
		on, onDE = " on "+dayLabelStr(is.Date), " am "+deDay(dayLabelStr(is.Date))
	}
	return fmt.Sprintf("it would create a problem%s: %s%s", on, planning.PlainText(is.Code).Label, figEN),
		fmt.Sprintf("es würde%s ein Problem entstehen: %s%s", onDE, labelDE, figDE)
}

// hardLabelDE is the plain label of a hard issue in German.
var hardLabelDE = map[string]string{
	planning.CodeTooManyCookOccasions:  "mehr getrennte Kochzeiten an einem Tag als erlaubt",
	planning.CodeMeatDaysExceeded:      "mehr Fleischtage in der Woche als erlaubt",
	planning.CodeDietIncompatible:      "ein Lebensmittel passt nicht zur Ernährungsweise",
	planning.CodeKcalBelowFloor:        "der Tag läge unter seinem Mindestwert",
	planning.CodeVarietyRepeat:         "ein Hauptgericht wurde zu kürzlich gekocht oder gegessen",
	"leftover_before_cook":             "Reste würden vor dem Kochen gegessen",
	"past_keep_days":                   "Reste wären nicht mehr haltbar",
	"deficit":                          "es würde mehr gegessen, als gekocht wird",
	"frozen_deficit":                   "es würde mehr vom eingefrorenen Vorrat gegessen, als eingefroren ist",
	planning.CodeMissingPlate:          "einer Mahlzeit fehlt der Teller",
	planning.CodeExtraPlate:            "ein Teller steht in einem Slot, der leer bleiben soll",
	planning.CodePlateInNoneCell:       "ein Teller steht in einem Slot ohne Mahlzeit",
	planning.CodeCookInNoneCell:        "es würde in einem Slot ohne Mahlzeit gekocht",
	planning.CodeConfirmedInTarget:     "die Änderung würde eine bestätigte Mahlzeit berühren",
	planning.CodeSessionOutsideTarget:  "ein Gericht, das außerhalb des Bereichs gekocht wird, wäre betroffen",
	planning.CodeDuplicateSessionKey:   "zwei Kochvorgänge hätten denselben Namen",
	planning.CodeInvalidPortions:       "eine Portionsangabe ist ungültig",
	planning.CodeNotComposed:           "ein Lebensmittel, das kein Rezept ist, würde wie eines verwendet",
	planning.CodePlateOnConfirmedCell:  "ein Teller läge auf einem bereits bestätigten Slot",
	planning.CodePlateOutsideTarget:    "ein Teller läge außerhalb des geplanten Bereichs",
	planning.CodePlateWithoutComponent: "ein Teller hätte kein Essen darauf",
	planning.CodePortionsOverCap:       "die Portionen lägen über dem erlaubten Höchstwert",
	planning.CodeSessionFoodMismatch:   "ein Teller äße von einem Kochvorgang eines anderen Rezepts",
	planning.CodeUnknownFood:           "ein Lebensmittel im Plan existiert nicht",
	planning.CodeUnknownSession:        "ein Teller äße von einem Kochvorgang, den es nicht gibt",
}

// hardWhyIn is hardWhyL with, for a day of too many cooking times, the
// batches and meals that already cook that day, so the reason names what is in
// the way and not only the count.
func hardWhyIn(ctx context.Context, svc Services, w window.Window, is planning.Issue) (en, de string) {
	en, de = hardWhyL(is)
	if is.Code != planning.CodeTooManyCookOccasions || svc.State == nil || is.Date == "" {
		return en, de
	}
	st, err := svc.State(ctx, w, todayFrom(ctx))
	if err != nil {
		return en, de
	}
	cooks, cooksDE := cooksOnDay(st, is.Date)
	if len(cooks) == 0 || !strings.HasSuffix(en, ")") || !strings.HasSuffix(de, ")") {
		return en, de
	}
	return strings.TrimSuffix(en, ")") + "; already cooking that day: " + strings.Join(cooks, ", ") + ")",
		strings.TrimSuffix(de, ")") + "; an dem Tag wird schon gekocht: " + strings.Join(cooksDE, ", ") + ")"
}

// cooksOnDay lists the cooking times of a day in the plan as it is: each batch
// cooked there and each meal that cooks its own dish, with the cell, in
// English and German, one entry per cooking cell.
func cooksOnDay(st planning.State, date string) (en, de []string) {
	routine := map[string]bool{}
	for _, c := range st.Cells {
		if c.Mode == availability.ModeRoutine {
			routine[fmt.Sprintf("%s|%d", c.Date.Format(time.DateOnly), c.SlotID)] = true
		}
	}
	type cook struct {
		cell   string
		slot   int64
		dishes []string
	}
	var cooks []*cook
	add := func(slot int64, dish string) {
		if routine[fmt.Sprintf("%s|%d", date, slot)] || dish == "" {
			return
		}
		for _, c := range cooks {
			if c.slot == slot {
				if !slices.Contains(c.dishes, dish) {
					c.dishes = append(c.dishes, dish)
				}
				return
			}
		}
		cooks = append(cooks, &cook{cell: dayLabelStr(date) + " " + strings.TrimPrefix(st.SlotNames[slot], "slot."), slot: slot, dishes: []string{dish}})
	}
	for _, s := range st.Sessions {
		if s.CookDate.Format(time.DateOnly) == date {
			add(s.CookSlotID, foodName(st, s.FoodID))
		}
	}
	for _, p := range st.Plates {
		if p.Date.Format(time.DateOnly) != date {
			continue
		}
		for _, c := range p.Components {
			if c.SessionID == 0 && c.SessionKey == "" && c.Portions != nil {
				add(p.SlotID, foodName(st, c.FoodID))
			}
		}
	}
	for _, c := range cooks {
		var q, qd []string
		for _, d := range c.dishes {
			q, qd = append(q, food.Quote(d)), append(qd, deQuote(d))
		}
		en = append(en, joinAnd(q, "and")+" at "+c.cell)
		de = append(de, joinAnd(qd, "und")+" bei "+deCell(c.cell))
	}
	return en, de
}

// refusedPart names what the guard left out: the action with the dish and the
// day and meal it concerns, in English and German, from the plan before the call.
func refusedPart(ctx context.Context, svc Services, w window.Window, op changeOp, r applyRun) (en, de string) {
	var st planning.State
	haveSt := false
	if svc.State != nil {
		if s, err := svc.State(ctx, w, todayFrom(ctx)); err == nil {
			st, haveSt = s, true
		}
	}
	dish, cell, old := "", "", ""
	if haveSt {
		for _, p := range st.Plates {
			match := p.ID == op.PlateID && op.PlateID != 0
			for _, c := range p.Components {
				if op.PlateComponentID != 0 && c.ID == op.PlateComponentID {
					match, dish = true, foodName(st, c.FoodID)
				}
			}
			if match {
				cell = mealName(st, p)
				// The dish a swap or replace takes out: the named component,
				// else the plate's dish at the op's position (main by default).
				for _, c := range p.Components {
					if (op.PlateComponentID != 0 && c.ID == op.PlateComponentID) ||
						(op.PlateComponentID == 0 && string(c.Position) == cmp.Or(op.Position, string(plate.PositionMain))) {
						old = foodName(st, c.FoodID)
					}
				}
				if dish == "" && op.FoodID == 0 {
					for _, c := range p.Components {
						if c.Position == plate.PositionMain {
							dish = foodName(st, c.FoodID)
						}
					}
				}
			}
		}
		if dish == "" && op.FoodID != 0 {
			dish = foodName(st, op.FoodID)
		}
		if cell == "" && op.Date != "" {
			cell = dayLabelStr(op.Date)
			if n := strings.TrimPrefix(st.SlotNames[op.SlotID], "slot."); n != "" && op.SlotID != 0 {
				cell += " " + n
			}
		}
	}
	if applied, ok := r.result["applied"].([]map[string]any); ok && len(applied) > 0 {
		if dish == "" {
			dish, _ = applied[0]["food"].(string)
		}
		if cell == "" && applied[0]["date"] != nil {
			cell = dayLabelStr(fmt.Sprint(applied[0]["date"]))
		}
	}
	cellDE := deCell(cell)
	if len(strings.Fields(cell)) == 3 {
		cellDE = deDay(cell)
	}
	q, qd := food.Quote(dish), deQuote(dish)
	at := func(prep string) string {
		if cell == "" {
			return ""
		}
		return " " + prep + " " + cell
	}
	atDE := func(prep string) string {
		if cell == "" {
			return ""
		}
		return " " + prep + " " + cellDE
	}
	if dish == "" {
		q, qd = "the food", "das Essen"
	}
	switch op.Op {
	case "add":
		return "adding " + q + at("to"), qd + atDE("zu") + " hinzufügen"
	case "remove":
		return "removing " + q + at("from"), qd + atDE("von") + " entfernen"
	case "swap", "replace":
		// "replacing X with Y": the dish taken out and the one asked for, so the
		// line never reads as if the new dish were replaced (V81, tune-v2c
		// g2-replan-01: "replacing "Easy chicken curry"" for curry in place of
		// Shakshuka); the main that stays is named.
		if nw := foodName(st, op.FoodID); haveSt && old != "" && op.FoodID != 0 && nw != "" && old != nw {
			return "replacing " + food.Quote(old) + " with " + food.Quote(nw) + at("at"), deQuote(old) + atDE("bei") + " durch " + deQuote(nw) + " ersetzen"
		}
		return "replacing " + q + at("at"), qd + atDE("bei") + " ersetzen"
	case "set_quantity", "set_portions":
		return "the new portion size of " + q + at("at"), "die neue Portionsgröße von " + qd + atDE("bei")
	case "set_eaters":
		return "the change of people" + at("at"), "die Änderung der Personenzahl" + atDE("bei")
	case "move":
		// The destination too: the user named it, and a reply may say it back (G4.1).
		to, toDE := "", ""
		if c, err := op.toCell(); err == nil && c.Date != "" {
			to = dayLabelStr(c.Date)
			if n := strings.TrimPrefix(st.SlotNames[c.SlotID], "slot."); n != "" && haveSt {
				to += " " + n
			}
			toDE = deCell(to)
			if len(strings.Fields(to)) == 3 {
				toDE = deDay(to)
			}
			to, toDE = " to "+to, " nach "+toDE
		}
		return "moving " + q + at("from") + to, qd + atDE("von") + toDE + " verschieben"
	}
	return "the change" + at("at"), "die Änderung" + atDE("bei")
}

// refusedFact is a part the guard left out, with its reason and numbers.
func refusedFact(whatEN, whatDE, why, whyDE string) map[string]any {
	return map[string]any{
		"en": fmt.Sprintf("Left out: %s (%s).", whatEN, why),
		"de": fmt.Sprintf("Weggelassen: %s (%s).", whatDE, whyDE),
	}
}

// runGuarded is apply_changes' write path: a call whose ops would leave a day
// under its minimum, or create any other hard issue the request did not
// explicitly ask for (away and delete_plate are explicit), is not saved as it
// stands. A call of several independent ops saves the safe subset (each op
// alone, then together) and says what it left out and why, with a verified
// replacement offered for a removed add-on; a single unsafe op is refused with
// the reason and the offer. The model gets the right outcome in one call: it
// never has to work out the subset itself.
func runGuarded(ctx context.Context, svc Services, w window.Window, in applyInput, ignored map[int][]string) (applyRun, error) {
	if in.quiet || len(in.Ops) == 0 {
		return runApply(ctx, svc, w, in, ignored)
	}
	if !slicesAnyOp(in.Ops, func(o changeOp) bool { return !exemptOps[o.Op] }) {
		// Explicit ops are not judged, except against a day the user set to a
		// number under its minimum: an away mark must not carry out "0 kcal".
		if t, ok := turnFrom(ctx); ok && len(t.dayKcal) > 0 {
			probe := in
			probe.DryRun, probe.quiet = true, !in.DryRun
			if pr, err := runApply(ctx, svc, w, probe, ignored); err == nil {
				if res, ok := underFloorRequest(ctx, pr, in.DryRun); ok {
					return res, nil
				}
			}
		}
		return runApply(ctx, svc, w, in, ignored)
	}
	probe := in
	probe.DryRun, probe.quiet = true, !in.DryRun
	pr, err := runApply(ctx, svc, w, probe, ignored)
	if err != nil {
		return applyRun{}, err
	}
	if res, ok := underFloorRequest(ctx, pr, in.DryRun); ok {
		return res, nil
	}
	if len(newHard(pr.before, pr.after)) == 0 {
		if in.DryRun {
			return pr, nil
		}
		return runApply(ctx, svc, w, in, ignored)
	}

	trial := func(ops []changeOp) (applyRun, error) {
		t := in
		t.Ops, t.DryRun, t.quiet = ops, true, true
		return runApply(ctx, svc, w, t, nil)
	}
	type skip struct {
		idx                      int
		what, whatDE, why, whyDE string
	}
	var skipped []skip
	var safe []int
	for i, op := range in.Ops {
		if exemptOps[op.Op] {
			continue
		}
		r, err := trial([]changeOp{op})
		if err != nil {
			// It may depend on an earlier op: the combination decides.
			safe = append(safe, i)
			continue
		}
		if hs := newHard(r.before, r.after); len(hs) > 0 {
			en, de := hardWhyIn(ctx, svc, w, hs[0])
			wEN, wDE := refusedPart(ctx, svc, w, op, r)
			skipped = append(skipped, skip{i, wEN, wDE, en, de})
			continue
		}
		safe = append(safe, i)
	}
	subset := func(idx []int) []changeOp {
		var out []changeOp
		for i, op := range in.Ops {
			if exemptOps[op.Op] || containsInt(idx, i) {
				out = append(out, op)
			}
		}
		return out
	}
	for len(safe) > 0 {
		r, err := trial(subset(safe))
		if err == nil && len(newHard(r.before, r.after)) == 0 {
			break
		}
		last := safe[len(safe)-1]
		why, whyDE := "together with the other changes it would leave a day under its minimum", "zusammen mit den anderen Änderungen würde ein Tag unter seinen Mindestwert fallen"
		if err == nil {
			en, de := hardWhyIn(ctx, svc, w, newHard(r.before, r.after)[0])
			why, whyDE = en+" together with the other changes", de+" zusammen mit den anderen Änderungen"
		}
		wEN, wDE := refusedPart(ctx, svc, w, in.Ops[last], applyRun{})
		skipped = append(skipped, skip{last, wEN, wDE, why, whyDE})
		safe = safe[:len(safe)-1]
	}

	var skippedOut []string
	var skippedList []map[string]any
	var offers []map[string]any
	for _, s := range skipped {
		entry := map[string]any{"op": s.idx + 1, "what": s.what, "why": s.why, "why_de": s.whyDE}
		for k, v := range refusedFact(s.what, s.whatDE, s.why, s.whyDE) {
			entry[k] = v
		}
		skippedList = append(skippedList, entry)
		skippedOut = append(skippedOut, s.what+" ("+s.why+")")
		if o := refusalOffer(ctx, svc, w, in.Ops[s.idx], len(offers) == 0); o != nil {
			offers = append(offers, o)
		}
	}
	var res applyRun
	switch {
	case in.DryRun:
		// A check shows everything the ops would do, and what to pass instead.
		res = pr
		var safeOps []map[string]any
		for _, op := range subset(safe) {
			safeOps = append(safeOps, opJSON(op))
		}
		res.result["safe_ops"] = safeOps
	case len(safe) == 0 && !slicesAnyOp(in.Ops, func(o changeOp) bool { return exemptOps[o.Op] }):
		res = applyRun{before: pr.before, after: pr.after, result: map[string]any{
			"dry_run": false, "saved": false, "applied": []any{}, "refused": skippedList[0]["why"],
		}}
	default:
		final := in
		final.Ops = subset(safe)
		if res, err = runApply(ctx, svc, w, final, nil); err != nil {
			return applyRun{}, err
		}
	}
	res.result["skipped"] = skippedList
	if !in.DryRun && res.result["saved"] != false {
		offers = withBackToRange(ctx, svc, w, res, offers)
	}
	if len(offers) > 0 {
		res.result["offers"] = offers
	}
	left := "left out: " + strings.Join(skippedOut, "; ")
	tell := "; the program tells what was left out and why, "
	if len(offers) > 0 {
		tell += "and offers offers[0]; apply nothing more unasked"
	} else {
		tell += "and that no checked alternative exists: name none, promise none; apply nothing more unasked"
	}
	switch {
	case in.DryRun && len(safe) > 0:
		res.result["next"] = fmt.Sprintf("this check would leave a day under its minimum (%s): apply safe_ops (%d of %d changes), not these ops", left, len(safe), len(in.Ops)) + tell
	case in.DryRun:
		res.result["next"] = "this check would leave a day under its minimum, so nothing here is safe to apply (" + left + ")" + tell
	case res.result["saved"] == false:
		res.result["next"] = "nothing was saved (" + left + ")" + tell
	default:
		res.result["next"] = fmt.Sprintf("the safe part is applied (%d of %d changes); %s", len(in.Ops)-len(skipped), len(in.Ops), left) + tell
	}
	return res, nil
}

// underFloorRequest refuses a call that changes a day the user's message set
// to a kcal number under that day's minimum (C-11, NU9, V77 s-floor-00/-01:
// "Saturday around 600 kcal" was carried out as a lighter dinner that kept
// the day just above its minimum, "left for you to decide"). The request
// itself is under the minimum, so nothing of it is saved, whichever op the
// model picked; the refusal names the minimum.
func underFloorRequest(ctx context.Context, pr applyRun, dryRun bool) (applyRun, bool) {
	t, ok := turnFrom(ctx)
	if !ok || len(t.dayKcal) == 0 {
		return applyRun{}, false
	}
	for _, b := range pr.before.Days {
		want, named := t.dayKcal[b.Date]
		if !named || b.FloorKcal <= 0 || want >= b.FloorKcal-0.5 {
			continue
		}
		i := slices.IndexFunc(pr.after.Days, func(a planning.DayEval) bool { return a.Date == b.Date })
		if i < 0 && t.away {
			continue // the user is away that day: it may leave the plan
		}
		// A day that left the plan counts as 0 kcal.
		if i >= 0 && pr.after.Days[i].Kcal >= b.Kcal-0.5 {
			continue // the call leaves that day as it is or raises it: never refused
		}
		day := dayLabelStr(b.Date)
		whatEN := fmt.Sprintf("taking %s to about %.0f kcal", day, want)
		whatDE := fmt.Sprintf("%s auf etwa %.0f kcal bringen", deDay(day), want)
		why := fmt.Sprintf("%.0f kcal is below the day's minimum of %.0f kcal", want, b.FloorKcal)
		whyDE := fmt.Sprintf("%.0f kcal liegen unter dem Mindestwert des Tages von %.0f kcal", want, b.FloorKcal)
		entry := map[string]any{"op": 1, "what": whatEN, "why": why, "why_de": whyDE}
		for k, v := range refusedFact(whatEN, whatDE, why, whyDE) {
			entry[k] = v
		}
		return applyRun{before: pr.before, after: pr.before, result: map[string]any{
			"dry_run": dryRun, "saved": false, "applied": []any{}, "refused": why,
			"skipped": []map[string]any{entry},
			"next":    "nothing was saved: the user asked for " + whatEN + ", and " + why + "; the program says so and names the minimum. Change nothing else; offer only what find_ways (goal lower_day with this target_kcal) offers",
		}}, true
	}
	return applyRun{}, false
}

// opJSON writes an op as the model sends it: only the fields it set.
func opJSON(op changeOp) map[string]any {
	raw, _ := json.Marshal(op)
	var m map[string]any
	_ = json.Unmarshal(raw, &m)
	for k, v := range m {
		switch x := v.(type) {
		case nil:
			delete(m, k)
		case float64:
			if x == 0 {
				delete(m, k)
			}
		case string:
			if x == "" {
				delete(m, k)
			}
		case []any:
			if len(x) == 0 {
				delete(m, k)
			}
		}
	}
	return m
}

func slicesAnyOp(ops []changeOp, f func(changeOp) bool) bool {
	for _, o := range ops {
		if f(o) {
			return true
		}
	}
	return false
}

func containsInt(l []int, v int) bool {
	for _, x := range l {
		if x == v {
			return true
		}
	}
	return false
}

// replacementOffer offers, for a skipped removal, a fitting option that keeps
// the day above its minimum: the best verified replacement of the component
// (no new hard issue, no new warning), recommended when it is the first offer.
func replacementOffer(ctx context.Context, svc Services, w window.Window, op changeOp, first bool) map[string]any {
	if op.Op != "remove" || op.PlateComponentID == 0 {
		return nil
	}
	st, err := svc.State(ctx, w, todayFrom(ctx))
	if err != nil {
		return nil
	}
	st.Avoid, st.PairAddOns = svc.Removed.avoid(ctx), true
	for _, p := range st.Plates {
		for _, c := range p.Components {
			if c.ID != op.PlateComponentID {
				continue
			}
			res, err := planning.Alternatives(st, planning.WindowTarget(st), planning.AltRequest{PlateID: p.ID, What: planning.AltSide, ComponentID: c.ID, Limit: 4})
			if err != nil || res.Current == nil {
				return nil
			}
			if a, ok := firstClean(res.Alternatives, func(ops []planning.OptionOp) bool { return offerIsClean(ctx, svc, w, ops) }); ok {
				return map[string]any{
					"what":    fmt.Sprintf("swap %s's %s for %s so the day stays above its minimum", mealName(st, p), food.Quote(res.Current.Name), food.Quote(a.Name)),
					"what_de": fmt.Sprintf("bei %s %s durch %s ersetzen, damit der Tag über seinem Mindestwert bleibt", deCell(mealName(st, p)), deQuote(res.Current.Name), deQuote(a.Name)),
					"ops":     a.Ops, "recommended": first, "recommended_because": "keeps the day above its minimum, no new warning",
					"effect": map[string]any{"day": dayLabel(p.Date), "day_kcal_after": a.DayKcal},
				}
			}
		}
	}
	return nil
}

// changeOpsOf reads the ops of a planner way or alternative as apply_changes ops.
func changeOpsOf(in []planning.OptionOp) ([]changeOp, error) {
	ops := make([]changeOp, 0, len(in))
	for _, o := range in {
		raw, err := json.Marshal(o)
		if err != nil {
			return nil, err
		}
		var op changeOp
		if err := json.Unmarshal(raw, &op); err != nil {
			return nil, err
		}
		ops = append(ops, op)
	}
	return ops, nil
}

// firstClean is the best-ranked alternative that lists no new issue and passes
// the check of its own ops: a recommended offer never creates a warning.
func firstClean(alts []planning.Alternative, clean func([]planning.OptionOp) bool) (planning.Alternative, bool) {
	for _, a := range alts {
		if len(a.NewIssues) == 0 && len(a.Ops) > 0 && clean(a.Ops) {
			return a, true
		}
	}
	return planning.Alternative{}, false
}

// offerIsClean runs an offer's ops through the check apply_changes uses
// (rolled back) and says whether it adds no problem and no warning, by the same
// issue keys as report.new_warnings: guard, offers and report share one set.
func offerIsClean(ctx context.Context, svc Services, w window.Window, in []planning.OptionOp) bool {
	ops, err := changeOpsOf(in)
	if err != nil || len(ops) == 0 {
		return false
	}
	r, err := runApply(ctx, svc, w, applyInput{Ops: ops, DryRun: true, quiet: true}, nil)
	if err != nil {
		return false
	}
	was := issueMap(r.before.Issues)
	for _, is := range r.after.Issues {
		if _, old := was[issueKey(is)]; !old {
			return false
		}
	}
	return true
}

// newWarnings are the issues a write of ops makes new, or worse by the same
// measure the report uses: what report.new_warnings would say.
func newWarnings(before, after planning.Evaluation) []planning.Issue {
	was := issueMap(before.Issues)
	var out []planning.Issue
	for _, is := range after.Issues {
		b, old := was[issueKey(is)]
		if !old || worsened(b, is) {
			is.Options = nil
			out = append(out, is)
		}
	}
	return out
}

// verifiedAlternatives passes the pool of find_alternatives through the check
// of a write of each candidate's own ops, so what the model is shown carries
// the warnings the write would produce (a dish repeated this week is only
// seen there). Clean candidates come first, in rank; a candidate that makes a
// problem is dropped; one that only makes a warning is listed after them with
// that warning, up to n, and is never recommended: only the first clean one is.
func verifiedAlternatives(ctx context.Context, svc Services, w window.Window, pool []planning.Alternative, n int) ([]planning.Alternative, []string) {
	var clean, warned []planning.Alternative
	// why holds, per blocking rule, what the real check refused (one line each),
	// for the empty_reason of a list the check emptied.
	var why []string
	note := func(s string) {
		if !slices.Contains(why, s) {
			why = append(why, s)
		}
	}
	for _, a := range pool {
		ops, err := changeOpsOf(a.Ops)
		if err != nil || len(ops) == 0 {
			continue
		}
		r, err := runApply(ctx, svc, w, applyInput{Ops: ops, DryRun: true, quiet: true}, nil)
		if err != nil {
			if errors.Is(err, domain.ErrRowConfirmed) {
				note(fmt.Sprintf("%q would change an accepted meal or cooking", a.Name))
			} else {
				note(fmt.Sprintf("%q could not be applied in the real check", a.Name))
			}
			continue
		}
		if hard := newHard(r.before, r.after); len(hard) > 0 {
			en, _ := hardWhyL(hard[0])
			note(fmt.Sprintf("%q: %s", a.Name, en))
			continue
		}
		if applied, ok := r.result["applied"].([]map[string]any); ok {
			if extra, said := refitOpsOf(applied); len(extra) > 0 {
				a.Ops = append(extra, a.Ops...)
				a.Also = strings.Join(said, "; ")
			}
		}
		a.NewIssues = newWarnings(r.before, r.after)
		a.Recommended, a.Because = false, ""
		if len(a.NewIssues) == 0 {
			clean = append(clean, a)
		} else {
			warned = append(warned, a)
		}
	}
	out := slices.Concat(clean, warned)
	if len(out) > n {
		out = out[:n]
	}
	switch {
	case len(clean) > 0:
		out[0].Recommended = true
		out[0].Because = "the best-ranked candidate: no new problem, no new warning, no extra cooking where possible"
	case len(out) > 0:
		// Every candidate brings a warning: the best is still the one to apply
		// (spec S2/C-5: the first verified alternative), and the program tells
		// its warning (round 7 R4, tune-v2b g2-ask-16: no recommendation, so the
		// model asked again after the user had named the meal).
		out[0].Recommended = true
		out[0].Because = "the best-ranked candidate; it brings a warning the program tells after the change (see new_issues)"
	}
	return out, why
}

// mealName says which meal a plate is, "Wed 25 Nov dinner".
func mealName(st planning.State, p planning.Plate) string {
	return dayLabel(p.Date) + " " + strings.TrimPrefix(st.SlotNames[p.SlotID], "slot.")
}

// withBackToRange adds, for a day the saved part left under its range (the
// household allows it, and the reply says so), one recommended offer that brings
// it back: ONE change (spec C1/G3, OF2), the recommended option of the first
// such day that is no cheese or nut filler (RL6) and checks out clean. An offer
// for the skipped part already there stays the one offer: the under-range days
// are in the facts block, never a second change in the same sentence.
func withBackToRange(ctx context.Context, svc Services, w window.Window, res applyRun, offers []map[string]any) []map[string]any {
	if len(offers) > 0 {
		return offers
	}
	was := issueMap(res.before.Issues)
	for _, is := range res.after.Issues {
		if _, old := was[issueKey(is)]; old || is.Code != planning.CodeKcalBelowBand || len(is.Options) == 0 {
			continue
		}
		o := is.Options[0]
		if isNutFiller(o) {
			continue
		}
		ops := o.Ops
		if len(ops) == 0 {
			ops = []planning.OptionOp{o.Op}
		}
		if !offerIsClean(ctx, svc, w, ops) {
			continue
		}
		return []map[string]any{{
			"what":                fmt.Sprintf("add %s on %s so that day is back in its range", food.Quote(o.Name), dayLabelStr(is.Date)),
			"what_de":             fmt.Sprintf("%s am %s hinzufügen, damit dieser Tag wieder im Bereich liegt", deQuote(o.Name), deDay(dayLabelStr(is.Date))),
			"ops":                 ops,
			"recommended":         true,
			"recommended_because": "brings that day back into its range, no new warning",
			"effect":              map[string]any{"day": dayLabelStr(is.Date), "day_kcal_after": o.DayKcal},
		}}
	}
	return offers
}

// isNutFiller reports an add of a cheese or a nut: never a filler (RL6).
func isNutFiller(o planning.Option) bool {
	return o.Action == "add" && planning.IsCheeseOrNut(o.Name)
}

// hardFigures are the numbers of a hard issue, ", N days with meat, at most M":
// the reason of a refusal carries its figures.
func hardFigures(is planning.Issue) (en, de string) {
	n := func(k string) float64 { return paramNum(is, k) }
	switch is.Code {
	case planning.CodeMeatDaysExceeded:
		return fmt.Sprintf(" (%.0f days with meat, at most %.0f)", n("days"), n("max")), fmt.Sprintf(" (%.0f Tage mit Fleisch, höchstens %.0f)", n("days"), n("max"))
	case planning.CodeTooManyCookOccasions:
		return fmt.Sprintf(" (%.0f cooking times, at most %.0f)", n("count"), n("max")), fmt.Sprintf(" (%.0f Kochzeiten, höchstens %.0f)", n("count"), n("max"))
	}
	keys := make([]string, 0, len(is.Params))
	for k := range is.Params {
		if k != "id" && !strings.HasSuffix(k, "_id") && paramNum(is, k) != 0 {
			keys = append(keys, k)
		}
	}
	slices.Sort(keys)
	var parts, partsDE []string
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s %s", strings.ReplaceAll(k, "_", " "), fmtNum(n(k))))
		partsDE = append(partsDE, fmt.Sprintf("%s %s", strings.ReplaceAll(k, "_", " "), fmtComma(n(k))))
	}
	if len(parts) == 0 {
		return "", ""
	}
	return " (" + strings.Join(parts, ", ") + ")", " (" + strings.Join(partsDE, ", ") + ")"
}
