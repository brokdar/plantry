package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
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

// hardWhy says in plain words, with the numbers, what a new hard issue is.
func hardWhy(is planning.Issue) string {
	if is.Code == planning.CodeKcalBelowFloor {
		kcal, _ := is.Params["kcal"].(float64)
		floor, _ := is.Params["floor_kcal"].(float64)
		return fmt.Sprintf("%s would fall to %.0f kcal, under its minimum of %.0f kcal", plainDay(is.Date), kcal, floor)
	}
	return fmt.Sprintf("it would create a problem on %s: %s", plainDay(is.Date), planning.PlainText(is.Code).Label)
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
		idx       int
		what, why string
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
			skipped = append(skipped, skip{i, describeOp(op, r), hardWhy(hs[0])})
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
		why := "together with the other changes it would leave a day under its minimum"
		if err == nil {
			why = hardWhy(newHard(r.before, r.after)[0]) + " together with the other changes"
		}
		skipped = append(skipped, skip{last, describeOp(in.Ops[last], applyRun{}), why})
		safe = safe[:len(safe)-1]
	}

	var skippedOut []string
	var skippedList []map[string]any
	var offers []map[string]any
	for _, s := range skipped {
		skippedList = append(skippedList, map[string]any{"op": s.idx + 1, "what": s.what, "why": s.why})
		skippedOut = append(skippedOut, s.what+" ("+s.why+")")
		if o := replacementOffer(ctx, svc, w, in.Ops[s.idx], len(offers) == 0); o != nil {
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
	if len(offers) > 0 {
		res.result["offers"] = offers
	}
	left := "left out: " + strings.Join(skippedOut, "; ")
	tell := ". Tell the user plainly what was left out and why, with the numbers, "
	if len(offers) > 0 {
		tell += "and present offers[0] (marked recommended) as the way to do it safely; apply nothing more unasked"
	} else {
		tell += "and say what they could change; apply nothing more unasked"
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

// plainDay writes an ISO date as the user reads it ("Wed 25 Nov"); text that
// is not a date stays as it is.
func plainDay(iso string) string {
	d, err := time.Parse(time.DateOnly, iso)
	if err != nil {
		return iso
	}
	return dayLabel(d)
}

// describeOp names an op in words for the skipped list: a removal by food and
// date (from the check that ran it), else the op and its date.
func describeOp(op changeOp, r applyRun) string {
	if applied, ok := r.result["applied"].([]map[string]any); ok && len(applied) > 0 {
		if food, ok := applied[0]["food"].(string); ok {
			return fmt.Sprintf("%s %s on %v", op.Op, food, plainDay(fmt.Sprint(applied[0]["date"])))
		}
	}
	if op.Date != "" {
		return fmt.Sprintf("%s on %s", op.Op, plainDay(op.Date))
	}
	return op.Op
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
			for _, a := range res.Alternatives {
				if len(a.NewIssues) == 0 && len(a.Ops) > 0 {
					return map[string]any{
						"what": fmt.Sprintf("replace %s on %s with %s so the day stays above its minimum", res.Current.Name, dayLabel(p.Date), a.Name),
						"ops":  a.Ops, "recommended": first, "recommended_because": "keeps the day above its minimum, no new warning",
					}
				}
			}
		}
	}
	return nil
}
