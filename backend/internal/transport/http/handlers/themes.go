package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/jaltszeimer/plantry/backend/internal/domain/theme"
)

// ThemeHandler serves a window's week themes (THEME-2..5).
type ThemeHandler struct{ svc *theme.Service }

// NewThemeHandler creates a ThemeHandler.
func NewThemeHandler(svc *theme.Service) *ThemeHandler { return &ThemeHandler{svc: svc} }

func windowStart(w http.ResponseWriter, r *http.Request) (time.Time, bool) {
	start, err := time.Parse(time.DateOnly, chi.URLParam(r, "start"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return time.Time{}, false
	}
	return start, true
}

func (h *ThemeHandler) fail(w http.ResponseWriter, err error) {
	status, key := toHTTPWithResource(err, "theme")
	writeError(w, status, key)
}

// Get handles GET /api/windows/{start}/themes: {themes, again, suggestion}.
func (h *ThemeHandler) Get(w http.ResponseWriter, r *http.Request) {
	start, ok := windowStart(w, r)
	if !ok {
		return
	}
	v, err := h.svc.Window(r.Context(), start)
	if err != nil {
		h.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

// Post handles POST /api/windows/{start}/themes {kind, value, source?}:
// source is user (default) or again. Returns {theme, library, finding}.
func (h *ThemeHandler) Post(w http.ResponseWriter, r *http.Request) {
	start, ok := windowStart(w, r)
	if !ok {
		return
	}
	var req struct {
		Kind   theme.Kind   `json:"kind"`
		Value  string       `json:"value"`
		Source theme.Source `json:"source"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	switch req.Source {
	case "":
		req.Source = theme.SourceUser
	case theme.SourceUser, theme.SourceAgain:
	default:
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	res, err := h.svc.Set(r.Context(), start, theme.Theme{Kind: req.Kind, Value: req.Value, Source: req.Source})
	if err != nil {
		h.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// Delete handles DELETE /api/windows/{start}/themes/{id}.
func (h *ThemeHandler) Delete(w http.ResponseWriter, r *http.Request) {
	start, ok := windowStart(w, r)
	if !ok {
		return
	}
	id, ok := parseIDParam(w, r, "id")
	if !ok {
		return
	}
	if err := h.svc.Delete(r.Context(), start, id); err != nil {
		h.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Accept handles POST /api/windows/{start}/themes/suggestion/accept.
func (h *ThemeHandler) Accept(w http.ResponseWriter, r *http.Request) {
	start, ok := windowStart(w, r)
	if !ok {
		return
	}
	res, err := h.svc.Accept(r.Context(), start)
	if err != nil {
		h.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// Decline handles POST /api/windows/{start}/themes/suggestion/decline.
func (h *ThemeHandler) Decline(w http.ResponseWriter, r *http.Request) {
	start, ok := windowStart(w, r)
	if !ok {
		return
	}
	if err := h.svc.Decline(r.Context(), start); err != nil {
		h.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
