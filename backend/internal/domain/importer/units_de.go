package importer

type unitInfo struct {
	Canonical    string  // "g" or "ml"
	FactorToBase float64 // multiplier from the given unit to the canonical base.
	Approximate  bool    // true → confidence="approximate", false → "parsed".
	Display      string  // canonical spelling preserved in OriginalUnit when input matched an alias form.
}

// stückFactor is the assumed grams-per-piece for unit-less counts.
const stückFactor = 120.0

// stückDisplay is the OriginalUnit label we assign to unit-less counts.
const stückDisplay = "Stück"

// aliasesDE maps German unit tokens (lowercase, trailing dot stripped) to conversion info.
// Display is the canonical spelling we write back to OriginalUnit when the user's token
// was in some normalized form (kept case-insensitive; we generally echo the user's token).
var aliasesDE = map[string]unitInfo{
	// Mass
	"g":     {"g", 1, false, "g"},
	"gr":    {"g", 1, false, "g"},
	"gramm": {"g", 1, false, "g"},
	"kg":    {"g", 1000, false, "kg"},
	"mg":    {"g", 0.001, false, "mg"},
	// Volume
	"ml":         {"ml", 1, false, "ml"},
	"milliliter": {"ml", 1, false, "ml"},
	"l":          {"ml", 1000, false, "l"},
	"liter":      {"ml", 1000, false, "l"},
	"cl":         {"ml", 10, false, "cl"},
	"dl":         {"ml", 100, false, "dl"},
	// Spoons / dash
	"el":           {"ml", 15, true, "EL"},
	"essl":         {"ml", 15, true, "EL"},
	"esslöffel":    {"ml", 15, true, "EL"},
	"tl":           {"ml", 5, true, "TL"},
	"teel":         {"ml", 5, true, "TL"},
	"teelöffel":    {"ml", 5, true, "TL"},
	"msp":          {"g", 1, true, "Msp"},
	"messerspitze": {"g", 1, true, "Msp"},
	// Count-as-grams
	"prise":    {"g", 0.5, true, "Prise"},
	"prisen":   {"g", 0.5, true, "Prise"},
	"zehe":     {"g", 4, true, "Zehe"},
	"zehen":    {"g", 4, true, "Zehe"},
	"bund":     {"g", 30, true, "Bund"},
	"stange":   {"g", 60, true, "Stange"},
	"stangen":  {"g", 60, true, "Stange"},
	"stück":    {"g", 120, true, "Stück"},
	"stueck":   {"g", 120, true, "Stück"},
	"stk":      {"g", 120, true, "Stück"},
	"pck":      {"g", 8, true, "Pck"},
	"päckchen": {"g", 8, true, "Pck"},
	"packung":  {"g", 8, true, "Pck"},
	"dose":     {"g", 400, true, "Dose"},
	"dosen":    {"g", 400, true, "Dose"},
	"glas":     {"g", 200, true, "Glas"},
	"gläser":   {"g", 200, true, "Glas"},
	"becher":   {"g", 200, true, "Becher"},
	"handvoll": {"g", 30, true, "Handvoll"},
	"scheibe":  {"g", 30, true, "Scheibe"},
	"scheiben": {"g", 30, true, "Scheibe"},
}
