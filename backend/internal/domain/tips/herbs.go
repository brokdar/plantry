package tips

import (
	"regexp"
	"slices"
	"strings"

	"github.com/jaltszeimer/plantry/backend/internal/domain/platefit"
)

// Garnish names by language fragment, as the chef picks them.
var (
	chives   = []string{"schnittlauch", "chive"}
	parsley  = []string{"petersilie", "parsley"}
	coriand  = []string{"koriander", "coriander", "cilantro"}
	basil    = []string{"basilikum", "basil"}
	mint     = []string{"minze", "mint"}
	dill     = []string{"dill"}
	chervil  = []string{"kerbel", "chervil"}
	tarragon = []string{"estragon", "tarragon"}
	scallion = []string{"frühlingszwiebel", "spring onion", "scallion"}
	lemon    = []string{"zitrone", "lemon"}
	lime     = []string{"limette", "lime"}
	yogurt   = []string{"joghurt", "yogurt", "yoghurt", "raita"}
	peanut   = []string{"erdnuss", "erdnüsse", "peanut", "cashew"}
	sesame   = []string{"sesam"}
	soured   = []string{"saure sahne", "sour cream", "schmand", "crème fraîche", "creme fraiche"}
	pickled  = []string{"eingelegt", "pickle"}
	nuts     = []string{"nuss", "nüsse", "nut", "mandel", "almond", "pinienkern", "pine nut", "kerne", "seed", "crouton"}
)

// bold are the garnishes a household that refuses food does not get: the
// strong, divisive herb and citrus of a curry or taco.
var bold = [][]string{coriand, lime}

// herbsByCuisine are the herbs a salad, bowl or wrap of a cuisine takes,
// best first. A cuisine not listed takes chives, then parsley.
var herbsByCuisine = map[string][][]string{
	"indian":         {coriand, mint},
	"thai":           {coriand, basil, mint},
	"mexican":        {coriand},
	"chinese":        {coriand},
	"japanese":       {chives},
	"middle_eastern": {parsley, mint, coriand},
	"italian":        {parsley, basil},
	"mediterranean":  {parsley, basil},
	"french":         {parsley, chervil, tarragon},
}

// contrastGarnish are the classic garnishes of a curry, stir-fry or Mexican
// dish by cuisine, best first.
var contrastGarnish = map[string][][]string{
	"indian":   {coriand, lime, yogurt},
	"thai":     {coriand, lime, peanut},
	"chinese":  {scallion, coriand, peanut, sesame},
	"japanese": {scallion, sesame},
	"mexican":  {coriand, lime, soured, pickled},
}

var (
	fishRe   = regexp.MustCompile(`(?i)fisch|fish|lachs|salmon|kabeljau|cod\b|forelle|trout|thunfisch|tuna|hering|herring|makrele|mackerel|seelachs|scholle|dorade|zander|garnele|shrimp|prawn`)
	tomatoRe = regexp.MustCompile(`(?i)tomate|tomato`)
	brothRe  = regexp.MustCompile(`(?i)brühe|broth|bouillon|consomm|klare`)
	potatoRe = regexp.MustCompile(`(?i)kartoffel|potato`)
)

// classicGarnish is the classic dish–garnish table (culinary knowledge, keyed
// by the main's dish kind, technique, format and cuisine, never by recipe):
// the garnishes the main takes, best first, nil for a main no garnish suits.
// Eggs take chives or parsley; fish from the pan or oven dill, parsley or
// lemon; a curry, stir-fry or Mexican dish its cuisine's herb, citrus, cool
// or crunch (no coriander or lime when the household refuses food); a clear
// broth or potato soup parsley or chives; a salad, bowl or wrap its
// cuisine's herbs, lemon or nuts; a tomato pasta from the stove basil; a
// crisp fried cutlet lemon; another German pan dish (Geschnetzeltes,
// Frikadellen) parsley, chives or lemon. Bakes, gratins, roasts, braises, other soups and
// stews and anything else take none: silence beats noise.
func classicGarnish(main platefit.Component, refuses bool) [][]string {
	a := main.Attrs
	cu := platefit.CuisineOf(a.Cuisine).Name
	desc := strings.Join(append([]string{main.Name}, a.Heroes...), " ")
	baked := a.Kind == "bake" || a.Kind == "roast" || a.Technique == "bake" || a.Technique == "roast" || a.Format == "casserole"
	contrast, isContrast := contrastGarnish[cu]
	switch {
	case a.Kind == "egg":
		if cu == "middle_eastern" || cu == "mediterranean" {
			return [][]string{parsley, chives}
		}
		return [][]string{chives, parsley}
	case !isContrast && (a.Kind == "fish" || fishRe.MatchString(desc)) && !slices.Contains([]string{"curry", "soup", "stew", "pasta"}, a.Kind):
		return [][]string{dill, parsley, lemon}
	case isContrast && !baked:
		if refuses {
			return slices.DeleteFunc(slices.Clone(contrast), func(g []string) bool {
				return slices.ContainsFunc(bold, func(b []string) bool { return slices.Equal(b, g) })
			})
		}
		return contrast
	case a.Kind == "soup" || a.Kind == "stew":
		lead := main.Name
		if len(a.Heroes) > 0 {
			lead += " " + a.Heroes[0]
		}
		if brothRe.MatchString(main.Name) || potatoRe.MatchString(lead) {
			return [][]string{parsley, chives}
		}
		return nil
	case !baked && !breakfastish(main) && (a.Kind == "salad" || a.Kind == "bowl" || a.Kind == "wrap" || a.Format == "salad" || a.Format == "wrap"):
		herbs, ok := herbsByCuisine[cu]
		if !ok {
			herbs = [][]string{chives, parsley}
		}
		return slices.Concat(herbs, [][]string{lemon, nuts})
	case a.Kind == "pasta" && !baked && tomatoRe.MatchString(desc):
		return [][]string{basil}
	case a.Kind == "pan" && !baked && (a.Texture == "crisp" || a.Texture == "crunchy") && a.Technique == "fry":
		return [][]string{lemon}
	case a.Kind == "pan" && !baked && cu == "german":
		return [][]string{parsley, chives, lemon}
	}
	return nil
}

// garnishRank is the index of the first entry of the table row prefs that
// garnish g is, or -1 when it is none of them.
func garnishRank(prefs [][]string, g platefit.Component) int {
	n := strings.ToLower(g.Name)
	return slices.IndexFunc(prefs, func(frags []string) bool {
		return slices.ContainsFunc(frags, func(f string) bool { return strings.Contains(n, f) })
	})
}
