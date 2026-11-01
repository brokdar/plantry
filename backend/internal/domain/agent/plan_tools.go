package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planview"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

// PlanReader is the read model behind GET /api/plan (planview.Service).
type PlanReader interface {
	Plan(ctx context.Context, start *time.Time, today time.Time) (*planview.Plan, error)
}

// toolGetPlan returns the chat window's plan: the same cells, sessions and
// issues as GET /api/plan for that window, in a compact shape (D10).
func toolGetPlan(svc Services) Tool {
	return Tool{
		Name:        "get_plan",
		Description: "Read the current plan window: every cell (date, slot, availability mode, plate with its components), the cook sessions with consumption and warnings, the checker's issues and the draft summary. Call it to see the effect of your changes.",
		Schema:      json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
		Handler: func(ctx context.Context, _ json.RawMessage) (json.RawMessage, ToolEffect, error) {
			w, ok := windowFrom(ctx)
			if !ok {
				return nil, ToolEffectNone, fmt.Errorf("%w: no plan window in this conversation turn", domain.ErrInvalidInput)
			}
			p, err := svc.PlanView.Plan(ctx, &w.Start, todayFrom(ctx))
			if err != nil {
				return nil, ToolEffectNone, err
			}
			return mustJSON(planOutput(p)), ToolEffectNone, nil
		},
	}
}

type planOut struct {
	Window   planWindowOut    `json:"window"`
	Cells    []planCellOut    `json:"cells"`
	Sessions []planSessionOut `json:"sessions"`
	Issues   []planning.Issue `json:"issues"`
	Draft    *planDraftOut    `json:"draft"`
}

type planWindowOut struct {
	Start       string `json:"start"`
	End         string `json:"end"`
	Today       string `json:"today"`
	ShoppingDay int    `json:"shopping_day"`
}

type planCellOut struct {
	Date       string        `json:"date"`
	SlotID     int64         `json:"slot_id"`
	CellMode   string        `json:"mode"`
	PresetName *string       `json:"preset_name,omitempty"`
	Note       *string       `json:"note,omitempty"`
	Plate      *planPlateOut `json:"plate"`
}

type planPlateOut struct {
	ID         int64              `json:"id"`
	State      string             `json:"state"`
	Eaters     float64            `json:"eaters"`
	Reason     *string            `json:"reason,omitempty"`
	Components []planComponentOut `json:"components"`
}

type planComponentOut struct {
	ID        int64    `json:"id"`
	FoodID    int64    `json:"food_id"`
	Name      string   `json:"name"`
	Portions  *float64 `json:"portions,omitempty"`
	Amount    *float64 `json:"amount,omitempty"`
	Unit      *string  `json:"unit,omitempty"`
	SessionID *int64   `json:"session_id,omitempty"`
}

type planSessionOut struct {
	ID             int64            `json:"id"`
	FoodID         int64            `json:"food_id"`
	Name           string           `json:"name"`
	CookDate       string           `json:"cook_date"`
	CookSlotID     int64            `json:"cook_slot_id"`
	Portions       float64          `json:"portions"`
	FrozenPortions float64          `json:"frozen_portions"`
	State          string           `json:"state"`
	Cooked         bool             `json:"cooked"`
	Consumed       float64          `json:"consumed"`
	FrozenConsumed float64          `json:"frozen_consumed"`
	UseBy          string           `json:"use_by"`
	Links          []planLinkOut    `json:"links"`
	Warnings       []planWarningOut `json:"warnings"`
}

type planLinkOut struct {
	PlateID     int64   `json:"plate_id"`
	ComponentID int64   `json:"component_id"`
	Portions    float64 `json:"portions"`
	Kind        string  `json:"kind"`
}

type planWarningOut struct {
	Code    string `json:"code"`
	PlateID *int64 `json:"plate_id,omitempty"`
}

type planDraftOut struct {
	Plates   int     `json:"plates"`
	Sessions int     `json:"sessions"`
	RunID    *int64  `json:"run_id"`
	Notes    *string `json:"notes"`
}

func planOutput(p *planview.Plan) planOut {
	out := planOut{
		Window: planWindowOut{
			Start: p.Window.Start.Format(time.DateOnly), End: p.Window.End.Format(time.DateOnly),
			Today: p.Today.Format(time.DateOnly), ShoppingDay: p.ShoppingDay,
		},
		Cells:    make([]planCellOut, len(p.Cells)),
		Sessions: make([]planSessionOut, len(p.Sessions)),
		Issues:   p.Issues,
	}
	if out.Issues == nil {
		out.Issues = []planning.Issue{}
	}
	for i, c := range p.Cells {
		out.Cells[i] = planCellOut{
			Date: c.Date.Format(time.DateOnly), SlotID: c.SlotID, CellMode: string(c.Mode),
			PresetName: c.PresetName, Note: c.Note,
		}
		if c.Plate != nil {
			out.Cells[i].Plate = planPlate(c.Plate)
		}
	}
	for i, v := range p.Sessions {
		s := planSessionOut{
			ID: v.ID, FoodID: v.FoodID, Name: v.FoodName,
			CookDate: v.CookDate.Format(time.DateOnly), CookSlotID: v.CookSlotID,
			Portions: v.Portions, FrozenPortions: v.FrozenPortions, State: string(v.State),
			Cooked: v.CookedAt != nil, Consumed: v.Consumed, FrozenConsumed: v.FrozenConsumed,
			UseBy: v.UseBy.Format(time.DateOnly),
			Links: make([]planLinkOut, len(v.Links)), Warnings: make([]planWarningOut, len(v.Warnings)),
		}
		for j, l := range v.Links {
			s.Links[j] = planLinkOut{PlateID: l.PlateID, ComponentID: l.ComponentID, Portions: l.Portions, Kind: string(l.Kind)}
		}
		for j, w := range v.Warnings {
			s.Warnings[j] = planWarningOut{Code: w.Code, PlateID: w.PlateID}
		}
		out.Sessions[i] = s
	}
	if d := p.Draft; d != nil {
		out.Draft = &planDraftOut{Plates: d.Plates, Sessions: d.Sessions, RunID: d.RunID, Notes: d.Notes}
	}
	return out
}

func planPlate(p *plate.Plate) *planPlateOut {
	out := &planPlateOut{
		ID: p.ID, State: string(p.State), Eaters: p.Eaters, Reason: p.Reason,
		Components: make([]planComponentOut, len(p.Components)),
	}
	for i, c := range p.Components {
		out.Components[i] = planComponentOut{
			ID: c.ID, FoodID: c.FoodID, Name: c.FoodName,
			Portions: c.Portions, Amount: c.Amount, Unit: c.Unit, SessionID: c.CookSessionID,
		}
	}
	return out
}
