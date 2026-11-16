package shopping_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/shopping"
)

// Leaf names from the seeding run whose auto section was wrong: dried herbs
// are not produce, fresh coriander and juices of fruit were "other".
func TestClassify_RealWorld(t *testing.T) {
	cases := []struct {
		name string
		want food.ShopSection
	}{
		{"Basil (dried)", spices},
		{"Thyme (dried)", spices},
		{"Marjoram (dried)", spices},
		{"Basilikum, getrocknet", spices},
		{"Majoran", spices},
		{"Bay leaf", spices},
		{"Coriander (fresh)", produce},
		{"Basil", produce},
		{"Ground coriander", spices},
		{"Coriander seeds", spices},
		{"Koriandersamen", spices},
		{"Koriander, gemahlen", spices},
		{"Thyme", produce},
		// Tofu stays dairy_eggs on purpose: German supermarkets stock it in the
		// chilled aisle next to dairy, which is where a shopper looks for it.
		{"Tofu (firm)", dairy},
		{"Räuchertofu", dairy},
		{"Lime juice", produce},
		{"Lemon juice", produce},
		{"Zitronensaft", produce},
		{"Limettensaft", produce},
		{"Dried butter beans", food.SectionDryGoods},
		{"Cooked rice", food.SectionCansJars},
		// Unchanged neighbours.
		{"Orangensaft", other},
		{"Dried apricots", other},
		{"Apple juice", other},
		{"Vegetable broth", spices},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, shopping.Classify(tc.name))
		})
	}
}
