package chefhelp

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
)

func TestKitchenAmount(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		amount float64
		unit   string
		want   string
		name   string
	}{
		{1200, "g", "1.2 kg", ""},
		{85.05, "g", "85 g", ""},
		{37.5, "ml", "2.5 tbsp", ""},
		{60, "ml", "4 tbsp", ""},
		{75, "ml", "75 ml", ""},
		{1.88, "ml", "0.5 tsp", ""},
		{28.12, "ml", "2 tbsp", ""},
		{12, "ml", "2.5 tsp", ""},
		{0.75, "piece", "1 piece", ""},
		{2.2, "clove", "2 clove", ""},
		{1.3, "tbsp", "1.5 tbsp", ""},
		{3.5, "piece", "4 piece", "Spring onions"},
		{2.5, "piece", "3 piece", ""},
		{0.5, "head", "0.5 head", "Cabbage"},
		{0.25, "can", "0.5 can", "Tomatoes"},
		{0.2, "slice", "1 slice", "Bacon"},
		{440, "ml", "185 g", "Cheddar"},
		{480, "ml", "480 ml", "Beef broth"},
		{480, "ml", "480 ml", "Milk"},
	} {
		assert.Equal(t, c.want, kitchenAmount(c.amount, c.unit, c.name), "%v %s", c.amount, c.unit)
	}
}

func TestAmountsListed(t *testing.T) {
	t.Parallel()
	allowed := []float64{600, 3, 2, 1.5}
	for line, want := range map[string]bool{
		"600 g beans: keep them in the pot.":        true,
		"3 Zwiebeln: dice them, 5-10 min at 180 °C": true,
		"1,5 tbsp soy sauce: stir in.":              true,
		"1 tsp sugar: stir in now.":                 true,
		"½ tsp salt: add.":                          true,
		"Move it to a clean pan, see step 5.":       true,
		"1.88 ml oil: add.":                         false,
		"0.75 Zwiebel: add.":                        false,
		"Simmer 3 to 4 minutes more.":               true,
		"Cook 5 bis 6 Minuten.":                     true,
		"Wait 2 more minutes.":                      true,
		"Cut into 4 pieces.":                        true,
		"Set to level 3.":                           true,
		"1200 g lentils: add.":                      false,
		"85.05 g cheddar on top.":                   false,
		"+100 ml water: add and simmer 5 min.":      false,
	} {
		assert.Equal(t, want, amountsListed(line, allowed, "Zwiebel", "lentils"), line)
	}
}

func TestBuildRequestRetryHint(t *testing.T) {
	t.Parallel()
	recipe := &food.Food{Name: "Soup"}
	text := func(hint string) string {
		return buildRequest("m", Request{Kind: KindExplainStep}, recipe, 2, nil, hint).Messages[0].Content[0].Text
	}
	assert.NotContains(t, text(""), "not in the list above")
	assert.Contains(t, text(retryHint), "not in the list above")
}

// The ingredient list a help request offers: pinches above two and catalogue
// qualifiers never reach the answer.
func TestBuildRequestIngredientList(t *testing.T) {
	t.Parallel()
	ref := 2.0
	recipe := &food.Food{Name: "Soup", ReferencePortions: &ref, Children: []food.FoodComponent{
		{ChildName: "Salt", Amount: 1, Unit: "pinch"},
		{ChildName: "Pepper", Amount: 0.5, Unit: "pinch"},
		{ChildName: "Water, bottled, generic", Amount: 180, Unit: "ml"},
	}}
	text := buildRequest("m", Request{Kind: KindExplainStep}, recipe, 6, nil, "").Messages[0].Content[0].Text
	assert.Contains(t, text, "- Salt (to taste)")
	assert.Contains(t, text, "- 1.5 pinch Pepper", "two pinches or fewer stay an amount")
	assert.Contains(t, text, "- 540 ml Water\n")
	assert.NotContains(t, text, "bottled")
	assert.False(t, toTaste(food.FoodComponent{Amount: 1, Unit: "pinch"}, 2), "2 pinches stay an amount")
	assert.NotContains(t, allowedNumbers(recipe, 6), 3.0)
}
