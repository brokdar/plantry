package importer_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/importer"
)

type stubExtractor struct{ rec *importer.RawRecipe }

func (s stubExtractor) ExtractRecipe(string) (*importer.RawRecipe, error) { return s.rec, nil }

func TestService_Extract_PassesSourceKcalThrough(t *testing.T) {
	kcal := 512.0
	rec := &importer.RawRecipe{
		Name:                 "Spaghetti",
		RecipeIngredient:     []string{"200 g Spaghetti"},
		SourceKcalPerServing: &kcal,
	}
	svc := importer.NewService(nil, stubExtractor{rec: rec}, nil, nil)

	d, err := svc.Extract(context.Background(), importer.ExtractInput{HTML: "<html></html>"})
	require.NoError(t, err)
	require.NotNil(t, d.SourceKcalPerServing)
	require.InDelta(t, 512.0, *d.SourceKcalPerServing, 0.001)
}
