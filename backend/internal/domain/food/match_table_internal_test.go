package food

import "testing"

// Compound dishes must not fall to a generic suffix (review B1: "Kartoffelsalat"
// resolved to iceberg lettuce through a bare "salat" alias), and pastry must
// not borrow pizza-dough nutrition.
func TestSynonyms_NoWrongIdentity(t *testing.T) {
	t.Parallel()
	notTo := map[string]string{
		"kartoffelsalat": "iceberg",
		"gurkensalat":    "iceberg",
		"nudelsalat":     "iceberg",
		"blätterteig":    "pizza_dough",
		"mürbeteig":      "pizza_dough",
		"sojaschnetzel":  "soybeans",
		"mehl type 550":  "flour_wholemeal",
	}
	for name, wrong := range notTo {
		k := matchKey(name)
		e := synonyms.lookup([]string{k})
		if e == nil {
			e = synonyms.suffix(k)
		}
		if e != nil && e.ID == wrong {
			t.Errorf("%q resolves to %s", name, wrong)
		}
	}
}
