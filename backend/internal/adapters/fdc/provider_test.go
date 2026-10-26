package fdc_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/fdc"
	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
)

func TestProvider_GetByID(t *testing.T) {
	data, err := os.ReadFile("testdata/food_honey_nutrients.json")
	require.NoError(t, err)
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(data)
	}))
	defer srv.Close()

	p := fdc.NewProvider(fdc.New("test-key", fdc.WithBaseURL(srv.URL)))
	c, err := p.GetByID(context.Background(), 169640)
	require.NoError(t, err)

	assert.Equal(t, "/fdc/v1/food/169640", gotPath)
	assert.Equal(t, "Honey", c.Name)
	assert.Equal(t, food.SourceFDC, c.Source)
	assert.Equal(t, 169640, c.FdcID)
	require.NotNil(t, c.Kcal100g)
	assert.InDelta(t, 304.0, *c.Kcal100g, 0.001)
	require.NotNil(t, c.Carbs100g)
	assert.InDelta(t, 82.4, *c.Carbs100g, 0.001)
	require.NotNil(t, c.Sodium100g)
	assert.InDelta(t, 0.004, *c.Sodium100g, 0.0001, "sodium converted from mg to g")
	require.NotNil(t, c.Sugar100g)
	assert.InDelta(t, 82.1, *c.Sugar100g, 0.001)
	assert.Nil(t, c.Fat100g, "missing nutrients stay nil")
}

func TestProvider_GetByID_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	p := fdc.NewProvider(fdc.New("test-key", fdc.WithBaseURL(srv.URL)))
	_, err := p.GetByID(context.Background(), 999999)
	require.ErrorIs(t, err, domain.ErrNotFound)
}
