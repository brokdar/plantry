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

func TestNoCook_DerivedExcludesCookingIngredients(t *testing.T) {
	yes := true
	produce, dairy, bakery := sectionPtr(food.SectionProduce), sectionPtr(food.SectionDairyEggs), sectionPtr(food.SectionBakery)
	never := []struct {
		name string
		sec  *food.ShopSection
	}{
		{"Garlic", produce},
		{"Knoblauch", produce},
		{"Onion", produce},
		{"Rote Zwiebel", produce},
		{"Zwiebeln", produce},
		{"Kräuter", produce},
		{"Shallot", produce},
		{"Ginger", produce},
		{"Ingwer", produce},
		{"Red chili", produce},
		{"Chives", produce},
		{"Schnittlauch", produce},
		{"Fresh coriander", produce},
		{"Petersilie", produce},
		{"Thyme", produce},
		{"Wild garlic paste", produce},
		{"Butter", dairy},
		{"Butterschmalz", dairy},
		{"Ghee", dairy},
		{"Cream", dairy},
		{"Cooking cream", dairy},
		{"Schlagsahne", dairy},
		{"Kochcreme", dairy},
		{"Olive oil", produce},
		{"Olivenöl", produce},
		{"Vegetable broth", produce},
		{"Gemüsebrühe", produce},
		{"Tomato paste", produce},
		{"Tomatenmark", produce},
		{"Apple vinegar", produce},
		{"Apfelessig", produce},
		{"Puff pastry", bakery},
		{"Pizzateig", bakery},
		{"Cornstarch", bakery},
		{"Lemon juice", produce},
		{"Rapsöl", produce},
		{"Cooking creams", dairy},
		{"Chilis", produce},
		{"Herbs", produce},
	}
	for _, tt := range never {
		f := food.Food{Kind: food.KindLeaf, Name: tt.name, ShopSection: tt.sec}
		assert.False(t, planning.NoCook(&f), tt.name)
	}
	keep := []struct {
		name string
		sec  *food.ShopSection
	}{
		{"Apple", produce},
		{"Cucumber", produce},
		{"Tomato", produce},
		{"Carrot", produce},
		{"Banana", produce},
		{"Greek yogurt", dairy},
		{"Cream cheese", dairy},
		{"Frischkäse", dairy},
		{"Skyr", dairy},
		{"Cheddar", dairy},
		{"Wholegrain bread", bakery},
		{"Ciabatta", bakery},
		{"Strawberries", produce},
		{"Salted almonds", produce},
		{"Pasteurized milk", dairy},
		{"Buttermilk", dairy},
		{"Creamy yogurt", dairy},
		{"Erdnussbutter", dairy},
		{"Mandelbutter", dairy},
		{"Peanut butter", dairy},
		{"Thunfisch in Öl", produce},
	}
	for _, tt := range keep {
		f := food.Food{Kind: food.KindLeaf, Name: tt.name, ShopSection: tt.sec}
		assert.True(t, planning.NoCook(&f), tt.name)
	}
	// The override still wins over the tightened default.
	f := food.Food{Kind: food.KindLeaf, Name: "Garlic", ShopSection: produce, NoCookOverride: &yes}
	assert.True(t, planning.NoCook(&f))
}

func TestServingGrams(t *testing.T) {
	assert.Equal(t, float64(planning.TypicalLeafGrams), planning.ServingGrams(planning.FoodFacts{}))
	assert.Equal(t, 150.0, planning.ServingGrams(planning.FoodFacts{ServingG: 150}))
}
