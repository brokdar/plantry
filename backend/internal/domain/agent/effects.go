package agent

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
)

// affected names the rows a plan-writing tool touched, so its result can say
// what changed and the model need not call get_plan to verify.
type affected struct {
	Plates   []int64
	Sessions []int64
	Dates    []string // extra dates whose issues matter (a deleted plate's day)
}

// effectPlate and effectSession are the compact shapes of a tool's `effect`.
type effectPlate struct {
	ID         int64    `json:"plate_id"`
	Date       string   `json:"date"`
	SlotID     int64    `json:"slot_id"`
	State      string   `json:"state"`
	Eaters     float64  `json:"eaters"`
	Components []string `json:"components"`
	// Per eater, from the evaluation (apply_changes `after` only).
	Kcal     *float64                 `json:"kcal,omitempty"`
	ProteinG *float64                 `json:"protein_g,omitempty"`
	Details  []planning.ComponentEval `json:"component_details,omitempty"`
}

type effectSession struct {
	ID             int64    `json:"session_id"`
	Food           string   `json:"food"`
	CookDate       string   `json:"cook_date"`
	Portions       float64  `json:"portions"`
	FrozenPortions float64  `json:"frozen_portions"`
	Consumed       float64  `json:"consumed"`
	FrozenConsumed float64  `json:"frozen_consumed"`
	Surplus        float64  `json:"surplus"`
	State          string   `json:"state"`
	Cooked         bool     `json:"cooked"`
	Warnings       []string `json:"warnings"`
}

type effectIssue struct {
	Rule     string         `json:"rule"`
	Code     string         `json:"code"`
	Severity string         `json:"severity"`
	Date     string         `json:"date,omitempty"`
	SlotID   *int64         `json:"slot_id,omitempty"`
	PlateID  *int64         `json:"plate_id,omitempty"`
	Session  *int64         `json:"session_id,omitempty"`
	Params   map[string]any `json:"params,omitempty"`
	// Options are the computed fixes of a soft issue (planning.Option).
	Options     []planning.Option `json:"options,omitempty"`
	NoFixFound  bool              `json:"no_fix_found,omitempty"`
	NoFixReason string            `json:"no_fix_reason,omitempty"`
	// WasUnderRange marks a day that was under its range before the write too.
	WasUnderRange bool `json:"was_under_range,omitempty"`
}

// effectWorsened is an issue open before and after a write whose numbers got
// worse: Before and After are its params then and now.
type effectWorsened struct {
	Rule     string         `json:"rule"`
	Code     string         `json:"code"`
	Severity string         `json:"severity"`
	Date     string         `json:"date,omitempty"`
	SlotID   *int64         `json:"slot_id,omitempty"`
	PlateID  *int64         `json:"plate_id,omitempty"`
	Session  *int64         `json:"session_id,omitempty"`
	Before   map[string]any `json:"before,omitempty"`
	After    map[string]any `json:"after,omitempty"`
}

// changeEffect reads the affected plates and sessions after a write, plus the
// checker issues and session warnings (H3/H4: deficit, frozen_deficit, ...)
// that touch them. It never fails the tool: a read that fails is logged and
// the part is left out.
func changeEffect(ctx context.Context, svc Services, a affected) map[string]any {
	if svc.Plates == nil {
		return map[string]any{"issues_checked": false, "issues_unchecked_reason": "no plate reader; call get_plan to check"}
	}
	plates := []effectPlate{}
	dates := map[string]bool{}
	for _, d := range a.Dates {
		dates[d] = true
	}
	sessionIDs := slices.Clone(a.Sessions)
	plateIDs := map[int64]bool{}
	for _, id := range a.Plates {
		p, err := svc.Plates.Get(ctx, id)
		if err != nil {
			continue // deleted by the call
		}
		plateIDs[id] = true
		dates[p.DateString()] = true
		names := make([]string, len(p.Components))
		for i, c := range p.Components {
			names[i] = food.Quote(c.FoodName)
			if c.FoodRole != nil && *c.FoodRole != "" {
				names[i] += " (" + *c.FoodRole + ")"
			}
			if c.CookSessionID != nil {
				sessionIDs = append(sessionIDs, *c.CookSessionID)
			}
		}
		plates = append(plates, effectPlate{
			ID: p.ID, Date: p.DateString(), SlotID: p.SlotID, State: string(p.State), Eaters: p.Eaters, Components: names,
		})
	}
	slices.Sort(sessionIDs)
	sessionIDs = slices.Compact(sessionIDs)
	sessions := []effectSession{}
	sessSet := map[int64]bool{}
	for _, id := range sessionIDs {
		v, err := svc.Plates.Session(ctx, id)
		if err != nil {
			continue
		}
		sessSet[id] = true
		warn := make([]string, len(v.Warnings))
		for i, w := range v.Warnings {
			warn[i] = w.Code
		}
		sessions = append(sessions, effectSession{
			ID: v.ID, Food: food.Data(v.FoodName), CookDate: v.CookDate.Format(time.DateOnly),
			Portions: v.Portions, FrozenPortions: v.FrozenPortions, Consumed: v.Consumed,
			FrozenConsumed: v.FrozenConsumed, Surplus: v.Surplus, State: string(v.State),
			Cooked: v.CookedAt != nil, Warnings: warn,
		})
	}
	out := map[string]any{"plates": plates, "sessions": sessions}
	issueDelta(ctx, svc, out)
	return out
}

// maxEffectIssues caps each issue list of an `after` block.
const maxEffectIssues = 10

func issueKey(is planning.Issue) string {
	n := func(p *int64) int64 {
		if p == nil {
			return 0
		}
		return *p
	}
	// A warning is the same warning wherever its plate went: an extra is
	// identified by its food and reason, a repeated dish by its food, a light
	// or filling meal by its cell. Plate and batch ids are not part of it, so
	// a move or an undo (new ids) does not make an old warning new.
	switch is.Rule {
	case "S9":
		return fmt.Sprintf("S9|%s|%v|%v", is.Code, is.Params["food_id"], is.Params["reason"])
	case "S8":
		return fmt.Sprintf("S8|%s|%v", is.Code, is.Params["food_id"])
	case "S6", "S6b":
		return fmt.Sprintf("%s|%s|%s|%d", is.Rule, is.Code, is.Date, n(is.SlotID))
	}
	return fmt.Sprintf("%s|%s|%s|%d|%d|%d", is.Rule, is.Code, is.Date, n(is.SlotID), n(is.PlateID), n(is.SessionID))
}

type issueBaselineKey struct{}

// issueBaseline is the window's issues before a write, keyed by issueKey.
// ok is false when they could not be read (reason says why).
type issueBaseline struct {
	ok     bool
	reason string
	issues map[string]planning.Issue
}

// readIssues returns the window's checker issues by key, or why they could
// not be read.
func readIssues(ctx context.Context, svc Services) (map[string]planning.Issue, string) {
	w, ok := windowFrom(ctx)
	if !ok {
		return nil, "no planning window"
	}
	if svc.PlanView == nil {
		return nil, "no plan reader"
	}
	p, err := svc.PlanView.Plan(ctx, &w.Start, todayFrom(ctx))
	if err != nil {
		slog.WarnContext(ctx, "agent.effect.plan failed", "error", err)
		return nil, "plan read failed"
	}
	m := make(map[string]planning.Issue, len(p.Issues))
	for _, is := range p.Issues {
		m[issueKey(is)] = is
	}
	return m, ""
}

// withIssueBaseline snapshots the window's issues before a write tool runs,
// so its `after` block can report what the write changed.
func withIssueBaseline(svc Services, h ToolHandler) ToolHandler {
	return func(ctx context.Context, in json.RawMessage) (json.RawMessage, ToolEffect, error) {
		m, reason := readIssues(ctx, svc)
		ctx = context.WithValue(ctx, issueBaselineKey{}, &issueBaseline{ok: reason == "", reason: reason, issues: m})
		return h(ctx, in)
	}
}

// issueDelta adds the issues this write made new or resolved, with the open
// total, to out. When the issues could not be read before or after the write
// it says so (issues_checked false) instead of returning an empty list that
// would read as a clean result.
func issueDelta(ctx context.Context, svc Services, out map[string]any) {
	base, _ := ctx.Value(issueBaselineKey{}).(*issueBaseline)
	reason := "issues not read before the change"
	if base != nil && !base.ok {
		reason += ": " + base.reason
	}
	now, afterReason := readIssues(ctx, svc)
	if afterReason != "" {
		reason = "issues not read after the change: " + afterReason
	}
	if base == nil || !base.ok || afterReason != "" {
		out["issues_checked"] = false
		out["issues_unchecked_reason"] = reason + "; call get_plan to check"
		return
	}
	res := deltaOf(base.issues, now)
	out["issues_checked"] = true
	out["issues"] = res
}

// deltaOf is the issues now that base lacked (new), base's that are gone
// (resolved) and those open in both whose numbers got worse (worsened, only
// when there are any), each list capped at maxEffectIssues, with the open
// total.
func deltaOf(base, now map[string]planning.Issue) map[string]any {
	noOptions := func(is planning.Issue) effectIssue {
		e := toEffectIssue(is)
		e.Options = nil // a resolved issue offers nothing: its options were checked against the old plan
		return e
	}
	newIs, resolved := []effectIssue{}, []effectIssue{}
	var worse []effectWorsened
	for k, is := range now {
		had, ok := base[k]
		switch {
		case !ok:
			newIs = append(newIs, toEffectIssue(is))
		case worsened(had, is):
			worse = append(worse, effectWorsened{
				Rule: is.Rule, Code: is.Code, Severity: string(is.Severity), Date: is.Date,
				SlotID: is.SlotID, PlateID: is.PlateID, Session: is.SessionID, Before: had.Params, After: is.Params,
			})
		}
	}
	for k, is := range base {
		if _, still := now[k]; !still {
			resolved = append(resolved, noOptions(is))
		}
	}
	res := map[string]any{"total_open": len(now)}
	for name, list := range map[string][]effectIssue{"new": newIs, "resolved": resolved} {
		slices.SortFunc(list, func(a, b effectIssue) int {
			return cmp.Or(cmp.Compare(a.Date, b.Date), cmp.Compare(a.Rule, b.Rule), cmp.Compare(a.Code, b.Code))
		})
		if len(list) > maxEffectIssues {
			res[name+"_more"] = len(list) - maxEffectIssues
			list = list[:maxEffectIssues]
		}
		res[name] = list
	}
	if len(worse) > 0 {
		slices.SortFunc(worse, func(a, b effectWorsened) int {
			return cmp.Or(cmp.Compare(a.Date, b.Date), cmp.Compare(a.Rule, b.Rule), cmp.Compare(a.Code, b.Code))
		})
		if len(worse) > maxEffectIssues {
			res["worsened_more"] = len(worse) - maxEffectIssues
			worse = worse[:maxEffectIssues]
		}
		res["worsened"] = worse
	}
	return res
}

// badness is how bad an open issue is, larger is worse, from the numbers in
// its params; false for an issue without a measure (a rule that is either
// broken or not).
func badness(is planning.Issue) (float64, bool) {
	p := func(k string) float64 {
		switch v := is.Params[k].(type) {
		case float64:
			return v
		case int:
			return float64(v)
		}
		return 0
	}
	switch is.Code {
	case planning.CodeKcalBelowBand:
		return p("band_low_kcal") - p("kcal"), true
	case planning.CodeKcalAboveBand:
		return p("kcal") - p("band_high_kcal"), true
	case planning.CodeKcalBelowFloor:
		return p("floor_kcal") - p("kcal"), true
	case planning.CodeProteinLow:
		return p("min_g") - p("protein_g"), true
	case planning.CodeMealNotLight:
		return p("kcal") - p("max_kcal"), true
	case planning.CodeMealNotHearty:
		return p("min_kcal") - p("kcal"), true
	case planning.CodeWeekMeatAboveDGE:
		return p("meat_g") - p("max_g"), true
	case planning.CodeMeatDaysExceeded:
		return p("days") - p("max"), true
	case planning.CodeWeekKcalBelowBand:
		return p("band_low_pct") - p("pct"), true
	case planning.CodeWeekKcalAboveBand:
		return p("pct") - p("band_high_pct"), true
	case planning.CodeWeekProteinLow:
		return p("target_protein_g") - p("avg_protein_g"), true
	case planning.CodeWeekCarbsRange, planning.CodeWeekFatRange:
		return max(p("range_low_pct")-p("pct"), p("pct")-p("range_high_pct")), true
	case planning.CodeTooManyCookOccasions:
		return p("count") - p("max"), true
	case planning.CodeDishRepeats:
		return p("meals") - p("max"), true
	}
	return 0, false
}

// worseStep is the smallest step in badness's unit that counts as "got
// worse" for an issue that was already open: a share moving 0.2 points or the
// week's meat 4 g further out is the same warning, not a new one. Day and meal
// kcal: 50 kcal; protein grams: 5 g; week meat: 25 g; shares (weekly energy,
// carbs, fat): 1 percentage point; counts (meat days, cooking times, dish
// repeats): 1.
func worseStep(code string) float64 {
	switch code {
	case planning.CodeKcalBelowBand, planning.CodeKcalAboveBand, planning.CodeKcalBelowFloor, planning.CodeMealNotLight, planning.CodeMealNotHearty:
		return 50
	case planning.CodeProteinLow, planning.CodeWeekProteinLow:
		return 5
	case planning.CodeWeekMeatAboveDGE:
		return 25
	}
	return 1
}

// worsened reports whether an issue open before and after got worse by at
// least its worseStep.
func worsened(before, after planning.Issue) bool {
	b, ok := badness(before)
	if !ok {
		return false
	}
	a, ok := badness(after)
	return ok && a >= b+worseStep(after.Code)-1e-9
}

func toEffectIssue(is planning.Issue) effectIssue {
	return effectIssue{
		Rule: is.Rule, Code: is.Code, Severity: string(is.Severity), Date: is.Date,
		SlotID: is.SlotID, PlateID: is.PlateID, Session: is.SessionID, Params: is.Params, Options: is.Options,
		NoFixFound: is.NoFixFound, NoFixReason: is.NoFixReason,
	}
}

// withEffect renders result (a JSON object) with the call's `after` state added.
// A result that is not an object is returned as it is.
func withEffect(ctx context.Context, svc Services, result any, a affected) json.RawMessage {
	raw := mustJSON(result)
	var obj map[string]json.RawMessage
	if json.Unmarshal(raw, &obj) != nil || obj == nil {
		return raw
	}
	obj["after"] = mustJSON(changeEffect(ctx, svc, a))
	return mustJSON(obj)
}
