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
func adviceFor(ctx context.Context, svc Services, w window.Window, before, after planning.Evaluation, dry bool) (string, []map[string]any) {
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
		// The cleanest replacement (the list is ranked by new warnings); one that
		// still creates a warning says which.
		for _, a := range res.Alternatives {
			if len(a.Ops) == 0 {
				continue
			}
			offer := map[string]any{
				"what": fmt.Sprintf("replace %s on %s with %s", res.Current.Name, is.Date, a.Name), "ops": a.Ops,
				"recommended": len(offers) == 0, "recommended_because": "removes the repeat, no new warning",
			}
			if len(a.NewIssues) > 0 {
				var labels []string
				for _, ni := range a.NewIssues {
					labels = append(labels, planning.PlainText(ni.Code).Label)
				}
				offer["creates_warning"] = labels
				offer["recommended_because"] = "removes the repeat; the closest available, it creates a warning (creates_warning)"
			}
			offers = append(offers, offer)
			break
		}
	}
	switch {
	case len(offers) > 0:
		return "the change is done as asked and now repeats a dish: tell the user, present offers[0] as your recommended way out, and apply nothing unasked", offers
	case hard:
		return "the change is done as asked and left a day under its minimum: tell the user plainly and offer the fix marked recommended as a question; apply nothing unasked", nil
	case options:
		return "the change is done as asked and made new issues: report them and offer the option marked recommended as a question; apply nothing unasked", nil
	case len(fresh) > 0:
		return "the change is done as asked and made new issues: report them plainly; apply nothing unasked", nil
	}
	return "", nil
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
