package fdc

import (
	"context"
	"sync"

	"github.com/jaltszeimer/plantry/backend/internal/domain/ingredient"
	"github.com/jaltszeimer/plantry/backend/internal/domain/settings"
)

// DynamicProvider resolves the FDC API key from the settings service on every
// call, reconstructing the underlying Provider when the key changes. When no
// key is configured, SearchByName returns an empty slice — matching the
// existing "fdc disabled" behaviour in ingredient.Resolver.
type DynamicProvider struct {
	settings *settings.Service

	mu       sync.RWMutex
	keyHash  string
	provider *Provider
}

// NewDynamicProvider constructs a DynamicProvider.
func NewDynamicProvider(svc *settings.Service) *DynamicProvider {
	return &DynamicProvider{settings: svc}
}

// SearchByName implements ingredient.FoodProvider.
func (d *DynamicProvider) SearchByName(ctx context.Context, query string, limit int) ([]ingredient.Candidate, error) {
	key, err := d.settings.EffectiveFDCKey(ctx)
	if err != nil {
		return nil, err
	}
	if key == "" {
		return []ingredient.Candidate{}, nil
	}
	p := d.providerFor(key)
	return p.SearchByName(ctx, query, limit)
}

func (d *DynamicProvider) providerFor(key string) *Provider {
	hash := key // lightweight comparison key; full cryptographic hashing unnecessary here
	d.mu.RLock()
	if d.provider != nil && d.keyHash == hash {
		p := d.provider
		d.mu.RUnlock()
		return p
	}
	d.mu.RUnlock()

	d.mu.Lock()
	defer d.mu.Unlock()
	if d.provider != nil && d.keyHash == hash {
		return d.provider
	}
	d.provider = NewProvider(New(key))
	d.keyHash = hash
	return d.provider
}
