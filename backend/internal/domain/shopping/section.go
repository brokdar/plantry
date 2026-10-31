package shopping

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
)

// SectionOf returns a leaf's effective store section (D18): its override, or
// the section classified from its name.
func SectionOf(f *food.Food) food.ShopSection {
	if f.ShopSection != nil {
		return *f.ShopSection
	}
	return Classify(f.Name)
}

// classifier is one section's keywords. The table is checked in this order,
// so more specific sections win: "Kidneybohnen (Dose)" is a can before it is
// anything else, "Zwiebelpulver" a spice before it is an onion.
//
// A keyword matches a word of the name when the word equals it or ends with
// it (German compounds put the head noun last: "Vollkornbrot" ends in
// "brot"). Keywords of five or more letters also match anywhere inside a
// word ("Hähnchenbrust"). A keyword starting with "=" matches whole words
// only, for short words that would hit too much ("ei" ends "Salbei").
var classifier = []struct {
	section  food.ShopSection
	keywords []string
}{
	{food.SectionFrozen, []string{
		"tiefkühl", "tiefgekühlt", "gefroren", "=tk", "frozen",
	}},
	{food.SectionCansJars, []string{
		"=dose", "dosen", "konserve", "=glas", "passata", "tomatenmark",
		"=can", "=canned", "=tin", "=tinned", "=jar",
	}},
	{food.SectionSpicesOils, []string{
		"öl", "essig", "salz", "pfeffer", "pulver", "gewürz", "zimt", "kümmel",
		"curry", "oregano", "muskat", "vanille", "senf", "brühe",
		"=oil", "vinegar", "=salt", "peppercorn", "powder", "spice", "cinnamon",
		"cumin", "nutmeg", "vanilla", "mustard", "broth", "=stock",
	}},
	{food.SectionDairyEggs, []string{
		"milch", "käse", "joghurt", "jogurt", "quark", "sahne", "butter", "=ei",
		"eier", "schmand", "mozzarella", "parmesan", "feta", "ricotta", "mascarpone",
		"milk", "cheese", "yogurt", "yoghurt", "cream", "=egg", "=eggs",
	}},
	{food.SectionMeatFish, []string{
		"fleisch", "hähnchen", "hühnchen", "huhn", "pute", "rind", "schwein",
		"hack", "speck", "schinken", "wurst", "lachs", "fisch", "garnele", "lamm",
		"kabeljau",
		"meat", "chicken", "turkey", "beef", "pork", "mince", "bacon", "=ham",
		"sausage", "salmon", "tuna", "fish", "shrimp", "prawn", "lamb", "=cod",
	}},
	{food.SectionBakery, []string{
		"brot", "brötchen", "toast", "baguette", "croissant", "tortilla", "brezel",
		"bread", "=bun", "=buns", "=pita", "=wrap", "=wraps",
	}},
	{food.SectionDryGoods, []string{
		"reis", "basmati", "nudel", "pasta", "spaghetti", "penne", "mehl",
		"zucker", "linsen", "hafer", "haferflocken", "quinoa", "couscous", "bulgur",
		"nuss", "nüsse", "mandel", "kichererbsen", "honig", "hefe", "müsli",
		"rosinen", "schokolade", "kakao", "polenta", "grieß",
		"=rice", "flour", "sugar", "lentil", "lentils", "=oats", "almond", "almonds",
		"chickpeas", "honey", "yeast", "granola", "raisins", "chocolate", "cocoa",
		"noodle", "noodles",
	}},
	{food.SectionProduce, []string{
		"zwiebel", "knoblauch", "tomate", "tomaten", "kartoffel", "karotte", "möhre",
		"möhren", "paprika", "salat", "gurke", "zucchini", "apfel", "äpfel",
		"banane", "zitrone", "limette", "orange", "beere", "beeren", "spinat",
		"brokkoli", "kohl", "lauch", "sellerie", "pilz", "pilze", "champignon",
		"avocado", "ingwer", "petersilie", "basilikum", "koriander", "schnittlauch",
		"kräuter", "salbei", "thymian", "rosmarin", "minze", "=dill", "obst",
		"gemüse", "birne", "aubergine", "kürbis", "erbsen", "chili",
		"onion", "onions", "garlic", "tomato", "tomatoes", "potato", "potatoes",
		"carrot", "carrots", "lettuce", "cucumber", "apple", "apples", "banana",
		"lemon", "=lime", "berry", "berries", "spinach", "broccoli", "cabbage",
		"=leek", "celery", "mushroom", "mushrooms", "ginger", "parsley", "=basil",
		"cilantro", "chives", "herbs", "=sage", "thyme", "rosemary", "=mint",
		"fruit", "vegetable", "vegetables", "=pear", "eggplant", "pumpkin",
		"=peas",
	}},
}

// Classify derives a store section from a food name with a deterministic
// en/de keyword table, falling back to other.
func Classify(name string) food.ShopSection {
	words := strings.FieldsFunc(strings.ToLower(name), func(r rune) bool { return !unicode.IsLetter(r) })
	for _, c := range classifier {
		for _, kw := range c.keywords {
			for _, w := range words {
				if matches(w, kw) {
					return c.section
				}
			}
		}
	}
	return food.SectionOther
}

func matches(word, kw string) bool {
	if exact, ok := strings.CutPrefix(kw, "="); ok {
		return word == exact
	}
	if strings.HasSuffix(word, kw) {
		return true
	}
	return utf8.RuneCountInString(kw) >= 5 && strings.Contains(word, kw)
}
