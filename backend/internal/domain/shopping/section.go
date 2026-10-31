package shopping

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
)

// SectionOf returns a leaf's effective store section (D18): its override,
// or the section classified from its name.
func SectionOf(f *food.Food) food.ShopSection {
	if f.ShopSection != nil {
		return *f.ShopSection
	}
	return Classify(f.Name)
}

// rule lists one section's keywords. A keyword matches a word of the name
// when:
//
//   - "=word": the word is exactly it
//   - "stem-": the word starts or ends with it ("pute-" in "Putenbrust")
//   - otherwise: the word ends with it ("brot" in "Vollkornbrot"), or, for
//     keywords of five or more letters, contains it ("hähnchen" in
//     "Hähnchenbrust")
type rule struct {
	section  food.ShopSection
	keywords []string
}

// markers are packaging and state words. They decide the section wherever
// they stand: "Erbsen (TK)" is frozen, "Kidney beans (canned)" a can.
var markers = []rule{
	{food.SectionFrozen, []string{"tiefkühl", "tiefgekühlt", "gefroren", "=tk", "=frozen"}},
	{food.SectionCansJars, []string{"=dose", "dosen-", "konserve", "=glas", "=can", "=canned", "=tin", "=tinned", "=jar"}},
}

// phrases are names a word rule gets wrong, matched anywhere in the
// lowercased name (words joined by single spaces), in this order.
var phrases = []struct {
	phrase  string
	section food.ShopSection
}{
	{"nussbutter", food.SectionDryGoods},
	{"studentenfutter", food.SectionDryGoods},
	{"kaffeebohnen", food.SectionDryGoods},
	{"cornflakes", food.SectionDryGoods},
	{"corn flakes", food.SectionDryGoods},
	{"granatapfel", food.SectionProduce},
	{"peanut butter", food.SectionDryGoods},
	{"almond butter", food.SectionDryGoods},
	{"kakao", food.SectionDryGoods},
	{"backpulver", food.SectionDryGoods},
	{"baking powder", food.SectionDryGoods},
	{"baking soda", food.SectionDryGoods},
	{"kokosmilch", food.SectionCansJars},
	{"coconut milk", food.SectionCansJars},
	{"gehackte tomaten", food.SectionCansJars},
	{"passierte tomaten", food.SectionCansJars},
	{"stückige tomaten", food.SectionCansJars},
	{"pizzatomaten", food.SectionCansJars},
	{"tomato paste", food.SectionCansJars},
	{"diced tomatoes", food.SectionCansJars},
	{"crushed tomatoes", food.SectionCansJars},
	{"essiggurke", food.SectionCansJars},
	{"gewürzgurke", food.SectionCansJars},
	{"chiliflocken", food.SectionSpicesOils},
	{"chili flakes", food.SectionSpicesOils},
	{"pepper flakes", food.SectionSpicesOils},
	{"fischsauce", food.SectionSpicesOils},
	{"fish sauce", food.SectionSpicesOils},
	{"soy sauce", food.SectionSpicesOils},
	{"ice cream", food.SectionFrozen},
	{"speiseeis", food.SectionFrozen},
	{"grüne bohnen", food.SectionProduce},
	{"brechbohnen", food.SectionProduce},
	{"green beans", food.SectionProduce},
	{"peppercorn", food.SectionSpicesOils},
	{"bell pepper", food.SectionProduce},
	{"red pepper", food.SectionProduce},
	{"green pepper", food.SectionProduce},
	{"yellow pepper", food.SectionProduce},
	{"rote bete", food.SectionProduce},
	{"crème fraîche", food.SectionDairyEggs},
	{"creme fraiche", food.SectionDairyEggs},
	{"leberkäse", food.SectionMeatFish},
	{"fleischkäse", food.SectionMeatFish},
	{"tomatensauce", food.SectionCansJars},
	{"tomato sauce", food.SectionCansJars},
	{"coconut cream", food.SectionCansJars},
	{"kokoscreme", food.SectionCansJars},
	{"apfelmus", food.SectionCansJars},
	{"cocoa", food.SectionDryGoods},
	{"smoked paprika", food.SectionSpicesOils},
	{"paprika edelsüß", food.SectionSpicesOils},
	{"paprika rosenscharf", food.SectionSpicesOils},
	{"rosenpaprika", food.SectionSpicesOils},
	{"ground ginger", food.SectionSpicesOils},
	{"bay leaves", food.SectionSpicesOils},
	{"pumpkin seeds", food.SectionDryGoods},
}

// sections is the head-noun table. Names are decided by their head noun,
// which comes last in German compounds and English phrases: the match in
// the last matching word that ends rightmost wins ("Fleischtomaten" is a
// tomato), and on a tie the earlier section in this table wins
// ("Muskatnuss" is a spice).
var sections = []rule{
	{food.SectionFrozen, []string{
		"=eis", "fischstäbchen", "pommes",
	}},
	{food.SectionCansJars, []string{
		"bohnen", "beans", "kichererbsen", "chickpeas", "passata", "tomatenmark",
		"mais", "=corn", "sweetcorn", "oliven", "olives", "=olive", "kapern", "capers",
		"sauerkraut", "pickles", "thunfisch", "=tuna", "pesto", "tahini",
		"marmelade", "konfitüre", "=jam", "salsa", //nolint:misspell // German "Marmelade"
	}},
	{food.SectionSpicesOils, []string{
		"öl", "essig", "balsamico", "salz", "pfeffer", "pulver", "gewürz", "zimt", "zimtstange",
		"kümmel", "kurkuma", "lorbeer", "nelken", "kardamom", "anis", "curry",
		"oregano", "muskat", "muskatnuss", "vanille", "vanilleschote", "senf",
		"brühe", "brühwürfel", "fond", "sojasauce", "worcester", "ketchup",
		"mayonnaise", "=mayo", "sauce", "soße", "sambal", "sriracha", "tabasco", "masala", "ghee",
		"=oil", "vinegar", "=salt", "=pepper", "peppercorn", "cayenne", "powder",
		"spice", "cinnamon", "cumin", "turmeric", "nutmeg", "vanilla", "mustard",
		"broth", "=stock", "bouillon", "paprika edelsüß",
	}},
	{food.SectionDairyEggs, []string{
		"milch", "käse", "joghurt", "jogurt", "quark", "sahne", "rahm", "butter",
		"=ei", "eier", "hühnerei", "eigelb", "eiweiß", "schmand", "mozzarella",
		"parmesan", "feta", "ricotta", "mascarpone", "skyr", "halloumi", "gouda",
		"emmentaler", "cheddar", "margarine", "kefir",
		"milk", "cheese", "yogurt", "yoghurt", "cream", "=egg", "=eggs", "yolk",
	}},
	{food.SectionMeatFish, []string{
		"fleisch", "hähnchen", "hühnchen", "huhn", "hühner-", "pute-", "rind-",
		"schwein", "kalb-", "lamm-", "=ente", "enten-", "hack", "speck", "schinken", "wurst",
		"würstchen", "salami", "chorizo", "prosciutto", "schnitzel", "steak", "filet",
		"lachs", "fisch", "garnele", "forelle", "muscheln", "kabeljau", "sardellen",
		"krabben", "calamari", "anchovies", "squid", "=crab",
		"meat", "chicken", "turkey", "beef", "pork", "mince", "bacon", "=ham",
		"sausage", "pepperoni", "salmon", "fish", "shrimp", "prawn", "lamb", "=cod",
		"fillet", "duck", "thighs",
	}},
	{food.SectionBakery, []string{
		"brot", "brötchen", "semmel", "toast", "baguette", "ciabatta", "croissant",
		"tortilla", "brezel", "=naan", "bread", "sourdough", "=bun", "=buns",
		"=pita", "=wrap", "=wraps", "bagel",
	}},
	{food.SectionDryGoods, []string{
		"reis", "basmati", "nudel", "pasta", "spaghetti", "penne", "lasagne", "mehl",
		"zucker", "linsen", "hafer", "haferflocken", "quinoa", "couscous", "bulgur",
		"polenta", "grieß", "nuss", "nüsse", "mandel", "kerne", "sesam", "cashew",
		"honig", "sirup", "hefe", "müsli", "rosinen", "datteln", "schokolade",
		"stärke", "semmelbrösel", "kokosraspeln", "leinsamen",
		"=rice", "noodle", "noodles", "flour", "sugar", "lentil", "lentils", "=oats",
		"almond", "almonds", "peanut", "peanuts", "=nuts", "walnuts", "pecans",
		"cornstarch", "breadcrumbs", "honey", "syrup", "yeast", "granola",
		"raisins", "chocolate", "=chia", "sunflower", "flaxseed",
	}},
	{food.SectionProduce, []string{
		"zwiebel", "schalotte", "knoblauch", "tomate", "tomaten", "kartoffel",
		"karotte", "möhre", "möhren", "paprika", "salat", "rucola", "gurke",
		"zucchini", "aubergine", "apfel", "äpfel", "banane", "zitrone", "limette",
		"orange", "beere", "beeren", "kirsche", "kirschen", "kirsch-", "trauben",
		"birne", "pfirsich", "pflaume", "mango", "ananas", "melone", "=kiwi",
		"spinat", "brokkoli", "kohl", "kohlrabi", "lauch", "sellerie", "fenchel",
		"spargel", "radieschen", "rettich", "pastinake", "mangold", "pilz", "pilze",
		"champignon", "avocado", "ingwer", "petersilie", "basilikum", "koriander",
		"schnittlauch", "kräuter", "salbei", "thymian", "rosmarin", "minze", "=dill",
		"obst", "gemüse", "kürbis", "erbsen", "chili", "schote", "schoten", "peperoni",
		"onion", "onions", "shallot", "shallots", "scallion", "scallions", "garlic",
		"tomato", "tomatoes", "potato", "potatoes", "carrot", "carrots", "lettuce",
		"arugula", "cucumber", "zucchini", "eggplant", "apple", "apples", "banana",
		"bananas", "lemon", "lemons", "=lime", "=limes", "berry", "berries",
		"spinach", "broccoli", "cabbage", "=kale", "cauliflower", "=leek", "=leeks",
		"celery", "fennel", "asparagus", "radish", "mushroom", "mushrooms", "ginger",
		"parsley", "=basil", "cilantro", "chives", "herbs", "=sage", "thyme",
		"rosemary", "=mint", "fruit", "vegetable", "vegetables", "=pear", "=pears",
		"pumpkin", "=peas", "=peppers", "jalapeño", "jalapeno", "avocados", "grapes",
		"peaches", "melon", "courgette", "courgettes", "sprouts", "squash", "butternut",
		"feige", "feigen", "figs",
	}},
	// Head nouns with no section of their own: "Zitronensaft" is a juice,
	// not a lemon.
	{food.SectionOther, []string{"saft", "juice", "wasser", "water", "wein", "wine"}},
}

// Classify derives a store section from a food name with a deterministic
// en/de keyword table, falling back to other: packaging markers first, then
// the phrase table, then the head noun.
func Classify(name string) food.ShopSection {
	words := strings.FieldsFunc(strings.ToLower(name), func(r rune) bool { return !unicode.IsLetter(r) })
	for _, m := range markers {
		for _, w := range words {
			for _, kw := range m.keywords {
				if matchEnd(w, kw) >= 0 {
					return m.section
				}
			}
		}
	}
	joined := strings.Join(words, " ")
	for _, p := range phrases {
		if strings.Contains(joined, p.phrase) {
			return p.section
		}
	}
	best, bestScore := food.SectionOther, -1
	for i, w := range words {
		for _, r := range sections {
			for _, kw := range r.keywords {
				if end := matchEnd(w, kw); end >= 0 {
					if score := i<<16 + end; score > bestScore {
						best, bestScore = r.section, score
					}
				}
			}
		}
	}
	return best
}

// matchEnd returns where keyword kw ends in word (a byte offset), or -1
// when it does not match.
func matchEnd(word, kw string) int {
	if exact, ok := strings.CutPrefix(kw, "="); ok {
		if word == exact {
			return len(word)
		}
		return -1
	}
	stem, prefix := strings.CutSuffix(kw, "-")
	switch {
	case strings.HasSuffix(word, stem):
		return len(word)
	case prefix && strings.HasPrefix(word, stem):
		return len(stem)
	case !prefix && utf8.RuneCountInString(stem) >= 5:
		if i := strings.LastIndex(word, stem); i >= 0 {
			return i + len(stem)
		}
	}
	return -1
}
