package handlers

import (
	"context"
	"net/http"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/shopping"
)

// plateRangeService is the subset of plate.Service used by ShoppingRangeHandler.
type plateRangeService interface {
	Range(ctx context.Context, from, to time.Time) ([]plate.Plate, error)
}

// ShoppingRangeHandler serves the date-range shopping list endpoint.
type ShoppingRangeHandler struct {
	plates   plateRangeService
	shopping *shopping.Resolver
}

// NewShoppingRangeHandler wires a concrete plate.Service.
func NewShoppingRangeHandler(plates *plate.Service, shop *shopping.Resolver) *ShoppingRangeHandler {
	return &ShoppingRangeHandler{plates: plates, shopping: shop}
}

// NewShoppingRangeHandlerFromService accepts any plateRangeService; intended
// for tests that inject a stub.
func NewShoppingRangeHandlerFromService(plates plateRangeService, shop *shopping.Resolver) *ShoppingRangeHandler {
	return &ShoppingRangeHandler{plates: plates, shopping: shop}
}

// List handles GET /api/shopping-list?from=YYYY-MM-DD&to=YYYY-MM-DD.
func (h *ShoppingRangeHandler) List(w http.ResponseWriter, r *http.Request) {
	from, to, ok := parseDateRange(w, r)
	if !ok {
		return
	}
	plates, err := h.plates.Range(r.Context(), from, to)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error.server")
		return
	}
	items, err := h.shopping.FromPlates(r.Context(), plates)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error.server")
		return
	}
	if items == nil {
		items = []shopping.Item{}
	}
	writeJSON(w, http.StatusOK, shoppingListResponse{Items: items})
}

// parseDateRange reads and validates from/to query params shared by shopping
// and nutrition range handlers. Returns false and writes the error response
// itself on failure.
func parseDateRange(w http.ResponseWriter, r *http.Request) (from, to time.Time, ok bool) {
	fromStr := r.URL.Query().Get("from")
	toStr := r.URL.Query().Get("to")

	if fromStr == "" {
		writeError(w, http.StatusBadRequest, "error.invalid_from")
		return
	}
	if toStr == "" {
		writeError(w, http.StatusBadRequest, "error.invalid_to")
		return
	}

	var err error
	from, err = time.Parse("2006-01-02", fromStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_from")
		return
	}
	to, err = time.Parse("2006-01-02", toStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_to")
		return
	}

	if from.After(to) {
		writeError(w, http.StatusBadRequest, "error.invalid_date_range")
		return
	}
	if to.Sub(from) > 366*24*time.Hour {
		writeError(w, http.StatusBadRequest, "error.date_range_too_large")
		return
	}

	return from, to, true
}
