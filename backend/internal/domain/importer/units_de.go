package importer

// unitlessDisplay is the OriginalUnit label for a count written without a
// unit ("1 Zwiebel"), which is stored as unit piece.
const unitlessDisplay = "Stück"

// compoundUnitSuffixes lists count units that commonly appear as the tail of
// a German compound word ("Knoblauchzehe" ends in "-zehe"), with the label
// written to OriginalUnit. Ordered longest-suffix-first so "zehen" beats
// "zehe" on plural forms. Grams per unit live in units.CountGrams.
var compoundUnitSuffixes = []struct{ suffix, display string }{
	{"knoblauchzehen", "Zehe"},
	{"knoblauchzehe", "Zehe"},
	{"zehen", "Zehe"},
	{"zehe", "Zehe"},
	{"scheiben", "Scheibe"},
	{"scheibe", "Scheibe"},
	{"stangen", "Stange"},
	{"stange", "Stange"},
}
