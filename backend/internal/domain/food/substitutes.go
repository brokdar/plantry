package food

import (
	_ "embed"
	"encoding/json"
	"fmt"
)

// substitutesJSON is the curated substitution table (HELP-3, HELP-7): one
// row per "instead of from, use to", both ids of the synonym table. Ratio is
// grams of to per gram of from.
//
//go:embed matchdata/substitutes.json
var substitutesJSON []byte

// Substitute is one row of the table, for the target entry's names.
type Substitute struct {
	// To is the synonym id of the substitute.
	To     string
	Ratio  float64
	NoteEN string
	NoteDE string
}

type substituteRow struct {
	From   string  `json:"from"`
	To     string  `json:"to"`
	Ratio  float64 `json:"ratio"`
	NoteEN string  `json:"note_en"`
	NoteDE string  `json:"note_de"`
}

var substitutes = mustLoadSubstitutes()

func mustLoadSubstitutes() map[string][]Substitute {
	var rows []substituteRow
	if err := json.Unmarshal(substitutesJSON, &rows); err != nil {
		panic(fmt.Sprintf("food: substitute table: %v", err))
	}
	out := map[string][]Substitute{}
	for _, r := range rows {
		if synonyms.byID[r.From] == nil || synonyms.byID[r.To] == nil || r.From == r.To || r.Ratio <= 0 {
			panic(fmt.Sprintf("food: substitute row %s -> %s is invalid", r.From, r.To))
		}
		out[r.From] = append(out[r.From], Substitute{To: r.To, Ratio: r.Ratio, NoteEN: r.NoteEN, NoteDE: r.NoteDE})
	}
	return out
}

// SubstitutesOf returns the table's substitutes for the food a name stands
// for (any en/de name of its synonym entry), in table order; nil when the
// table does not know the food.
func SubstitutesOf(name string) []Substitute {
	keys := lineKeys(name, "")
	e := synonyms.lookup(keys)
	if e == nil && len(keys) > 0 {
		e = synonyms.suffix(keys[0])
	}
	if e == nil {
		return nil
	}
	return substitutes[e.ID]
}

// IsSynonym reports whether leafName is a name of synonym entry id (the
// catalogue identity used by matching).
func IsSynonym(id, leafName string) bool {
	e := synonyms.byID[id]
	if e == nil {
		return false
	}
	_, full := e.names[matchKey(leafName)]
	_, base := e.names[baseKey(leafName)]
	return full || base
}

// SynonymName returns the English name of synonym entry id, "" when the
// table has no such entry.
func SynonymName(id string) string {
	e := synonyms.byID[id]
	if e == nil || len(e.EN) == 0 {
		return ""
	}
	return e.EN[0]
}
