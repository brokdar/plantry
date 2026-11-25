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
	transport "github.com/jaltszeimer/plantry/backend/internal/transport/http"
	"github.com/jaltszeimer/plantry/backend/internal/transport/http/handlers"
)

func TestChefHandler_Spend(t *testing.T) {
	serve := func(spend func(context.Context) (chef.MonthSpend, error)) *httptest.ResponseRecorder {
		r := transport.NewRouter(slog.New(slog.DiscardHandler), http.NotFoundHandler(), transport.Handlers{
			Chef: handlers.NewChefHandler(spend),
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
