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

func TestProvider_GetByID_UpstreamErrorIsLookupFailed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	p := fdc.NewProvider(fdc.New("test-key", fdc.WithBaseURL(srv.URL)))
	_, err := p.GetByID(context.Background(), 169640)
	require.ErrorIs(t, err, domain.ErrLookupFailed)
	assert.NotErrorIs(t, err, domain.ErrNotFound)
}

func TestProvider_GetByID_LookupFailedKeepsCause(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	p := fdc.NewProvider(fdc.New("test-key", fdc.WithBaseURL(srv.URL)))
	_, err := p.GetByID(ctx, 169640)
	require.ErrorIs(t, err, domain.ErrLookupFailed)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestProvider_GetByID_Energy(t *testing.T) {
	f := func(v float64) *float64 { return &v }
	tests := []struct {
		name      string
		nutrients string
		wantKcal  *float64
	}{
		{"1008 only", `{"nutrient":{"id":1008},"amount":304}`, f(304)},
		{"2047 only", `{"nutrient":{"id":2047},"amount":310}`, f(310)},
		{"2048 only", `{"nutrient":{"id":2048},"amount":320}`, f(320)},
		{"2048 preferred over 2047", `{"nutrient":{"id":2047},"amount":310},{"nutrient":{"id":2048},"amount":320}`, f(320)},
		{"1008 preferred over 2048", `{"nutrient":{"id":2048},"amount":320},{"nutrient":{"id":1008},"amount":304}`, f(304)},
		// 4*10 + 4*20 + 9*5 = 165
		{"macros only", `{"nutrient":{"id":1003},"amount":10},{"nutrient":{"id":1005},"amount":20},{"nutrient":{"id":1004},"amount":5}`, f(165)},
		{"nil amount entry", `{"nutrient":{"id":1008}}`, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := `{"fdcId":1,"description":"X","dataType":"Foundation","foodNutrients":[` + tt.nutrients + `]}`
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(body))
			}))
			defer srv.Close()

			p := fdc.NewProvider(fdc.New("test-key", fdc.WithBaseURL(srv.URL)))
			c, err := p.GetByID(context.Background(), 1)
			require.NoError(t, err)
			if tt.wantKcal == nil {
				assert.Nil(t, c.Kcal100g)
				return
			}
			require.NotNil(t, c.Kcal100g)
			assert.InDelta(t, *tt.wantKcal, *c.Kcal100g, 0.001)
		})
	}
}
