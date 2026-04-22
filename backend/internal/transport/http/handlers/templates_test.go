package handlers_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain/component"
	"github.com/jaltszeimer/plantry/backend/internal/domain/ingredient"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planner"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/slot"
	"github.com/jaltszeimer/plantry/backend/internal/domain/template"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
	"github.com/jaltszeimer/plantry/backend/internal/transport/http/handlers"
)

type templateHarness struct {
	db         *sql.DB
	router     http.Handler
	templates  *sqlite.TemplateRepo
	plates     *sqlite.PlateRepo
	components *sqlite.ComponentRepo
	week       *planner.Week
	slot       *slot.TimeSlot
	compA      *component.Component
	compB      *component.Component
}

func newTemplateHarness(t *testing.T) *templateHarness {
	t.Helper()
	db := testhelper.NewTestDB(t)
	ingRepo := sqlite.NewIngredientRepo(db)
	compRepo := sqlite.NewComponentRepo(db)
	weekRepo := sqlite.NewWeekRepo(db)
	slotRepo := sqlite.NewSlotRepo(db)
	plateRepo := sqlite.NewPlateRepo(db)
	templateRepo := sqlite.NewTemplateRepo(db)
	txRunner := sqlite.NewTxRunner(db)

	ctx := context.Background()
	ing := &ingredient.Ingredient{Name: "Chicken", Source: "manual", Kcal100g: 100}
	require.NoError(t, ingRepo.Create(ctx, ing))

	compA := &component.Component{
		Name: "Curry", Role: component.RoleMain, ReferencePortions: 1,
		Ingredients: []component.ComponentIngredient{
			{IngredientID: ing.ID, Amount: 100, Unit: "g", Grams: 100},
		},
	}
	require.NoError(t, compRepo.Create(ctx, compA))
	compB := &component.Component{
		Name: "Rice", Role: component.RoleSideStarch, ReferencePortions: 1,
		Ingredients: []component.ComponentIngredient{
			{IngredientID: ing.ID, Amount: 100, Unit: "g", Grams: 100},
		},
	}
	require.NoError(t, compRepo.Create(ctx, compB))

	w := &planner.Week{Year: 2026, WeekNumber: 16}
	require.NoError(t, weekRepo.Create(ctx, w))
	s := &slot.TimeSlot{NameKey: "slot.dinner", Icon: "Moon", SortOrder: 1, Active: true}
	require.NoError(t, slotRepo.Create(ctx, s))

	svc := template.NewService(templateRepo, compRepo, plateRepo, txRunner)
	h := handlers.NewTemplateHandler(svc)
	r := chi.NewRouter()
	r.Route("/api/templates", func(r chi.Router) {
		r.Get("/", h.List)
		r.Post("/", h.Create)
		r.Route("/{id}", func(r chi.Router) {
			r.Get("/", h.Get)
			r.Put("/", h.Update)
			r.Delete("/", h.Delete)
			r.Post("/apply", h.Apply)
		})
	})

	return &templateHarness{
		db: db, router: r,
		templates: templateRepo, plates: plateRepo, components: compRepo,
		week: w, slot: s, compA: compA, compB: compB,
	}
}

func (h *templateHarness) createPlate(t *testing.T, components ...plate.PlateComponent) *plate.Plate {
	t.Helper()
	p := &plate.Plate{WeekID: h.week.ID, Day: 1, SlotID: h.slot.ID, Components: components}
	require.NoError(t, h.plates.Create(context.Background(), p))
	return p
}

func TestTemplateHandler_CreateEmpty(t *testing.T) {
	h := newTemplateHarness(t)
	resp := httptest.NewRecorder()
	h.router.ServeHTTP(resp, httptest.NewRequest(http.MethodPost, "/api/templates",
		bytes.NewBufferString(`{"name":"Empty"}`)))
	require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())

	var body map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, "Empty", body["name"])
	assert.Empty(t, body["components"])
}

func TestTemplateHandler_CreateInline(t *testing.T) {
	h := newTemplateHarness(t)
	payload := fmt.Sprintf(`{"name":"Inline","components":[{"component_id":%d,"portions":2}]}`, h.compA.ID)
	resp := httptest.NewRecorder()
	h.router.ServeHTTP(resp, httptest.NewRequest(http.MethodPost, "/api/templates", bytes.NewBufferString(payload)))
	require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())

	var body map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	comps := body["components"].([]any)
	require.Len(t, comps, 1)
}

func TestTemplateHandler_CreateFromPlate(t *testing.T) {
	h := newTemplateHarness(t)
	p := h.createPlate(t,
		plate.PlateComponent{ComponentID: h.compA.ID, Portions: 1, SortOrder: 0},
		plate.PlateComponent{ComponentID: h.compB.ID, Portions: 1, SortOrder: 1},
	)
	payload := fmt.Sprintf(`{"name":"From Plate","from_plate_id":%d}`, p.ID)
	resp := httptest.NewRecorder()
	h.router.ServeHTTP(resp, httptest.NewRequest(http.MethodPost, "/api/templates", bytes.NewBufferString(payload)))
	require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())

	var body map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Len(t, body["components"], 2)
}

func TestTemplateHandler_CreateMissingName(t *testing.T) {
	h := newTemplateHarness(t)
	resp := httptest.NewRecorder()
	h.router.ServeHTTP(resp, httptest.NewRequest(http.MethodPost, "/api/templates", bytes.NewBufferString(`{"name":""}`)))
	require.Equal(t, http.StatusBadRequest, resp.Code)
	var body map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, "error.invalid_body", body["message_key"])
}

func TestTemplateHandler_CreateBothSourcesRejected(t *testing.T) {
	h := newTemplateHarness(t)
	p := h.createPlate(t, plate.PlateComponent{ComponentID: h.compA.ID, Portions: 1, SortOrder: 0})
	payload := fmt.Sprintf(`{"name":"X","from_plate_id":%d,"components":[{"component_id":%d,"portions":1}]}`,
		p.ID, h.compA.ID)
	resp := httptest.NewRecorder()
	h.router.ServeHTTP(resp, httptest.NewRequest(http.MethodPost, "/api/templates", bytes.NewBufferString(payload)))
	require.Equal(t, http.StatusBadRequest, resp.Code)
}

func TestTemplateHandler_Get_NotFound(t *testing.T) {
	h := newTemplateHarness(t)
	resp := httptest.NewRecorder()
	h.router.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/api/templates/999", nil))
	require.Equal(t, http.StatusNotFound, resp.Code)
	var body map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, "error.not_found", body["message_key"])
}

func TestTemplateHandler_InvalidID(t *testing.T) {
	h := newTemplateHarness(t)
	resp := httptest.NewRecorder()
	h.router.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/api/templates/abc", nil))
	require.Equal(t, http.StatusBadRequest, resp.Code)
}

func TestTemplateHandler_Update(t *testing.T) {
	h := newTemplateHarness(t)
	ctx := context.Background()
	tpl := &template.Template{Name: "Old"}
	require.NoError(t, h.templates.Create(ctx, tpl))

	resp := httptest.NewRecorder()
	h.router.ServeHTTP(resp, httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/templates/%d", tpl.ID),
		bytes.NewBufferString(`{"name":"New"}`)))
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	var body map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, "New", body["name"])
}

func TestTemplateHandler_Delete(t *testing.T) {
	h := newTemplateHarness(t)
	ctx := context.Background()
	tpl := &template.Template{Name: "X"}
	require.NoError(t, h.templates.Create(ctx, tpl))

	resp := httptest.NewRecorder()
	h.router.ServeHTTP(resp, httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/templates/%d", tpl.ID), nil))
	require.Equal(t, http.StatusNoContent, resp.Code)
}

func TestTemplateHandler_Apply(t *testing.T) {
	h := newTemplateHarness(t)
	ctx := context.Background()
	tpl := &template.Template{Name: "Curry", Components: []template.TemplateComponent{
		{ComponentID: h.compA.ID, Portions: 2, SortOrder: 0},
		{ComponentID: h.compB.ID, Portions: 1, SortOrder: 1},
	}}
	require.NoError(t, h.templates.Create(ctx, tpl))
	p := h.createPlate(t)

	payload := fmt.Sprintf(`{"plate_id":%d}`, p.ID)
	resp := httptest.NewRecorder()
	h.router.ServeHTTP(resp, httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/api/templates/%d/apply", tpl.ID), bytes.NewBufferString(payload)))
	require.Equal(t, http.StatusNoContent, resp.Code, resp.Body.String())

	got, err := h.plates.Get(ctx, p.ID)
	require.NoError(t, err)
	assert.Len(t, got.Components, 2)
}

func TestTemplateHandler_Apply_MissingPlate(t *testing.T) {
	h := newTemplateHarness(t)
	ctx := context.Background()
	tpl := &template.Template{Name: "X"}
	require.NoError(t, h.templates.Create(ctx, tpl))

	resp := httptest.NewRecorder()
	h.router.ServeHTTP(resp, httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/api/templates/%d/apply", tpl.ID), bytes.NewBufferString(`{"plate_id":999}`)))
	require.Equal(t, http.StatusNotFound, resp.Code)
}

func TestTemplateHandler_Apply_MissingPlateID(t *testing.T) {
	h := newTemplateHarness(t)
	ctx := context.Background()
	tpl := &template.Template{Name: "X"}
	require.NoError(t, h.templates.Create(ctx, tpl))

	resp := httptest.NewRecorder()
	h.router.ServeHTTP(resp, httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/api/templates/%d/apply", tpl.ID), bytes.NewBufferString(`{}`)))
	require.Equal(t, http.StatusBadRequest, resp.Code)
}

func TestTemplateHandler_MalformedBody(t *testing.T) {
	h := newTemplateHarness(t)
	resp := httptest.NewRecorder()
	h.router.ServeHTTP(resp, httptest.NewRequest(http.MethodPost, "/api/templates", bytes.NewBufferString(`{bad`)))
	require.Equal(t, http.StatusBadRequest, resp.Code)
}

func TestTemplateHandler_List(t *testing.T) {
	h := newTemplateHarness(t)
	ctx := context.Background()
	require.NoError(t, h.templates.Create(ctx, &template.Template{Name: "A"}))
	require.NoError(t, h.templates.Create(ctx, &template.Template{Name: "B"}))

	resp := httptest.NewRecorder()
	h.router.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/api/templates", nil))
	require.Equal(t, http.StatusOK, resp.Code)
	var body map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	items := body["items"].([]any)
	assert.Len(t, items, 2)
}
