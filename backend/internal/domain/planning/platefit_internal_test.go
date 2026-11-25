package planning

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
)

func TestFreshStarch(t *testing.T) {
	b := func(v bool) *bool { return &v }
	recipe := func(name string, role food.Role) FoodFacts {
		return FoodFacts{Name: name, Kind: food.KindComposed, Role: role}
	}
	leaf := func(name string) FoodFacts { return FoodFacts{Name: name, Kind: food.KindLeaf} }
	lines := func(kv ...any) []food.LeafGrams {
		var out []food.LeafGrams
		for i := 0; i < len(kv); i += 2 {
			out = append(out, food.LeafGrams{Name: kv[i].(string), Grams: kv[i+1].(float64)})
		}
		return out
	}
	tests := []struct {
		name          string
		f             FoodFacts
		leaves        []food.LeafGrams
		fresh, starch *bool
	}{
		{"pasta by name", recipe("Spaghetti Bolognese", food.RoleMain), lines("Rinderhack", 125.0, "Tomaten", 100.0), b(false), b(true)},
		{"potato by ingredient", recipe("Auflauf", food.RoleMain), lines("Kartoffeln", 200.0, "Sahne", 50.0), b(false), b(true)},
		{"a roux is no starch", recipe("Geschnetzeltes", food.RoleMain), lines("Putenbrust", 150.0, "Mehl", 10.0), b(false), b(false)},
		{"a quiche crust is", recipe("Quiche", food.RoleMain), lines("Mehl", 60.0, "Eier", 50.0), b(false), b(true)},
		{"a starch side is a starch", recipe("Knödel", food.RoleSideStarch), lines("Semmel", 80.0), b(false), b(true)},
		{"herbs make it fresh", recipe("Curry", food.RoleMain), lines("Hähnchen", 150.0, "Koriander", 5.0), b(true), b(false)},
		{"a salad is fresh", recipe("Gurkensalat", food.RoleSideVeg), lines("Gurken", 150.0, "Joghurt", 50.0), b(true), b(false)},
		{"no ingredients: unknown", recipe("Mystery", food.RoleMain), nil, nil, nil},
		{"a raw leaf by name", leaf("Gurken"), nil, b(true), nil},
		{"bread is not fresh", leaf("Brot"), nil, b(false), nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fresh, starch := freshStarch(tt.f, tt.leaves)
			assert.Equal(t, tt.fresh, fresh, "fresh")
			assert.Equal(t, tt.starch, starch, "starch")
		})
	}
}
