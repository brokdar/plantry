package fdc

import (
	"context"
	"fmt"
	"sync"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
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

// SearchByName implements food.FoodProvider.
func (d *DynamicProvider) SearchByName(ctx context.Context, query string, limit int) ([]food.Candidate, error) {
	key, err := d.settings.EffectiveFDCKey(ctx)
	if err != nil {
		return nil, err
	}
	if key == "" {
		return []food.Candidate{}, nil
	}
	p := d.providerFor(key)
	return p.SearchByName(ctx, query, limit)
}

// Configured reports whether an FDC API key is set. It reads the same key
// SearchByName and GetByID use; a settings error counts as not configured.
func (d *DynamicProvider) Configured(ctx context.Context) bool {
	key, err := d.settings.EffectiveFDCKey(ctx)
	return err == nil && key != ""
}

// GetByID implements food.FoodByIDProvider. Without a configured FDC key
// there is nothing to fetch, so it returns domain.ErrLookupFailed.
func (d *DynamicProvider) GetByID(ctx context.Context, fdcID int) (food.Candidate, error) {
	key, err := d.settings.EffectiveFDCKey(ctx)
	if err != nil {
		return food.Candidate{}, err
	}
	if key == "" {
		return food.Candidate{}, fmt.Errorf("%w: fdc api key not configured", domain.ErrLookupFailed)
	}
	p := d.providerFor(key)
	return p.GetByID(ctx, fdcID)
}

// GetFoodPortions implements food.PortionProvider. Returns an empty
// slice when no FDC key is configured so callers can degrade gracefully.
func (d *DynamicProvider) GetFoodPortions(ctx context.Context, fdcID int) ([]food.PortionInfo, error) {
	key, err := d.settings.EffectiveFDCKey(ctx)
	if err != nil {
		return nil, err
	}
	if key == "" {
		return []food.PortionInfo{}, nil
	}
	p := d.providerFor(key)
	return p.GetFoodPortions(ctx, fdcID)
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
