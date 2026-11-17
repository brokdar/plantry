package shopping_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/shopping"
)

func TestEffectiveRole(t *testing.T) {
	sec := func(s food.ShopSection) *food.ShopSection { return &s }
	role := func(r food.Role) *food.Role { return &r }
	tests := []struct {
		name       string
		f          food.Food
		want       *food.Role
		wantSource string
	}{
		{"produce", food.Food{Kind: food.KindLeaf, Name: "x", ShopSection: sec(food.SectionProduce)}, role(food.RoleSideVeg), "derived"},
		{"bakery", food.Food{Kind: food.KindLeaf, Name: "x", ShopSection: sec(food.SectionBakery)}, role(food.RoleSideStarch), "derived"},
		{"meat and fish", food.Food{Kind: food.KindLeaf, Name: "x", ShopSection: sec(food.SectionMeatFish)}, role(food.RoleSideProtein), "derived"},
		{"other", food.Food{Kind: food.KindLeaf, Name: "x", ShopSection: sec(food.SectionOther)}, nil, ""},
		{"classified by name", food.Food{Kind: food.KindLeaf, Name: "Gurke"}, role(food.RoleSideVeg), "derived"},
		{"override wins", food.Food{Kind: food.KindLeaf, Name: "x", ShopSection: sec(food.SectionProduce), Role: role(food.RoleSauce)}, role(food.RoleSauce), "user"},
		{"recipe keeps its role", food.Food{Kind: food.KindComposed, Role: role(food.RoleMain)}, role(food.RoleMain), "user"},
		{"recipe without role", food.Food{Kind: food.KindComposed}, nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, src := shopping.EffectiveRole(&tt.f)
			assert.Equal(t, tt.wantSource, src)
			if tt.want == nil {
				assert.Nil(t, got)
				return
			}
			require.NotNil(t, got)
			assert.Equal(t, *tt.want, *got)
		})
	}
}
