package agent

import (
	"context"
	"encoding/json"
	"log/slog"
	"slices"
	"strconv"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
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
}

type effectSession struct {
	ID             int64    `json:"session_id"`
	Food           string   `json:"food"`
	CookDate       string   `json:"cook_date"`
	Portions       float64  `json:"portions"`
	FrozenPortions float64  `json:"frozen_portions"`
	Consumed       float64  `json:"consumed"`
	FrozenConsumed float64  `json:"frozen_consumed"`
	Spare          float64  `json:"spare"`
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
}

// changeEffect reads the affected plates and sessions after a write, plus the
// checker issues and session warnings (H3/H4: deficit, frozen_deficit, ...)
// that touch them. It never fails the tool: a read that fails is logged and
// the part is left out.
func changeEffect(ctx context.Context, svc Services, a affected) map[string]any {
	if svc.Plates == nil {
		return map[string]any{}
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
			names[i] = c.FoodName
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
			ID: v.ID, Food: v.FoodName, CookDate: v.CookDate.Format(time.DateOnly),
			Portions: v.Portions, FrozenPortions: v.FrozenPortions, Consumed: v.Consumed,
			FrozenConsumed: v.FrozenConsumed, Spare: v.Surplus, State: string(v.State),
			Cooked: v.CookedAt != nil, Warnings: warn,
		})
	}
	issues := []effectIssue{}
	if w, ok := windowFrom(ctx); ok && svc.PlanView != nil {
		p, err := svc.PlanView.Plan(ctx, &w.Start, todayFrom(ctx))
		if err != nil {
			slog.WarnContext(ctx, "agent.effect.plan failed", "error", err)
		} else {
			for _, is := range p.Issues {
				hit := dates[is.Date]
				if is.PlateID != nil && plateIDs[*is.PlateID] {
					hit = true
				}
				if is.SessionID != nil && sessSet[*is.SessionID] {
					hit = true
				}
				if hit {
					issues = append(issues, effectIssue{
						Rule: is.Rule, Code: is.Code, Severity: string(is.Severity), Date: is.Date,
						SlotID: is.SlotID, PlateID: is.PlateID, Session: is.SessionID, Params: is.Params,
					})
				}
			}
		}
	}
	return map[string]any{"plates": plates, "sessions": sessions, "issues": issues}
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

// plateIDsOf lists the ids of plates.
func plateIDsOf(ps ...*plate.Plate) []int64 {
	ids := make([]int64, len(ps))
	for i, p := range ps {
		ids[i] = p.ID
	}
	return ids
}

// fnum formats a portion count for an error message.
func fnum(v float64) string { return strconv.FormatFloat(v, 'g', -1, 64) }
