package handlers_test

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/chef"
	"github.com/jaltszeimer/plantry/backend/internal/domain/flavor"
	transport "github.com/jaltszeimer/plantry/backend/internal/transport/http"
	"github.com/jaltszeimer/plantry/backend/internal/transport/http/handlers"
)

func TestChefHandler_Spend(t *testing.T) {
	serve := func(spend func(context.Context) (chef.MonthSpend, error)) *httptest.ResponseRecorder {
		r := transport.NewRouter(slog.New(slog.DiscardHandler), http.NotFoundHandler(), transport.Handlers{
			Chef: handlers.NewChefHandler(spend, nil),
		})
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/chef/spend", nil))
		return w
	}

	w := serve(func(context.Context) (chef.MonthSpend, error) {
		return chef.MonthSpend{Month: "2026-11", SpentUSD: 1.25, CapUSD: 5}, nil
	})
	require.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `{"month":"2026-11","spent_usd":1.25,"cap_usd":5,"at_cap":false}`, w.Body.String())

	w = serve(func(context.Context) (chef.MonthSpend, error) { return chef.MonthSpend{}, errors.New("db down") })
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

type stubLabelling struct {
	est  flavor.Estimate
	jobs int
	err  error
}

func (s stubLabelling) Estimate(context.Context) (flavor.Estimate, error) { return s.est, s.err }
func (s stubLabelling) LabelLibrary(context.Context) (int, error)         { return s.jobs, s.err }

func TestChefHandler_Labelling(t *testing.T) {
	serve := func(l stubLabelling, method string) *httptest.ResponseRecorder {
		r := transport.NewRouter(slog.New(slog.DiscardHandler), http.NotFoundHandler(), transport.Handlers{
			Chef: handlers.NewChefHandler(nil, l),
		})
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(method, "/api/chef/labelling", nil))
		return w
	}
	cost := 0.12
	w := serve(stubLabelling{est: flavor.Estimate{Recipes: 20, Calls: 3, CostUSD: &cost, Minutes: 1}}, http.MethodGet)
	require.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `{"recipes":20,"calls":3,"cost_usd":0.12,"minutes":1}`, w.Body.String())

	w = serve(stubLabelling{}, http.MethodGet)
	assert.JSONEq(t, `{"recipes":0,"calls":0,"cost_usd":null,"minutes":0}`, w.Body.String(), "unpriced: cost null")

	w = serve(stubLabelling{jobs: 3}, http.MethodPost)
	require.Equal(t, http.StatusAccepted, w.Code)
	assert.JSONEq(t, `{"jobs":3}`, w.Body.String())

	w = serve(stubLabelling{err: flavor.ErrChefOff}, http.MethodPost)
	assert.Equal(t, http.StatusConflict, w.Code)
	assert.Contains(t, w.Body.String(), "error.chef.off")

	w = serve(stubLabelling{err: errors.New("db down")}, http.MethodGet)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}
