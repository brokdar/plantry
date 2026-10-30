package units

import "testing"

func TestDisplay(t *testing.T) {
	cases := []struct {
		name       string
		amount     float64
		unit       string
		wantAmount float64
		wantUnit   string
	}{
		// B5-AC1
		{"from 100 g rounds to 5 g", 1234, "g", 1235, "g"},
		{"below 100 g rounds to 1 g", 87.3, "g", 87, "g"},
		{"clean kilograms switch to kg", 1500, "g", 1.5, "kg"},
		{"tbsp rounds to half", 1.25, "tbsp", 1.5, "tbsp"},

		{"exactly 100 g", 100, "g", 100, "g"},
		{"just below 100 g", 99.4, "g", 99, "g"},
		{"102.4 g rounds to 100", 102.4, "g", 100, "g"},
		{"1000 g is 1 kg", 1000, "g", 1, "kg"},
		{"1498 g rounds to 1500 then kg", 1498, "g", 1.5, "kg"},
		{"1040 g stays in g", 1040, "g", 1040, "g"},
		{"kg input normalised through grams", 1.5, "kg", 1.5, "kg"},
		{"kg input that does not convert cleanly", 1.234, "kg", 1235, "g"},
		{"ml like g", 87.3, "ml", 87, "ml"},
		{"clean litres", 2000, "ml", 2, "l"},
		{"l input", 0.25, "l", 250, "ml"},
		{"cl input", 2, "cl", 20, "ml"},
		{"tsp rounds to half", 0.7, "tsp", 0.5, "tsp"},
		{"count stays exact", 2.0 / 3.0, "can", 2.0 / 3.0, "can"},
		{"count fraction stays exact", 1.5, "piece", 1.5, "piece"},
		{"alias is normalised", 2, "Zehen", 2, "clove"},
		{"other units pass through", 1.37, "cup", 1.37, "cup"},
		{"zero", 0, "g", 0, "g"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Display(tc.amount, tc.unit)
			if !almost(got.Amount, tc.wantAmount) || got.Unit != tc.wantUnit {
				t.Errorf("Display(%v, %q) = %v %s, want %v %s",
					tc.amount, tc.unit, got.Amount, got.Unit, tc.wantAmount, tc.wantUnit)
			}
		})
	}
}

func TestBuy(t *testing.T) {
	cases := []struct {
		name       string
		amount     float64
		unit       string
		wantAmount float64
		wantUnit   string
		wantOK     bool
	}{
		{"B5-AC1 partial can rounds up", 0.67, "can", 1, "can", true},
		{"whole count stays", 3, "piece", 3, "piece", true},
		{"half rounds up", 1.5, "piece", 2, "piece", true},
		{"float noise does not add a unit", 3.0000000001, "piece", 3, "piece", true},
		{"alias is normalised", 2.2, "Zehen", 3, "clove", true},
		{"mass has no buy", 500, "g", 0, "", false},
		{"spoons have no buy", 1, "tbsp", 0, "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := Buy(tc.amount, tc.unit)
			if ok != tc.wantOK {
				t.Fatalf("Buy(%v, %q) ok = %v, want %v", tc.amount, tc.unit, ok, tc.wantOK)
			}
			if !ok {
				return
			}
			if !almost(got.Amount, tc.wantAmount) || got.Unit != tc.wantUnit {
				t.Errorf("Buy(%v, %q) = %v %s, want %v %s",
					tc.amount, tc.unit, got.Amount, got.Unit, tc.wantAmount, tc.wantUnit)
			}
		})
	}
}

func almost(a, b float64) bool {
	d := a - b
	return d < 1e-9 && d > -1e-9
}
