package units

import "testing"

func TestToBase(t *testing.T) {
	cases := []struct {
		amount   float64
		unit     string
		want     float64
		wantUnit string
	}{
		{1, "kg", 1000, "g"},
		{200, "g", 200, "g"},
		{1.5, "l", 1500, "ml"},
		{2, "dl", 200, "ml"},
		{5, "cl", 50, "ml"},
		{3, "Stück", 3, "piece"},
		{2, "tbsp", 2, "tbsp"},
	}
	for _, c := range cases {
		got, u := ToBase(c.amount, c.unit)
		if got != c.want || u != c.wantUnit {
			t.Errorf("ToBase(%v, %q) = %v %q; want %v %q", c.amount, c.unit, got, u, c.want, c.wantUnit)
		}
	}
}
