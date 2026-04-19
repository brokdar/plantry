// Package llmresolver implements a dynamic llm.Resolver that reads the
// current effective AI configuration from the settings service on every
// call. Client instances are memoized so repeated calls with the same
// (provider, apiKey, model) tuple avoid reconstruction.
package llmresolver

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sync"

	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
	"github.com/jaltszeimer/plantry/backend/internal/domain/settings"
)

// Factory builds a concrete llm.Client for a given provider + API key. The
// fake provider additionally needs its script path, passed through fakeScript.
type Factory func(provider, apiKey, fakeScript string) (llm.Client, error)

type cacheKey struct {
	provider   string
	apiKeyHash string
	model      string
	fakeScript string
}

// Dynamic is the production-wiring Resolver.
type Dynamic struct {
	settings *settings.Service
	factory  Factory

	mu     sync.RWMutex
	key    cacheKey
	client llm.Client
}

// New constructs a Dynamic resolver.
func New(svc *settings.Service, factory Factory) *Dynamic {
	return &Dynamic{settings: svc, factory: factory}
}

// Current returns the Client + model to use for this request. A cached client
// is returned when configuration has not changed since the last call; a fresh
// client is built otherwise.
func (d *Dynamic) Current(ctx context.Context) (llm.Client, string, error) {
	cfg, err := d.settings.EffectiveAI(ctx)
	if err != nil {
		return nil, "", err
	}
	if cfg.Provider == "" {
		return nil, "", llm.ErrProviderMissing
	}
	key := cacheKey{
		provider:   cfg.Provider,
		apiKeyHash: hashKey(cfg.APIKey),
		model:      cfg.Model,
		fakeScript: cfg.FakeScript,
	}

	d.mu.RLock()
	if d.client != nil && d.key == key {
		c := d.client
		d.mu.RUnlock()
		return c, cfg.Model, nil
	}
	d.mu.RUnlock()

	d.mu.Lock()
	defer d.mu.Unlock()
	// Re-check after acquiring the write lock — another goroutine may have
	// rebuilt the client while we were waiting.
	if d.client != nil && d.key == key {
		return d.client, cfg.Model, nil
	}
	client, err := d.factory(cfg.Provider, cfg.APIKey, cfg.FakeScript)
	if err != nil {
		return nil, "", err
	}
	d.client = client
	d.key = key
	return client, cfg.Model, nil
}

func hashKey(key string) string {
	if key == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}
