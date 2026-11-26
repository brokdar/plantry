package tips

import (
	"slices"
	"testing"

	"github.com/jaltszeimer/plantry/backend/internal/domain/flavor"
	"github.com/jaltszeimer/plantry/backend/internal/domain/platefit"
)

// garnishLibrary holds one garnish of each kind the table names.
var garnishLibrary = []string{
	"Chives", "Parsley", "Coriander (fresh)", "Basil", "Mint", "Dill", "Chervil", "Tarragon", "Spring onion",
	"Lemon wedges", "Lime wedges", "Natural yogurt", "Roasted peanuts", "Sesame seeds", "Sour cream", "Pickled red onions", "Walnuts",
}

// offered is the library garnishes the main takes, best first.
func offered(main platefit.Component, refuses bool) []string {
	prefs := classicGarnish(main, refuses)
	var out []string
	for _, n := range garnishLibrary {
		if garnishRank(prefs, platefit.Component{Name: n}) >= 0 {
			out = append(out, n)
		}
	}
	slices.SortStableFunc(out, func(a, b string) int {
		return garnishRank(prefs, platefit.Component{Name: a}) - garnishRank(prefs, platefit.Component{Name: b})
	})
	return out
}

type tableCase struct {
	name, main string
	attrs      flavor.Attributes
	want       []string // best first; nil: no garnish
}

func checkTable(t *testing.T, refuses bool, tests []tableCase) {
	t.Helper()
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := offered(platefit.Component{Name: tc.main, Attrs: tc.attrs}, refuses); !slices.Equal(got, tc.want) {
				t.Errorf("garnishes %v, want %v", got, tc.want)
			}
		})
	}
}

// TestClassicGarnish_Eggs: eggs and omelettes take chives or parsley
// (blind judges: chives on a cheese omelette was real lift), shakshuka
// parsley first.
func TestClassicGarnish_Eggs(t *testing.T) {
	t.Parallel()
	checkTable(t, false, []tableCase{
		{"cheese omelette", "Cheese omelette", flavor.Attributes{Cuisine: "neutral", Kind: "egg", Technique: "fry"}, []string{"Chives", "Parsley"}},
		{"shakshuka", "Shakshuka", flavor.Attributes{Cuisine: "mediterranean", Kind: "egg", Technique: "simmer"}, []string{"Parsley", "Chives"}},
	})
}

// TestClassicGarnish_Fish: pan-fried or baked fish takes dill, parsley or
// lemon; fish in a curry follows the curry, fish in a pasta the pasta.
func TestClassicGarnish_Fish(t *testing.T) {
	t.Parallel()
	checkTable(t, false, []tableCase{
		{"baked cod", "Baked cod with lemon potatoes", flavor.Attributes{Cuisine: "neutral", Kind: "fish", Technique: "bake"}, []string{"Dill", "Parsley", "Lemon wedges"}},
		{"pan prawns", "Garnelen-Zucchini-Pfanne", flavor.Attributes{Cuisine: "mediterranean", Kind: "pan", Technique: "saute"}, []string{"Dill", "Parsley", "Lemon wedges"}},
		{"prawn curry", "Prawn coconut curry", flavor.Attributes{Cuisine: "indian", Kind: "curry"}, []string{"Coriander (fresh)", "Lime wedges", "Natural yogurt"}},
		{"tuna pasta", "Tuna pasta with tomatoes", flavor.Attributes{Cuisine: "italian", Kind: "pasta", Technique: "simmer"}, []string{"Basil"}},
	})
}

// TestClassicGarnish_Contrast: curries, stir-fries and Mexican dishes take
// their cuisine's herb and citrus, cool or crunch.
func TestClassicGarnish_Contrast(t *testing.T) {
	t.Parallel()
	checkTable(t, false, []tableCase{
		{"indian curry", "Easy chicken curry", flavor.Attributes{Cuisine: "indian", Kind: "curry"}, []string{"Coriander (fresh)", "Lime wedges", "Natural yogurt"}},
		{"thai curry", "Green curry", flavor.Attributes{Cuisine: "thai", Kind: "curry"}, []string{"Coriander (fresh)", "Lime wedges", "Roasted peanuts"}},
		{"stir-fry", "Tofu stir-fry", flavor.Attributes{Cuisine: "chinese", Kind: "pan", Technique: "fry"}, []string{"Spring onion", "Coriander (fresh)", "Roasted peanuts", "Sesame seeds"}},
		{"chili", "Chili con carne", flavor.Attributes{Cuisine: "mexican", Kind: "stew"}, []string{"Coriander (fresh)", "Lime wedges", "Sour cream", "Pickled red onions"}},
	})
}

// TestClassicGarnish_ContrastRefusingHousehold: a household that refuses food
// gets no coriander or lime (blind judges: coriander on a curry for a picky
// child was noise); the mild garnishes stay.
func TestClassicGarnish_ContrastRefusingHousehold(t *testing.T) {
	t.Parallel()
	checkTable(t, true, []tableCase{
		{"indian curry", "Easy chicken curry", flavor.Attributes{Cuisine: "indian", Kind: "curry"}, []string{"Natural yogurt"}},
		{"chili", "Chili con carne", flavor.Attributes{Cuisine: "mexican", Kind: "stew"}, []string{"Sour cream", "Pickled red onions"}},
		{"omelette unchanged", "Cheese omelette", flavor.Attributes{Kind: "egg"}, []string{"Chives", "Parsley"}},
	})
}

// TestClassicGarnish_SoupsStews: only a clear broth or a potato soup takes
// parsley or chives; other soups and stews none.
func TestClassicGarnish_SoupsStews(t *testing.T) {
	t.Parallel()
	checkTable(t, false, []tableCase{
		{"clear broth", "Klare Rinderbrühe", flavor.Attributes{Cuisine: "german", Kind: "soup"}, []string{"Parsley", "Chives"}},
		{"potato soup", "Kartoffelsuppe", flavor.Attributes{Cuisine: "german", Kind: "soup"}, []string{"Parsley", "Chives"}},
		{"bean soup", "Serbische Bohnensuppe", flavor.Attributes{Cuisine: "german", Kind: "soup", Heroes: []string{"Butter beans", "Smoked sausage", "Beef broth"}}, nil},
		{"cabbage stew", "Grandma's cabbage stew", flavor.Attributes{Cuisine: "german", Kind: "stew", Heroes: []string{"Savoy cabbage", "Potatoes"}}, nil},
		{"goulash", "Beef goulash", flavor.Attributes{Cuisine: "german", Kind: "stew", Technique: "braise"}, nil},
	})
}

// TestClassicGarnish_SaladsBowls: salads, bowls and wraps take their
// cuisine's herbs, lemon or nuts.
func TestClassicGarnish_SaladsBowls(t *testing.T) {
	t.Parallel()
	checkTable(t, false, []tableCase{
		{"greek salad", "Greek salad with chickpeas", flavor.Attributes{Cuisine: "mediterranean", Kind: "salad", Technique: "raw"}, []string{"Parsley", "Basil", "Lemon wedges", "Roasted peanuts", "Sesame seeds", "Walnuts"}},
		{"neutral bowl", "Grain bowl", flavor.Attributes{Cuisine: "neutral", Kind: "bowl"}, []string{"Chives", "Parsley", "Lemon wedges", "Roasted peanuts", "Sesame seeds", "Walnuts"}},
		{"fruit salad", "Fruit salad", flavor.Attributes{Cuisine: "neutral", Kind: "salad"}, nil},
	})
}

// TestClassicGarnish_NoGarnish: bakes, gratins, lasagne, roasts, braises and
// mains outside the table take none (blind judges: parsley on lasagne and
// chives on a roast plate were noise).
func TestClassicGarnish_NoGarnish(t *testing.T) {
	t.Parallel()
	checkTable(t, false, []tableCase{
		{"lasagne", "Lasagne bolognese", flavor.Attributes{Cuisine: "italian", Kind: "pasta", Technique: "bake", Format: "casserole"}, nil},
		{"spinach lasagne", "Spinach lasagne", flavor.Attributes{Cuisine: "italian", Kind: "bake"}, nil},
		{"gratin", "Kartoffelgratin", flavor.Attributes{Cuisine: "french", Kind: "bake", Format: "casserole"}, nil},
		{"roast chicken", "Roast chicken legs with sweet potatoes", flavor.Attributes{Cuisine: "neutral", Kind: "roast", Technique: "roast"}, nil},
		{"traybake", "Chicken and vegetable traybake", flavor.Attributes{Kind: "roast"}, nil},
		{"indian bake", "Tandoori bake", flavor.Attributes{Cuisine: "indian", Kind: "bake", Technique: "bake"}, nil},
		{"coq au vin", "Coq au Vin", flavor.Attributes{Cuisine: "french", Kind: "stew", Technique: "braise"}, nil},
		{"risotto", "Pilzrisotto", flavor.Attributes{Cuisine: "italian", Kind: "grain"}, nil},
		{"unknown kind", "Omelett", flavor.Attributes{}, nil},
	})
}

// TestClassicGarnish_PastaAndCutlet: a tomato pasta from the stove takes
// basil, a crisp fried cutlet lemon, another German pan dish parsley, chives
// or lemon; other pasta and pan dishes none.
func TestClassicGarnish_PastaAndCutlet(t *testing.T) {
	t.Parallel()
	checkTable(t, false, []tableCase{
		{"tomato pasta", "Spaghetti al pomodoro", flavor.Attributes{Cuisine: "italian", Kind: "pasta", Technique: "simmer", Heroes: []string{"Tomato"}}, []string{"Basil"}},
		{"carbonara", "Spaghetti Carbonara", flavor.Attributes{Cuisine: "italian", Kind: "pasta", Technique: "fry"}, nil},
		{"schnitzel", "Pork schnitzel", flavor.Attributes{Cuisine: "german", Kind: "pan", Technique: "fry", Texture: "crunchy"}, []string{"Lemon wedges"}},
		{"german pan dish", "Frikadellen", flavor.Attributes{Cuisine: "german", Kind: "pan", Technique: "fry", Texture: "soft"}, []string{"Parsley", "Chives", "Lemon wedges"}},
		{"neutral pancakes", "Easy vegan pancakes", flavor.Attributes{Cuisine: "neutral", Kind: "pan", Technique: "fry", Texture: "soft"}, nil},
	})
}
