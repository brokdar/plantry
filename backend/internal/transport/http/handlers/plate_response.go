package handlers

import (
	"context"
	"net/http"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain/nutrition"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

type plateComponentResponse struct {
	ID            int64    `json:"id"`
	PlateID       int64    `json:"plate_id"`
	FoodID        int64    `json:"food_id"`
	FoodName      string   `json:"food_name"`
	Kind          string   `json:"kind"`
	Role          *string  `json:"role,omitempty"`
	Portions      *float64 `json:"portions,omitempty"`
	CookSessionID *int64   `json:"cook_session_id,omitempty"`
	Amount        *float64 `json:"amount,omitempty"`
	Unit          *string  `json:"unit,omitempty"`
	Grams         *float64 `json:"grams,omitempty"`
	GramsSource   *string  `json:"grams_source,omitempty"`
	SortOrder     int      `json:"sort_order"`
}

type plateFeedbackResponse struct {
	Status  string  `json:"status"`
	Note    *string `json:"note"`
	RatedAt string  `json:"rated_at"`
}

type plateResponse struct {
	ID         int64                    `json:"id"`
	Date       string                   `json:"date"`
	SlotID     int64                    `json:"slot_id"`
	Eaters     float64                  `json:"eaters"`
	Note       *string                  `json:"note"`
	State      string                   `json:"state"`
	AIRunID    *int64                   `json:"ai_run_id"`
	Reason     *string                  `json:"reason"`
	Components []plateComponentResponse `json:"components"`
	Feedback   *plateFeedbackResponse   `json:"feedback"`
	CreatedAt  string                   `json:"created_at"`
	UpdatedAt  string                   `json:"updated_at"`
}

type macrosResponse struct {
	Kcal    float64 `json:"kcal"`
	Protein float64 `json:"protein"`
	Fat     float64 `json:"fat"`
	Carbs   float64 `json:"carbs"`
	Fiber   float64 `json:"fiber"`
	Sodium  float64 `json:"sodium"`
}

func toPlateComponentResponse(pc *plate.PlateComponent) plateComponentResponse {
	return plateComponentResponse{
		ID: pc.ID, PlateID: pc.PlateID, FoodID: pc.FoodID,
		FoodName:      pc.FoodName,
		Kind:          pc.FoodKind,
		Role:          pc.FoodRole,
		Portions:      pc.Portions,
		CookSessionID: pc.CookSessionID,
		Amount:        pc.Amount,
		Unit:          pc.Unit,
		Grams:         pc.Grams,
		GramsSource:   pc.GramsSource,
		SortOrder:     pc.SortOrder,
	}
}

func toPlateResponse(p *plate.Plate) plateResponse {
	comps := make([]plateComponentResponse, len(p.Components))
	for i := range p.Components {
		comps[i] = toPlateComponentResponse(&p.Components[i])
	}
	resp := plateResponse{
		ID: p.ID, Date: p.DateString(), SlotID: p.SlotID, Eaters: p.Eaters,
		Note: p.Note, State: string(p.State), AIRunID: p.AIRunID, Reason: p.Reason, Components: comps,
		CreatedAt: p.CreatedAt.Format(time.RFC3339),
		UpdatedAt: p.UpdatedAt.Format(time.RFC3339),
	}
	if p.Feedback != nil {
		resp.Feedback = &plateFeedbackResponse{
			Status:  p.Feedback.Status,
			Note:    p.Feedback.Note,
			RatedAt: p.Feedback.RatedAt.UTC().Format(time.RFC3339),
		}
	}
	return resp
}

func toMacrosResponse(m nutrition.Macros) macrosResponse {
	return macrosResponse{
		Kcal: m.Kcal, Protein: m.Protein, Fat: m.Fat,
		Carbs: m.Carbs, Fiber: m.Fiber, Sodium: m.Sodium,
	}
}

// rowSetResponse names the plates and sessions a state change returns
// (confirm, unlock, accept, unaccept).
type rowSetResponse struct {
	PlateIDs   []int64 `json:"plate_ids"`
	SessionIDs []int64 `json:"session_ids"`
}

func toRowSetResponse(r plate.RowSet) rowSetResponse {
	resp := rowSetResponse{PlateIDs: r.PlateIDs, SessionIDs: r.SessionIDs}
	if resp.PlateIDs == nil {
		resp.PlateIDs = []int64{}
	}
	if resp.SessionIDs == nil {
		resp.SessionIDs = []int64{}
	}
	return resp
}

// stateChange is a Confirm or Unlock of plate.Service.
type stateChange func(ctx context.Context, seed plate.RowSet) (plate.RowSet, error)

// writeStateChange runs change on the batch closure of seed (D3) and writes
// the closure.
func writeStateChange(w http.ResponseWriter, r *http.Request, seed plate.RowSet, change stateChange) {
	rows, err := change(r.Context(), seed)
	if err != nil {
		writePlateError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toRowSetResponse(rows))
}

// removedResponse is the body of every removing mutation: what restores it.
type removedResponse struct {
	Removed plate.PlanSnapshot `json:"removed"`
}
