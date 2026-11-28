package gates

import (
	"strings"

	"github.com/jaltszeimer/plantry/backend/internal/domain/diet"
)

// AllergenCodes are the allergen codes a Household may list (the EU's main
// allergen groups that matter for a home kitchen).
var AllergenCodes = []string{"nuts", "peanut", "dairy", "egg", "gluten", "fish", "shellfish", "soy", "celery", "mustard", "sesame"}

// plainAllergens maps allergens to folded keywords found in an ingredient
// name (en/de). "=x" is an exact word, "-x" a suffix.
var plainAllergens = []struct {
	code     string
	keywords []string
}{
	{"nuts", []string{"mandel", "walnuss", "walnuesse", "haselnuss", "haselnuesse", "cashew", "pistazie", "pekannuss", "paranuss", "macadamia", "pinienkern", "almond", "walnut", "hazelnut", "pistachio", "pecan", "pine nut", "marzipan", "nougat"}},
	{"peanut", []string{"erdnuss", "erdnuesse", "peanut"}},
	{"dairy", []string{"milch", "kaese", "butter", "sahne", "joghurt", "quark", "frischkaese", "mozzarella", "parmesan", "gouda", "feta", "schmand", "creme fraiche", "milk", "cheese", "cream", "yogurt", "yoghurt", "ghee", "molke", "mascarpone", "ricotta", "=skyr"}},
	{"egg", []string{"=ei", "=eier", "eigelb", "eiweiss", "=egg", "=eggs", "egg yolk", "egg white"}},
	{"gluten", []string{"weizen", "dinkel", "roggen", "gerste", "nudel", "pasta", "spaghetti", "penne", "tagliatelle", "fusilli", "lasagne", "brot", "broetchen", "couscous", "bulgur", "seitan", "semmel", "paniermehl", "wheat", "bread", "noodle", "=mehl", "=flour", "-weizenmehl", "=toast"}},
	{"fish", []string{"fisch", "lachs", "thunfisch", "forelle", "kabeljau", "dorade", "seelachs", "hering", "sardine", "sardelle", "anchov", "makrele", "salmon", "tuna", "cod", "trout", "fish"}},
	{"shellfish", []string{"garnele", "shrimp", "prawn", "krabbe", "crab", "hummer", "lobster", "muschel", "mussel", "auster", "oyster", "tintenfisch", "calamari", "krebs"}},
	{"soy", []string{"soja", "soy", "tofu", "tempeh", "edamame", "miso"}},
	{"celery", []string{"sellerie", "celery"}},
	{"mustard", []string{"senf", "mustard"}},
	{"sesame", []string{"sesam", "tahini", "tahin"}},
}

// plantDairy are plant products named after dairy; they are cut out before
// the dairy check ("Kokosmilch", "Erdnussbutter").
var plantDairy = strings.NewReplacer(
	"kokosmilch", " ", "mandelmilch", " ", "hafermilch", " ", "sojamilch", " ", "reismilch", " ", "cashewmilch", " ",
	"erdnussbutter", " ", "kakaobutter", " ", "kokosbutter", " ", "sheabutter", " ", "sojajoghurt", " ", "kokosjoghurt", " ",
	"coconut milk", " ", "almond milk", " ", "oat milk", " ", "soy milk", " ", "peanut butter", " ", "cocoa butter", " ",
	"fischsauce", " ", "veganer kaese", " ", "vegan cheese", " ",
)

// glutenFree are gluten-free products named after gluten carriers.
var glutenFree = strings.NewReplacer("reisnudel", " ", "glasnudel", " ", "reismehl", " ", "maismehl", " ", "kichererbsenmehl", " ", "kartoffelmehl", " ",
	"rice noodle", " ", "glass noodle", " ", "rice flour", " ", "corn flour", " ", "chickpea flour", " ", "potato flour", " ")

// allergensOf lists the allergen codes an ingredient name states plainly.
func allergensOf(folded string) []string {
	var out []string
	for _, a := range plainAllergens {
		s := folded
		switch a.code {
		case "dairy":
			s = plantDairy.Replace(folded)
		case "gluten":
			s = glutenFree.Replace(folded)
		}
		if anyWord(words(s), a.keywords) || containsAny(s, multiWord(a.keywords)) {
			out = append(out, a.code)
		}
	}
	return out
}

// multiWord returns the keywords that contain a space (they cannot match a
// single word).
func multiWord(kws []string) []string {
	var out []string
	for _, k := range kws {
		if strings.Contains(k, " ") {
			out = append(out, k)
		}
	}
	return out
}

// hidden lists composite ingredients whose parts are not in the name
// (spec §5.10 "hidden ones"; recipe-pipeline §7 block 2). class is the least
// restrictive diet class the product forces; implies are extra folded terms a
// never item can hit. Sources: EU 1169/2011 Annex II, common recipes.
var hidden = []struct {
	keywords  []string
	allergens []string
	class     diet.Class
	implies   []string
}{
	{[]string{"pesto"}, []string{"nuts", "dairy"}, diet.Vegetarian, []string{"pinienkern", "nuss", "kaese", "parmesan", "cheese"}},
	{[]string{"worcester"}, []string{"fish", "celery"}, diet.Pescetarian, []string{"fisch", "sardelle", "anchov"}},
	{[]string{"fischsauce", "fish sauce", "nam pla"}, []string{"fish"}, diet.Pescetarian, []string{"fisch"}},
	{[]string{"austernsauce", "oyster sauce"}, []string{"shellfish"}, diet.Pescetarian, []string{"auster", "muschel"}},
	{[]string{"gelatine", "gelatin"}, nil, diet.Omnivore, []string{"schwein", "rind", "tier"}},
	{[]string{"bruehwuerfel", "bruehe", "bouillon", "stock cube", "brueh", "gemuesebruehe"}, []string{"celery", "gluten"}, diet.Unknown, nil},
	{[]string{"mayonnaise", "mayo", "remoulade", "aioli"}, []string{"egg", "mustard"}, diet.Vegetarian, []string{"ei"}},
	{[]string{"caesar"}, []string{"fish", "egg", "dairy"}, diet.Pescetarian, []string{"fisch", "sardelle", "anchov", "ei"}},
	{[]string{"sojasauce", "soy sauce", "shoyu", "tamari"}, []string{"soy", "gluten"}, diet.Vegan, nil},
	{[]string{"nutella", "nuss-nougat", "nusscreme"}, []string{"nuts", "dairy"}, diet.Vegetarian, []string{"haselnuss", "nuss"}},
	{[]string{"tzatziki", "raita"}, []string{"dairy"}, diet.Vegetarian, []string{"joghurt"}},
	{[]string{"hummus"}, []string{"sesame"}, diet.Vegan, []string{"sesam"}},
	{[]string{"tiramisu"}, []string{"egg", "dairy", "gluten"}, diet.Vegetarian, nil},
}
