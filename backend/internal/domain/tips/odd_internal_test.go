package tips

import (
	"testing"

	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/platefit"
)

// TestOddPlate_PaletteStarchIsFine: rice beside a stir-fry is no oddity.
func TestOddPlate_PaletteStarchIsFine(t *testing.T) {
	t.Parallel()
	tr := true
	p := platefit.Plate{
		Main:  platefit.Component{Name: "Pfanne", Role: food.RoleMain},
		Sides: []platefit.Component{{Name: "Reis", Role: food.RoleSideStarch, Starch: &tr}},
	}
	p.Main.Attrs.Cuisine = "chinese"
	if oddPlate(p) {
		t.Error("rice beside a chinese main is odd")
	}
	p.Sides[0].Name = "Rosmarinkartoffeln"
	if !oddPlate(p) {
		t.Error("roast potatoes beside a chinese main are not odd")
	}
}
