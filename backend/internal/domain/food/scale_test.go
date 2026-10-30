package food_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/units"
)

func chiliForFour() *food.Food {
	ref := 4.0
	return &food.Food{
		ID: 7, Kind: food.KindComposed, Name: "Chili", ReferencePortions: &ref,
		Children: []food.FoodComponent{
			{ChildID: 1, ChildName: "Kidney beans", ChildKind: food.KindLeaf, Amount: 400, Unit: "g", Grams: 400},
			{ChildID: 2, ChildName: "Onion", ChildKind: food.KindLeaf, Amount: 2, Unit: "piece", Grams: 240},
			{ChildID: 3, ChildName: "Rice", ChildKind: food.KindComposed, Amount: 1, Unit: "serving", Grams: 180},
		},
	}
}

// B5-AC2
func TestScale_Chili(t *testing.T) {
	t.Run("portions 6", func(t *testing.T) {
		got, err := food.Scale(chiliForFour(), 6)
		require.NoError(t, err)
		assert.Equal(t, int64(7), got.FoodID)
		assert.InDelta(t, 6, got.Portions, 1e-9)
		assert.InDelta(t, 4, got.ReferencePortions, 1e-9)
		assert.InDelta(t, 1.5, got.Factor, 1e-9)
		require.Len(t, got.Ingredients, 3)

		beans := got.Ingredients[0]
		assert.Equal(t, int64(1), beans.ChildID)
		assert.Equal(t, "Kidney beans", beans.Name)
		assert.Equal(t, food.KindLeaf, beans.Kind)
		assert.InDelta(t, 600, beans.Amount, 1e-9)
		assert.Equal(t, "g", beans.Unit)
		assert.InDelta(t, 600, beans.Grams, 1e-9)
		assert.Equal(t, units.Quantity{Amount: 600, Unit: "g"}, beans.Display)
		assert.Nil(t, beans.Buy)

		onions := got.Ingredients[1]
		assert.InDelta(t, 3, onions.Amount, 1e-9)
		assert.Equal(t, "piece", onions.Unit)
		assert.InDelta(t, 360, onions.Grams, 1e-9)
		assert.Equal(t, units.Quantity{Amount: 3, Unit: "piece"}, onions.Display)
		require.NotNil(t, onions.Buy)
		assert.Equal(t, units.Quantity{Amount: 3, Unit: "piece"}, *onions.Buy)

		rice := got.Ingredients[2]
		assert.Equal(t, food.KindComposed, rice.Kind, "a composed child is one line")
		assert.InDelta(t, 1.5, rice.Amount, 1e-9)
		assert.InDelta(t, 270, rice.Grams, 1e-9)
		assert.Nil(t, rice.Buy)
	})

	t.Run("portions 3", func(t *testing.T) {
		got, err := food.Scale(chiliForFour(), 3)
		require.NoError(t, err)
		onions := got.Ingredients[1]
		assert.InDelta(t, 1.5, onions.Amount, 1e-9)
		assert.Equal(t, units.Quantity{Amount: 1.5, Unit: "piece"}, onions.Display, "counts are exact")
		require.NotNil(t, onions.Buy)
		assert.Equal(t, units.Quantity{Amount: 2, Unit: "piece"}, *onions.Buy)
	})
}

func TestScale_Portions(t *testing.T) {
	tests := []struct {
		name     string
		portions float64
		wantErr  bool
	}{
		{"zero", 0, true},
		{"negative", -1, true},
		{"above 100", 100.5, true},
		{"exactly 100", 100, false},
		{"fractional", 0.5, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := food.Scale(chiliForFour(), tt.portions)
			if tt.wantErr {
				assert.ErrorIs(t, err, domain.ErrInvalidPortions)
				return
			}
			assert.NoError(t, err)
		})
	}
}

func TestScale_MissingReferencePortionsMeansOne(t *testing.T) {
	f := chiliForFour()
	f.ReferencePortions = nil
	got, err := food.Scale(f, 2)
	require.NoError(t, err)
	assert.InDelta(t, 1, got.ReferencePortions, 1e-9)
	assert.InDelta(t, 2, got.Factor, 1e-9)
	assert.InDelta(t, 800, got.Ingredients[0].Amount, 1e-9)
}

func TestScale_LeafIsInvalid(t *testing.T) {
	_, err := food.Scale(&food.Food{ID: 1, Kind: food.KindLeaf, Name: "Onion"}, 2)
	assert.ErrorIs(t, err, domain.ErrInvalidInput)
}
