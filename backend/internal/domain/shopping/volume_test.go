package shopping

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
)

// Solids the importer converted from spoons to ml (no density known) are
// listed in grams; liquids keep ml.
func TestVolumeFood(t *testing.T) {
	tests := []struct {
		name string
		sec  food.ShopSection
		want bool
	}{
		{"Wheat flour", food.SectionDryGoods, false},
		{"Pine nuts", food.SectionDryGoods, false},
		{"Honey", food.SectionBakery, false},
		{"Cinnamon", food.SectionSpicesOils, false},
		{"Olive oil", food.SectionSpicesOils, true},
		{"Soy sauce", food.SectionSpicesOils, true},
		{"Oat milk", food.SectionDairyEggs, true},
		{"Coconut milk", food.SectionCansJars, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, volumeFood(tt.sec, tt.name))
		})
	}
}
