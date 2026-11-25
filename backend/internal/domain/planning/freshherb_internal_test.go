package planning

import "testing"

func TestFreshHerbLeaf(t *testing.T) {
	t.Parallel()
	for name, want := range map[string]bool{
		"Parsley": true, "Frische Petersilie": true, "Chives": true, "Coriander (fresh)": true, "Spring onions": true,
		"Basil pesto": false, "Dried oregano": false, "Getrocknete Minze": false, "Garlic powder": false, "Cucumber": false,
	} {
		if got := freshHerbLeaf(name); got != want {
			t.Errorf("%q: got %v, want %v", name, got, want)
		}
	}
}
