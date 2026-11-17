package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

// Meal operations (meal editing slice 2b): thin handlers over plate.Service
// that return {plates, session_ids, also_changed, undo}.

type mealCell struct {
	Date   string `json:"date"`
	SlotID int64  `json:"slot_id"`
}

func (c mealCell) key() (plate.CellKey, bool) {
	d, err := time.Parse(time.DateOnly, c.Date)
	if err != nil || c.SlotID <= 0 {
		return plate.CellKey{}, false
	}
	return plate.CellKey{Date: d, SlotID: c.SlotID}, true
}

type mealResultResponse struct {
	Plates      []plateResponse     `json:"plates"`
	SessionIDs  []int64             `json:"session_ids"`
	AlsoChanged []plate.AlsoChanged `json:"also_changed"`
	Undo        plate.Undo          `json:"undo"`
}

func toMealResultResponse(res plate.MealResult) mealResultResponse {
	out := mealResultResponse{
		Plates: make([]plateResponse, len(res.Plates)), SessionIDs: make([]int64, len(res.Sessions)),
		AlsoChanged: res.AlsoChanged, Undo: res.Undo,
	}
	for i := range res.Plates {
		out.Plates[i] = toPlateResponse(&res.Plates[i])
	}
	for i, s := range res.Sessions {
		out.SessionIDs[i] = s.ID
	}
	if out.AlsoChanged == nil {
		out.AlsoChanged = []plate.AlsoChanged{}
	}
	if out.Undo.Cells == nil {
		out.Undo.Cells = []plate.CellUndo{}
	}
	return out
}

// writeMealError writes err; a taken cell's 409 names the occupant in
// occupant_plate_id.
func writeMealError(w http.ResponseWriter, err error) {
	var occ *plate.OccupiedError
	if errors.As(err, &occ) {
		writeJSON(w, http.StatusConflict, map[string]any{
			"message_key": "error.plate.cell_occupied", "status": http.StatusConflict, "occupant_plate_id": occ.PlateID,
		})
		return
	}
	writePlateError(w, err)
}

type moveRequest struct {
	Date     string          `json:"date"`
	SlotID   int64           `json:"slot_id"`
	Occupant json.RawMessage `json:"occupant"`
}

// parseOccupant reads "swap", "drop" or {date, slot_id}; absent or null is
// no occupant.
func parseOccupant(raw json.RawMessage) (plate.Occupant, bool) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return plate.Occupant{}, true
	}
	var kind string
	if json.Unmarshal(raw, &kind) == nil {
		switch plate.OccupantKind(kind) {
		case plate.OccupantSwap, plate.OccupantDrop:
			return plate.Occupant{Kind: plate.OccupantKind(kind)}, true
		}
		return plate.Occupant{}, false
	}
	var c mealCell
	if json.Unmarshal(raw, &c) != nil {
		return plate.Occupant{}, false
	}
	to, ok := c.key()
	if !ok {
		return plate.Occupant{}, false
	}
	return plate.Occupant{Kind: plate.OccupantTo, To: to}, true
}

// Move handles POST /api/plates/{id}/move with {date, slot_id, occupant?}.
func (h *PlateHandler) Move(w http.ResponseWriter, r *http.Request) {
	id, ok := parseIDParam(w, r, "id")
	if !ok {
		return
	}
	var req moveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	to, ok := mealCell{Date: req.Date, SlotID: req.SlotID}.key()
	occ, okOcc := parseOccupant(req.Occupant)
	if !ok || !okOcc {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	res, err := h.svc.MoveMeal(r.Context(), id, to, occ)
	if err != nil {
		writeMealError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toMealResultResponse(res))
}

type replaceRequest struct {
	Position         string    `json:"position"`
	PlateComponentID int64     `json:"plate_component_id"`
	FoodID           int64     `json:"food_id"`
	SessionID        int64     `json:"session_id"`
	CookAt           *mealCell `json:"cook_at"`
	Portions         *float64  `json:"portions"`
	Amount           *float64  `json:"amount"`
	Unit             *string   `json:"unit"`
}

// Replace handles POST /api/plates/{id}/replace with {position? |
// plate_component_id?, food_id, session_id?, cook_at?, portions? |
// amount?+unit?}.
func (h *PlateHandler) Replace(w http.ResponseWriter, r *http.Request) {
	id, ok := parseIDParam(w, r, "id")
	if !ok {
		return
	}
	var req replaceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	if req.FoodID <= 0 || (req.Position == "") == (req.PlateComponentID == 0) {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	src := plate.Source{SessionID: req.SessionID}
	if req.CookAt != nil {
		c, ok := req.CookAt.key()
		if !ok {
			writeError(w, http.StatusBadRequest, "error.invalid_body")
			return
		}
		src.CookAt = &c
	}
	var qty *plate.PlateComponent
	if req.Portions != nil || req.Amount != nil || req.Unit != nil {
		qty = &plate.PlateComponent{Portions: req.Portions, Amount: req.Amount, Unit: req.Unit}
	}
	sel := plate.Selector{Position: plate.Position(req.Position), ComponentID: req.PlateComponentID}
	res, err := h.svc.ReplaceComponent(r.Context(), id, sel, req.FoodID, src, qty)
	if err != nil {
		writeMealError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toMealResultResponse(res))
}

type copyRequest struct {
	Cells []mealCell `json:"cells"`
	As    string     `json:"as"`
}

// Copy handles POST /api/plates/{id}/copy with {cells, as}.
func (h *PlateHandler) Copy(w http.ResponseWriter, r *http.Request) {
	id, ok := parseIDParam(w, r, "id")
	if !ok {
		return
	}
	var req copyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.Cells) == 0 {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	cells := make([]plate.CellKey, len(req.Cells))
	for i, c := range req.Cells {
		k, ok := c.key()
		if !ok {
			writeError(w, http.StatusBadRequest, "error.invalid_body")
			return
		}
		cells[i] = k
	}
	res, err := h.svc.CopyMeal(r.Context(), id, cells, plate.CopyAs(req.As))
	if err != nil {
		writeMealError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toMealResultResponse(res))
}

// MoveCook handles POST /api/cook-sessions/{id}/move-cook with {date,
// slot_id}.
func (h *CookSessionHandler) MoveCook(w http.ResponseWriter, r *http.Request) {
	id, ok := parseIDParam(w, r, "id")
	if !ok {
		return
	}
	var req mealCell
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	to, ok := req.key()
	if !ok {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	res, err := h.svc.MoveCook(r.Context(), id, to)
	if err != nil {
		writeMealError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toMealResultResponse(res))
}

// Revert handles POST /api/plan/revert with a hand edit's undo token
// {first_event_id, last_event_id, cells?} and returns {plate_ids,
// session_ids}. A stale token is 409 error.plan.change_stale.
func (h *AvailabilityHandler) Revert(w http.ResponseWriter, r *http.Request) {
	var u plate.Undo
	if err := json.NewDecoder(r.Body).Decode(&u); err != nil || u.FirstEventID < 0 || u.LastEventID < u.FirstEventID {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	rows, err := h.svc.Revert(r.Context(), u)
	if err != nil {
		writePlateError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toRowSetResponse(rows))
}
