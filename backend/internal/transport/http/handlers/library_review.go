package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/jaltszeimer/plantry/backend/internal/domain/libreview"
)

// LibraryReviewHandler serves the monthly library review card (LIB-1..3).
type LibraryReviewHandler struct{ svc *libreview.Service }

// NewLibraryReviewHandler creates a LibraryReviewHandler.
func NewLibraryReviewHandler(svc *libreview.Service) *LibraryReviewHandler {
	return &LibraryReviewHandler{svc: svc}
}

func (h *LibraryReviewHandler) fail(w http.ResponseWriter, err error) {
	status, key := toHTTPWithResource(err, "review")
	writeError(w, status, key)
}

// Get handles GET /api/library-review: {card}, null when the review is off,
// dismissed or has nothing to say this month.
func (h *LibraryReviewHandler) Get(w http.ResponseWriter, r *http.Request) {
	card, err := h.svc.Get(r.Context())
	if err != nil {
		h.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"card": card})
}

type reviewKey struct {
	Key string `json:"key"`
}

// Dismiss handles POST /api/library-review/dismiss {key?}: one finding, or
// the whole card without a key.
func (h *LibraryReviewHandler) Dismiss(w http.ResponseWriter, r *http.Request) {
	var req reviewKey
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	if err := h.svc.Dismiss(r.Context(), req.Key); err != nil {
		h.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// MakeVariants handles POST /api/library-review/variants {key}: groups the
// two recipes of a duplicates finding as variants.
func (h *LibraryReviewHandler) MakeVariants(w http.ResponseWriter, r *http.Request) {
	var req reviewKey
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Key == "" {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	if err := h.svc.MakeVariants(r.Context(), req.Key); err != nil {
		h.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
