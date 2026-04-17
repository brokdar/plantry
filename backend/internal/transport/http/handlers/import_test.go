package handlers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/importer"
	"github.com/jaltszeimer/plantry/backend/internal/domain/ingredient"
	"github.com/jaltszeimer/plantry/backend/internal/transport/http/handlers"
)

type fakeImportSvc struct {
	extractDraft *importer.Draft
	extractErr   error
	lookupResult []ingredient.Candidate
	lookupErr    error
	finalizeOut  *importer.FinalizedComponent
	finalizeErr  error

	gotExtract  importer.ExtractInput
	gotFinalize importer.FinalizeInput
	gotQuery    string
}

func (f *fakeImportSvc) Extract(_ context.Context, in importer.ExtractInput) (*importer.Draft, error) {
	f.gotExtract = in
	return f.extractDraft, f.extractErr
}

func (f *fakeImportSvc) ResolveLine(_ context.Context, q, _ string) ([]ingredient.Candidate, error) {
	f.gotQuery = q
	return f.lookupResult, f.lookupErr
}

func (f *fakeImportSvc) Finalize(_ context.Context, in importer.FinalizeInput) (*importer.FinalizedComponent, error) {
	f.gotFinalize = in
	return f.finalizeOut, f.finalizeErr
}

func TestExtract_HappyPath(t *testing.T) {
	draft := &importer.Draft{Name: "Carbonara", ExtractMethod: "jsonld"}
	svc := &fakeImportSvc{extractDraft: draft}
	h := handlers.NewImportHandler(svc)

	body := strings.NewReader(`{"url":"https://chefkoch.de/r/1"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/import/extract", body)
	rec := httptest.NewRecorder()
	h.Extract(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var out struct {
		Draft importer.Draft `json:"draft"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	require.Equal(t, "Carbonara", out.Draft.Name)
	require.Equal(t, "https://chefkoch.de/r/1", svc.gotExtract.URL)
}

func TestExtract_InvalidBody(t *testing.T) {
	svc := &fakeImportSvc{}
	h := handlers.NewImportHandler(svc)

	// Neither url nor html.
	req := httptest.NewRequest(http.MethodPost, "/api/import/extract", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()
	h.Extract(rec, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "error.invalid_body")
}

func TestExtract_BothInputs_Rejected(t *testing.T) {
	svc := &fakeImportSvc{}
	h := handlers.NewImportHandler(svc)
	req := httptest.NewRequest(http.MethodPost, "/api/import/extract", strings.NewReader(`{"url":"u","html":"<html/>"}`))
	rec := httptest.NewRecorder()
	h.Extract(rec, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestExtract_ProviderMissing_MapsTo503(t *testing.T) {
	svc := &fakeImportSvc{extractErr: domain.ErrAIProviderMissing}
	h := handlers.NewImportHandler(svc)
	req := httptest.NewRequest(http.MethodPost, "/api/import/extract", strings.NewReader(`{"url":"u"}`))
	rec := httptest.NewRecorder()
	h.Extract(rec, req)
	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
	require.Contains(t, rec.Body.String(), "error.ai.provider_missing")
}

func TestExtract_FetchFailed_MapsTo502(t *testing.T) {
	svc := &fakeImportSvc{extractErr: domain.ErrImportFetchFailed}
	h := handlers.NewImportHandler(svc)
	req := httptest.NewRequest(http.MethodPost, "/api/import/extract", strings.NewReader(`{"url":"u"}`))
	rec := httptest.NewRecorder()
	h.Extract(rec, req)
	require.Equal(t, http.StatusBadGateway, rec.Code)
	require.Contains(t, rec.Body.String(), "error.import.fetch_failed")
}

func TestExtract_NoRecipe_MapsTo422(t *testing.T) {
	svc := &fakeImportSvc{extractErr: domain.ErrImportNoRecipe}
	h := handlers.NewImportHandler(svc)
	req := httptest.NewRequest(http.MethodPost, "/api/import/extract", strings.NewReader(`{"url":"u"}`))
	rec := httptest.NewRecorder()
	h.Extract(rec, req)
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	require.Contains(t, rec.Body.String(), "error.import.no_recipe")
}

func TestLookupLine_HappyPath(t *testing.T) {
	svc := &fakeImportSvc{lookupResult: []ingredient.Candidate{{Name: "Mehl", Source: "fdc"}}}
	h := handlers.NewImportHandler(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/import/lookup?query=Mehl", nil)
	rec := httptest.NewRecorder()
	h.LookupLine(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "Mehl", svc.gotQuery)
}

func TestLookupLine_MissingQuery(t *testing.T) {
	svc := &fakeImportSvc{}
	h := handlers.NewImportHandler(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/import/lookup", nil)
	rec := httptest.NewRecorder()
	h.LookupLine(rec, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestResolve_HappyPath(t *testing.T) {
	svc := &fakeImportSvc{finalizeOut: &importer.FinalizedComponent{Name: "Carbonara", Role: "main"}}
	h := handlers.NewImportHandler(svc)

	body := strings.NewReader(`{
		"name":"Carbonara","role":"main","reference_portions":4,
		"ingredients":[{"resolution":"existing","existing_ingredient_id":42,"amount":200,"unit":"g"}],
		"instructions":[{"step_number":1,"text":"Kochen"}]
	}`)
	req := httptest.NewRequest(http.MethodPost, "/api/import/resolve", body)
	rec := httptest.NewRecorder()
	h.Resolve(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "main", svc.gotFinalize.Role)
	require.Equal(t, int64(42), svc.gotFinalize.Ingredients[0].IngredientID)
}

func TestResolve_InvalidResolution_MapsTo422(t *testing.T) {
	svc := &fakeImportSvc{finalizeErr: domain.ErrImportInvalidResolution}
	h := handlers.NewImportHandler(svc)
	req := httptest.NewRequest(http.MethodPost, "/api/import/resolve", strings.NewReader(`{"name":"x","role":"main","reference_portions":1}`))
	rec := httptest.NewRecorder()
	h.Resolve(rec, req)
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	require.Contains(t, rec.Body.String(), "error.import.invalid_resolution")
}
