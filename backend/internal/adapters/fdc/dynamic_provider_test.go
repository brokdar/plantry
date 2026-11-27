package fdc_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/crypto"
	"github.com/jaltszeimer/plantry/backend/internal/adapters/fdc"
	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/settings"
)

type memRepo struct {
	mu   sync.Mutex
	rows map[string]settings.Row
}

func newMemRepo() *memRepo { return &memRepo{rows: map[string]settings.Row{}} }

func (r *memRepo) Get(_ context.Context, k string) (settings.Row, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	row, ok := r.rows[k]
	return row, ok, nil
}
func (r *memRepo) List(_ context.Context) ([]settings.Row, error) { return nil, nil }
func (r *memRepo) Upsert(_ context.Context, row settings.Row) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rows[row.Key] = row
	return nil
}

func (r *memRepo) Delete(_ context.Context, k string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.rows, k)
	return nil
}

func newTestCipher(t *testing.T) crypto.Cipher {
	t.Helper()
	c, err := crypto.New("0123456789abcdef0123456789abcdef")
	require.NoError(t, err)
	return c
}

func TestDynamicProvider_EmptyKey_ReturnsEmptyResults(t *testing.T) {
	// When no FDC API key is configured anywhere, the provider must return
	// an empty slice — the ingredient.Resolver treats that as "disabled".
	svc := settings.NewService(newMemRepo(), settings.NewEnvSnapshot(nil), newTestCipher(t))
	p := fdc.NewDynamicProvider(svc)

	got, err := p.SearchByName(context.Background(), "chicken", 5)
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestDynamicProvider_GetByID_EmptyKey_ReturnsLookupFailed(t *testing.T) {
	svc := settings.NewService(newMemRepo(), settings.NewEnvSnapshot(nil), newTestCipher(t))
	p := fdc.NewDynamicProvider(svc)

	_, err := p.GetByID(context.Background(), 1234)
	require.ErrorIs(t, err, domain.ErrLookupFailed)
}

func TestDynamicProvider_Configured(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want bool
	}{
		{"no key", nil, false},
		{"env key", map[string]string{"PLANTRY_FDC_API_KEY": "abc"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := settings.NewService(newMemRepo(), settings.NewEnvSnapshot(tt.env), newTestCipher(t))
			p := fdc.NewDynamicProvider(svc)
			assert.Equal(t, tt.want, p.Configured(context.Background()))
		})
	}
}

// stubFallback stands in for the offline catalogue (adapters/fdclocal).
type stubFallback struct{}

func (stubFallback) SearchByName(context.Context, string, int) ([]food.Candidate, error) {
	return []food.Candidate{{Name: "Vinegar, cider", Source: food.SourceFDC, FdcID: 173469}}, nil
}

func (stubFallback) GetByID(_ context.Context, id int) (food.Candidate, error) {
	if id != 173469 {
		return food.Candidate{}, domain.ErrNotFound
	}
	return food.Candidate{Name: "Vinegar, cider", Source: food.SourceFDC, FdcID: id}, nil
}

func (stubFallback) GetFoodPortions(context.Context, int) ([]food.PortionInfo, error) {
	return []food.PortionInfo{{RawUnit: "undetermined", Modifier: "tbsp", GramWeight: 15}}, nil
}

// TestDynamicProvider_Fallback: without a key the offline catalogue answers;
// with a key it answers when the API fails (5xx, 429) or finds nothing.
func TestDynamicProvider_Fallback(t *testing.T) {
	ctx := context.Background()
	t.Run("no key", func(t *testing.T) {
		svc := settings.NewService(newMemRepo(), settings.NewEnvSnapshot(nil), newTestCipher(t))
		p := fdc.NewDynamicProvider(svc).WithFallback(stubFallback{})
		assert.True(t, p.Configured(ctx))
		cs, err := p.SearchByName(ctx, "cider vinegar", 5)
		require.NoError(t, err)
		require.Len(t, cs, 1)
		assert.Equal(t, 173469, cs[0].FdcID)
		c, err := p.GetByID(ctx, 173469)
		require.NoError(t, err)
		assert.Equal(t, "Vinegar, cider", c.Name)
		ps, err := p.GetFoodPortions(ctx, 173469)
		require.NoError(t, err)
		assert.Len(t, ps, 1)
	})
	for _, tt := range []struct {
		name   string
		status int
		body   string
	}{
		{"server error", http.StatusServiceUnavailable, ""},
		{"rate limited", http.StatusTooManyRequests, ""},
		{"no results", http.StatusOK, `{"foods":[]}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer srv.Close()
			svc := settings.NewService(newMemRepo(), settings.NewEnvSnapshot(map[string]string{"PLANTRY_FDC_API_KEY": "abc"}), newTestCipher(t))
			p := fdc.WithTestBaseURL(fdc.NewDynamicProvider(svc).WithFallback(stubFallback{}), srv.URL)
			cs, err := p.SearchByName(ctx, "cider vinegar", 5)
			require.NoError(t, err)
			require.Len(t, cs, 1)
			assert.Equal(t, 173469, cs[0].FdcID)
			if tt.status != http.StatusOK {
				c, err := p.GetByID(ctx, 173469)
				require.NoError(t, err)
				assert.Equal(t, 173469, c.FdcID)
			}
		})
	}
}
