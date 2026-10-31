package handlers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/settings"
	"github.com/jaltszeimer/plantry/backend/internal/domain/shopping"
	"github.com/jaltszeimer/plantry/backend/internal/domain/slot"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper/testsvc"
	"github.com/jaltszeimer/plantry/backend/internal/transport/http/handlers"
)

// shopEnv is a shopping handler over a real DB, with the AC-1 recipe: Chili
// for 4 with 400 g kidney beans and 2 piece onions (120 g each), and an
// apple leaf with a 150 g piece.
type shopEnv struct {
	router   http.Handler
	plates   *plate.Service
	settings *settings.Service
	foods    *sqlite.FoodRepo
	slotID   int64
	chili    *food.Food
	beans    *food.Food
	onion    *food.Food
	apple    *food.Food
}

func newShopEnv(t *testing.T) *shopEnv {
	t.Helper()
	ctx := context.Background()
	db := testhelper.NewTestDB(t)
	foods := sqlite.NewFoodRepo(db)
	set := testsvc.Settings(db)
	svc := shopping.NewService(shopping.Deps{
		Source: sqlite.NewShoppingRepo(db), Foods: foods, Settings: set, Slots: sqlite.NewSlotRepo(db),
	})
	r := chi.NewRouter()
	r.Get("/api/shopping-list", handlers.NewShoppingHandler(svc).List)

	s := &slot.TimeSlot{NameKey: "dinner", Icon: "moon", Active: true}
	require.NoError(t, sqlite.NewSlotRepo(db).Create(ctx, s))

	src := food.SourceManual
	leaf := func(name string) *food.Food {
		f := &food.Food{Name: name, Kind: food.KindLeaf, Source: &src}
		require.NoError(t, foods.Create(ctx, f))
		return f
	}
	e := &shopEnv{
		router: r, plates: testsvc.Plates(db), settings: set, foods: foods, slotID: s.ID,
		beans: leaf("Kidneybohnen (Dose)"), onion: leaf("Zwiebel"), apple: leaf("Apfel"),
	}
	require.NoError(t, foods.UpsertPortion(ctx, &food.Portion{FoodID: e.apple.ID, Unit: "piece", Grams: 150}))
	ref := 4.0
	e.chili = &food.Food{Name: "Chili", Kind: food.KindComposed, ReferencePortions: &ref, Children: []food.FoodComponent{
		{ChildID: e.beans.ID, Amount: 400, Unit: "g", Grams: 400},
		{ChildID: e.onion.ID, Amount: 2, Unit: "piece", Grams: 240, SortOrder: 1},
	}}
	require.NoError(t, foods.Create(ctx, e.chili))
	return e
}

// chiliBatch cooks Chili on Sunday 2026-11-01 for 6 portions with leftovers
// on Monday 2026-11-02 (AC-1) and returns the session id.
func (e *shopEnv) chiliBatch(t *testing.T) int64 {
	t.Helper()
	ctx := context.Background()
	one, two := 1.0, 2.0
	p, err := e.plates.Create(ctx, plate.PlateInput{
		Date: day(t, "2026-11-01"), SlotID: e.slotID, Eaters: &two,
		Components: []plate.PlateComponent{{FoodID: e.chili.ID, Portions: &one}},
	})
	require.NoError(t, err)
	sid := *p.Components[0].CookSessionID
	_, err = e.plates.AddLeftovers(ctx, sid, []plate.CellKey{
		{Date: day(t, "2026-11-02"), SlotID: e.slotID},
		{Date: day(t, "2026-11-03"), SlotID: e.slotID},
	})
	require.NoError(t, err)
	_, err = e.plates.SetSessionPortions(ctx, sid, plate.SessionPortions{Portions: 6})
	require.NoError(t, err)
	return sid
}

func day(t *testing.T, s string) time.Time {
	t.Helper()
	d, err := time.Parse(time.DateOnly, s)
	require.NoError(t, err)
	return d
}

type shopBody struct {
	Range struct {
		From        string `json:"from"`
		To          string `json:"to"`
		WindowStart string `json:"window_start"`
		WindowEnd   string `json:"window_end"`
	} `json:"range"`
	Sections []struct {
		Section string `json:"section"`
		Items   []struct {
			FoodID     int64   `json:"food_id"`
			Name       string  `json:"name"`
			TotalGrams float64 `json:"total_grams"`
			Display    struct {
				Amount float64 `json:"amount"`
				Unit   string  `json:"unit"`
			} `json:"display"`
			Buy *struct {
				Amount float64 `json:"amount"`
				Unit   string  `json:"unit"`
			} `json:"buy"`
			UsedIn []struct {
				SessionID *int64 `json:"session_id"`
				FoodID    int64  `json:"food_id"`
				FoodName  string `json:"food_name"`
				Date      string `json:"date"`
				SlotID    int64  `json:"slot_id"`
			} `json:"used_in"`
		} `json:"items"`
	} `json:"sections"`
	Count     int  `json:"count"`
	HasDrafts bool `json:"has_drafts"`
}

func (e *shopEnv) get(t *testing.T, query string) (int, shopBody, string) {
	t.Helper()
	w := httptest.NewRecorder()
	e.router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/shopping-list"+query, nil))
	var b shopBody
	if w.Code == http.StatusOK {
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &b))
	}
	return w.Code, b, w.Body.String()
}

func TestShoppingList_AC1_BatchSession(t *testing.T) {
	e := newShopEnv(t)
	sid := e.chiliBatch(t)

	code, b, raw := e.get(t, "?from=2026-11-01&to=2026-11-07")
	require.Equal(t, http.StatusOK, code, raw)
	assert.Equal(t, 2, b.Count)
	require.Len(t, b.Sections, 2)

	produce := b.Sections[0]
	assert.Equal(t, "produce", produce.Section)
	onion := produce.Items[0]
	assert.Equal(t, e.onion.ID, onion.FoodID)
	assert.InDelta(t, 3, onion.Display.Amount, 1e-9)
	assert.Equal(t, "piece", onion.Display.Unit)
	require.NotNil(t, onion.Buy)
	assert.Equal(t, 3.0, onion.Buy.Amount)
	require.Len(t, onion.UsedIn, 1)
	assert.Equal(t, sid, *onion.UsedIn[0].SessionID)
	assert.Equal(t, "Chili", onion.UsedIn[0].FoodName)
	assert.Equal(t, "2026-11-01", onion.UsedIn[0].Date)
	assert.Equal(t, e.slotID, onion.UsedIn[0].SlotID)

	cans := b.Sections[1]
	assert.Equal(t, "cans_jars", cans.Section)
	beans := cans.Items[0]
	assert.InDelta(t, 600, beans.TotalGrams, 1e-9)
	assert.Equal(t, 600.0, beans.Display.Amount)
	assert.Equal(t, "g", beans.Display.Unit)
	assert.Nil(t, beans.Buy)
	require.Len(t, beans.UsedIn, 1)
	assert.Equal(t, sid, *beans.UsedIn[0].SessionID)

	assert.Contains(t, raw, `"buy":null`, "buy is always present")
}

// The list buys for the session's portions, frozen and surplus included,
// not for what the linked plates eat (AC-3, AC-4).
func TestShoppingList_BuysSessionPortionsNotConsumption(t *testing.T) {
	e := newShopEnv(t)
	sid := e.chiliBatch(t) // 3 plates × 2 eaters consume 6
	require.NoError(t, e.settings.Set(context.Background(), settings.KeyPlanFreezingEnabled, "true"))
	_, err := e.plates.SetSessionPortions(context.Background(), sid, plate.SessionPortions{Portions: 8, FrozenPortions: 2})
	require.NoError(t, err)

	code, b, raw := e.get(t, "?from=2026-11-01&to=2026-11-07")
	require.Equal(t, http.StatusOK, code, raw)
	require.Len(t, b.Sections, 2)
	onion := b.Sections[0].Items[0]
	assert.InDelta(t, 4, onion.Display.Amount, 1e-9, "2 × 8/4")
	require.NotNil(t, onion.Buy)
	assert.Equal(t, 4.0, onion.Buy.Amount)
	beans := b.Sections[1].Items[0]
	assert.Equal(t, 800.0, beans.Display.Amount, "400 × 8/4")
	assert.Equal(t, "g", beans.Display.Unit)
}

func TestShoppingList_AC2_LeftoversOfAnEarlierSessionAddNothing(t *testing.T) {
	e := newShopEnv(t)
	e.chiliBatch(t)

	code, b, raw := e.get(t, "?from=2026-11-02&to=2026-11-07")
	require.Equal(t, http.StatusOK, code, raw)
	assert.Zero(t, b.Count)
	assert.Empty(t, b.Sections)
	assert.Contains(t, raw, `"sections":[]`)
}

func TestShoppingList_AC3_LeafComponentTimesEaters(t *testing.T) {
	e := newShopEnv(t)
	eaters, one, piece := 3.5, 1.0, "piece"
	_, err := e.plates.Create(context.Background(), plate.PlateInput{
		Date: day(t, "2026-11-04"), SlotID: e.slotID, Eaters: &eaters,
		Components: []plate.PlateComponent{{FoodID: e.apple.ID, Amount: &one, Unit: &piece}},
	})
	require.NoError(t, err)

	code, b, raw := e.get(t, "?from=2026-11-02&to=2026-11-08")
	require.Equal(t, http.StatusOK, code, raw)
	require.Len(t, b.Sections, 1)
	apple := b.Sections[0].Items[0]
	assert.InDelta(t, 3.5, apple.Display.Amount, 1e-9)
	assert.Equal(t, "piece", apple.Display.Unit)
	require.NotNil(t, apple.Buy)
	assert.Equal(t, 4.0, apple.Buy.Amount)
	assert.InDelta(t, 525, apple.TotalGrams, 1e-9)
	require.Len(t, apple.UsedIn, 1)
	assert.Nil(t, apple.UsedIn[0].SessionID)
	assert.Equal(t, e.apple.ID, apple.UsedIn[0].FoodID)
	assert.Equal(t, "2026-11-04", apple.UsedIn[0].Date)
}

func TestShoppingList_AC5_SectionOverrideMovesItem(t *testing.T) {
	e := newShopEnv(t)
	e.chiliBatch(t)
	frozen := food.SectionFrozen
	_, err := e.foods.SetShopSection(context.Background(), e.beans.ID, &frozen)
	require.NoError(t, err)

	_, b, _ := e.get(t, "?from=2026-11-01&to=2026-11-07")
	require.Len(t, b.Sections, 2)
	assert.Equal(t, "produce", b.Sections[0].Section)
	assert.Equal(t, "frozen", b.Sections[1].Section)
	assert.Equal(t, e.beans.ID, b.Sections[1].Items[0].FoodID)
}

func TestShoppingList_AC6_DefaultRange(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name                   string
		anchor                 string
		today                  string
		from, to, wStart, wEnd string
	}{
		// plan.shopping_day defaults to Monday.
		{"window containing today starts today", "today", "2026-11-04", "2026-11-04", "2026-11-08", "2026-11-02", "2026-11-08"},
		{"next shopping day anchor", "next_shopping_day", "2026-11-04", "2026-11-09", "2026-11-15", "2026-11-09", "2026-11-15"},
		{"on the shopping day", "today", "2026-11-02", "2026-11-02", "2026-11-08", "2026-11-02", "2026-11-08"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newShopEnv(t)
			require.NoError(t, e.settings.Set(ctx, settings.KeyPlanAnchor, tt.anchor))
			code, b, raw := e.get(t, "?today="+tt.today)
			require.Equal(t, http.StatusOK, code, raw)
			assert.Equal(t, tt.from, b.Range.From)
			assert.Equal(t, tt.to, b.Range.To)
			assert.Equal(t, tt.wStart, b.Range.WindowStart)
			assert.Equal(t, tt.wEnd, b.Range.WindowEnd)
		})
	}
}

func TestShoppingList_DefaultRangeExcludesPastDays(t *testing.T) {
	e := newShopEnv(t)
	e.chiliBatch(t) // cooks Sunday 2026-11-01, window Mon 26 Oct → Sun 1 Nov
	require.NoError(t, e.settings.Set(context.Background(), settings.KeyPlanAnchor, "today"))

	_, b, _ := e.get(t, "?today=2026-10-30")
	assert.Equal(t, 2, b.Count, "Sunday is still ahead")
	assert.Equal(t, "2026-10-30", b.Range.From)

	_, b, _ = e.get(t, "?today=2026-11-02")
	assert.Zero(t, b.Count, "the next window has only leftovers")
}

func TestShoppingList_ExplicitRangeEchoesContainingWindow(t *testing.T) {
	e := newShopEnv(t)
	code, b, raw := e.get(t, "?from=2026-11-04&to=2026-11-10&today=2026-10-01")
	require.Equal(t, http.StatusOK, code, raw)
	assert.Equal(t, "2026-11-04", b.Range.From)
	assert.Equal(t, "2026-11-10", b.Range.To)
	assert.Equal(t, "2026-11-02", b.Range.WindowStart)
	assert.Equal(t, "2026-11-08", b.Range.WindowEnd)
}

func TestShoppingList_400(t *testing.T) {
	e := newShopEnv(t)
	tests := []struct {
		name, query, key string
	}{
		{"from without to", "?from=2026-04-26", "error.invalid_to"},
		{"to without from", "?to=2026-04-30", "error.invalid_from"},
		{"malformed from", "?from=not-a-date&to=2026-04-30", "error.invalid_from"},
		{"malformed to", "?from=2026-04-26&to=bad", "error.invalid_to"},
		{"from after to", "?from=2026-05-01&to=2026-04-26", "error.invalid_date_range"},
		{"span too large", "?from=2024-01-01&to=2025-12-31", "error.date_range_too_large"},
		{"malformed today", "?today=yesterday", "error.invalid_body"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, _, raw := e.get(t, tt.query)
			assert.Equal(t, http.StatusBadRequest, code)
			assert.Contains(t, raw, tt.key)
		})
	}
}
