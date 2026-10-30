package units

import "math"

// Quantity is an amount in a canonical unit, as the API hands it to the UI.
type Quantity struct {
	Amount float64 `json:"amount"`
	Unit   string  `json:"unit"`
}

// metricBase maps metric mass and volume units to their base unit (g or ml)
// and the factor into it. Display works in the base unit.
var metricBase = map[string]struct {
	base   string
	factor float64
}{
	"g":  {"g", 1},
	"kg": {"g", 1000},
	"ml": {"ml", 1},
	"l":  {"ml", 1000},
	"cl": {"ml", 10},
	"dl": {"ml", 100},
}

// bigUnit is the unit a base unit switches to from 1000.
var bigUnit = map[string]string{"g": "kg", "ml": "l"}

// Display rounds an exact amount for reading (Phase 2 D26):
//
//   - metric mass and volume work in g/ml: to 1 below 100 and to 5 from 100;
//     from 1000 they switch to kg/l when the rounded value is a whole number
//     of 100 g/ml, so it reads with one decimal (1500 g → 1.5 kg, 1235 g stays)
//   - tbsp and tsp round to 0.5
//   - everything else, counts included, is returned exact in its canonical
//     unit; the frontend renders count fractions (⅔).
func Display(amount float64, unit string) Quantity {
	u := Normalize(unit)
	if m, ok := metricBase[u]; ok {
		v := amount * m.factor
		if v < 100 {
			v = math.Round(v)
		} else {
			v = math.Round(v/5) * 5
		}
		if v >= 1000 && math.Mod(v, 100) == 0 {
			return Quantity{Amount: v / 1000, Unit: bigUnit[m.base]}
		}
		return Quantity{Amount: v, Unit: m.base}
	}
	if u == "tbsp" || u == "tsp" {
		return Quantity{Amount: math.Round(amount*2) / 2, Unit: u}
	}
	return Quantity{Amount: amount, Unit: u}
}

// Buy is what to buy of a count unit: the amount rounded up to whole units.
// ok is false for units that are not counts (mass and volume are bought as
// displayed).
func Buy(amount float64, unit string) (q Quantity, ok bool) {
	u := Normalize(unit)
	if !IsCount(u) {
		return Quantity{}, false
	}
	// The epsilon keeps float noise (3.0000000001) from buying an extra unit.
	return Quantity{Amount: math.Ceil(amount - 1e-9), Unit: u}, true
}
