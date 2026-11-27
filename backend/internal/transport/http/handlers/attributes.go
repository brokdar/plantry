package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/jaltszeimer/plantry/backend/internal/domain/flavor"
	"github.com/jaltszeimer/plantry/backend/internal/domain/foodevent"
)

// AttributeHandler serves a food's recipe attributes (ATTR).
type AttributeHandler struct {
	svc *flavor.Service
}

// NewAttributeHandler creates an AttributeHandler.
func NewAttributeHandler(svc *flavor.Service) *AttributeHandler {
	return &AttributeHandler{svc: svc}
}

type attributesResponse struct {
	flavor.View
	Undo *foodevent.Undo `json:"undo,omitempty"`
}

// Get handles GET /api/foods/{id}/attributes: {food_id, version,
// attributes, sources}; an unknown value is left out of attributes.
func (h *AttributeHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_id")
		return
	}
	v, err := h.svc.Get(r.Context(), id)
	if err != nil {
		status, key := foodHTTPError(err)
		writeError(w, status, key)
		return
	}
	writeJSON(w, http.StatusOK, attributesResponse{View: v})
}

// Put handles PUT /api/foods/{id}/attributes with {field: value | null}:
// each value becomes the user's (it always wins), null resets the field to
// its automatic value. It returns the attributes and, when something
// changed, the undo token for POST /api/food-events/revert.
func (h *AttributeHandler) Put(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_id")
		return
	}
	p, ok := decodeAttributePatch(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	v, undo, err := h.svc.Set(r.Context(), id, p)
	if err != nil {
		status, key := foodHTTPError(err)
		writeError(w, status, key)
		return
	}
	resp := attributesResponse{View: v}
	if undo.FirstEventID > 0 {
		resp.Undo = &undo
	}
	writeJSON(w, http.StatusOK, resp)
}

// decodeAttributePatch reads {field: value | null}; an unknown field or a
// value of the wrong type is not ok.
func decodeAttributePatch(r *http.Request) (flavor.Patch, bool) {
	var raw json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		return flavor.Patch{}, false
	}
	return flavor.DecodePatch(raw)
}
