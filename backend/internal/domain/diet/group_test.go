package diet_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaltszeimer/plantry/backend/internal/domain/diet"
)

func TestGroupOf(t *testing.T) {
	tests := []struct {
		name string
		want diet.Group
	}{
		{"Hähnchenbrust", diet.GroupMeat},
		{"Haehnchenbrust, roh", diet.GroupMeat},
		{"Salami", diet.GroupMeat},
		{"Speck", diet.GroupMeat},
		{"Leberkäse", diet.GroupMeat},
		{"Rinderhackfleisch", diet.GroupMeat},
		{"Schinken, gekocht", diet.GroupMeat},
		{"chicken breast", diet.GroupMeat},
		{"bacon", diet.GroupMeat},
		{"Hähnchen in Kokosmilch", diet.GroupMeat},
		{"Lachsfilet", diet.GroupFish},
		{"Thunfisch in Wasser", diet.GroupFish},
		{"Garnelen", diet.GroupFish},
		{"salmon fillet", diet.GroupFish},
		{"crabmeat", diet.GroupFish},
		{"Kichererbsen", diet.GroupLegume},
		{"Rote Linsen", diet.GroupLegume},
		{"Erbsen", diet.GroupLegume},
		{"Kidneybohnen", diet.GroupLegume},
		{"Butterbohnen", diet.GroupLegume},
		{"Tofu", diet.GroupLegume},
		{"Räuchertofu", diet.GroupLegume},
		{"Tempeh", diet.GroupLegume},
		{"chickpeas", diet.GroupLegume},
		{"lentils", diet.GroupLegume},
		{"Erdnüsse", diet.GroupNone},
		{"Erdnussbutter", diet.GroupNone},
		{"Grüne Bohnen", diet.GroupNone},
		{"green beans", diet.GroupNone},
		{"Sojasauce", diet.GroupNone},
		{"Sojamilch", diet.GroupNone},
		{"Hühnereier", diet.GroupNone},
		{"Vegane Wurst", diet.GroupNone},
		{"Worcestersauce", diet.GroupNone},
		{"Gelatine", diet.GroupNone},
		{"Vollmilch", diet.GroupNone},
		{"Brokkoli", diet.GroupNone},
		{"Hühnerbrühe", diet.GroupNone},
		{"Rinderbrühe", diet.GroupNone},
		{"Kalbsfond", diet.GroupNone},
		{"Fleischbrühe", diet.GroupNone},
		{"Chicken stock", diet.GroupNone},
		{"Beef broth", diet.GroupNone},
		{"Beef bouillon", diet.GroupNone},
		{"Gemüsebrühe mit Huhn", diet.GroupMeat},
		{"Gänseschmalz", diet.GroupNone},
		{"Hühnerfett", diet.GroupNone},
		{"Lebertran", diet.GroupNone},
		{"Lachsöl", diet.GroupNone},
		{"Fischsauce", diet.GroupNone},
		{"Fish sauce", diet.GroupNone},
		{"Oyster sauce", diet.GroupNone},
		{"Austernsauce", diet.GroupNone},
		{"Fischfond", diet.GroupNone},
		{"Pflanzliche Bratwurst", diet.GroupNone},
		{"Vegetarische Wurst", diet.GroupNone},
		{"Veggie Burger", diet.GroupNone},
		{"plant-based chicken", diet.GroupNone},
		{"meat-free sausage", diet.GroupNone},
		{"Bohnenkraut", diet.GroupNone},
		{"Kaffeebohnen", diet.GroupNone},
		{"Kakaobohnen", diet.GroupNone},
		{"Tonkabohne", diet.GroupNone},
		{"coffee beans", diet.GroupNone},
		{"cocoa beans", diet.GroupNone},
		{"tonka bean", diet.GroupNone},
		{"Kassler", diet.GroupMeat},
		{"Wiener", diet.GroupMeat},
		{"Herz", diet.GroupMeat},
		{"Zunge", diet.GroupMeat},
		{"Dorsch", diet.GroupFish},
		{"Meeresfrüchte", diet.GroupFish},
		{"Cannellini", diet.GroupLegume},
		{"Borlotti", diet.GroupLegume},
		{"Falafel", diet.GroupLegume},
		// Protein isolates are not a legume meal, in either language.
		{"Erbsenprotein", diet.GroupNone},
		{"pea protein", diet.GroupNone},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, diet.GroupOf(tt.name))
		})
	}
}
