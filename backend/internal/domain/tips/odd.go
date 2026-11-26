package tips

import (
	"regexp"

	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/platefit"
)

var (
	fruitRe     = regexp.MustCompile(`(?i)apfel|apple|banane|banana|beere|berry|obst|fruit|mango|ananas|pineapple|birne|pear|traube|grape|müsli|muesli|porridge|granola`)
	fruitSaucRe = regexp.MustCompile(`(?i)mus\b|sauce|soße|kompott|chutney`)
)

// oddPlate reports whether p is mismatched or crowded in a way a herb on top
// cannot mend: a second protein beside the main, a starch side the main does
// not take (its own starch, or a food world outside the main's cuisine), or a
// breakfast or fruit dish beside a savoury main. A garnish tip is silent there.
func oddPlate(p platefit.Plate) bool {
	cu := platefit.CuisineOf(p.Main.Attrs.Cuisine)
	savoury := !breakfastish(p.Main)
	for _, s := range p.Sides {
		if s.Leaf {
			continue
		}
		if s.Role == food.RoleMain || s.Role == food.RoleSideProtein {
			return true
		}
		if savoury && breakfastish(s) {
			return true
		}
		if starch(s) && (p.Main.Starch != nil && *p.Main.Starch || !belongs(cu, s) && foreignStarch(cu, s)) {
			return true
		}
	}
	return false
}

// foreignStarch: a starch side of another known food world than the main's,
// or any starch outside the palette of a contrast cuisine (curry, stir-fry).
func foreignStarch(cu platefit.Cuisine, s platefit.Component) bool {
	if cu.Group == "" {
		return false
	}
	if g := platefit.CuisineOf(s.Attrs.Cuisine).Group; g != "" && g != cu.Group {
		return true
	}
	return cu.Style == platefit.Contrast
}

// breakfastish: a breakfast dish or a fruit that is no sauce.
func breakfastish(c platefit.Component) bool {
	return c.Attrs.Kind == "breakfast" || fruitRe.MatchString(c.Name) && !fruitSaucRe.MatchString(c.Name)
}
