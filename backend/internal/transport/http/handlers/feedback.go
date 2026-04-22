package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/jaltszeimer/plantry/backend/internal/domain/feedback"
)

// FeedbackHandler exposes plate-feedback endpoints.
type FeedbackHandler struct {
	svc *feedback.Service
}

// NewFeedbackHandler creates a FeedbackHandler.
func NewFeedbackHandler(svc *feedback.Service) *FeedbackHandler {
	return &FeedbackHandler{svc: svc}
}

type feedbackRequest struct {
	Status string  `json:"status"`
	Note   *string `json:"note"`
}

type feedbackResponse struct {
	PlateID int64   `json:"plate_id"`
	Status  string  `json:"status"`
	Note    *string `json:"note,omitempty"`
	RatedAt string  `json:"rated_at"`
}

func toFeedbackResponse(f *feedback.PlateFeedback) feedbackResponse {
	return feedbackResponse{
		PlateID: f.PlateID,
		Status:  string(f.Status),
		Note:    f.Note,
		RatedAt: f.RatedAt.UTC().Format(time.RFC3339),
	}
}

// Put handles PUT /api/plates/{id}/feedback — upserts feedback for the plate.
func (h *FeedbackHandler) Put(w http.ResponseWriter, r *http.Request) {
	plateID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_id")
		return
	}
	var req feedbackRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	fb, err := h.svc.RecordFeedback(r.Context(), plateID, feedback.Status(req.Status), req.Note)
	if err != nil {
		status, key := toHTTPWithResource(err, "plate")
		writeError(w, status, key)
		return
	}
	writeJSON(w, http.StatusOK, toFeedbackResponse(fb))
}

// Delete handles DELETE /api/plates/{id}/feedback.
func (h *FeedbackHandler) Delete(w http.ResponseWriter, r *http.Request) {
	plateID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_id")
		return
	}
	if err := h.svc.DeleteFeedback(r.Context(), plateID); err != nil {
		status, key := toHTTPWithResource(err, "plate")
		writeError(w, status, key)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
