package agent

import (
	"context"
	"fmt"
	"math"
	"slices"
	"sync"

	"github.com/jaltszeimer/plantry/backend/internal/domain/food"

	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/domain/window"
)

// removedMemo remembers, per conversation, the foods the user had taken off a
// plate: a later way, option or alternative never adds one back ("remove the
// yogurt from all dinners" followed by a fix that adds yogurt). It lives in
// memory like the unlock previews; a restart forgets it.
type removedMemo struct {
	mu sync.Mutex
	m  map[int64]map[int64]bool
}

func (r *removedMemo) add(ctx context.Context, foods []int64) {
	t, ok := turnFrom(ctx)
	if r == nil || !ok || len(foods) == 0 {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.m == nil {
		r.m = map[int64]map[int64]bool{}
	}
	if r.m[t.conversationID] == nil {
		r.m[t.conversationID] = map[int64]bool{}
	}
	for _, f := range foods {
		r.m[t.conversationID][f] = true
	}
}

type extraAvoidKey struct{}

// withExtraAvoid carries the foods the running call has removed so far, which
// the memory only learns once the call is saved: the options of that same
// call must not suggest them back.
func withExtraAvoid(ctx context.Context, foods []int64) context.Context {
	return context.WithValue(ctx, extraAvoidKey{}, foods)
}

// avoid is the set of foods removed in ctx's conversation and by the running
// call.
func (r *removedMemo) avoid(ctx context.Context) map[int64]bool {
	out := map[int64]bool{}
	extra, _ := ctx.Value(extraAvoidKey{}).([]int64)
	for _, f := range extra {
		out[f] = true
	}
	t, ok := turnFrom(ctx)
	if r == nil || !ok {
		return out
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for f := range r.m[t.conversationID] {
		out[f] = true
	}
	return out
}

// recommend marks exactly one of the verified ways and offers as the
// recommendation, with the reason in one phrase: the first way when it creates
// no new warning (the named meal alone, as the household asked), else the first
// clean offer, else the first way or offer with the warning it creates named.
func recommend(ways, offers []planning.Way, toLimit bool) {
	clean := func(a planning.Way) bool { return len(a.NewIssues) == 0 && len(a.Worsened) == 0 }
	mark := func(a *planning.Way, because string) { a.Recommended, a.Because = true, because }
	switch {
	case toLimit && len(offers) > 0 && offers[0].Reaches && (len(ways) == 0 || !ways[0].Reaches):
		// A light meal was asked for and only the offer is one.
		because := "the only way to a light meal: it reaches the limit"
		if !clean(offers[0]) {
			because += ", but it creates a new warning (see why)"
		}
		mark(&offers[0], because)
	case len(ways) > 0 && clean(ways[0]) && ways[0].Reaches:
		mark(&ways[0], "the least change that reaches the goal, no new warning")
	case len(ways) > 0 && clean(ways[0]):
		mark(&ways[0], "the furthest this meal can go on its own, no new warning")
	case len(offers) > 0 && !offers[0].Reaches && clean(offers[0]):
		mark(&offers[0], "the closest the plan allows, no new warning (not a light meal)")
	case len(offers) > 0 && clean(offers[0]):
		mark(&offers[0], "the only way to reach it that keeps the day in its range, no new warning")
	case len(ways) > 0:
		mark(&ways[0], "the closest way, but it creates a new warning (see why)")
	case len(offers) > 0:
		mark(&offers[0], "the closest way, but it creates a new warning (see why)")
	}
}

// adviceFor builds what a plain apply_changes call (no goal) tells the model
// about what it left: the program tells new issues and their offer (G4), the
// model applies nothing unasked; for a dish that now repeats on another plate
// it offers a verified replacement.
func adviceFor(ctx context.Context, svc Services, w window.Window, before, after planning.Evaluation, dry bool, applied []map[string]any) (string, []map[string]any) {
	was := issueMap(before.Issues)
	var fresh []planning.Issue
	options, hard, other := false, false, false
	for _, is := range after.Issues {
		if _, old := was[issueKey(is)]; !old {
			hard = hard || (is.Severity == planning.SeverityViolation && is.Code == planning.CodeKcalBelowFloor)
			other = other || (is.Severity == planning.SeverityViolation && is.Code != planning.CodeKcalBelowFloor)
			fresh = append(fresh, is)
		}
	}
	var offers []map[string]any
	var st *planning.State
	for _, is := range fresh {
		options = options || len(is.Options) > 0
		// A dry run rolled back: its plates are not there to look alternatives up.
		if dry || is.Code != planning.CodeVarietyRepeatWeek || len(offers) >= 2 {
			continue
		}
		if st == nil {
			s, err := svc.State(ctx, w, todayFrom(ctx))
			if err != nil {
				continue
			}
			s.Avoid, s.PairAddOns = svc.Removed.avoid(ctx), true
			st = &s
		}
		plateID := repeatedPlate(*st, is)
		if plateID == 0 {
			continue
		}
		res, err := planning.Alternatives(*st, planning.WindowTarget(*st), planning.AltRequest{PlateID: plateID, What: planning.AltMain, Limit: 3})
		if err != nil || res.Current == nil {
			continue
		}
		// The cleanest replacement (the list is ranked by new warnings).
		meal := mealName(*st, st.Plates[indexPlate(*st, plateID)])
		if a, ok := firstClean(res.Alternatives, func(ops []planning.OptionOp) bool { return offerIsClean(ctx, svc, w, ops) }); ok {
			offers = append(offers, map[string]any{
				"what": fmt.Sprintf("swap %s's %s for %s", meal, food.Quote(res.Current.Name), food.Quote(a.Name)), "ops": a.Ops,
				"what_de":     fmt.Sprintf("bei %s %s durch %s ersetzen", deCell(meal), deQuote(res.Current.Name), deQuote(a.Name)),
				"recommended": len(offers) == 0, "recommended_because": "removes the repeat, no new warning",
				"effect": map[string]any{"day": dayLabelStr(is.Date), "day_kcal_after": a.DayKcal},
			})
		} else if o := replaceAndFixOffer(ctx, svc, w, *st, meal, res, is.Date); o != nil {
			o["recommended"] = len(offers) == 0
			offers = append(offers, o)
		}
	}
	repeated := len(offers) > 0
	// A day left under its minimum gets the verified fix that reaches it, or
	// none: the reply then says so instead of promising one.
	if hard && !dry {
		for _, is := range fresh {
			if is.Code != planning.CodeKcalBelowFloor {
				continue
			}
			if o := floorFixOffer(ctx, svc, w, is, after.Issues); o != nil {
				offers = append(offers, o)
				break
			}
			// RL9: no unmarked meal can take a fix: the offer lifts a light mark.
			if o := unLightOffer(ctx, svc, w, is); o != nil {
				offers = append(offers, o)
				break
			}
		}
	}
	// A named meat dish saves, and its week effect is told with one way to
	// balance it (a meat meal elsewhere swapped); tools never offer such a dish.
	meat := ""
	if planning.MeatOverStep(before, after) {
		var limit float64
		for _, is := range after.Issues {
			if is.Code == planning.CodeWeekMeatAboveDGE {
				limit = paramNum(is, "max_g")
			}
		}
		meat = fmt.Sprintf("the week's meat goes from %.0f to %.0f g, over the %.0f g limit", before.Week.MeatG, after.Week.MeatG, limit)
		if !dry && len(offers) == 0 {
			if o := meatBalanceOffer(ctx, svc, w, touchedPlates(applied), after.Week.MeatG, limit); o != nil {
				offers = append(offers, o)
			}
		}
	}
	// A check says what the change would do; a saved change says what it did.
	// The offer is the extra step: it is never applied unasked.
	lead, tail, told := "the change is done as asked and", "; do not apply the offer unasked and do not offer it in your text (the program does)", ": the program tells it and its offer"
	if len(offers) > 0 {
		told = ": the program tells it and offers offers[0]"
	}
	var what string
	switch {
	case repeated:
		what = "repeat a dish"
	case hard:
		what = "leave a day under its minimum"
	case other:
		what = "leave a problem open"
	case options, len(fresh) > 0:
		what = "make new issues"
	case meat != "":
		what = "raise the week's meat"
	default:
		return "", nil
	}
	if dry {
		// Warnings never stop a requested change: it is saved, then reported.
		return "NOT saved (a check). A question about what would happen: answer from this result. A requested change: save it now with the same ops and dry_run false, never ask first; it would " + what + ", which the program then tells", offers
	}
	if meat != "" && !hard {
		return lead + " now " + meat + told + tail, offers
	}
	switch {
	case repeated:
		return lead + " now repeats a dish" + told + tail, offers
	case hard && len(offers) > 0:
		return lead + " left a day under its minimum" + told + tail, offers
	case hard:
		return lead + " left a day under its minimum and no checked fix reaches it" + told + "; offer no fix, name none", nil
	case other && options:
		return lead + " left a problem open (see issues.new)" + told + tail, nil
	case other:
		return lead + " left a problem open (see issues.new) and no checked fix exists" + told + "; offer no fix, name none", nil
	case options:
		return lead + " made new issues" + told + tail, nil
	}
	return lead + " made new issues" + told, nil
}

// repeatedPlate is the latest draft plate that eats the dish a repeat warning
// names (the warning sits on the batch, not on a plate), 0 when there is none.
func repeatedPlate(st planning.State, is planning.Issue) int64 {
	foodID, _ := is.Params["food_id"].(int64)
	if f, ok := is.Params["food_id"].(float64); ok {
		foodID = int64(f)
	}
	var best planning.Plate
	for _, p := range st.Plates {
		if p.Confirmed || p.ID <= 0 || (!best.Date.IsZero() && !p.Date.After(best.Date)) {
			continue
		}
		for _, c := range p.Components {
			if c.FoodID == foodID && (is.SessionID == nil || c.SessionID == *is.SessionID) && !p.Date.Before(st.Today) {
				best = p
			}
		}
	}
	return best.ID
}

// touchedPlates are the plates the applied ops of a call named.
func touchedPlates(applied []map[string]any) map[int64]bool {
	out := map[int64]bool{}
	for _, a := range applied {
		switch id := a["plate_id"].(type) {
		case int64:
			out[id] = true
		case float64:
			out[int64(id)] = true
		}
	}
	return out
}

// meatBalanceOffer is the verified swap of a meat main on another draft plate
// for a vegetarian one that takes the most meat off the week, with no new
// problem or warning; nil when none helps by a real step.
func meatBalanceOffer(ctx context.Context, svc Services, w window.Window, touched map[int64]bool, now, limit float64) map[string]any {
	if svc.State == nil {
		return nil
	}
	st, err := svc.State(ctx, w, todayFrom(ctx))
	if err != nil {
		return nil
	}
	st.Avoid, st.PairAddOns = svc.Removed.avoid(ctx), true
	var best map[string]any
	bestMeat := now - planning.MeatStepG
	for _, p := range st.Plates {
		if touched[p.ID] || p.Confirmed || !st.Window.Contains(p.Date) {
			continue
		}
		res, err := planning.Alternatives(st, planning.WindowTarget(st), planning.AltRequest{PlateID: p.ID, What: planning.AltMain, Constraints: planning.AltConstraints{Diet: "vegetarian"}, Limit: 5})
		if err != nil || res.Current == nil {
			continue
		}
		for _, a := range res.Alternatives {
			if len(a.NewIssues) == 0 && len(a.Ops) > 0 && a.WeekMeatG < bestMeat && offerIsClean(ctx, svc, w, a.Ops) {
				bestMeat = a.WeekMeatG
				best = map[string]any{
					"what":    fmt.Sprintf("swap %s's %s for %s: the week's meat falls from %.0f to %.0f g", mealName(st, p), food.Quote(res.Current.Name), food.Quote(a.Name), now, a.WeekMeatG),
					"what_de": fmt.Sprintf("bei %s %s durch %s ersetzen: das Fleisch der Woche sinkt von %.0f auf %.0f g", deCell(mealName(st, p)), deQuote(res.Current.Name), deQuote(a.Name), now, a.WeekMeatG),
					"ops":     a.Ops, "recommended": true, "recommended_because": "takes the most meat off the week, no new warning",
					"effect": map[string]any{"week_meat_g_now": math.Round(now), "week_meat_g_after": math.Round(a.WeekMeatG), "week_meat_limit_g": limit},
				}
			}
		}
	}
	return best
}

// sideClashOffer turns the clash a user-named main swap left (applied entry
// "side_clash") into the one recommended offer. Its effect is the dry run of
// its own ops on the plan the swap leaves (the saved plan, or for a check the
// swap's ops followed by the offer's), so what it promises is what it does.
func sideClashOffer(ctx context.Context, svc Services, w window.Window, in applyInput, applied []map[string]any) map[string]any {
	var clash map[string]any
	for _, a := range applied {
		if c, ok := a["side_clash"].(map[string]any); ok {
			clash = c
			break
		}
	}
	if clash == nil {
		return nil
	}
	offerOps, _ := clash["ops"].([]planning.OptionOp)
	ops, err := changeOpsOf(offerOps)
	if err != nil || len(ops) == 0 {
		return nil
	}
	if in.DryRun {
		ops = slices.Concat(in.Ops, ops)
	}
	r, err := runApply(ctx, svc, w, applyInput{Ops: ops, DryRun: true, quiet: true}, nil)
	if err != nil {
		return nil
	}
	fresh := newWarnings(r.before, r.after)
	plateID, _ := clash["plate_id"].(int64)
	date, _ := clash["date"].(string)
	effect := map[string]any{"new_warnings": len(fresh)}
	if date != "" {
		f := planning.FiguresOf(r.after, plateID, date)
		effect["day_kcal_after"] = f.DayKcal
	}
	return map[string]any{
		"what": clash["what"], "what_de": clash["what_de"], "ops": offerOps, "clash": clash["clash"],
		"recommended": true, "recommended_because": "one starch on the plate, the change you asked for stays as it is",
		"effect": effect,
	}
}

// clashNext puts the clash into the next-step text: told, offered, never done.
func clashNext(next string, offer map[string]any, dry bool) string {
	text := fmt.Sprintf("the swap is done as asked and nothing else was changed; the program tells the clash and offers offers[0] (%v): apply it only on a yes", offer["what"])
	if dry {
		text = "the swap as asked changes nothing else; after saving it, say plainly that the side does not suit the main, without naming the dish; the program offers offers[0] (" + fmt.Sprint(offer["what"]) + "): apply it only on a yes"
	}
	if next == "" {
		return text
	}
	return text + ". Also: " + next
}

// replaceAndFixOffer is the way out of a repeated dish when every replacement
// leaves its day under its range: the first replacement whose only new issue is
// that, together with the one normal-food fix that brings the day inside, as
// one offer. Its effect is the dry run of both together, which must add no
// issue; nil when none does.
func replaceAndFixOffer(ctx context.Context, svc Services, w window.Window, st planning.State, meal string, res planning.AltResult, date string) map[string]any {
	ctx = context.WithValue(ctx, fixDateKey{}, date)
	for _, a := range res.Alternatives {
		if len(a.Ops) == 0 || slices.ContainsFunc(a.NewIssues, func(is planning.Issue) bool { return is.Code != planning.CodeKcalBelowBand || is.Date != date }) {
			continue
		}
		ops, err := changeOpsOf(a.Ops)
		if err != nil {
			continue
		}
		r, err := runApply(ctx, svc, w, applyInput{Ops: ops, DryRun: true, quiet: true}, nil)
		if err != nil {
			continue
		}
		for _, is := range r.after.Issues {
			if is.Date != date || is.Code != planning.CodeKcalBelowBand {
				continue
			}
			for _, o := range fixCandidates(is.Options, num(is.Params["kcal"]), num(is.Params["band_low_kcal"]), num(is.Params["band_high_kcal"])) {
				fixOps := o.Ops
				if len(fixOps) == 0 {
					fixOps = []planning.OptionOp{o.Op}
				}
				both := slices.Concat(a.Ops, fixOps)
				bothOps, err := changeOpsOf(both)
				if err != nil {
					continue
				}
				rb, err := runApply(ctx, svc, w, applyInput{Ops: bothOps, DryRun: true, quiet: true}, nil)
				if err != nil {
					continue
				}
				was := issueMap(rb.before.Issues)
				clean := true
				for _, x := range rb.after.Issues {
					if _, old := was[issueKey(x)]; !old {
						clean = false
					}
				}
				if !clean {
					continue
				}
				fix := planning.WayFix{Action: o.Action, Name: o.Name, Date: o.Date, SlotID: o.SlotID}
				if o.Grams != nil {
					fix.Grams = *o.Grams
				}
				if o.Portions != nil {
					fix.Portions = *o.Portions
				}
				return map[string]any{
					"what":    fmt.Sprintf("swap %s's %s for %s and, so the day stays inside its range, %s", meal, food.Quote(res.Current.Name), food.Quote(a.Name), fixWords(st, &fix)),
					"what_de": fmt.Sprintf("bei %s %s durch %s ersetzen und, damit der Tag im Bereich bleibt, %s", deCell(meal), deQuote(res.Current.Name), deQuote(a.Name), fixWordsDE(st, &fix)),
					"ops":     both, "recommended_because": "removes the repeat and keeps the day inside its range, no new warning",
					"effect": map[string]any{"day": dayLabelStr(date), "day_kcal_after": planning.FiguresOf(rb.after, 0, date).DayKcal},
				}
			}
		}
	}
	return nil
}
