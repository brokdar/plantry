package importer

import (
	"math"
	"testing"

	"github.com/jaltszeimer/plantry/backend/internal/domain/units"
)

func TestParseLineDE(t *testing.T) {
	cases := []struct {
		name         string
		raw          string
		amount       float64
		unit         string
		originalUnit string
		ingrName     string
		note         string
		confidence   string
	}{
		// 1. mass / volume exact
		{"grams", "200 g Spaghetti", 200, "g", "g", "Spaghetti", "", ConfidenceParsed},
		{"kilograms", "1 kg Kartoffeln", 1000, "g", "kg", "Kartoffeln", "", ConfidenceParsed},
		{"milligrams", "50 mg Safran", 0.05, "g", "mg", "Safran", "", ConfidenceParsed},
		{"milliliters", "250 ml Milch", 250, "ml", "ml", "Milch", "", ConfidenceParsed},
		{"liters comma decimal", "0,5 l Sahne", 500, "ml", "l", "Sahne", "", ConfidenceParsed},
		{"centiliters", "2 cl Rum", 20, "ml", "cl", "Rum", "", ConfidenceParsed},

		// 2. spoons / pinch (approximate)
		{"tablespoon", "1 EL Olivenöl", 15, "ml", "EL", "Olivenöl", "", ConfidenceApproximate},
		{"teaspoon", "1 TL Salz", 5, "ml", "TL", "Salz", "", ConfidenceApproximate},
		{"knife tip", "1 Msp Muskat", 1, "pinch", "Msp", "Muskat", "", ConfidenceApproximate},
		{"pinch", "1 Prise Salz", 1, "pinch", "Prise", "Salz", "", ConfidenceApproximate},

		// 3. count units are kept (D19)
		{"clove plural", "2 Zehen Knoblauch", 2, "clove", "Zehen", "Knoblauch", "", ConfidenceApproximate},
		{"bunch", "1 Bund Petersilie", 1, "bunch", "Bund", "Petersilie", "", ConfidenceApproximate},
		{"stalk", "1 Stange Sellerie", 1, "stick", "Stange", "Sellerie", "", ConfidenceApproximate},
		{"piece abbreviated", "1 Stk. Paprika", 1, "piece", "Stk.", "Paprika", "", ConfidenceApproximate},
		{"package", "1 Pck Vanillezucker", 1, "packet", "Pck", "Vanillezucker", "", ConfidenceApproximate},
		{"can", "1 Dose Tomaten", 1, "can", "Dose", "Tomaten", "", ConfidenceApproximate},
		{"jar", "1 Glas Kapern", 1, "jar", "Glas", "Kapern", "", ConfidenceApproximate},

		// 4. fractions
		{"ascii fraction", "1/2 Bund Petersilie", 0.5, "bunch", "Bund", "Petersilie", "", ConfidenceApproximate},
		{"unicode half", "½ TL Zimt", 2.5, "ml", "TL", "Zimt", "", ConfidenceApproximate},
		{"unicode quarter", "¼ l Milch", 250, "ml", "l", "Milch", "", ConfidenceParsed},
		{"mixed ascii", "1 1/2 EL Olivenöl", 22.5, "ml", "EL", "Olivenöl", "", ConfidenceApproximate},
		{"mixed unicode glued", "1½ TL Salz", 7.5, "ml", "TL", "Salz", "", ConfidenceApproximate},

		// 5. ranges (midpoint, approximate)
		{"ascii range", "1-2 EL Zucker", 22.5, "ml", "EL", "Zucker", "", ConfidenceApproximate},
		{"en-dash range", "1–2 EL Zucker", 22.5, "ml", "EL", "Zucker", "", ConfidenceApproximate},

		// 6. qualifiers / approximations
		{"ca. prefix", "ca. 200 g Mehl", 200, "g", "g", "Mehl", "", ConfidenceParsed},
		{"etwa prefix", "etwa 1 TL Salz", 5, "ml", "TL", "Salz", "", ConfidenceApproximate},
		{"etwas", "etwas Pfeffer", 0, "", "", "Pfeffer", "etwas", ConfidenceUnparsed},
		{"nach geschmack", "nach Geschmack Salz", 0, "", "", "Salz", "nach Geschmack", ConfidenceUnparsed},

		// 7. notes (parenthetical + trailing comma)
		{"parenthetical note", "200 g Mehl (Type 405)", 200, "g", "g", "Mehl", "Type 405", ConfidenceParsed},
		{"comma note", "2 Zehen Knoblauch, fein gehackt", 2, "clove", "Zehen", "Knoblauch", "fein gehackt", ConfidenceApproximate},

		// 8. unit-less count → piece
		{"unitless count eggs", "3 Eier", 3, "piece", "Stück", "Eier", "", ConfidenceApproximate},

		// 9. compound-word suffix match: "Knoblauchzehe(n)" → Zehe unit.
		{"knoblauchzehe singular", "1 Knoblauchzehe", 1, "clove", "Zehe", "Knoblauchzehe", "", ConfidenceApproximate},
		{"knoblauchzehen plural", "2 Knoblauchzehen", 2, "clove", "Zehe", "Knoblauchzehen", "", ConfidenceApproximate},

		// 10. glued number + unit parses like the spaced form.
		{"glued grams", "100g Mehl", 100, "g", "g", "Mehl", "", ConfidenceParsed},
		{"glued kilograms comma", "1,5kg Kartoffeln", 1500, "g", "kg", "Kartoffeln", "", ConfidenceParsed},
		{"glued milliliters", "250ml Milch", 250, "ml", "ml", "Milch", "", ConfidenceParsed},
		{"glued spoon", "2EL Olivenöl", 30, "ml", "EL", "Olivenöl", "", ConfidenceApproximate},
		{"glued unit with period", "2Stk. Eier", 2, "piece", "Stk.", "Eier", "", ConfidenceApproximate},
		{"glued after qualifier", "ca. 200g Mehl", 200, "g", "g", "Mehl", "", ConfidenceParsed},

		// 11. plural suffixes are stripped, never notes.
		{"plural (n) on name", "1 Zwiebel(n)", 1, "piece", "Stück", "Zwiebel", "", ConfidenceApproximate},
		{"plural /n on name", "2 Tomate/n", 2, "piece", "Stück", "Tomate", "", ConfidenceApproximate},
		{"plural (s) on name", "2 Steak(s)", 2, "piece", "Stück", "Steak", "", ConfidenceApproximate},
		{"plural /s on name", "2 Steak/s", 2, "piece", "Stück", "Steak", "", ConfidenceApproximate},
		{"plural (e) on name", "200 g Pilz(e)", 200, "g", "g", "Pilz", "", ConfidenceParsed},
		{"plural on unit", "2 Dose/n Tomaten", 2, "can", "Dose", "Tomaten", "", ConfidenceApproximate},
		{"plural plus prose note", "2 Zwiebel(n) (rot), gewürfelt", 2, "piece", "Stück", "Zwiebel", "rot; gewürfelt", ConfidenceApproximate},

		// 12. Zehe forms are a clove.
		{"clove singular", "1 Zehe Knoblauch", 1, "clove", "Zehe", "Knoblauch", "", ConfidenceApproximate},
		{"clove (n)", "2 Zehe(n) Knoblauch", 2, "clove", "Zehe", "Knoblauch", "", ConfidenceApproximate},
		{"clove /n", "2 Zehe/n Knoblauch", 2, "clove", "Zehe", "Knoblauch", "", ConfidenceApproximate},

		// 13. quantity-less lines stay unparsed (the review UI defaults them to skip).
		{"quantity-less salt and pepper", "Salz und Pfeffer", 0, "", "", "Salz und Pfeffer", "", ConfidenceUnparsed},
		{"quantity-less etwas", "etwas Öl", 0, "", "", "Öl", "etwas", ConfidenceUnparsed},

		// 14. units shared with units.Normalize.
		{"tub", "1 Becher Sahne", 1, "tub", "Becher", "Sahne", "", ConfidenceApproximate},
		{"handful", "1 Handvoll Rucola", 1, "handful", "Handvoll", "Rucola", "", ConfidenceApproximate},
		{"deciliter", "2 dl Milch", 200, "ml", "dl", "Milch", "", ConfidenceParsed},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseLineDE(tc.raw)
			if got.RawText != tc.raw {
				t.Errorf("RawText: got %q, want %q", got.RawText, tc.raw)
			}
			if !almostEqual(got.Amount, tc.amount) {
				t.Errorf("Amount: got %v, want %v", got.Amount, tc.amount)
			}
			if got.Unit != tc.unit {
				t.Errorf("Unit: got %q, want %q", got.Unit, tc.unit)
			}
			if got.OriginalUnit != tc.originalUnit {
				t.Errorf("OriginalUnit: got %q, want %q", got.OriginalUnit, tc.originalUnit)
			}
			if got.Name != tc.ingrName {
				t.Errorf("Name: got %q, want %q", got.Name, tc.ingrName)
			}
			if got.Note != tc.note {
				t.Errorf("Note: got %q, want %q", got.Note, tc.note)
			}
			if got.Confidence != tc.confidence {
				t.Errorf("Confidence: got %q, want %q", got.Confidence, tc.confidence)
			}
			if units.IsCount(tc.unit) {
				if got.PortionGrams == nil || *got.PortionGrams != units.CountGrams[tc.unit] {
					t.Errorf("PortionGrams: got %v, want units.CountGrams[%q]", got.PortionGrams, tc.unit)
				}
			} else if got.PortionGrams != nil {
				t.Errorf("PortionGrams: got %v, want nil", *got.PortionGrams)
			}
		})
	}
}

func TestDetectLanguage(t *testing.T) {
	cases := []struct {
		name  string
		lines []string
		want  string
	}{
		{"german", []string{"200 g Mehl", "1 Prise Salz", "2 Zehen Knoblauch"}, "de"},
		{"english", []string{"2 cups flour", "1 tablespoon salt", "3 cloves garlic"}, "en"},
		{"empty", []string{}, "unknown"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := DetectLanguage(tc.lines); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func almostEqual(a, b float64) bool {
	return math.Abs(a-b) < 1e-6
}
