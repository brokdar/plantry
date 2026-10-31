// Package diet holds the diet classes of foods (spec §3.6, D28): vegan <
// vegetarian < pescetarian < omnivore, plus unknown. It imports nothing from
// the rest of the domain.
package diet

import "slices"

// Class is a food's diet class.
type Class string

// The diet classes. Unknown means no class could be derived; it is never
// stored.
const (
	Vegan       Class = "vegan"
	Vegetarian  Class = "vegetarian"
	Pescetarian Class = "pescetarian"
	Omnivore    Class = "omnivore"
	Unknown     Class = "unknown"
)

// Order lists the known classes from most to least restrictive.
var Order = []Class{Vegan, Vegetarian, Pescetarian, Omnivore}

// Valid reports whether c is a known class (Unknown is not).
func Valid(c Class) bool { return slices.Contains(Order, c) }

// Max returns the least restrictive of classes: a recipe's class from its
// children's. Any Unknown (or invalid) class, and an empty list, give
// Unknown.
func Max(classes ...Class) Class {
	best := -1
	for _, c := range classes {
		i := slices.Index(Order, c)
		if i < 0 {
			return Unknown
		}
		best = max(best, i)
	}
	if best < 0 {
		return Unknown
	}
	return Order[best]
}

// Compatible reports whether a food of class fits a household eating
// dietType: class ≤ dietType. Unknown is never compatible; callers that treat
// it as a warning check for it first.
func Compatible(class, dietType Class) bool {
	c, d := slices.Index(Order, class), slices.Index(Order, dietType)
	return c >= 0 && d >= 0 && c <= d
}
