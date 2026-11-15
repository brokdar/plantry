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
		{"1 Peperoni", 1, "piece", 45, "Peperoni"},
		{"1 Stk Fantasiefrucht", 1, "piece", 120, "Fantasiefrucht"},
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
