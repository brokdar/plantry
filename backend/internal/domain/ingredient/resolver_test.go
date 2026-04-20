package ingredient_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/ingredient"
	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
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

	resolver := ingredient.NewResolver(newFakeRepo(), off, fdc, nil)
	results, _, err := resolver.Lookup(context.Background(), "123456", "", "en", 5)

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

	resolver := ingredient.NewResolver(newFakeRepo(), off, fdc, nil)
	results, _, err := resolver.Lookup(context.Background(), "123456", "", "en", 5)

	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "FDC Product", results[0].Name)
}

func TestLookup_BarcodeNotFoundAnywhere(t *testing.T) {
	off := &fakeBarcodeProvider{}
	fdc := &fakeFoodProvider{}

	resolver := ingredient.NewResolver(newFakeRepo(), off, fdc, nil)
	results, _, err := resolver.Lookup(context.Background(), "000000", "", "en", 5)

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

	resolver := ingredient.NewResolver(newFakeRepo(), off, fdc, nil)
	results, _, err := resolver.Lookup(context.Background(), "", "chicken", "en", 5)

	require.NoError(t, err)
	require.Len(t, results, 1)
	// FDC path swaps Name to original query; canonical label lives on SourceName.
	assert.Equal(t, "chicken", results[0].Name)
	assert.Equal(t, "Chicken Breast", results[0].SourceName)
	assert.False(t, offCalled, "OFF should not be called for text queries")
}

func TestLookup_NilProviders(t *testing.T) {
	resolver := ingredient.NewResolver(newFakeRepo(), nil, nil, nil)

	// barcode with nil OFF
	results, _, err := resolver.Lookup(context.Background(), "123456", "", "en", 5)
	require.NoError(t, err)
	assert.Empty(t, results)

	// query with nil FDC
	results, _, err = resolver.Lookup(context.Background(), "", "chicken", "en", 5)
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

	resolver := ingredient.NewResolver(repo, nil, fdc, nil)
	results, _, err := resolver.Lookup(context.Background(), "", "chicken", "en", 5)

	require.NoError(t, err)
	require.Len(t, results, 1)
	require.NotNil(t, results[0].ExistingID)
	assert.Equal(t, int64(1), *results[0].ExistingID)
}

func TestLookup_EmptyInputReturnsEmpty(t *testing.T) {
	resolver := ingredient.NewResolver(newFakeRepo(), nil, nil, nil)
	results, _, err := resolver.Lookup(context.Background(), "", "", "en", 5)

	require.NoError(t, err)
	assert.Empty(t, results)
}

func TestLookup_ProviderError(t *testing.T) {
	off := &fakeBarcodeProvider{
		barcodeFn: func(_ context.Context, _ string) ([]ingredient.Candidate, error) {
			return nil, errors.New("network error")
		},
	}

	resolver := ingredient.NewResolver(newFakeRepo(), off, nil, nil)
	_, _, err := resolver.Lookup(context.Background(), "123", "", "en", 5)
	assert.Error(t, err)
}

// TestLookup_MissingKcalFallback — some FDC rows (e.g. raw chicken breast)
// return protein/fat/carbs but no kcal. Without an Atwater fallback the UI
// silently saved a 0-kcal ingredient and every downstream nutrition total was
// wrong.
func TestLookup_MissingKcalFallback(t *testing.T) {
	p := 22.5
	f := 1.93
	c := 0.0
	fdc := &fakeFoodProvider{
		searchFn: func(_ context.Context, _ string, _ int) ([]ingredient.Candidate, error) {
			return []ingredient.Candidate{{
				Name:        "Raw Chicken",
				Source:      "fdc",
				Kcal100g:    nil,
				Protein100g: &p,
				Fat100g:     &f,
				Carbs100g:   &c,
			}}, nil
		},
	}

	resolver := ingredient.NewResolver(newFakeRepo(), nil, fdc, nil)
	results, _, err := resolver.Lookup(context.Background(), "", "chicken", "en", 5)

	require.NoError(t, err)
	require.Len(t, results, 1)
	require.NotNil(t, results[0].Kcal100g, "resolver should fill kcal from macros")
	assert.InDelta(t, 4*22.5+4*0+9*1.93, *results[0].Kcal100g, 0.001)
}

// TestLookup_KcalNotOverwrittenWhenPresent — don't clobber a real kcal value.
func TestLookup_KcalNotOverwrittenWhenPresent(t *testing.T) {
	k := 539.0
	fdc := &fakeFoodProvider{
		searchFn: func(_ context.Context, _ string, _ int) ([]ingredient.Candidate, error) {
			return []ingredient.Candidate{{Name: "Nutella", Source: "fdc", Kcal100g: &k}}, nil
		},
	}
	resolver := ingredient.NewResolver(newFakeRepo(), nil, fdc, nil)
	results, _, err := resolver.Lookup(context.Background(), "", "nutella", "en", 5)
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.NotNil(t, results[0].Kcal100g)
	assert.Equal(t, 539.0, *results[0].Kcal100g)
}

// --- AI-enabled resolver ---

// stubLLMClient returns a fixed text response on every Stream call.
type stubLLMClient struct{ text string }

func (s *stubLLMClient) Stream(_ context.Context, _ llm.Request, out chan<- llm.Event) (*llm.Response, error) {
	defer close(out)
	return &llm.Response{
		Message: llm.Message{
			Role: llm.RoleAssistant,
			Content: []llm.ContentBlock{
				{Type: llm.ContentTypeText, Text: s.text},
			},
		},
		StopReason: llm.StopReasonEndTurn,
	}, nil
}

// scriptedResolver serves different text for translate vs pick-best by
// counting calls in order: 1st call = translate, 2nd+ = pick-best.
type scriptedResolver struct {
	translate string
	pickBest  string
	calls     int
}

func (s *scriptedResolver) Current(_ context.Context) (llm.Client, string, error) {
	s.calls++
	if s.calls == 1 {
		return &stubLLMClient{text: s.translate}, "fake-model", nil
	}
	return &stubLLMClient{text: s.pickBest}, "fake-model", nil
}

func TestLookup_AI_TranslatesGermanQueryAndPicksBest(t *testing.T) {
	var observed string
	fdc := &fakeFoodProvider{
		searchFn: func(_ context.Context, query string, _ int) ([]ingredient.Candidate, error) {
			observed = query
			k1 := 100.0
			k2 := 120.0
			return []ingredient.Candidate{
				{Name: "Chicken breast, cooked", Source: "fdc", Kcal100g: &k1},
				{Name: "Chicken breast, raw", Source: "fdc", Kcal100g: &k2},
			}, nil
		},
	}
	llmR := &scriptedResolver{
		translate: `["chicken breast raw"]`,
		pickBest:  `[1]`,
	}
	r := ingredient.NewResolver(newFakeRepo(), nil, fdc, llmR)

	trace := ingredient.NewLookupTrace()
	ctx := ingredient.WithTrace(context.Background(), trace)
	results, idx, err := r.Lookup(ctx, "", "Hähnchenbrust", "de", 5)
	require.NoError(t, err)
	require.Len(t, results, 2)
	// FDC received the translated term, not the original German.
	assert.Equal(t, "chicken breast raw", observed)
	// Original query pre-fills the Name; canonical label stays on SourceName.
	assert.Equal(t, "Hähnchenbrust", results[0].Name)
	assert.Equal(t, "Chicken breast, cooked", results[0].SourceName)
	// Pick-best returned index 1.
	assert.Equal(t, 1, idx)
	// Trace contains translate + fdc + pick-best steps.
	entries := trace.Entries()
	require.GreaterOrEqual(t, len(entries), 3)
}

func TestLookup_AI_EnglishQuerySkipsTranslation(t *testing.T) {
	var observed string
	fdc := &fakeFoodProvider{
		searchFn: func(_ context.Context, query string, _ int) ([]ingredient.Candidate, error) {
			observed = query
			k := 120.0
			return []ingredient.Candidate{{Name: "Chicken", Source: "fdc", Kcal100g: &k}}, nil
		},
	}
	llmR := &scriptedResolver{translate: `["should not be called"]`, pickBest: `[0]`}
	r := ingredient.NewResolver(newFakeRepo(), nil, fdc, llmR)
	_, _, err := r.Lookup(context.Background(), "", "chicken", "en", 5)
	require.NoError(t, err)
	assert.Equal(t, "chicken", observed, "English queries must not be translated")
	assert.Equal(t, 0, llmR.calls, "AI must not be invoked for en+single-result")
}

func TestLookup_Empty_ReturnsNegativeOne(t *testing.T) {
	r := ingredient.NewResolver(newFakeRepo(), nil, &fakeFoodProvider{}, nil)
	results, idx, err := r.Lookup(context.Background(), "", "x", "en", 5)
	require.NoError(t, err)
	assert.Empty(t, results)
	assert.Equal(t, -1, idx)
}
