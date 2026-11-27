package chefsets

import (
	"embed"
	"encoding/json"
	"fmt"
	"slices"
)

//go:embed matching/*.json
var matchingFS embed.FS

// Line kinds of the matching sets (spec section 5.5, MATCH-1).
var LineKinds = []string{
	"plain", "alternatives", "compound", "to_taste", "vague_count", "container_range", "branded",
}

// Origins of a recorded provider response.
const (
	OriginLive = "live" // recorded from the provider's API
	OriginBulk = "bulk" // built from FDC's public SR Legacy download, in the search-response shape
)

// Expect is one food a line must resolve to. Exactly one of Catalogue,
// FDCID (with FDCQuery) or OFFQuery (with OFFMatch) names where the
// expected identity lives; Identity says what it is, with its state.
type Expect struct {
	Identity  string   `json:"identity"`
	Catalogue string   `json:"catalogue,omitempty"` // leaf name of the dev catalogue
	FDCQuery  string   `json:"fdc_query,omitempty"` // key of a recorded FDC response
	FDCID     int      `json:"fdc_id,omitempty"`    // present in that response
	OFFQuery  string   `json:"off_query,omitempty"`
	OFFLang   string   `json:"off_lang,omitempty"`
	OFFMatch  string   `json:"off_match,omitempty"` // lowercase name fragment of an acceptable product
	Grams     *float64 `json:"grams,omitempty"`     // expected grams of the whole line, nil where none
}

// MatchLine is one real ingredient line with its hand-checked expectation.
// Expect holds all foods of a compound line, any one food of an AnyOf line,
// and is empty for a line that is not a food (a sub-recipe reference) or
// needs no match (water).
type MatchLine struct {
	ID     string   `json:"id"`
	Raw    string   `json:"raw"`
	Lang   string   `json:"lang"`
	Kind   string   `json:"kind"`
	Name   string   `json:"name"` // the cleaned name
	AnyOf  bool     `json:"any_of,omitempty"`
	Note   string   `json:"note,omitempty"`
	Expect []Expect `json:"expect"`

	Recipe string `json:"-"`
	Source string `json:"-"`
}

// MatchSet is one language's lines in recipe order.
type MatchSet struct {
	Set   string
	Lines []MatchLine
}

// CatalogueLeaf is a leaf of the dev catalogue the baseline matches against.
type CatalogueLeaf struct {
	Name     string   `json:"name"`
	Kcal100g *float64 `json:"kcal_100g"`
	Protein  *float64 `json:"protein_100g"`
	Fat      *float64 `json:"fat_100g"`
	Carbs    *float64 `json:"carbs_100g"`
	// Seeded reports whether the leaf was in the 106-leaf catalogue of the
	// 2026-11-16 seeded dev backup (the research baseline).
	Seeded bool `json:"seeded_2026_11_16"`
}

// Recorded is one provider response and where it came from.
type Recorded struct {
	Origin   string          `json:"origin"`
	Note     string          `json:"note,omitempty"`
	Response json.RawMessage `json:"response"`
}

// Matching is the DATA-2 test data of the Chef matching core.
type Matching struct {
	DE, EN    MatchSet
	Catalogue []CatalogueLeaf
	fdc       map[string]Recorded
	off       map[string]Recorded
}

// LoadMatching reads the embedded matching sets and recorded responses.
func LoadMatching() (*Matching, error) {
	m := &Matching{}
	for name, set := range map[string]*MatchSet{"de": &m.DE, "en": &m.EN} {
		var raw struct {
			Set     string `json:"set"`
			Recipes []struct {
				Title  string      `json:"title"`
				Source string      `json:"source"`
				Lines  []MatchLine `json:"lines"`
			} `json:"recipes"`
		}
		if err := readMatching(name+".json", &raw); err != nil {
			return nil, err
		}
		set.Set = raw.Set
		for _, r := range raw.Recipes {
			for _, l := range r.Lines {
				l.Recipe, l.Source = r.Title, r.Source
				set.Lines = append(set.Lines, l)
			}
		}
	}
	for name, dst := range map[string]any{"catalogue.json": &m.Catalogue, "fdc.json": &m.fdc, "off.json": &m.off} {
		if err := readMatching(name, dst); err != nil {
			return nil, err
		}
	}
	return m, nil
}

func readMatching(name string, dst any) error {
	b, err := matchingFS.ReadFile("matching/" + name)
	if err != nil {
		return fmt.Errorf("chefsets: %w", err)
	}
	if err := json.Unmarshal(b, dst); err != nil {
		return fmt.Errorf("chefsets: %s: %w", name, err)
	}
	return nil
}

// FDCResponse returns the recorded FDC search response for an English query.
func (m *Matching) FDCResponse(query string) (Recorded, bool) {
	r, ok := m.fdc[query]
	return r, ok
}

// OFFResponse returns the recorded Open Food Facts search response for a
// query in a language.
func (m *Matching) OFFResponse(lang, query string) (Recorded, bool) {
	r, ok := m.off[lang+":"+query]
	return r, ok
}

// FDCQueries lists the recorded FDC queries, sorted.
func (m *Matching) FDCQueries() []string { return sortedKeys(m.fdc) }

// OFFKeys lists the recorded OFF keys ("lang:query"), sorted.
func (m *Matching) OFFKeys() []string { return sortedKeys(m.off) }

func sortedKeys(m map[string]Recorded) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}
