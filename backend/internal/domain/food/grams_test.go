package food_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubPortions implements food.PortionLookup for the resolver-only tests.
type stubPortions struct {
	byID map[int64][]food.Portion
}

func (s stubPortions) ListPortions(_ context.Context, foodID int64) ([]food.Portion, error) {
	return s.byID[foodID], nil
}

func TestResolveGrams(t *testing.T) {
	type want struct {
		grams  float64
		source string
	}
	tests := []struct {
		name        string
		portions    map[int64][]food.Portion
		amount      float64
		unit        string
		manualGrams float64
		want        want
		wantErr     bool
	}{
		{
			name:   "mass_direct",
			amount: 200, unit: "g",
			want: want{grams: 200, source: food.GramsSourceDirect},
		},
		{
			name:   "mass_kg_direct_to_g",
			amount: 1, unit: "kg",
			want: want{grams: 1000, source: food.GramsSourceDefault},
		},
		{
			name:   "mass_oz_default",
			amount: 7, unit: "oz",
			want: want{grams: 7 * 28.3495, source: food.GramsSourceDefault},
		},
		{
			name:   "volume_ml_fallback",
			amount: 200, unit: "ml",
			want: want{grams: 200, source: food.GramsSourceFallback},
		},
		{
			name:     "portion_override_apple",
			portions: map[int64][]food.Portion{1: {{Unit: "apple", Grams: 180}}},
			amount:   1, unit: "apple",
			want: want{grams: 180, source: food.GramsSourcePortion},
		},
		{
			name:   "unknown_unit_errors",
			amount: 1, unit: "blarg",
			wantErr: true,
		},
		{
			name:   "count_unit_without_portion_errors",
			amount: 1, unit: "slice",
			wantErr: true,
		},
		{
			name:   "manual_grams_with_count_unit_succeeds",
			amount: 1, unit: "slice",
			manualGrams: 30,
			want:        want{grams: 30, source: food.GramsSourceManual},
		},
		{
			name:   "empty_unit_errors",
			amount: 1, unit: "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lookup := stubPortions{byID: tt.portions}
			gotG, gotSrc, err := food.ResolveGrams(
				context.Background(), lookup, 1, tt.amount, tt.unit, tt.manualGrams,
			)
			if tt.wantErr {
				require.Error(t, err)
				require.True(t, errors.Is(err, domain.ErrInvalidInput),
					"want ErrInvalidInput, got %v", err)
				return
			}
			require.NoError(t, err)
			assert.InDelta(t, tt.want.grams, gotG, 0.01)
			assert.Equal(t, tt.want.source, gotSrc)
		})
	}
}
