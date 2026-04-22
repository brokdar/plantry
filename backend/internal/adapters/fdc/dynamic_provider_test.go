package fdc_test

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/crypto"
	"github.com/jaltszeimer/plantry/backend/internal/adapters/fdc"
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
