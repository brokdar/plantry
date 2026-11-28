package food

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
	"unicode"
)

// synonymsJSON is the bundled bilingual (en/de) synonym table for common
// ingredients (spec MATCH-7). Each entry names one food with its state, the
// names it goes by in both languages, extra catalogue names that are the
// same food ("also"), its food group for the kcal band, and the English FDC
// and Open Food Facts search terms used without a provider.
//
//go:embed matchdata/synonyms.json
var synonymsJSON []byte

// Synonym is one food of the bundled table.
type Synonym struct {
	ID    string   `json:"id"`
	Group string   `json:"group"`
	State string   `json:"state"`
	EN    []string `json:"en"`
	DE    []string `json:"de"`
	Also  []string `json:"also"`
	// FDC is the English FDC search term; FDCPrefer a description fragment
	// that marks the generic food among the results.
	FDC       string `json:"fdc"`
	FDCPrefer string `json:"fdc_prefer"`
	// Strict: a result is confident only when it names the line's own
	// distinguishing words (the entry has no FDC food of its own).
	Strict bool `json:"strict"`
	// OFF is the Open Food Facts search term for products FDC lacks.
	OFF string `json:"off"`
	// Density is grams per ml; PieceG grams per piece (count units).
	Density float64 `json:"density"`
	PieceG  float64 `json:"piece_g"`

	names map[string]struct{} // every normalized name, for catalogue matching
}

// synonymTable indexes the bundled table by normalized name.
type synonymTable struct {
	byName map[string]*Synonym
	byID   map[string]*Synonym
}

var synonyms = mustLoadSynonyms()

func mustLoadSynonyms() *synonymTable {
	var entries []*Synonym
	if err := json.Unmarshal(synonymsJSON, &entries); err != nil {
		panic(fmt.Sprintf("food: synonym table: %v", err))
	}
	t := &synonymTable{byName: map[string]*Synonym{}, byID: map[string]*Synonym{}}
	for _, e := range entries {
		if _, ok := kcalBands[e.Group]; !ok && e.Group != "water" {
			panic(fmt.Sprintf("food: synonym %s: unknown group %q", e.ID, e.Group))
		}
		t.byID[e.ID] = e
		e.names = map[string]struct{}{}
		for _, list := range [][]string{e.EN, e.DE} {
			for _, n := range list {
				k := matchKey(n)
				e.names[k] = struct{}{}
				if _, taken := t.byName[k]; !taken {
					t.byName[k] = e
				}
			}
		}
		for _, n := range e.Also {
			e.names[matchKey(n)] = struct{}{}
		}
	}
	return t
}

// lookup returns the entry of the first key the table knows.
func (t *synonymTable) lookup(keys []string) *Synonym {
	for _, k := range keys {
		if e := t.byName[k]; e != nil {
			return e
		}
	}
	return nil
}

// suffix returns the entry whose longest German name (at least 4 letters)
// ends a compound word: "Bio-Zwiebeln" -> onion. Used last.
func (t *synonymTable) suffix(key string) *Synonym {
	if strings.Contains(key, " ") {
		return nil
	}
	var best *Synonym
	bestLen := 0
	for name, e := range t.byName {
		if len(name) >= 4 && len(name) > bestLen && len(name) < len(key) && strings.HasSuffix(key, name) {
			best, bestLen = e, len(name)
		}
	}
	return best
}

// matchKey normalizes a name for matching: lowercase, folded diacritics,
// punctuation as spaces ("Crème fraîche" and "creme-fraiche" are equal).
func matchKey(s string) string {
	s = normalizeName(s)
	s = strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return r
		}
		return ' '
	}, s)
	return strings.Join(strings.Fields(s), " ")
}

// baseKey is matchKey without a parenthetical: "Milk (whole)" -> "milk".
func baseKey(name string) string {
	if i := strings.Index(name, "("); i > 0 {
		return matchKey(name[:i])
	}
	return matchKey(name)
}

// Food states (MATCH-2). An empty state is unknown and matches any.
const (
	StateRaw    = "raw"
	StateDry    = "dry"
	StateCooked = "cooked"
	StateCanned = "canned"
	StateDried  = "dried"
	StateFresh  = "fresh"
)

// stateWords map words of a name or FDC description to a state.
var stateWords = map[string]string{
	"raw": StateRaw, "roh": StateRaw, "uncooked": StateDry, "as purchased": StateRaw,
	"dry": StateDry, "trocken": StateDry,
	"cooked": StateCooked, "boiled": StateCooked, "steamed": StateCooked, "gekocht": StateCooked,
	"gekochter": StateCooked, "gegart": StateCooked, "broiled": StateCooked, "roasted": StateCooked,
	"canned": StateCanned, "tinned": StateCanned, "dose": StateCanned, "drained": StateCanned,
	"dried": StateDried, "getrocknet": StateDried, "getrockneter": StateDried, "getrocknete": StateDried,
	"fresh": StateFresh, "frisch": StateFresh, "frische": StateFresh, "frischer": StateFresh,
}

// stateOf reads the state a name or description states, "" when none. The
// first state word wins ("Pasta, fresh-refrigerated, plain, cooked" is
// cooked because "cooked" is checked before "fresh").
func stateOf(text string) string {
	k := " " + matchKey(text) + " "
	for _, s := range []string{StateCooked, StateCanned, StateDry, StateDried, StateRaw, StateFresh} {
		for w, ws := range stateWords {
			if ws == s && strings.Contains(k, " "+w+" ") {
				return s
			}
		}
	}
	return ""
}

// statesConflict reports two known states that are not the same food:
// cooked or canned against dry or raw (3x the kcal), dried against fresh.
func statesConflict(a, b string) bool {
	if a == "" || b == "" || a == b {
		return false
	}
	uncooked := func(s string) bool { return s == StateDry || s == StateRaw }
	prepared := func(s string) bool { return s == StateCooked || s == StateCanned }
	switch {
	case uncooked(a) && prepared(b), prepared(a) && uncooked(b):
		return true
	case a == StateDried && (b == StateFresh || b == StateRaw), b == StateDried && (a == StateFresh || a == StateRaw):
		return true
	}
	return false
}

// kcalBands are plausible kcal per 100 g per food group (MATCH-2): a match
// outside its group's band is not the food it claims ("dry lentils" at
// 116 kcal are cooked lentils).
var kcalBands = map[string][2]float64{
	"veg": {5, 160}, "fruit": {15, 130}, "herb": {10, 330}, "spice": {0, 600},
	"fat": {650, 900}, "dairy_milk": {25, 90}, "cream": {100, 450}, "yogurt": {40, 160},
	"cheese": {120, 460}, "egg": {120, 170}, "meat": {90, 460}, "poultry": {90, 250},
	"fish": {70, 250}, "grain_dry": {300, 420}, "grain_cooked": {90, 200}, "grain_fresh": {200, 320},
	"bread": {200, 400}, "legume_dry": {280, 400}, "legume_canned": {60, 170}, "broth": {0, 40},
	"condiment": {0, 450}, "sugar": {280, 410}, "chocolate": {450, 620}, "alcohol": {50, 300},
	"nuts": {500, 720},
}

// groupDensity is grams per ml by group when the entry gives none.
var groupDensity = map[string]float64{
	"fat": 0.92, "spice": 0.5, "herb": 0.25, "grain_dry": 0.6, "sugar": 0.85, "cheese": 0.42,
	"veg": 0.6, "legume_canned": 0.7,
}

// seasoningGroups may use the closest same-kind food when immaterial
// (research section 3.2 "Materiality").
var seasoningGroups = map[string]bool{"spice": true, "herb": true}

// LineDensity returns the grams per ml of a line's food from the bundled
// table: the entry's own density, else its group's, 1 for water. False when
// the table does not know the food or has no density for it.
func LineDensity(name, note string) (float64, bool) {
	keys := lineKeys(name, note)
	e := synonyms.lookup(keys)
	if e == nil && len(keys) > 0 {
		e = synonyms.suffix(keys[0])
	}
	switch {
	case e == nil:
		return 0, false
	case e.Density > 0:
		return e.Density, true
	case groupDensity[e.Group] > 0:
		return groupDensity[e.Group], true
	case e.Group == "water":
		return 1, true
	}
	return 0, false
}

// LineGroup returns the bundled table's food group of a line ("spice",
// "fat", ...), or "" when the table does not know the food.
func LineGroup(name, note string) string {
	keys := lineKeys(name, note)
	e := synonyms.lookup(keys)
	if e == nil && len(keys) > 0 {
		e = synonyms.suffix(keys[0])
	}
	if e == nil {
		return ""
	}
	return e.Group
}

// IsToTasteGroup reports whether a group may be "to taste" (MATCH-6: salt,
// pepper, dried spices, fresh herbs).
func IsToTasteGroup(group string) bool { return seasoningGroups[group] }
