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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, diet.GroupOf(tt.name))
		})
	}
}
