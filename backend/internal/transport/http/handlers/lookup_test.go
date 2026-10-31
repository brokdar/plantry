package handlers_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
	"github.com/jaltszeimer/plantry/backend/internal/transport/http/handlers"
)

// stubFDC records every call so tests can assert which FDC path was used.
type stubFDC struct {
	byID      food.Candidate
	byIDErr   error
	byIDCalls []int
	searches  []string
}

func (s *stubFDC) SearchByName(_ context.Context, query string, _ int) ([]food.Candidate, error) {
	s.searches = append(s.searches, query)
	return []food.Candidate{}, nil
}

func (s *stubFDC) GetByID(_ context.Context, fdcID int) (food.Candidate, error) {
	s.byIDCalls = append(s.byIDCalls, fdcID)
	return s.byID, s.byIDErr
}

func setupLookupRouter(t *testing.T, stub *stubFDC) (http.Handler, *food.Service) {
	t.Helper()
	db := testhelper.NewTestDB(t)
	repo := sqlite.NewFoodRepo(db)
	svc := food.NewService(repo)
	resolver := food.NewResolver(repo, nil, stub, nil)
	h := handlers.NewLookupHandler(resolver, svc, stub)
	r := chi.NewRouter()
	r.Post("/api/foods/{id}/refetch", h.Refetch)
	r.Post("/api/foods/resolve", h.Resolve)
	return r, svc
}

func createFDCLeaf(t *testing.T, svc *food.Service, fdcID string) int64 {
	t.Helper()
	src := food.SourceFDC
	f := &food.Food{Name: "Honey", Kind: food.KindLeaf, Source: &src, FdcID: &fdcID}
	require.NoError(t, svc.Create(context.Background(), f))
	return f.ID
}

func postRefetch(t *testing.T, r http.Handler, id int64) *httptest.ResponseRecorder {
	t.Helper()
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/foods/%d/refetch", id), nil))
	return resp
}

func TestLookupHandler_Refetch_FDCUsesGetByID(t *testing.T) {
	kcal := 304.0
	stub := &stubFDC{byID: food.Candidate{Name: "Honey", Source: food.SourceFDC, FdcID: 1234, Kcal100g: &kcal}}
	r, svc := setupLookupRouter(t, stub)
	id := createFDCLeaf(t, svc, "1234")

	resp := postRefetch(t, r, id)
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())

	var got map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	assert.InDelta(t, 304.0, got["kcal_100g"], 0.001)
	assert.Equal(t, []int{1234}, stub.byIDCalls)
	assert.Empty(t, stub.searches, "refetch must not search FDC by the id as text")
}

func TestLookupHandler_Refetch_FDCInvalidID(t *testing.T) {
	for _, fdcID := range []string{"not-a-number", "0", "-5"} {
		t.Run(fdcID, func(t *testing.T) {
			stub := &stubFDC{}
			r, svc := setupLookupRouter(t, stub)
			id := createFDCLeaf(t, svc, fdcID)

			resp := postRefetch(t, r, id)
			assert.Equal(t, http.StatusBadRequest, resp.Code)
			assert.Contains(t, resp.Body.String(), "error.invalid_body")
			assert.Empty(t, stub.byIDCalls)
			assert.Empty(t, stub.searches)
		})
	}
}

func TestLookupHandler_Refetch_FDCNotFound(t *testing.T) {
	stub := &stubFDC{byIDErr: fmt.Errorf("%w: fdc: food 1234 not found", domain.ErrNotFound)}
	r, svc := setupLookupRouter(t, stub)
	id := createFDCLeaf(t, svc, "1234")

	resp := postRefetch(t, r, id)
	assert.Equal(t, http.StatusNotFound, resp.Code)
	assert.Equal(t, []int{1234}, stub.byIDCalls)
}

func TestLookupHandler_Refetch_FDCUpstreamErrorIs502(t *testing.T) {
	stub := &stubFDC{byIDErr: fmt.Errorf("%w: fdc: status 503", domain.ErrLookupFailed)}
	r, svc := setupLookupRouter(t, stub)
	id := createFDCLeaf(t, svc, "1234")

	resp := postRefetch(t, r, id)
	assert.Equal(t, http.StatusBadGateway, resp.Code)
	assert.Contains(t, resp.Body.String(), "error.food.lookup_failed")
	assert.Equal(t, []int{1234}, stub.byIDCalls)
}

// The lookup responses carry used_in_count like every other leaf response.
func TestLookupHandler_LeafResponsesCarryUsedInCount(t *testing.T) {
	kcal := 304.0
	stub := &stubFDC{byID: food.Candidate{Name: "Honey", Source: food.SourceFDC, FdcID: 1234, Kcal100g: &kcal}}
	r, svc := setupLookupRouter(t, stub)
	id := createFDCLeaf(t, svc, "1234")
	role := food.RoleMain
	ref := 2.0
	require.NoError(t, svc.Create(context.Background(), &food.Food{
		Name: "Granola", Kind: food.KindComposed, Role: &role, ReferencePortions: &ref,
		Children: []food.FoodComponent{{ChildID: id, Amount: 20, Unit: "g", Grams: 20}},
	}))

	resp := postRefetch(t, r, id)
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	var got map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	assert.EqualValues(t, 1, got["used_in_count"])

	resp = httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest(http.MethodPost, "/api/foods/resolve",
		strings.NewReader(`{"name":"Oats","source":"manual","kcal_100g":370}`)))
	require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())
	got = map[string]any{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	assert.Contains(t, got, "used_in_count")
	assert.EqualValues(t, 0, got["used_in_count"])
}
