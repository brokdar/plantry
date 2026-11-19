package agent

import (
	"context"
	"fmt"
	"sync"

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
func recommend(ways, offers []planning.Way) {
	clean := func(a planning.Way) bool { return len(a.NewIssues) == 0 && len(a.Worsened) == 0 }
	mark := func(a *planning.Way, because string) { a.Recommended, a.Because = true, because }
	switch {
	case len(ways) > 0 && clean(ways[0]) && ways[0].Reaches:
		mark(&ways[0], "the least change that reaches the goal, no new warning")
	case len(ways) > 0 && clean(ways[0]):
		mark(&ways[0], "the furthest this meal can go on its own, no new warning")
	case len(offers) > 0 && clean(offers[0]):
		mark(&offers[0], "the only way to reach it that keeps the day in its range, no new warning")
	case len(ways) > 0:
		mark(&ways[0], "the closest way, but it creates a new warning (see why)")
	case len(offers) > 0:
		mark(&offers[0], "the closest way, but it creates a new warning (see why)")
	}
}

// adviceFor builds what a plain apply_changes call (no goal) tells the model
// about what it left: when the ops made issues new it says to report them and
// offer the recommended fix as a question, applying nothing unasked, and for a
// dish that now repeats on another plate it offers a verified replacement.
func adviceFor(ctx context.Context, svc Services, w window.Window, before, after planning.Evaluation, dry bool, applied []map[string]any) (string, []map[string]any) {
	was := issueMap(before.Issues)
	var fresh []planning.Issue
	options, hard := false, false
	for _, is := range after.Issues {
		if _, old := was[issueKey(is)]; !old {
			hard = hard || is.Severity == planning.SeverityViolation
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
		if a, ok := firstClean(res.Alternatives, func(ops []planning.OptionOp) bool { return offerIsClean(ctx, svc, w, ops) }); ok {
			offers = append(offers, map[string]any{
				"what": fmt.Sprintf("replace %s on %s with %s", res.Current.Name, plainDay(is.Date), a.Name), "ops": a.Ops,
				"recommended": len(offers) == 0, "recommended_because": "removes the repeat, no new warning",
			})
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
			if o := meatBalanceOffer(ctx, svc, w, touchedPlates(applied), after.Week.MeatG); o != nil {
				offers = append(offers, o)
			}
		}
	}
	// A check says what the change would do; a saved change says what it did.
	// The offer is the extra step: it is never applied unasked.
	lead, tail := "the change is done as asked and", "; do not apply the offer unasked"
	var what string
	switch {
	case len(offers) > 0:
		what = "repeat a dish"
	case hard:
		what = "leave a day under its minimum"
	case options, len(fresh) > 0:
		what = "make new issues"
	case meat != "":
		what = "raise the week's meat"
	default:
		return "", nil
	}
	if dry {
		// Warnings never stop a requested change: it is saved, then reported.
		return "NOT saved (a check). A question about what would happen: answer from this result. A requested change: save it now with the same ops and dry_run false, never ask first; it would " + what + ", which you then report", offers
	}
	if meat != "" {
		text := lead + " now " + meat + ": tell the user"
		if len(offers) > 0 {
			text += ", present offers[0] (a meat meal swapped for a vegetarian one) as your recommended way to balance it"
		} else {
			text += " and offer to swap a meat meal for a vegetarian one"
		}
		return text + tail, offers
	}
	switch {
	case len(offers) > 0:
		return lead + " now repeats a dish: tell the user, present offers[0] as your recommended way out" + tail, offers
	case hard:
		return lead + " left a day under its minimum: tell the user plainly and offer the fix marked recommended as a question" + tail, nil
	case options:
		return lead + " made new issues: report them and offer the option marked recommended as a question" + tail, nil
	}
	return lead + " made new issues: report them plainly", nil
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
func meatBalanceOffer(ctx context.Context, svc Services, w window.Window, touched map[int64]bool, now float64) map[string]any {
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
					"what": fmt.Sprintf("replace %s on %s with %s: the week's meat falls from %.0f to %.0f g", res.Current.Name, dayLabel(p.Date), a.Name, now, a.WeekMeatG),
					"ops":  a.Ops, "recommended": true, "recommended_because": "takes the most meat off the week, no new warning",
				}
			}
		}
	}
	return best
}
