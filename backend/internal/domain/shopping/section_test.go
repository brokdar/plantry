package shopping_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/shopping"
)

func TestClassify(t *testing.T) {
	tests := []struct {
		name string
		want food.ShopSection
	}{
		// de
		{"Kidneybohnen (Dose)", food.SectionCansJars},
		{"Zwiebel", food.SectionProduce},
		{"Hähnchenbrust", food.SectionMeatFish},
		{"Milch", food.SectionDairyEggs},
		{"Basmati", food.SectionDryGoods},
		{"xyz", food.SectionOther},
		{"Vollkornbrot", food.SectionBakery},
		{"Olivenöl", food.SectionSpicesOils},
		{"Rinderhack", food.SectionMeatFish},
		{"Erbsen (TK)", food.SectionFrozen},
		{"Basmatireis", food.SectionDryGoods},
		{"Eier", food.SectionDairyEggs},
		{"Salbei", food.SectionProduce},
		{"Zwiebelpulver", food.SectionSpicesOils},
		{"Thunfisch (Dose)", food.SectionCansJars},
		// en
		{"Kidney beans (canned)", food.SectionCansJars},
		{"Onion", food.SectionProduce},
		{"Chicken breast", food.SectionMeatFish},
		{"Milk", food.SectionDairyEggs},
		{"Basmati rice", food.SectionDryGoods},
		{"Sourdough bread", food.SectionBakery},
		{"Olive oil", food.SectionSpicesOils},
		{"Frozen peas", food.SectionFrozen},
		{"Eggs", food.SectionDairyEggs},
		{"", food.SectionOther},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, shopping.Classify(tt.name))
		})
	}
}
