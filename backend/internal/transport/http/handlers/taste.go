package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain/kitchen"
	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
	"github.com/jaltszeimer/plantry/backend/internal/domain/taste"
)

// TasteHandler serves the taste model's read API (TASTE-11's data; the
// Settings page comes with D2). The Undo tokens it returns are reverted by
// POST /api/food-events/revert.
type TasteHandler struct {
	svc     *taste.Service
	kitchen *kitchen.Service
	foods   LeafLookup
	profile *profile.Service
}

// LeafLookup finds a leaf food by its exact name (food.Service).
type LeafLookup interface {
	LeafIDByName(ctx context.Context, name string) (int64, error)
}

// NewTasteHandler creates a TasteHandler.
func NewTasteHandler(svc *taste.Service) *TasteHandler { return &TasteHandler{svc: svc} }

// WithKitchen adds the never items: the page lists them, a statement chip or
// "Never" on an item makes one (a food by name; none: a restriction chip).
func (h *TasteHandler) WithKitchen(k *kitchen.Service, foods LeafLookup, p *profile.Service) *TasteHandler {
	h.kitchen, h.foods, h.profile = k, foods, p
	return h
}

// page is GET /api/taste: the read model of the Taste page. Never are the
// kitchen's never items; Staples the recipes the household made its own.
type page struct {
	taste.View
	Never   []kitchenFood  `json:"never"`
	Staples []taste.Staple `json:"staples"`
}

// Get handles GET /api/taste[?window=YYYY-MM-DD]: {items, recent}. window
// is the plan window whose window-scoped items are included.
func (h *TasteHandler) Get(w http.ResponseWriter, r *http.Request) {
	var window time.Time
	if v := r.URL.Query().Get("window"); v != "" {
		d, err := time.Parse(time.DateOnly, v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "error.invalid_query")
			return
		}
		window = d
	}
	v, err := h.svc.View(r.Context(), window)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error.server")
		return
	}
	out := page{View: v, Never: []kitchenFood{}}
	if out.Staples, err = h.svc.StapleList(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "error.server")
		return
	}
	if h.kitchen != nil {
		never, err := h.kitchen.NeverItems(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "error.server")
			return
		}
		for _, f := range never {
			out.Never = append(out.Never, kitchenFood{ID: f.ID, Name: f.Name})
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// Confirm handles POST /api/taste/statements/confirm: the user's tap on a
// chat statement chip (TASTE-8). A taste statement
// {kind:"taste", level, key, polarity, scope:"always"|"window",
// window_start} becomes permanent or stays for the window; a health
// statement {kind:"health", word} becomes a never item (the food of that
// name; none: a dietary restriction chip). Returns {undo} for taste.
func (h *TasteHandler) Confirm(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Kind        string         `json:"kind"`
		Level       taste.Level    `json:"level"`
		Key         string         `json:"key"`
		Polarity    taste.Polarity `json:"polarity"`
		Scope       taste.Scope    `json:"scope"`
		WindowStart string         `json:"window_start"`
		Word        string         `json:"word"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	switch req.Kind {
	case "health":
		word := strings.TrimSpace(req.Word)
		if word == "" || h.kitchen == nil {
			writeError(w, http.StatusBadRequest, "error.invalid_body")
			return
		}
		if err := h.makeNever(r.Context(), word, true); err != nil {
			status, key := toHTTPWithResource(err, "taste_item")
			writeError(w, status, key)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case "taste":
		window, err := time.Parse(time.DateOnly, req.WindowStart)
		if err != nil && req.Scope == taste.ScopeWindow {
			writeError(w, http.StatusBadRequest, "error.invalid_body")
			return
		}
		it, undo, err := h.svc.ConfirmStatement(r.Context(), taste.Ref{Level: req.Level, Key: req.Key}, req.Polarity, req.Scope, window)
		if err != nil {
			status, key := toHTTPWithResource(err, "taste_item")
			writeError(w, status, key)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"item": it.ID, "undo": undo})
	default:
		writeError(w, http.StatusBadRequest, "error.invalid_body")
	}
}

// makeNever makes the leaf food named word a never item; without such a
// food it adds the word to the dietary restrictions when fallback is set.
func (h *TasteHandler) makeNever(ctx context.Context, word string, fallback bool) error {
	id, err := h.foods.LeafIDByName(ctx, word)
	if err == nil {
		return h.kitchen.AddNeverItem(ctx, id)
	}
	if !fallback || h.profile == nil {
		return err
	}
	p, err := h.profile.Get(ctx)
	if err != nil {
		return err
	}
	if !slices.ContainsFunc(p.DietaryRestrictions, func(x string) bool { return strings.EqualFold(x, word) }) {
		p.DietaryRestrictions = append(p.DietaryRestrictions, word)
		if _, err := h.profile.Update(ctx, p); err != nil {
			return err
		}
	}
	return nil
}

// MakeNever handles POST /api/taste/items/{id}/never: the item's food (a
// hero ingredient) becomes a never item. 404 when no food has that name.
func (h *TasteHandler) MakeNever(w http.ResponseWriter, r *http.Request) {
	id, ok := parseIDParam(w, r, "id")
	if !ok {
		return
	}
	if h.kitchen == nil {
		writeError(w, http.StatusNotFound, "error.not_found")
		return
	}
	it, err := h.svc.Item(r.Context(), id)
	if err != nil {
		status, key := toHTTPWithResource(err, "taste_item")
		writeError(w, status, key)
		return
	}
	if it.Level != taste.LevelHero {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	if err := h.makeNever(r.Context(), strings.ReplaceAll(it.Key, "_", " "), false); err != nil {
		status, key := toHTTPWithResource(err, "food")
		writeError(w, status, key)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// AddNever handles PUT /api/taste/never/{id}: idempotent.
func (h *TasteHandler) AddNever(w http.ResponseWriter, r *http.Request) {
	id, ok := parseIDParam(w, r, "id")
	if !ok || h.kitchen == nil {
		return
	}
	if err := h.kitchen.AddNeverItem(r.Context(), id); err != nil {
		status, key := toHTTPWithResource(err, "food")
		writeError(w, status, key)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// RemoveNever handles DELETE /api/taste/never/{id}.
func (h *TasteHandler) RemoveNever(w http.ResponseWriter, r *http.Request) {
	id, ok := parseIDParam(w, r, "id")
	if !ok || h.kitchen == nil {
		return
	}
	if err := h.kitchen.RemoveNeverItem(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, "error.server")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Delete handles DELETE /api/taste/items/{id}: {undo}.
func (h *TasteHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := parseIDParam(w, r, "id")
	if !ok {
		return
	}
	undo, err := h.svc.Remove(r.Context(), id)
	if err != nil {
		status, key := toHTTPWithResource(err, "taste_item")
		writeError(w, status, key)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"undo": undo})
}

// Put handles PUT /api/taste/items/{id} with {polarity: love|rather_not}:
// the user's own polarity, which outranks learned evidence (flip).
// Returns {item, undo}.
func (h *TasteHandler) Put(w http.ResponseWriter, r *http.Request) {
	id, ok := parseIDParam(w, r, "id")
	if !ok {
		return
	}
	var req struct {
		Polarity taste.Polarity `json:"polarity"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || !req.Polarity.Valid() {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	it, undo, err := h.svc.SetPolarity(r.Context(), id, req.Polarity)
	if err != nil {
		status, key := toHTTPWithResource(err, "taste_item")
		writeError(w, status, key)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"item": it, "undo": undo})
}
