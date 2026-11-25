package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/jaltszeimer/plantry/backend/internal/domain/kitchen"
	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
)

// KitchenHandler holds the HTTP handlers for the kitchen data (KIT).
type KitchenHandler struct {
	profile *profile.Service
	basics  *kitchen.Service
}

// NewKitchenHandler creates a KitchenHandler.
func NewKitchenHandler(p *profile.Service, b *kitchen.Service) *KitchenHandler {
	return &KitchenHandler{profile: p, basics: b}
}

type kitchenFood struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type kitchenRequest struct {
	MinutesWeekday int      `json:"minutes_weekday"`
	MinutesWeekend int      `json:"minutes_weekend"`
	Skill          string   `json:"skill"`
	Equipment      []string `json:"equipment"`
	Heat           string   `json:"heat"`
	Leanings       []string `json:"leanings"`
}

type kitchenResponse struct {
	kitchenRequest
	Basics []kitchenFood `json:"basics"`
}

func (h *KitchenHandler) respond(w http.ResponseWriter, r *http.Request, p *profile.Profile) {
	basics, err := h.basics.Basics(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error.server")
		return
	}
	k := p.Kitchen
	resp := kitchenResponse{
		kitchenRequest: kitchenRequest{
			MinutesWeekday: k.MinutesWeekday, MinutesWeekend: k.MinutesWeekend,
			Skill: string(k.Skill), Equipment: k.Equipment, Heat: string(k.Heat), Leanings: k.Leanings,
		},
		Basics: make([]kitchenFood, len(basics)),
	}
	for i, b := range basics {
		resp.Basics[i] = kitchenFood{ID: b.ID, Name: b.Name}
	}
	writeJSON(w, http.StatusOK, resp)
}

// Get handles GET /api/kitchen.
func (h *KitchenHandler) Get(w http.ResponseWriter, r *http.Request) {
	p, err := h.profile.Get(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error.server")
		return
	}
	h.respond(w, r, p)
}

// Update handles PUT /api/kitchen: a full replace of the settings; the basics
// have their own routes.
func (h *KitchenHandler) Update(w http.ResponseWriter, r *http.Request) {
	var req kitchenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	p, err := h.profile.UpdateKitchen(r.Context(), profile.Kitchen{
		MinutesWeekday: req.MinutesWeekday, MinutesWeekend: req.MinutesWeekend,
		Skill: profile.Skill(req.Skill), Equipment: req.Equipment, Heat: profile.Heat(req.Heat), Leanings: req.Leanings,
	})
	if err != nil {
		status, key := toHTTPWithResource(err, "kitchen")
		writeError(w, status, key)
		return
	}
	h.respond(w, r, p)
}

// AddBasic handles PUT /api/kitchen/basics/{id}: idempotent.
func (h *KitchenHandler) AddBasic(w http.ResponseWriter, r *http.Request) {
	id, ok := parseIDParam(w, r, "id")
	if !ok {
		return
	}
	if err := h.basics.AddBasic(r.Context(), id); err != nil {
		status, key := toHTTPWithResource(err, "food")
		writeError(w, status, key)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// RemoveBasic handles DELETE /api/kitchen/basics/{id}.
func (h *KitchenHandler) RemoveBasic(w http.ResponseWriter, r *http.Request) {
	id, ok := parseIDParam(w, r, "id")
	if !ok {
		return
	}
	if err := h.basics.RemoveBasic(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, "error.server")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
