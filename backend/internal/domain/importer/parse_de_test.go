package importer

import (
	"math"
	"testing"
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
		{"knife tip", "1 Msp Muskat", 1, "g", "Msp", "Muskat", "", ConfidenceApproximate},
		{"pinch", "1 Prise Salz", 0.5, "g", "Prise", "Salz", "", ConfidenceApproximate},

		// 3. count-like units
		{"clove plural", "2 Zehen Knoblauch", 8, "g", "Zehen", "Knoblauch", "", ConfidenceApproximate},
		{"bunch", "1 Bund Petersilie", 30, "g", "Bund", "Petersilie", "", ConfidenceApproximate},
		{"stalk", "1 Stange Sellerie", 60, "g", "Stange", "Sellerie", "", ConfidenceApproximate},
		{"piece abbreviated", "1 Stk. Paprika", 120, "g", "Stk.", "Paprika", "", ConfidenceApproximate},
		{"package", "1 Pck Vanillezucker", 8, "g", "Pck", "Vanillezucker", "", ConfidenceApproximate},
		{"can", "1 Dose Tomaten", 400, "g", "Dose", "Tomaten", "", ConfidenceApproximate},
		{"jar", "1 Glas Kapern", 200, "g", "Glas", "Kapern", "", ConfidenceApproximate},

		// 4. fractions
		{"ascii fraction", "1/2 Bund Petersilie", 15, "g", "Bund", "Petersilie", "", ConfidenceApproximate},
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
		{"comma note", "2 Zehen Knoblauch, fein gehackt", 8, "g", "Zehen", "Knoblauch", "fein gehackt", ConfidenceApproximate},

		// 8. unit-less count → Stück default
		{"unitless count eggs", "3 Eier", 360, "g", "Stück", "Eier", "", ConfidenceApproximate},

		// 9. compound-word suffix match: "Knoblauchzehe(n)" → Zehe unit.
		{"knoblauchzehe singular", "1 Knoblauchzehe", 4, "g", "Zehe", "Knoblauchzehe", "", ConfidenceApproximate},
		{"knoblauchzehen plural", "2 Knoblauchzehen", 8, "g", "Zehe", "Knoblauchzehen", "", ConfidenceApproximate},
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
