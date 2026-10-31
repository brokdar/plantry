package handlers

import (
	"context"
	"net/http"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain/shopping"
	"github.com/jaltszeimer/plantry/backend/internal/domain/units"
)

// shoppingLister is the subset of shopping.Service the handler uses.
type shoppingLister interface {
	Range(ctx context.Context, r *shopping.Range, today time.Time) (shopping.Range, error)
	List(ctx context.Context, from, to time.Time) (*shopping.List, error)
}

// ShoppingHandler serves GET /api/shopping-list.
type ShoppingHandler struct {
	svc shoppingLister
	now func() time.Time
}

// NewShoppingHandler creates a ShoppingHandler.
func NewShoppingHandler(svc shoppingLister) *ShoppingHandler {
	return &ShoppingHandler{svc: svc, now: time.Now}
}

type shoppingRangeResponse struct {
	From        string `json:"from"`
	To          string `json:"to"`
	WindowStart string `json:"window_start"`
	WindowEnd   string `json:"window_end"`
}

type shoppingUseResponse struct {
	SessionID *int64 `json:"session_id"`
	FoodID    int64  `json:"food_id"`
	FoodName  string `json:"food_name"`
	Date      string `json:"date"`
	SlotID    int64  `json:"slot_id"`
}

type shoppingItemResponse struct {
	FoodID     int64                 `json:"food_id"`
	Name       string                `json:"name"`
	TotalGrams float64               `json:"total_grams"`
	Display    units.Quantity        `json:"display"`
	Buy        *units.Quantity       `json:"buy"`
	UsedIn     []shoppingUseResponse `json:"used_in"`
}

type shoppingSectionResponse struct {
	Section string                 `json:"section"`
	Items   []shoppingItemResponse `json:"items"`
}

type shoppingListResponse struct {
	Range    shoppingRangeResponse     `json:"range"`
	Sections []shoppingSectionResponse `json:"sections"`
	Count    int                       `json:"count"`
}

// List handles GET /api/shopping-list?from&to&today. from and to come
// together or not at all; without them the range is the default of the
// anchored window as of today (D25).
func (h *ShoppingHandler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	today, ok := parseToday(w, q.Get("today"), h.now)
	if !ok {
		return
	}
	var explicit *shopping.Range
	if q.Get("from") != "" || q.Get("to") != "" {
		from, to, ok := parseDateRange(w, r)
		if !ok {
			return
		}
		explicit = &shopping.Range{From: from, To: to}
	}
	rng, err := h.svc.Range(r.Context(), explicit, today)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error.server")
		return
	}
	list, err := h.svc.List(r.Context(), rng.From, rng.To)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error.server")
		return
	}
	writeJSON(w, http.StatusOK, toShoppingListResponse(rng, list))
}

func toShoppingListResponse(rng shopping.Range, l *shopping.List) shoppingListResponse {
	resp := shoppingListResponse{
		Range: shoppingRangeResponse{
			From: dateString(rng.From), To: dateString(rng.To),
			WindowStart: dateString(rng.Window.Start), WindowEnd: dateString(rng.Window.End),
		},
		Sections: make([]shoppingSectionResponse, len(l.Sections)),
		Count:    l.Count,
	}
	for i, sec := range l.Sections {
		items := make([]shoppingItemResponse, len(sec.Items))
		for j, it := range sec.Items {
			uses := make([]shoppingUseResponse, len(it.UsedIn))
			for k, u := range it.UsedIn {
				uses[k] = shoppingUseResponse{
					SessionID: u.SessionID, FoodID: u.FoodID, FoodName: u.FoodName,
					Date: dateString(u.Date), SlotID: u.SlotID,
				}
			}
			items[j] = shoppingItemResponse{
				FoodID: it.FoodID, Name: it.Name, TotalGrams: it.TotalGrams,
				Display: it.Display, Buy: it.Buy, UsedIn: uses,
			}
		}
		resp.Sections[i] = shoppingSectionResponse{Section: string(sec.Section), Items: items}
	}
	return resp
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
