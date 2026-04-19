package llmresolver_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/crypto"
	"github.com/jaltszeimer/plantry/backend/internal/adapters/llmresolver"
	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
	"github.com/jaltszeimer/plantry/backend/internal/domain/settings"
)

type memRepo struct {
	mu   sync.Mutex
	rows map[string]settings.Row
}

func newMemRepo() *memRepo { return &memRepo{rows: map[string]settings.Row{}} }

func (r *memRepo) Get(_ context.Context, key string) (settings.Row, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	row, ok := r.rows[key]
	return row, ok, nil
}

func (r *memRepo) List(_ context.Context) ([]settings.Row, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]settings.Row, 0, len(r.rows))
	for _, v := range r.rows {
		out = append(out, v)
	}
	return out, nil
}

func (r *memRepo) Upsert(_ context.Context, row settings.Row) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rows[row.Key] = row
	return nil
}

func (r *memRepo) Delete(_ context.Context, key string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.rows, key)
	return nil
}

type stubClient struct{ id string }

func (s *stubClient) Stream(_ context.Context, _ llm.Request, out chan<- llm.Event) (*llm.Response, error) {
	close(out)
	return nil, nil
}

const testSecret = "0123456789abcdef0123456789abcdef"

func TestCurrent_ProviderMissing(t *testing.T) {
	svc := settings.NewService(newMemRepo(), settings.NewEnvSnapshot(nil), cipher(t))
	r := llmresolver.New(svc, func(string, string, string) (llm.Client, error) {
		t.Fatal("factory should not be called when provider is empty")
		return nil, nil
	})
	_, _, err := r.Current(context.Background())
	assert.True(t, errors.Is(err, llm.ErrProviderMissing))
}

func TestCurrent_CacheHit(t *testing.T) {
	svc := settings.NewService(newMemRepo(), settings.NewEnvSnapshot(nil), cipher(t))
	ctx := context.Background()
	require.NoError(t, svc.Set(ctx, settings.KeyAIProvider, "fake"))
	require.NoError(t, svc.Set(ctx, settings.KeyAIModel, "m1"))

	var built atomic.Int32
	r := llmresolver.New(svc, func(p, _, _ string) (llm.Client, error) {
		built.Add(1)
		return &stubClient{id: p}, nil
	})
	_, _, err := r.Current(ctx)
	require.NoError(t, err)
	_, _, err = r.Current(ctx)
	require.NoError(t, err)
	assert.Equal(t, int32(1), built.Load(), "factory should build client once when config unchanged")
}

func TestCurrent_RebuildsOnProviderChange(t *testing.T) {
	svc := settings.NewService(newMemRepo(), settings.NewEnvSnapshot(nil), cipher(t))
	ctx := context.Background()
	require.NoError(t, svc.Set(ctx, settings.KeyAIProvider, "fake"))
	require.NoError(t, svc.Set(ctx, settings.KeyAIModel, "m1"))

	var built atomic.Int32
	r := llmresolver.New(svc, func(p, _, _ string) (llm.Client, error) {
		built.Add(1)
		return &stubClient{id: p}, nil
	})
	_, _, err := r.Current(ctx)
	require.NoError(t, err)

	require.NoError(t, svc.Set(ctx, settings.KeyAIProvider, "openai"))
	_, _, err = r.Current(ctx)
	require.NoError(t, err)

	assert.Equal(t, int32(2), built.Load())
}

func TestCurrent_ConcurrentSafe(t *testing.T) {
	svc := settings.NewService(newMemRepo(), settings.NewEnvSnapshot(nil), cipher(t))
	ctx := context.Background()
	require.NoError(t, svc.Set(ctx, settings.KeyAIProvider, "fake"))
	require.NoError(t, svc.Set(ctx, settings.KeyAIModel, "m1"))

	r := llmresolver.New(svc, func(p, _, _ string) (llm.Client, error) {
		return &stubClient{id: p}, nil
	})

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, err := r.Current(ctx)
			assert.NoError(t, err)
		}()
	}
	wg.Wait()
}

func cipher(t *testing.T) crypto.Cipher {
	t.Helper()
	c, err := crypto.New(testSecret)
	require.NoError(t, err)
	return c
}
