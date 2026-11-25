package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/devclock"
)

// DevHandler serves the dev-only clock endpoint (registered only in dev mode).
type DevHandler struct {
	clock *devclock.Clock
}

// NewDevHandler creates a DevHandler over the shared clock.
func NewDevHandler(clock *devclock.Clock) *DevHandler { return &DevHandler{clock: clock} }

type devClockRequest struct {
	Now          string `json:"now"`           // RFC 3339 or YYYY-MM-DD (08:00 UTC)
	AdvanceHours *int   `json:"advance_hours"` // alternative: move forward
}

type devClockResponse struct {
	Now   string `json:"now"`
	Today string `json:"today"`
}

func (h *DevHandler) respond(w http.ResponseWriter) {
	n := h.clock.Now().UTC()
	writeJSON(w, http.StatusOK, devClockResponse{Now: n.Format(time.RFC3339), Today: n.Format(time.DateOnly)})
}

// GetClock returns the simulated time.
func (h *DevHandler) GetClock(w http.ResponseWriter, _ *http.Request) { h.respond(w) }

// SetClock sets the simulated time or advances it by whole hours.
func (h *DevHandler) SetClock(w http.ResponseWriter, r *http.Request) {
	var req devClockRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || (req.Now == "") == (req.AdvanceHours == nil) {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	if req.AdvanceHours != nil {
		if *req.AdvanceHours < 1 {
			writeError(w, http.StatusBadRequest, "error.invalid_body")
			return
		}
		h.clock.Advance(time.Duration(*req.AdvanceHours) * time.Hour)
	} else {
		t, err := devclock.Parse(req.Now)
		if err != nil {
			writeError(w, http.StatusBadRequest, "error.invalid_body")
			return
		}
		h.clock.Set(t)
	}
	h.respond(w)
}
