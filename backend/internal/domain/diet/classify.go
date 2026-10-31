package diet

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// The classifier (D29) is a deterministic en/de keyword table, data-driven
// like shopping.Classify. Names and keywords are lowercased and folded
// (ä → ae, ß → ss, é → e, …), so "Haehnchen" and "Hähnchen" are the same
// word. A keyword matches a word of the name when:
//
//   - "=word": the word is exactly it
//   - "stem-": the word starts or ends with it ("rind-" in "Rinderhack")
//   - otherwise: the word ends with it ("huhn" in "Suppenhuhn"), or, for
//     keywords of five or more letters, contains it ("fleisch" in
//     "Rinderhackfleisch")
//
// Classify runs three stages:
//
//  1. phrases: names a keyword gets wrong ("Kokosmilch", "Butternut"). Each
//     match adds its class and is cut out of the name, so it only
//     neutralises its own words: "Hähnchen in Kokosmilch" still has
//     "Hähnchen". A phrase matches anywhere in the folded name; "=phrase"
//     only as whole words, "phrase-" only at the start of a word.
//  2. markers: a remaining word starting with "vegan" or "vegetari"/"veggie"
//     names the class of the rest ("Vegane Wurst").
//  3. keywords: unlike the store section, the class is not the head noun's
//     but the least restrictive animal keyword anywhere in the rest, since
//     any animal part decides it ("Rinderbrühe", "Fischsauce").
//
// The result is the max (diet.Max) of everything found. With nothing found,
// a plant keyword makes the food vegan; with no plant keyword either it is
// Unknown.

// rule lists one class's keywords.
type rule struct {
	class    Class
	keywords []string
}

var markers = []rule{
	{Vegan, []string{"vegan-"}},
	{Vegetarian, []string{"vegetari-", "veggie-"}},
}

// phrases are checked in order; see Classify for the syntax.
var phrases = []struct {
	phrase string
	class  Class
}{
	// Plant drinks, creams and spreads named after dairy.
	{"kokosmilch", Vegan},
	{"mandelmilch", Vegan},
	{"hafermilch", Vegan},
	{"sojamilch", Vegan},
	{"reismilch", Vegan},
	{"cashewmilch", Vegan},
	{"sojajoghurt", Vegan},
	{"kokosjoghurt", Vegan},
	{"hafersahne", Vegan},
	{"sojasahne", Vegan},
	{"kokossahne", Vegan},
	{"erdnussbutter", Vegan},
	{"mandelbutter", Vegan},
	{"cashewbutter", Vegan},
	{"kakaobutter", Vegan},
	{"coconut milk", Vegan},
	{"almond milk", Vegan},
	{"oat milk", Vegan},
	{"soy milk", Vegan},
	{"rice milk", Vegan},
	{"soy yogurt", Vegan},
	{"coconut yogurt", Vegan},
	{"coconut cream", Vegan},
	{"peanut butter", Vegan},
	{"almond butter", Vegan},
	{"cocoa butter", Vegan},
	{"cream of tartar", Vegan},
	{"zartbitterschokolade", Vegan},
	{"dark chocolate", Vegan},
	// Plants named after animals or dairy.
	{"butternut", Vegan},
	{"butterbohnen", Vegan},
	{"butter beans", Vegan},
	{"fleischtomate", Vegan},
	{"ochsenherz", Vegan},
	{"honigmelone", Vegan},
	{"honeydew", Vegan},
	// Animal names that are not meat.
	{"=hühnerei", Vegetarian},
	{"=hühnereier", Vegetarian},
	{"hühnereiweiß-", Vegetarian},
	{"hühnereigelb-", Vegetarian},
	{"butterschmalz", Vegetarian},
	// Seafood named like meat.
	{"crabmeat", Pescetarian},
}

// animal holds the keywords of the non-vegan classes; the least restrictive
// match wins.
var animal = []rule{
	{Omnivore, []string{
		"fleisch", "hack-", "rind-", "schwein", "kalb-", "lamm-", "hähnchen",
		"hühnchen", "huhn", "hühner-", "pute-", "=ente", "enten-", "gans",
		"gänse-", "hirsch", "=reh", "kaninchen", "speck", "schinken",
		"wurst", "würstchen", "salami", "chorizo", "pancetta", "mortadella",
		"leber", "schnitzel", "steak", "gulasch", "frikadelle", "bulette",
		"kasseler", "schmalz", "gelatin", "gyros", "döner",
		"meat-", "chicken", "turkey", "beef", "pork", "bacon", "=ham", "lamb",
		"=veal", "duck", "sausage", "pepperoni", "prosciutto", "=lard",
		"venison", "=goose",
	}},
	{Pescetarian, []string{
		"fisch-", "lachs", "thunfisch", "kabeljau", "forelle", "garnele",
		"krabbe", "muschel", "calamari", "sardelle", "anchovis", "hering",
		"matjes", "makrele", "zander", "scampi", "kaviar", "worcester",
		"dorade", "hummer", "pangasius", "barsch", "surimi",
		"fish-", "salmon", "tuna", "=cod", "shrimp", "prawn", "crab-",
		"lobster", "mussel", "squid", "anchov-", "sardine", "mackerel",
		"trout", "scallop", "octopus", "oyster", "=clams",
	}},
	{Vegetarian, []string{
		"milch", "sahne", "rahm", "sauerrahm", "schmand", "fraiche", "quark",
		"joghurt", "jogurt", "käse", "parmesan", "mozzarella", "ricotta",
		"mascarpone", "gouda", "emmentaler", "halloumi", "=skyr", "kefir",
		"paneer", "butter", "=ei", "eier-", "eigelb", "eiweiß", "honig", "ghee",
		"mayonnaise", "pesto", "croissant", "schokolade", "molke",
		"milk", "cream", "cheese", "yogurt", "yoghurt", "=egg", "=eggs", "yolk",
		"honey", "whey", "cheddar", "=feta", "=brie",
	}},
}

// plant keywords make a food vegan when no animal keyword matches.
var plant = []string{
	// German produce.
	"gemüse", "obst", "zwiebel", "schalotte", "knoblauch", "kartoffel",
	"karotte", "möhre", "paprika", "tomate", "gurke", "zucchini",
	"aubergine", "brokkoli", "kohl", "spinat", "pilz", "champignon", "lauch",
	"sellerie", "fenchel", "spargel", "kürbis", "salat", "rucola", "avocado",
	"ingwer", "petersilie", "basilikum", "koriander", "schnittlauch",
	"kräuter", "thymian", "rosmarin", "minze", "chili", "peperoni", "apfel",
	"äpfel", "banane", "zitrone", "limette", "orange", "beere", "kirsch-",
	"traube", "birne", "mango", "ananas", "melone", "radieschen", "kohlrabi",
	"mais",
	// German pulses, grains, nuts and pantry.
	"bohnen", "erbsen", "linsen", "=reis", "basmatireis", "nudel", "pasta",
	"spaghetti", "penne", "mehl", "zucker", "hafer-", "quinoa", "couscous",
	"bulgur", "polenta", "grieß", "nuss", "nüsse", "mandel", "kerne", "sesam",
	"kokos-", "tofu", "tempeh", "seitan", "soja-", "öl", "essig",
	"salz", "pfeffer", "gewürz", "kümmel", "zimt", "kurkuma", "curry", "senf",
	"tomatenmark", "passata", "oliven", "kapern", "brot", "brötchen",
	"tortilla", "hefe", "sirup", "datteln", "rosinen", "saft", "wasser",
	"wein", "leinsamen",
	// English.
	"vegetable", "fruit", "onion", "shallot", "scallion", "garlic", "potato",
	"carrot", "=pepper", "=peppers", "tomato", "cucumber", "eggplant",
	"broccoli", "cauliflower", "cabbage", "spinach", "mushroom", "=leek",
	"celery", "squash", "pumpkin", "lettuce", "avocado", "ginger", "parsley",
	"=basil", "cilantro", "=mint", "apple", "banana", "lemon", "=lime",
	"berry", "berries", "beans", "chickpea", "lentil", "=peas", "=rice",
	"noodle", "flour", "sugar", "=oats", "almond", "peanut", "=nuts", "walnut",
	"=soy", "=oil", "vinegar", "=salt", "cumin", "turmeric", "cinnamon",
	"mustard", "bread", "sourdough", "yeast", "syrup", "juice", "water",
	"wine", "=corn",
}

var fold = strings.NewReplacer(
	"ä", "ae", "ö", "oe", "ü", "ue", "ß", "ss",
	"é", "e", "è", "e", "ê", "e", "à", "a", "â", "a",
	"î", "i", "ï", "i", "ô", "o", "û", "u", "ç", "c", "ñ", "n",
)

// words lowercases and folds s and splits it into letter runs.
func words(s string) []string {
	return strings.FieldsFunc(fold.Replace(strings.ToLower(s)), func(r rune) bool { return !unicode.IsLetter(r) })
}

func foldKeywords(kws []string) []string {
	out := make([]string, len(kws))
	for i, kw := range kws {
		out[i] = fold.Replace(kw)
	}
	return out
}

func init() {
	for _, rs := range [][]rule{markers, animal} {
		for i := range rs {
			rs[i].keywords = foldKeywords(rs[i].keywords)
		}
	}
	plant = foldKeywords(plant)
	for i := range phrases {
		phrases[i].phrase = phraseNeedle(fold.Replace(phrases[i].phrase))
	}
}

// phraseNeedle turns a phrase into the text to find in the space-padded,
// space-joined words of a name.
func phraseNeedle(phrase string) string {
	if whole, ok := strings.CutPrefix(phrase, "="); ok {
		return " " + whole + " "
	}
	if start, ok := strings.CutSuffix(phrase, "-"); ok {
		return " " + start
	}
	return phrase
}

// Classify derives a diet class from a food name, or Unknown.
func Classify(name string) Class {
	rest := " " + strings.Join(words(name), " ") + " "
	var found []Class
	for _, p := range phrases {
		if strings.Contains(rest, p.phrase) {
			found = append(found, p.class)
			rest = strings.ReplaceAll(rest, p.phrase, " ")
		}
	}
	ws := strings.Fields(rest)
	for _, m := range markers {
		if anyMatch(ws, m.keywords) {
			return Max(append(found, m.class)...)
		}
	}
	for _, r := range animal {
		if anyMatch(ws, r.keywords) {
			found = append(found, r.class)
		}
	}
	if len(found) > 0 {
		return Max(found...)
	}
	if anyMatch(ws, plant) {
		return Vegan
	}
	return Unknown
}

func anyMatch(words, keywords []string) bool {
	for _, w := range words {
		for _, kw := range keywords {
			if match(w, kw) {
				return true
			}
		}
	}
	return false
}

func match(word, kw string) bool {
	if exact, ok := strings.CutPrefix(kw, "="); ok {
		return word == exact
	}
	stem, prefix := strings.CutSuffix(kw, "-")
	switch {
	case strings.HasSuffix(word, stem):
		return true
	case prefix:
		return strings.HasPrefix(word, stem)
	default:
		return utf8.RuneCountInString(stem) >= 5 && strings.Contains(word, stem)
	}
}
