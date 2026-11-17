package planning_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
)

func TestNoCook_OverrideWinsOverDerivation(t *testing.T) {
	yes, no := true, false
	tests := []struct {
		name       string
		f          food.Food
		want       bool
		wantSource string
	}{
		{"derived leaf", food.Food{Kind: food.KindLeaf, Name: "Reis"}, false, "derived"},
		{"derived produce", food.Food{Kind: food.KindLeaf, Name: "Apfel", ShopSection: sectionPtr(food.SectionProduce)}, true, "derived"},
		{"override on a leaf", food.Food{Kind: food.KindLeaf, Name: "Reis", NoCookOverride: &yes}, true, "user"},
		{"override off a leaf", food.Food{Kind: food.KindLeaf, Name: "Apfel", ShopSection: sectionPtr(food.SectionProduce), NoCookOverride: &no}, false, "user"},
		{"override on a recipe", food.Food{Kind: food.KindComposed, Name: "Bowl", NoCookOverride: &yes}, true, "user"},
		{"override off a recipe", food.Food{Kind: food.KindComposed, Name: "Salat", CookMinutes: intPtr(0), NoCookOverride: &no}, false, "user"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, planning.NoCook(&tt.f))
			assert.Equal(t, tt.wantSource, planning.NoCookSource(&tt.f))
		})
	}
}

func sectionPtr(s food.ShopSection) *food.ShopSection { return &s }
func intPtr(v int) *int                               { return &v }
