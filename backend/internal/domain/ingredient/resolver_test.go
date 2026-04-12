package ingredient_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/ingredient"
)

// --- fake providers ---

type fakeBarcodeProvider struct {
	barcodeFn func(ctx context.Context, barcode string) ([]ingredient.Candidate, error)
	searchFn  func(ctx context.Context, query string, limit int) ([]ingredient.Candidate, error)
}

func (f *fakeBarcodeProvider) LookupBarcode(ctx context.Context, barcode string) ([]ingredient.Candidate, error) {
	if f.barcodeFn != nil {
		return f.barcodeFn(ctx, barcode)
	}
	return nil, nil
}

func (f *fakeBarcodeProvider) SearchByName(ctx context.Context, query string, limit int) ([]ingredient.Candidate, error) {
	if f.searchFn != nil {
		return f.searchFn(ctx, query, limit)
	}
	return nil, nil
}

type fakeFoodProvider struct {
	searchFn func(ctx context.Context, query string, limit int) ([]ingredient.Candidate, error)
}

func (f *fakeFoodProvider) SearchByName(ctx context.Context, query string, limit int) ([]ingredient.Candidate, error) {
	if f.searchFn != nil {
		return f.searchFn(ctx, query, limit)
	}
	return nil, nil
}

func kcal(v float64) *float64 { return &v }

func TestLookup_BarcodeFoundInOFF(t *testing.T) {
	offCalled := false
	fdcCalled := false

	off := &fakeBarcodeProvider{
		barcodeFn: func(_ context.Context, _ string) ([]ingredient.Candidate, error) {
			offCalled = true
			return []ingredient.Candidate{{Name: "OFF Product", Source: "off", Kcal100g: kcal(100)}}, nil
		},
	}
	fdc := &fakeFoodProvider{
		searchFn: func(_ context.Context, _ string, _ int) ([]ingredient.Candidate, error) {
			fdcCalled = true
			return []ingredient.Candidate{{Name: "FDC Product"}}, nil
		},
	}

	resolver := ingredient.NewResolver(newFakeRepo(), off, fdc)
	results, err := resolver.Lookup(context.Background(), "123456", "", "en", 5)

	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "OFF Product", results[0].Name)
	assert.True(t, offCalled)
	assert.False(t, fdcCalled, "FDC should not be called when OFF returns results")
}

func TestLookup_BarcodeFallsBackToFDC(t *testing.T) {
	off := &fakeBarcodeProvider{
		barcodeFn: func(_ context.Context, _ string) ([]ingredient.Candidate, error) {
			return nil, nil // no results
		},
	}
	fdc := &fakeFoodProvider{
		searchFn: func(_ context.Context, _ string, _ int) ([]ingredient.Candidate, error) {
			return []ingredient.Candidate{{Name: "FDC Product", Source: "fdc"}}, nil
		},
	}

	resolver := ingredient.NewResolver(newFakeRepo(), off, fdc)
	results, err := resolver.Lookup(context.Background(), "123456", "", "en", 5)

	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "FDC Product", results[0].Name)
}

func TestLookup_BarcodeNotFoundAnywhere(t *testing.T) {
	off := &fakeBarcodeProvider{}
	fdc := &fakeFoodProvider{}

	resolver := ingredient.NewResolver(newFakeRepo(), off, fdc)
	results, err := resolver.Lookup(context.Background(), "000000", "", "en", 5)

	require.NoError(t, err)
	assert.Empty(t, results)
}

func TestLookup_QuerySearchesFDCOnly(t *testing.T) {
	offCalled := false
	off := &fakeBarcodeProvider{
		barcodeFn: func(_ context.Context, _ string) ([]ingredient.Candidate, error) {
			offCalled = true
			return nil, nil
		},
		searchFn: func(_ context.Context, _ string, _ int) ([]ingredient.Candidate, error) {
			offCalled = true
			return nil, nil
		},
	}
	fdc := &fakeFoodProvider{
		searchFn: func(_ context.Context, _ string, _ int) ([]ingredient.Candidate, error) {
			return []ingredient.Candidate{{Name: "Chicken Breast", Source: "fdc"}}, nil
		},
	}

	resolver := ingredient.NewResolver(newFakeRepo(), off, fdc)
	results, err := resolver.Lookup(context.Background(), "", "chicken", "en", 5)

	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "Chicken Breast", results[0].Name)
	assert.False(t, offCalled, "OFF should not be called for text queries")
}

func TestLookup_NilProviders(t *testing.T) {
	resolver := ingredient.NewResolver(newFakeRepo(), nil, nil)

	// barcode with nil OFF
	results, err := resolver.Lookup(context.Background(), "123456", "", "en", 5)
	require.NoError(t, err)
	assert.Empty(t, results)

	// query with nil FDC
	results, err = resolver.Lookup(context.Background(), "", "chicken", "en", 5)
	require.NoError(t, err)
	assert.Empty(t, results)
}

func TestLookup_ExistingIDEnrichment(t *testing.T) {
	repo := newFakeRepo()
	svc := ingredient.NewService(repo)
	require.NoError(t, svc.Create(context.Background(), &ingredient.Ingredient{Name: "Chicken Breast"}))

	fdc := &fakeFoodProvider{
		searchFn: func(_ context.Context, _ string, _ int) ([]ingredient.Candidate, error) {
			return []ingredient.Candidate{{Name: "Chicken Breast", Source: "fdc"}}, nil
		},
	}

	resolver := ingredient.NewResolver(repo, nil, fdc)
	results, err := resolver.Lookup(context.Background(), "", "chicken", "en", 5)

	require.NoError(t, err)
	require.Len(t, results, 1)
	require.NotNil(t, results[0].ExistingID)
	assert.Equal(t, int64(1), *results[0].ExistingID)
}

func TestLookup_EmptyInputReturnsEmpty(t *testing.T) {
	resolver := ingredient.NewResolver(newFakeRepo(), nil, nil)
	results, err := resolver.Lookup(context.Background(), "", "", "en", 5)

	require.NoError(t, err)
	assert.Empty(t, results)
}

func TestLookup_ProviderError(t *testing.T) {
	off := &fakeBarcodeProvider{
		barcodeFn: func(_ context.Context, _ string) ([]ingredient.Candidate, error) {
			return nil, errors.New("network error")
		},
	}

	resolver := ingredient.NewResolver(newFakeRepo(), off, nil)
	_, err := resolver.Lookup(context.Background(), "123", "", "en", 5)
	assert.Error(t, err)
}
