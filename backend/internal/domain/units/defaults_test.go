package units

import "testing"

func TestNormalize(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", ""},
		{"  ", ""},
		{"g", "g"},
		{"G", "g"},
		{" g ", "g"},
		{"g.", "g"},
		{"TBSP", "tbsp"},
		{" tbsp ", "tbsp"},
		{"Tablespoon", "tbsp"},
		{"EL", "tbsp"},
		{"Esslöffel", "tbsp"},
		{"TL", "tsp"},
		{"teelöffel", "tsp"},
		{"Zehe", "clove"},
		{"Zehen", "clove"},
		{"cloves", "clove"},
		{"Stück", "piece"},
		{"Stueck", "piece"},
		{"Scheibe", "slice"},
		{"unknown-unit", "unknown-unit"},
	}
	for _, c := range cases {
		if got := Normalize(c.in); got != c.want {
			t.Errorf("Normalize(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestLookupDefault(t *testing.T) {
	d, ok := LookupDefault("tbsp")
	if !ok {
		t.Fatal("tbsp should have a default")
	}
	if d.Grams != 15 || d.Kind != KindVolume || !d.Approximate {
		t.Errorf("tbsp default unexpected: %+v", d)
	}

	d, ok = LookupDefault("g")
	if !ok || d.Grams != 1 || d.Kind != KindMass || d.Approximate {
		t.Errorf("g default unexpected: %+v ok=%v", d, ok)
	}

	if _, ok := LookupDefault("clove"); ok {
		t.Error("clove must not have a universal default")
	}
	if _, ok := LookupDefault("unknown"); ok {
		t.Error("unknown unit must not resolve to a default")
	}
}

func TestIsCount(t *testing.T) {
	counts := []string{"clove", "piece", "slice", "bunch", "pinch"}
	for _, u := range counts {
		if !IsCount(u) {
			t.Errorf("IsCount(%q) = false, want true", u)
		}
	}
	for _, u := range []string{"g", "ml", "tbsp", "kg", "", "serving"} {
		if IsCount(u) {
			t.Errorf("IsCount(%q) = true, want false", u)
		}
	}
}

// B5-AC4: one count-grams table serves the importer and the portion defaults.
func TestCountGrams(t *testing.T) {
	for u, g := range CountGrams {
		if g <= 0 {
			t.Errorf("CountGrams[%q] = %v, want > 0", u, g)
		}
		if _, ok := Defaults[u]; ok {
			t.Errorf("%q is both a count unit and a mass/volume default", u)
		}
	}
	want := map[string]float64{"piece": 120, "clove": 5, "pinch": 0.5, "can": 400}
	for u, g := range want {
		if CountGrams[u] != g {
			t.Errorf("CountGrams[%q] = %v, want %v", u, CountGrams[u], g)
		}
	}
}

func TestNormalize_ImporterCountTokens(t *testing.T) {
	cases := map[string]string{
		"Msp": "pinch", "Messerspitze": "pinch",
		"Becher": "tub", "Handvoll": "handful",
		"Dose": "can", "Pck.": "packet", "Stk.": "piece",
	}
	for in, want := range cases {
		if got := Normalize(in); got != want {
			t.Errorf("Normalize(%q) = %q, want %q", in, got, want)
		}
		if !IsCount(want) {
			t.Errorf("IsCount(%q) = false, want true", want)
		}
	}
}

func TestNormalize_RealWorldVolumeWords(t *testing.T) {
	cases := map[string]string{
		"Tasse": "cup", "Tassen": "cup", "tasse": "cup",
		"Schuss": "tbsp", "schuss": "tbsp",
	}
	for in, want := range cases {
		if got := Normalize(in); got != want {
			t.Errorf("Normalize(%q) = %q, want %q", in, got, want)
		}
		if _, ok := LookupDefault(want); !ok {
			t.Errorf("LookupDefault(%q) missing", want)
		}
	}
}

// EstimateCountGrams is the one place a count unit becomes grams when the
// user has not reviewed a weight: the parser's draft and Finalize both call it.
func TestEstimateCountGrams(t *testing.T) {
	cases := []struct {
		unit, name string
		want       float64
	}{
		{"piece", "Knoblauch", 3}, // recipes that count garlic "pieces" mean cloves
		{"clove", "Knoblauchzehe", 3},
		{"clove", "garlic", 3},
		{"clove", "Ingwer", 5}, // no name entry: the unit default
		{"piece", "Zwiebel", 110},
		{"piece", "2 red onions chopped", 110},
		{"piece", "Frühlingszwiebeln", 15}, // specific before the generic onion
		{"piece", "Eier", 50},
		{"piece", "Eier (Größe M)", 50},
		{"piece", "Eisbergsalat", 120},  // "ei" is a whole word only
		{"piece", "Süßkartoffeln", 130}, // specific before the generic potato
		{"piece", "Kartoffeln (vorwiegend festkochend)", 170},
		{"piece", "Cocktailtomaten", 17},
		{"piece", "Tomaten", 120},
		{"piece", "Karotten", 61},
		{"piece", "Paprika", 119},
		{"piece", "Peperoni", 45},
		{"piece", "Zucchini", 196},
		{"piece", "Brokkoli", 500},
		{"leaf", "Lorbeerblatt", 0.6},
		{"piece", "Lorbeerblatt", 0.6},
		{"piece", "Fantasiefrucht", 120}, // nothing known: the unit default
		{"piece", "", 120},
		{"can", "Zwiebel", 400}, // name entries only refine piece, clove and leaf
		{"bunch", "Zwiebel", 30},
	}
	for _, tc := range cases {
		if got := EstimateCountGrams(tc.unit, tc.name); got != tc.want {
			t.Errorf("EstimateCountGrams(%q, %q) = %v, want %v", tc.unit, tc.name, got, tc.want)
		}
	}
}
