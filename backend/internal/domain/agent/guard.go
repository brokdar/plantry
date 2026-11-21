package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

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
	return fmt.Sprintf("it would create a problem on %s: %s", dayLabelStr(is.Date), planning.PlainText(is.Code).Label),
		fmt.Sprintf("es würde am %s ein Problem entstehen: %s", deDay(dayLabelStr(is.Date)), labelDE)
}

// hardLabelDE is the plain label of a hard issue in German.
var hardLabelDE = map[string]string{
	planning.CodeTooManyCookOccasions: "mehr getrennte Kochzeiten an einem Tag als erlaubt",
	planning.CodeMeatDaysExceeded:     "mehr Fleischtage in der Woche als erlaubt",
	planning.CodeDietIncompatible:     "ein Lebensmittel passt nicht zur Ernährungsweise",
	planning.CodeKcalBelowFloor:       "der Tag läge unter seinem Mindestwert",
	planning.CodeVarietyRepeat:        "ein Hauptgericht wurde zu kürzlich gekocht oder gegessen",
	"leftover_before_cook":            "Reste würden vor dem Kochen gegessen",
	"past_keep_days":                  "Reste wären nicht mehr haltbar",
	"deficit":                         "es würde mehr gegessen, als gekocht wird",
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
	dish, cell := "", ""
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
		return "replacing " + q + at("at"), qd + atDE("bei") + " ersetzen"
	case "set_quantity", "set_portions":
		return "the new portion size of " + q + at("at"), "die neue Portionsgröße von " + qd + atDE("bei")
	case "set_eaters":
		return "the change of people" + at("at"), "die Änderung der Personenzahl" + atDE("bei")
	case "move":
		return "moving " + q + at("from"), qd + atDE("von") + " verschieben"
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
	if in.Goal != nil || in.quiet || len(in.Ops) == 0 || !slicesAnyOp(in.Ops, func(o changeOp) bool { return !exemptOps[o.Op] }) {
		return runApply(ctx, svc, w, in, ignored)
	}
	probe := in
	probe.DryRun, probe.quiet = true, !in.DryRun
	pr, err := runApply(ctx, svc, w, probe, ignored)
	if err != nil {
		return applyRun{}, err
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
			en, de := hardWhyL(hs[0])
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
			en, de := hardWhyL(newHard(r.before, r.after)[0])
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
		entry := map[string]any{"op": s.idx + 1, "what": s.what, "why": s.why}
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
	tell := ". Tell the user plainly what was left out and why, with the numbers, "
	if len(offers) > 0 {
		tell += "and present offers[0] (marked recommended) as the way to do it safely; apply nothing more unasked"
	} else {
		tell += "and say plainly that no checked alternative exists for it: name none, promise none; apply nothing more unasked"
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
func verifiedAlternatives(ctx context.Context, svc Services, w window.Window, pool []planning.Alternative, n int) []planning.Alternative {
	var clean, warned []planning.Alternative
	for _, a := range pool {
		ops, err := changeOpsOf(a.Ops)
		if err != nil || len(ops) == 0 {
			continue
		}
		r, err := runApply(ctx, svc, w, applyInput{Ops: ops, DryRun: true, quiet: true}, nil)
		if err != nil {
			continue
		}
		if len(newHard(r.before, r.after)) > 0 {
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
	if len(clean) > 0 {
		out[0].Recommended = true
		out[0].Because = "the best-ranked candidate: no new problem, no new warning, no extra cooking where possible"
	}
	return out
}

// mealName says which meal a plate is, "Wed 25 Nov dinner".
func mealName(st planning.State, p planning.Plate) string {
	return dayLabel(p.Date) + " " + strings.TrimPrefix(st.SlotNames[p.SlotID], "slot.")
}

// withBackToRange adds, for the days the saved part left under their range
// (the household allows it, and the reply says so), one recommended offer that
// brings them back: the recommended option of each such day's issue, verified
// together. With an offer for the skipped part already there, the two become
// one offer when they check out together.
func withBackToRange(ctx context.Context, svc Services, w window.Window, res applyRun, offers []map[string]any) []map[string]any {
	was := issueMap(res.before.Issues)
	var ops []planning.OptionOp
	var texts, textsDE []string
	var effect []map[string]any
	for _, is := range res.after.Issues {
		if _, old := was[issueKey(is)]; old || is.Code != planning.CodeKcalBelowBand || len(is.Options) == 0 {
			continue
		}
		o := is.Options[0]
		oops := o.Ops
		if len(oops) == 0 {
			oops = []planning.OptionOp{o.Op}
		}
		ops = append(ops, oops...)
		texts = append(texts, fmt.Sprintf("%s on %s", food.Quote(o.Name), dayLabelStr(is.Date)))
		textsDE = append(textsDE, fmt.Sprintf("%s am %s", deQuote(o.Name), deDay(dayLabelStr(is.Date))))
		effect = append(effect, map[string]any{"day": dayLabelStr(is.Date), "day_kcal_after": o.DayKcal})
	}
	if len(ops) == 0 {
		return offers
	}
	what := "add " + strings.Join(texts, " and ") + " so those days are back in their range"
	whatDE := strings.Join(textsDE, " und ") + " hinzufügen, damit diese Tage wieder im Bereich liegen"
	if len(offers) > 0 {
		first := offers[0]
		firstOps, _ := first["ops"].([]planning.OptionOp)
		merged := slices.Concat(firstOps, ops)
		if offerIsClean(ctx, svc, w, merged) {
			first["ops"] = merged
			first["what"] = fmt.Sprint(first["what"]) + "; and " + what
			first["what_de"] = fmt.Sprint(first["what_de"]) + "; und " + whatDE
			first["effect"] = []any{first["effect"], effect}
			first["recommended_because"] = "brings every day back above its minimum and into its range, no new warning"
			return offers
		}
		return append(offers, map[string]any{"what": what, "what_de": whatDE, "ops": ops, "recommended": false, "effect": effect})
	}
	if !offerIsClean(ctx, svc, w, ops) {
		return offers
	}
	return []map[string]any{{"what": what, "what_de": whatDE, "ops": ops, "recommended": true, "recommended_because": "brings those days back into their range, no new warning", "effect": effect}}
}
