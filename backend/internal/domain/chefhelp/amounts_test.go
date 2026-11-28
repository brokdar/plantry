package chefhelp

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestKitchenAmount(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		amount float64
		unit   string
		want   string
	}{
		{1200, "g", "1.2 kg"},
		{85.05, "g", "85 g"},
		{37.5, "ml", "38 ml"},
		{1.88, "ml", "0.5 tsp"},
		{28.12, "ml", "2 tbsp"},
		{12, "ml", "2.5 tsp"},
		{0.75, "piece", "1 piece"},
		{2.2, "clove", "2 clove"},
		{1.3, "tbsp", "1.5 tbsp"},
	} {
		assert.Equal(t, c.want, kitchenAmount(c.amount, c.unit), "%v %s", c.amount, c.unit)
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
		"85.05 g cheddar on top.":                   false,
		"+100 ml water: add and simmer 5 min.":      false,
	} {
		assert.Equal(t, want, amountsListed(line, allowed), line)
	}
}
