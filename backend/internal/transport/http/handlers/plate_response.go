package handlers

import (
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain/feedback"
	"github.com/jaltszeimer/plantry/backend/internal/domain/nutrition"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/shopping"
)

type plateComponentResponse struct {
	ID        int64   `json:"id"`
	PlateID   int64   `json:"plate_id"`
	FoodID    int64   `json:"food_id"`
	Portions  float64 `json:"portions"`
	SortOrder int     `json:"sort_order"`
}

type plateResponse struct {
	ID         int64                    `json:"id"`
	Date       string                   `json:"date"`
	SlotID     int64                    `json:"slot_id"`
	Note       *string                  `json:"note,omitempty"`
	Skipped    bool                     `json:"skipped"`
	Components []plateComponentResponse `json:"components"`
	Feedback   *feedbackResponse        `json:"feedback,omitempty"`
	CreatedAt  string                   `json:"created_at"`
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
		Portions: pc.Portions, SortOrder: pc.SortOrder,
	}
}

func toPlateResponse(p *plate.Plate, fb *feedback.PlateFeedback) plateResponse {
	comps := make([]plateComponentResponse, len(p.Components))
	for i := range p.Components {
		comps[i] = toPlateComponentResponse(&p.Components[i])
	}
	resp := plateResponse{
		ID: p.ID, Date: p.DateString(), SlotID: p.SlotID,
		Note: p.Note, Skipped: p.Skipped, Components: comps,
		CreatedAt: p.CreatedAt.Format(time.RFC3339),
	}
	if fb != nil {
		r := toFeedbackResponse(fb)
		resp.Feedback = &r
	}
	return resp
}

func toMacrosResponse(m nutrition.Macros) macrosResponse {
	return macrosResponse{
		Kcal: m.Kcal, Protein: m.Protein, Fat: m.Fat,
		Carbs: m.Carbs, Fiber: m.Fiber, Sodium: m.Sodium,
	}
}

type shoppingListResponse struct {
	Items []shopping.Item `json:"items"`
}
