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
	for _, u := range []string{"g", "ml", "tbsp", "kg", ""} {
		if IsCount(u) {
			t.Errorf("IsCount(%q) = true, want false", u)
		}
	}
}
