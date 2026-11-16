package importer

import (
	"math"
	"strings"
	"testing"
)

// Ingredient lines taken from real pages (gutekueche.at, emmikochteinfach.de,
// lecker.de, bbcgoodfood.com) that the parser got wrong: a garlic clove
// weighed 120 g, cups and splashes became pieces and "1 x 175ml glass" lost its
// volume. Each row pins the amount, unit and the weight estimate the review
// step shows.
func TestParseLine_RealWorld(t *testing.T) {
	cases := []struct {
		raw         string
		amount      float64
		unit        string
		portion     float64 // 0: no estimate (mass, volume, unparsed)
		nameContain string
	}{
		// Garlic is a clove count, in German and English, with or without "Stk".
		{"2 Stk Knoblauchzehen", 2, "clove", 3, "Knoblauch"},
		{"2 Knoblauchzehen", 2, "clove", 3, "Knoblauch"},
		{"3 garlic cloves finely chopped", 3, "clove", 3, "garlic"},
		{"1 garlic clove sliced", 1, "clove", 3, "garlic"},
		{"1 Stk Knoblauch", 1, "piece", 3, "Knoblauch"},
		// Pieces weigh what the ingredient weighs, not a flat 120 g.
		{"1 Stk Zwiebel", 1, "piece", 110, "Zwiebel"},
		{"2 red onions chopped", 2, "piece", 110, "onions"},
		{"6 Stk Eier (Größe M)", 6, "piece", 50, "Eier"},
		{"3 Eier (Größe M oder L)", 3, "piece", 50, "Eier"},
		{"3 Stk Süßkartoffeln", 3, "piece", 130, "Süßkartoffeln"},
		{"2 Stk Karotten", 2, "piece", 61, "Karotten"},
		{"5 Stk Cocktailtomaten", 5, "piece", 17, "Cocktailtomaten"},
		{"1 Stk Lorbeerblatt", 1, "piece", 0.6, "Lorbeerblatt"},
		{"1 Stk Brokkoli", 1, "piece", 500, "Brokkoli"},
		{"1 Peperoni", 1, "piece", 15, "Peperoni"},
		// Cups and splashes keep their volume.
		{"1 Tasse Reis", 240, "ml", 0, "Reis"},
		{"2 Tasse klare Suppe", 480, "ml", 0, "klare Suppe"},
		{"1 Schuss Öl", 15, "ml", 0, "Öl"},
		// "N x size container name" multiplies and drops the container word.
		{"1 x 175ml glass white wine", 175, "ml", 0, "white wine"},
		{"2 x 400g can chopped tomatoes", 800, "g", 0, "chopped tomatoes"},
		{"250g pack chestnut mushrooms chopped", 250, "g", 0, "chestnut mushrooms"},
	}
	for _, tc := range cases {
		t.Run(tc.raw, func(t *testing.T) {
			got := ParseLineDE(tc.raw)
			if math.Abs(got.Amount-tc.amount) > 1e-9 || got.Unit != tc.unit {
				t.Fatalf("%q: got %v %s, want %v %s", tc.raw, got.Amount, got.Unit, tc.amount, tc.unit)
			}
			if tc.portion == 0 {
				if got.PortionGrams != nil {
					t.Errorf("%q: portion = %v, want nil", tc.raw, *got.PortionGrams)
				}
			} else if got.PortionGrams == nil || math.Abs(*got.PortionGrams-tc.portion) > 1e-9 {
				t.Errorf("%q: portion = %v, want %v", tc.raw, got.PortionGrams, tc.portion)
			}
			if !strings.Contains(got.Name, tc.nameContain) {
				t.Errorf("%q: name = %q, want it to contain %q", tc.raw, got.Name, tc.nameContain)
			}
			if got.Confidence == ConfidenceUnparsed {
				t.Errorf("%q: confidence = unparsed", tc.raw)
			}
		})
	}
}

// Findings F1-F5 of the seeding run: lines the parser turned into 780 g of
// thyme, lost as amount 0, priced at the wrong weight or left with prep text
// in the name. needsWeight: no estimate exists, so the review must ask.
func TestParseLine_SeedFindings(t *testing.T) {
	cases := []struct {
		raw         string
		amount      float64
		unit        string
		portion     float64
		needsWeight bool
		name        string
		noteHas     string
		warning     string
	}{
		// F1: small units weigh what they weigh.
		{raw: "6-7 Stiel(e) Zitronenthymian", amount: 6.5, unit: "sprig", portion: 1, name: "Zitronenthymian"},
		{raw: "2 Spritzer Limettensaft", amount: 10, unit: "ml", name: "Limettensaft"},
		{raw: "1 Eigelb (Größe M)", amount: 1, unit: "piece", portion: 18, name: "Eigelb"},
		{raw: "12 Ciabatta-Brot", amount: 12, unit: "piece", portion: 30, name: "Ciabatta-Brot"},
		{raw: "2 Stg Staudensellerie", amount: 2, unit: "stalk", portion: 40, name: "Staudensellerie"},
		{raw: "1 Chili-Schote", amount: 1, unit: "piece", portion: 15, name: "Chili-Schote"},
		{raw: "2 dashes Tabasco", amount: 2, unit: "pinch", portion: 0.5, name: "Tabasco"},
		{raw: "1 small bunch of coriander leaves chopped", amount: 1, unit: "bunch", portion: 30, name: "coriander leaves", noteHas: "chopped"},
		{raw: "1 Stk Fantasiefrucht", amount: 1, unit: "piece", needsWeight: true, name: "Fantasiefrucht"},
		{raw: "1 piece of ginger peeled and chopped", amount: 1, unit: "piece", needsWeight: true, name: "ginger", noteHas: "peeled and chopped"},
		// F2: the metric value of a dual-unit line.
		{raw: "300g/12oz self-raising flour", amount: 300, unit: "g", name: "self-raising flour"},
		{raw: "400ml/14fl oz plant-based milk (oat, almond or soya)", amount: 400, unit: "ml", name: "plant-based milk"},
		{raw: "12oz/340g spaghetti", amount: 340, unit: "g", name: "spaghetti"},
		// F3: the weight the line states beats the table; mg for a liquid warns.
		{raw: "1 große Zwiebel (ca. 200 g)", amount: 1, unit: "piece", portion: 200, name: "Zwiebel"},
		{raw: "3 Karotten (ca. 120 g)", amount: 3, unit: "piece", portion: 40, name: "Karotten"},
		{raw: "2 Zwiebeln (je 100 g)", amount: 2, unit: "piece", portion: 100, name: "Zwiebeln"},
		{raw: "300 mg Gemüsesuppe", amount: 0.3, unit: "g", name: "Gemüsesuppe", warning: WarningSuspiciousUnit},
		// F5: prep text and size words are not part of the name.
		{raw: "onion roughly chopped", amount: 0, unit: "", name: "onion", noteHas: "roughly chopped"},
		{raw: "1 onion roughly chopped", amount: 1, unit: "piece", portion: 110, name: "onion", noteHas: "roughly chopped"},
		{raw: "light coconut or natural yogurt to serve", amount: 0, unit: "", name: "light coconut or natural yogurt", noteHas: "to serve"},
		{raw: "500 g medium-sized floury potatoes", amount: 500, unit: "g", name: "floury potatoes"},
	}
	for _, tc := range cases {
		t.Run(tc.raw, func(t *testing.T) {
			got := ParseLineDE(tc.raw)
			if tc.amount > 0 && (math.Abs(got.Amount-tc.amount) > 1e-9 || got.Unit != tc.unit) {
				t.Fatalf("got %v %s, want %v %s", got.Amount, got.Unit, tc.amount, tc.unit)
			}
			if got.Name != tc.name {
				t.Errorf("name = %q, want %q", got.Name, tc.name)
			}
			if !strings.Contains(got.Note, tc.noteHas) {
				t.Errorf("note = %q, want it to contain %q", got.Note, tc.noteHas)
			}
			if got.NeedsWeight != tc.needsWeight {
				t.Errorf("needs_weight = %v, want %v", got.NeedsWeight, tc.needsWeight)
			}
			switch {
			case tc.portion == 0 && got.PortionGrams != nil:
				t.Errorf("portion = %v, want nil", *got.PortionGrams)
			case tc.portion != 0 && (got.PortionGrams == nil || math.Abs(*got.PortionGrams-tc.portion) > 1e-9):
				t.Errorf("portion = %v, want %v", got.PortionGrams, tc.portion)
			}
			if got.Warning != tc.warning {
				t.Errorf("warning = %q, want %q", got.Warning, tc.warning)
			}
			if tc.amount > 0 && got.Confidence == ConfidenceUnparsed {
				t.Errorf("confidence = unparsed")
			}
		})
	}
}
