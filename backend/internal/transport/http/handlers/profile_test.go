package handlers_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
	"github.com/jaltszeimer/plantry/backend/internal/transport/http/handlers"
)

func setupProfileRouter(t *testing.T) http.Handler {
	t.Helper()
	db := testhelper.NewTestDB(t)
	repo := sqlite.NewProfileRepo(db)
	svc := profile.NewService(repo)
	h := handlers.NewProfileHandler(svc)
	r := chi.NewRouter()
	r.Route("/api/profile", func(r chi.Router) {
		r.Get("/", h.Get)
		r.Put("/", h.Update)
	})
	return r
}

func TestProfileHandler_Get_Default(t *testing.T) {
	r := setupProfileRouter(t)
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/api/profile", nil))
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())

	var body map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, "en", body["locale"])
	assert.Nil(t, body["kcal_target"])
}

func TestProfileHandler_Update_Valid(t *testing.T) {
	r := setupProfileRouter(t)

	payload := `{"kcal_target":1800,"protein_pct":35,"fat_pct":30,"carbs_pct":35,"dietary_restrictions":["vegetarian"],"locale":"de"}`
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest(http.MethodPut, "/api/profile", bytes.NewBufferString(payload)))
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())

	var body map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, float64(1800), body["kcal_target"])
	assert.Equal(t, "de", body["locale"])
}

func TestProfileHandler_Update_InvalidMacroSum(t *testing.T) {
	r := setupProfileRouter(t)

	payload := `{"protein_pct":50,"fat_pct":30,"carbs_pct":30}`
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest(http.MethodPut, "/api/profile", bytes.NewBufferString(payload)))
	require.Equal(t, http.StatusBadRequest, resp.Code)

	var body map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, "error.profile.invalid_macros", body["message_key"])
}

func TestProfileHandler_Update_InvalidKcal(t *testing.T) {
	r := setupProfileRouter(t)

	payload := `{"kcal_target":0}`
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest(http.MethodPut, "/api/profile", bytes.NewBufferString(payload)))
	require.Equal(t, http.StatusBadRequest, resp.Code)
}

func putProfile(t *testing.T, r http.Handler, payload string) (int, map[string]any) {
	t.Helper()
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest(http.MethodPut, "/api/profile", bytes.NewBufferString(payload)))
	var body map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	return resp.Code, body
}

func TestProfileHandler_Get_DefaultPlanningPrefs(t *testing.T) {
	r := setupProfileRouter(t)
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/api/profile", nil))
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())

	var body map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, "omnivore", body["diet_type"])
	assert.Contains(t, body, "meat_days_per_week_max")
	assert.Nil(t, body["meat_days_per_week_max"])
	assert.Equal(t, float64(1), body["household_portions"])
}

func TestProfileHandler_Update_PlanningPrefs(t *testing.T) {
	r := setupProfileRouter(t)

	code, body := putProfile(t, r, `{"diet_type":"vegan","meat_days_per_week_max":3,"household_portions":2.5}`)
	require.Equal(t, http.StatusOK, code, body)
	assert.Equal(t, "vegan", body["diet_type"])
	assert.Equal(t, float64(3), body["meat_days_per_week_max"])
	assert.Equal(t, 2.5, body["household_portions"])

	// Absent diet_type and household_portions keep the stored values;
	// meat_days_per_week_max: null clears.
	code, body = putProfile(t, r, `{"kcal_target":2000,"meat_days_per_week_max":null}`)
	require.Equal(t, http.StatusOK, code, body)
	assert.Equal(t, "vegan", body["diet_type"])
	assert.Equal(t, 2.5, body["household_portions"])
	assert.Nil(t, body["meat_days_per_week_max"])
	assert.Equal(t, float64(2000), body["kcal_target"])
}

func TestProfileHandler_NutritionPolicyFields(t *testing.T) {
	r := setupProfileRouter(t)

	// Defaults: maintain, no body data.
	_, body := putProfile(t, r, `{}`)
	assert.Equal(t, "maintain", body["goal"])
	assert.Nil(t, body["weight_kg"])
	assert.Nil(t, body["sex"])

	code, body := putProfile(t, r, `{"goal":"lose","weight_kg":82.5,"height_cm":180,"birth_year":1985,"sex":"male","activity_level":1.6,"protein_g_per_kg":1.8,"kcal_target":1900}`)
	require.Equal(t, http.StatusOK, code, body)
	assert.Equal(t, "lose", body["goal"])
	assert.Equal(t, 82.5, body["weight_kg"])
	assert.Equal(t, float64(180), body["height_cm"])
	assert.Equal(t, float64(1985), body["birth_year"])
	assert.Equal(t, "male", body["sex"])
	assert.Equal(t, 1.6, body["activity_level"])
	assert.Equal(t, 1.8, body["protein_g_per_kg"])

	// An absent goal keeps the stored one; body fields are a full replace.
	code, body = putProfile(t, r, `{"kcal_target":1900}`)
	require.Equal(t, http.StatusOK, code, body)
	assert.Equal(t, "lose", body["goal"])
	assert.Nil(t, body["weight_kg"])
	assert.Nil(t, body["sex"])
	assert.Nil(t, body["activity_level"])
	assert.Nil(t, body["protein_g_per_kg"])
}

func TestProfileHandler_Update_InvalidNutritionPolicyFields(t *testing.T) {
	cases := []struct {
		name    string
		payload string
		wantKey string
	}{
		{"goal", `{"goal":"bulk"}`, "error.profile.invalid_goal"},
		{"weight zero", `{"weight_kg":0}`, "error.profile.invalid_body_data"},
		{"weight huge", `{"weight_kg":900}`, "error.profile.invalid_body_data"},
		{"height negative", `{"height_cm":-1}`, "error.profile.invalid_body_data"},
		{"birth year old", `{"birth_year":1800}`, "error.profile.invalid_body_data"},
		{"birth year future", `{"birth_year":2999}`, "error.profile.invalid_body_data"},
		{"sex", `{"sex":"x"}`, "error.profile.invalid_body_data"},
		{"activity", `{"activity_level":1.5}`, "error.profile.invalid_body_data"},
		{"protein factor below the DGE floor", `{"protein_g_per_kg":0.7}`, "error.profile.invalid_body_data"},
		{"protein factor above the bound", `{"protein_g_per_kg":2.3}`, "error.profile.invalid_body_data"},
		{"kcal below the female floor", `{"sex":"female","kcal_target":1100}`, "error.profile.kcal_below_floor"},
		{"kcal below the male floor", `{"sex":"male","kcal_target":1400}`, "error.profile.kcal_below_floor"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := setupProfileRouter(t)
			code, body := putProfile(t, r, tc.payload)
			require.Equal(t, http.StatusBadRequest, code)
			assert.Equal(t, tc.wantKey, body["message_key"])
		})
	}

	t.Run("floor targets pass, and the lower floor applies without sex", func(t *testing.T) {
		r := setupProfileRouter(t)
		code, body := putProfile(t, r, `{"sex":"female","kcal_target":1200}`)
		require.Equal(t, http.StatusOK, code, body)
		code, body = putProfile(t, r, `{"kcal_target":1200}`)
		require.Equal(t, http.StatusOK, code, body)
		code, body = putProfile(t, r, `{"kcal_target":900}`)
		require.Equal(t, http.StatusBadRequest, code, body)
	})
}

func TestProfileHandler_Update_InvalidPlanningPrefs(t *testing.T) {
	cases := []struct {
		name    string
		payload string
		wantKey string
	}{
		{"unknown diet", `{"diet_type":"keto"}`, "error.profile.invalid_diet_type"},
		{"meat days 8", `{"meat_days_per_week_max":8}`, "error.profile.invalid_meat_days"},
		{"household zero", `{"household_portions":0}`, "error.profile.invalid_household_portions"},
		{"household too large", `{"household_portions":20.5}`, "error.profile.invalid_household_portions"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := setupProfileRouter(t)
			code, body := putProfile(t, r, tc.payload)
			require.Equal(t, http.StatusBadRequest, code)
			assert.Equal(t, tc.wantKey, body["message_key"])
		})
	}
}

func TestProfileHandler_Update_MalformedJSON(t *testing.T) {
	r := setupProfileRouter(t)

	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest(http.MethodPut, "/api/profile", bytes.NewBufferString(`{bad json`)))
	require.Equal(t, http.StatusBadRequest, resp.Code)
}
