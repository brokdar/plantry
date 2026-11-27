package fdc

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/settings"
)

// DynamicProvider resolves the FDC API key from the settings service on every
// call, reconstructing the underlying Provider when the key changes. When no
// key is configured it answers from the offline fallback (WithFallback), and
// with a key it falls back there when the API fails or finds nothing; with
// neither, SearchByName returns an empty slice ("fdc disabled").
type DynamicProvider struct {
	settings   *settings.Service
	fallback   Fallback
	clientOpts []Option // tests point the client at a fake server

	mu       sync.RWMutex
	keyHash  string
	provider *Provider
}

// NewDynamicProvider constructs a DynamicProvider.
func NewDynamicProvider(svc *settings.Service) *DynamicProvider {
	return &DynamicProvider{settings: svc}
}

// Fallback is the offline FDC catalogue (adapters/fdclocal).
type Fallback interface {
	food.FoodProvider
	food.FoodByIDProvider
	food.PortionProvider
}

// WithFallback sets the offline catalogue used without a key or when the
// API fails.
func (d *DynamicProvider) WithFallback(f Fallback) *DynamicProvider {
	d.fallback = f
	return d
}

// SearchByName implements food.FoodProvider.
func (d *DynamicProvider) SearchByName(ctx context.Context, query string, limit int) ([]food.Candidate, error) {
	key, err := d.settings.EffectiveFDCKey(ctx)
	if err != nil {
		return nil, err
	}
	if key == "" {
		if d.fallback != nil {
			return d.fallback.SearchByName(ctx, query, limit)
		}
		return []food.Candidate{}, nil
	}
	cs, err := d.providerFor(key).SearchByName(ctx, query, limit)
	if d.fallback != nil && (err != nil || len(cs) == 0) {
		if err != nil {
			slog.WarnContext(ctx, "fdc: search failed, using the offline catalogue", "query", query, "error", err)
		}
		return d.fallback.SearchByName(ctx, query, limit)
	}
	return cs, err
}

// Configured reports whether FDC foods can be searched: an FDC API key is
// set (a settings error counts as none) or the offline fallback is there.
func (d *DynamicProvider) Configured(ctx context.Context) bool {
	if d.fallback != nil {
		return true
	}
	key, err := d.settings.EffectiveFDCKey(ctx)
	return err == nil && key != ""
}

// GetByID implements food.FoodByIDProvider. Without a configured FDC key
// it reads the offline fallback, and without that it returns
// domain.ErrLookupFailed. A failed API call falls back too.
func (d *DynamicProvider) GetByID(ctx context.Context, fdcID int) (food.Candidate, error) {
	key, err := d.settings.EffectiveFDCKey(ctx)
	if err != nil {
		return food.Candidate{}, err
	}
	if key == "" {
		if d.fallback != nil {
			return d.fallback.GetByID(ctx, fdcID)
		}
		return food.Candidate{}, fmt.Errorf("%w: fdc api key not configured", domain.ErrLookupFailed)
	}
	c, err := d.providerFor(key).GetByID(ctx, fdcID)
	if err != nil && d.fallback != nil {
		if lc, lerr := d.fallback.GetByID(ctx, fdcID); lerr == nil {
			return lc, nil
		} else if !errors.Is(lerr, domain.ErrNotFound) {
			slog.WarnContext(ctx, "fdc: offline catalogue lookup failed", "fdc_id", fdcID, "error", lerr)
		}
	}
	return c, err
}

// GetFoodPortions implements food.PortionProvider. Without a key it reads
// the offline fallback, else returns an empty slice so callers degrade
// gracefully; a failed API call falls back too.
func (d *DynamicProvider) GetFoodPortions(ctx context.Context, fdcID int) ([]food.PortionInfo, error) {
	key, err := d.settings.EffectiveFDCKey(ctx)
	if err != nil {
		return nil, err
	}
	if key == "" {
		if d.fallback != nil {
			return d.fallback.GetFoodPortions(ctx, fdcID)
		}
		return []food.PortionInfo{}, nil
	}
	ps, err := d.providerFor(key).GetFoodPortions(ctx, fdcID)
	if err != nil && d.fallback != nil {
		return d.fallback.GetFoodPortions(ctx, fdcID)
	}
	return ps, err
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
	d.provider = NewProvider(New(key, d.clientOpts...))
	d.keyHash = hash
	return d.provider
}
