package flavor

import (
	"cmp"
	"regexp"
	"slices"
	"strings"
	"sync"

	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
)

// Line is one ingredient of a portion: a leaf's name and grams.
type Line struct {
	Name  string
	Grams float64
}

// Input is what derivation and validation read about a food: its leaves per
// portion (a leaf: itself at 100 g), the energy and fat of that amount and
// its tags.
type Input struct {
	Leaf      bool
	Lines     []Line
	Kcal, Fat float64
	Tags      []string
}

// InputOf builds the input of f from its resolved nutrition d.
func InputOf(f *food.Food, d food.Detailed) Input {
	in := Input{Leaf: f.Kind == food.KindLeaf, Kcal: d.Macros.Kcal, Fat: d.Macros.Fat, Tags: f.Tags}
	in.Lines = make([]Line, 0, len(d.Leaves))
	for _, l := range d.Leaves {
		in.Lines = append(in.Lines, Line{Name: l.Name, Grams: l.Grams})
	}
	return in
}

func (in Input) hasIngredient(name string) bool {
	return slices.ContainsFunc(in.Lines, func(l Line) bool { return strings.EqualFold(l.Name, name) })
}

// ingredientsOf returns the names in names that are ingredients of in (nil
// when none is).
func (in Input) ingredientsOf(names []string) []string {
	var kept []string
	for _, n := range names {
		if in.hasIngredient(n) {
			kept = append(kept, n)
		}
	}
	return kept
}

func (in Input) any(match func(string) bool) bool {
	return slices.ContainsFunc(in.Lines, func(l Line) bool { return match(l.Name) })
}

func (in Input) grams() float64 {
	var g float64
	for _, l := range in.Lines {
		g += l.Grams
	}
	return g
}

// Derive computes the attributes the data supports without a model (ATTR-4):
// heroes by weight, richness from the fat share of energy, saucy from the
// liquid share, colour family from the ingredients, cuisine from tags. A
// leaf gets its colour family only. Everything else stays unknown.
func Derive(in Input) Attributes {
	var a Attributes
	a.Colour = colourOf(in)
	if in.Leaf {
		return a
	}
	a.Heroes = heroes(in)
	if band, ok := richnessBand(in); ok {
		a.Richness = &band
	}
	if total := in.grams(); total > 0 {
		saucy := liquidGrams(in)/total >= saucyShare
		a.Saucy = &saucy
	}
	a.Cuisine = cuisineOf(in.Tags)
	return a
}

// Tunables, checked against the DATA-1 dev labels (derive_test.go).
const (
	// heroRatio: a further hero weighs at least this share of the first.
	heroRatio = 0.6
	// saucyShare: the liquid share of a portion's weight that makes it saucy.
	saucyShare = 0.13
	// plainTomatoLiquid: the liquid share of plain tomatoes in a cooked dish.
	plainTomatoLiquid = 0.5
	// lightKcal: under this a portion is light and richness is at most 1.
	lightKcal = 150
	// mixedShare: two distinct colours each over this share read as mixed.
	mixedShare = 0.35
	// colourant: weight factor of a small, strongly colouring ingredient.
	colourant = 4
)

// richnessBand maps the fat share of energy to 0-3; a light portion (under
// lightKcal) is at most 1. ok is false without energy.
func richnessBand(in Input) (int, bool) {
	if in.Leaf || in.Kcal <= 0 {
		return 0, false
	}
	share := in.Fat * 9 / in.Kcal
	band := 0
	switch {
	case share >= 0.5:
		band = 3
	case share >= 0.35:
		band = 2
	case share >= 0.2:
		band = 1
	}
	if in.Kcal < lightKcal {
		band = min(band, 1)
	}
	return band, true
}

func re(s string) *regexp.Regexp { return regexp.MustCompile(`(?i)` + s) }

// Name lexicons (German and English). Each is matched against a leaf's name.
var (
	// base ingredients carry a dish but are never its hero.
	baseRe = re(`^zwiebel|^onion|knoblauch|garlic|ingwer|ginger|öl\b|öl$|oil\b|butter|mehl|flour|brühe|fond|stock|broth|wasser|water|salz|salt|zucker|sugar|honig|honey|essig|vinegar|pfeffer|gewürz|spice|masala|kreuzkümmel|cumin|pulver|powder|sojasauce|soy sauce|currypaste|curry paste|tomatenmark|tomato paste|senf|mustard|semmelbrösel|breadcrumb|milch$|^milk|backpulver|hefe|yeast`)
	// liquid ingredients make a dish saucy.
	liquidRe = re(`brühe|fond|stock|broth|kokosmilch|coconut milk|sahne|cream|\bwein|rotwein|weißwein|weisswein|\bwine|passata|passiert|gehackte tomaten|stückige tomaten|tomaten.*dose|dosentomaten|canned tomato|chopped tomato|crushed tomato|tomatensauce|tomatensoße|tomato sauce`)
	tomatoRe = re(`tomate|tomato`)
	// raw vegetables mark a salad: its tomatoes are not liquid.
	rawRe  = re(`gurke|cucumber|kopfsalat|lettuce|blattsalat`)
	acidRe = re(`zitrone|lemon|limette|lime|essig|vinegar|joghurt|yogurt|yoghurt|tomate|tomato|\bwein|rotwein|weißwein|weisswein|\bwine|kapern|caper|gurke.*eingelegt|pickle|tamarind|sauerkraut|buttermilch|schmand|saure sahne|sour cream|crème fraîche|creme fraiche|orange|apfel|apple|ananas|pineapple|beeren|berr|rhabarber|rhubarb`)
	// colourants colour a dish beyond their weight.
	// creamRe are the creamy liquids that turn a red sauce orange.
	creamRe     = re(`sahne|cream|kokosmilch|coconut milk|joghurt|yogurt|yoghurt|milch|milk`)
	colourantRe = re(`tomatenmark|tomato paste|paprikapulver|paprika powder|kurkuma|turmeric|currypulver|curry powder|sojasauce|soy sauce|rotwein|red wine|miso`)
)

// colourRules map a name to its colour family, first match wins; a name that
// matches none (oil, spices, onions, broth) does not colour the dish.
var colourRules = []struct {
	re     *regexp.Regexp
	colour string
}{
	{re(`essig|vinegar|öl\b|öl$|oil\b|brühe|fond|stock|broth|wasser|water|salz|salt|zucker|sugar|honig|honey|masala|kreuzkümmel|cumin|knoblauch|garlic|ingwer|ginger|senf|mustard|semmelbrösel|breadcrumb|currypaste|curry paste`), ""},
	{re(`rotkohl|red cabbage|blaukraut|rote zwiebel|red onion|aubergine|eggplant|rote bete|beetroot|heidelbeer|blueberr`), "purple"},
	{re(`süßkartoffel|sweet potato|rote linsen|red lentil|kürbis|pumpkin|squash|karotte|möhre|carrot|lachs|salmon|mango|aprikose|apricot|kurkuma|turmeric|curry`), "orange"},
	{re(`röstzwiebel|fried onion|soja|soy|miso|nori|rind|beef|hack|mince|schweinenacken|schweinebauch|schweinefleisch|pork|speck|bacon|lamm|lamb|wurst|sausage|ente|duck|rotwein|red wine|linsen|lentil|schokolade|chocolate|kakao|cocoa|pilz|mushroom`), "brown"},
	{re(`kidney|tomate|tomato|paprika|bell pepper|chili|salsa|ketchup|erdbeer|strawberr|himbeer|raspberr`), "red"},
	{re(`spinat|spinach|brokkoli|broccoli|zucchini|courgette|gurke|cucumber|salat|lettuce|rucola|rocket|arugula|petersilie|parsley|koriander|cilantro|coriander|basilikum|basil|minze|mint|dill|schnittlauch|chive|\berbse|\bpeas?\b|grüne bohnen|green bean|pak choi|bok choy|grünkohl|kale|lauch|leek|frühlingszwiebel|spring onion|scallion|avocado|limette|\blime|kiwi|sellerie|celery|edamame|spargel|asparagus|wirsing|rosenkohl|sprout`), "green"},
	{re(`reis|rice|kartoffel|potato|joghurt|yogurt|yoghurt|quark|milch|milk|sahne|cream|kokos|coconut|kabeljau|\bcod\b|seelachs|fisch|fish|tofu|mozzarella|feta|blumenkohl|cauliflower|weißkohl|cabbage|banane|banana`), "white"},
	{re(`nudel|pasta|spaghetti|penne|mehl|flour|couscous|bulgur|quinoa|hafer|oat|brot|bread|tortilla|\beier?\b|\beggs?\b|hähnchen|chicken|pute|turkey|schweinefilet|kichererbs|chickpea|parmesan|gouda|käse|cheese|champignon|garnele|shrimp|prawn|nuss|nüsse|\bnuts?\b|erdnüss|peanut|sesam|sesame|tahini|zitrone|lemon|apfel|apple|beeren|berr|bohnen|beans`), "beige"},
}

// traits are what the lexicons say about one ingredient name.
type traits struct {
	base, liquid, tomato, raw, acid, cream, colourant bool
	colour                                            string
}

// traitCache memoizes traitsOf: the lexicons are large case-insensitive
// alternations, and a planning state resolves every food's leaves.
var traitCache sync.Map // string -> traits

func traitsOf(name string) traits {
	if v, ok := traitCache.Load(name); ok {
		return v.(traits)
	}
	t := traits{
		base: baseRe.MatchString(name), liquid: liquidRe.MatchString(name), tomato: tomatoRe.MatchString(name),
		raw: rawRe.MatchString(name), acid: acidRe.MatchString(name), cream: creamRe.MatchString(name),
		colourant: colourantRe.MatchString(name),
	}
	for _, r := range colourRules {
		if r.re.MatchString(name) {
			t.colour = r.colour
			break
		}
	}
	traitCache.Store(name, t)
	return t
}

func isAcid(name string) bool { return traitsOf(name).acid }

// colourOf is the colour family of the ingredients by weight: the dominant
// colour (a raw salad's creamy dressing does not count); a red fifth with
// creamy liquid of at least a third of the red (tomato and cream or coconut)
// reads orange; two distinct colours each over mixedShare
// of the coloured weight read mixed.
func colourOf(in Input) string {
	weight := map[string]float64{}
	var total, cream float64
	salad := in.any(func(name string) bool { return traitsOf(name).raw })
	for _, l := range in.Lines {
		t := traitsOf(l.Name)
		if t.cream {
			cream += l.Grams
			if salad {
				continue // a raw salad's creamy dressing does not colour it
			}
		}
		c := t.colour
		if c == "" {
			continue
		}
		g := l.Grams
		if t.colourant {
			g *= colourant
		}
		weight[c] += g
		total += g
	}
	if total == 0 {
		return ""
	}
	type share struct {
		colour string
		w      float64
	}
	ranked := make([]share, 0, len(weight))
	for c, w := range weight {
		ranked = append(ranked, share{c, w})
	}
	slices.SortFunc(ranked, func(a, b share) int {
		return cmp.Or(cmp.Compare(b.w, a.w), cmp.Compare(a.colour, b.colour))
	})
	top := ranked[0]
	if red := weight["red"]; red/total >= 0.2 && cream >= red/3 {
		return "orange"
	}
	if len(ranked) > 1 && ranked[1].w/total >= mixedShare && top.w/total < 0.5 {
		return "mixed"
	}
	return top.colour
}

// heroes are the heaviest non-base ingredients: the first, and up to two
// more weighing at least heroRatio of it.
func heroes(in Input) []string {
	lines := slices.Clone(in.Lines)
	lines = slices.DeleteFunc(lines, func(l Line) bool { return traitsOf(l.Name).base || l.Grams <= 0 })
	if len(lines) == 0 {
		return nil
	}
	slices.SortStableFunc(lines, func(a, b Line) int { return cmp.Compare(b.Grams, a.Grams) })
	out := []string{lines[0].Name}
	for _, l := range lines[1:] {
		if len(out) == MaxHeroes || l.Grams < heroRatio*lines[0].Grams {
			break
		}
		out = append(out, l.Name)
	}
	return out
}

// liquidGrams is the liquid weight of a portion: liquids in full, plain
// tomatoes in part unless the dish is a raw salad.
func liquidGrams(in Input) float64 {
	salad := in.any(func(name string) bool { return traitsOf(name).raw })
	var g float64
	for _, l := range in.Lines {
		t := traitsOf(l.Name)
		switch {
		case t.liquid:
			g += l.Grams
		case !salad && t.tomato && !t.colourant:
			g += l.Grams * plainTomatoLiquid
		}
	}
	return g
}

// cuisineTags map a tag (German or English, lower case) to a cuisine.
var cuisineTags = map[string]string{
	"deutsch": "german", "german": "german",
	"italienisch": "italian", "italian": "italian",
	"französisch": "french", "french": "french",
	"mediterran": "mediterranean", "mediterranean": "mediterranean", "griechisch": "mediterranean",
	"greek": "mediterranean", "spanish": "mediterranean",
	"spanisch": "mediterranean", //nolint:misspell // the German tag
	"indisch":  "indian", "indian": "indian",
	"chinesisch": "chinese", "chinese": "chinese",
	"japanisch": "japanese", "japanese": "japanese",
	"thai": "thai", "thailändisch": "thai",
	"mexikanisch": "mexican", "mexican": "mexican",
	"orientalisch": "middle_eastern", "arabisch": "middle_eastern", "libanesisch": "middle_eastern",
	"türkisch": "middle_eastern", "persisch": "middle_eastern", "middle eastern": "middle_eastern",
	"middle_eastern": "middle_eastern", "levantine": "middle_eastern", "turkish": "middle_eastern",
}

// cuisineOf is the cuisine the tags name; unknown when none or several
// different ones do.
func cuisineOf(tags []string) string {
	found := ""
	for _, t := range tags {
		c, ok := cuisineTags[strings.ToLower(strings.TrimSpace(t))]
		if !ok {
			continue
		}
		if found != "" && found != c {
			return ""
		}
		found = c
	}
	return found
}
