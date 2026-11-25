package tips

import (
	"regexp"
	"slices"
	"strings"

	"github.com/jaltszeimer/plantry/backend/internal/domain/platefit"
)

// Herb names by language fragment, as the chef picks them.
var (
	chives   = []string{"schnittlauch", "chive"}
	parsley  = []string{"petersilie", "parsley"}
	coriand  = []string{"koriander", "coriander", "cilantro"}
	basil    = []string{"basilikum", "basil"}
	mint     = []string{"minze", "mint"}
	dill     = []string{"dill"}
	chervil  = []string{"kerbel", "chervil"}
	tarragon = []string{"estragon", "tarragon"}
)

// herbsByCuisine are the herbs a cuisine's plate takes, best first. A cuisine
// not listed (German, neutral, anything new) takes chives, then parsley.
var herbsByCuisine = map[string][][]string{
	"indian":         {coriand, mint},
	"thai":           {coriand, basil, mint},
	"mexican":        {coriand},
	"chinese":        {coriand},
	"japanese":       {chives},
	"middle_eastern": {coriand, mint, parsley},
	"italian":        {parsley, basil},
	"mediterranean":  {parsley, basil},
	"french":         {parsley, chervil, tarragon},
}

var (
	fishRe   = regexp.MustCompile(`(?i)fisch|fish|lachs|salmon|kabeljau|cod\b|forelle|trout|thunfisch|tuna|hering|herring|makrele|mackerel|seelachs|scholle|dorade|zander|garnele|shrimp|prawn`)
	tomatoRe = regexp.MustCompile(`(?i)tomate|tomato`)
)

// herbPrefs are the herbs that suit the plate's main, best first: the
// cuisine's list, dill first on fish outside the contrast cuisines (curry
// and stir-fry take coriander), basil first on a tomato-led Italian plate.
func herbPrefs(main platefit.Component) [][]string {
	cu := platefit.CuisineOf(main.Attrs.Cuisine)
	prefs, ok := herbsByCuisine[cu.Name]
	if !ok {
		prefs = [][]string{chives, parsley}
	}
	desc := strings.Join(append([]string{main.Name}, main.Attrs.Heroes...), " ")
	if cu.Name == "italian" && tomatoRe.MatchString(desc) {
		prefs = [][]string{basil, parsley}
	}
	if cu.Style != platefit.Contrast && fishRe.MatchString(desc) {
		prefs = append([][]string{dill}, prefs...)
	}
	return prefs
}

// herbRank is the index of the first preferred herb g is; a herb outside the
// list ranks after all of them.
func herbRank(prefs [][]string, g platefit.Component) int {
	n := strings.ToLower(g.Name)
	i := slices.IndexFunc(prefs, func(frags []string) bool {
		return slices.ContainsFunc(frags, func(f string) bool { return strings.Contains(n, f) })
	})
	if i < 0 {
		return len(prefs)
	}
	return i
}
