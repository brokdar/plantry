package food

import "testing"

func TestSubstituteTable(t *testing.T) {
	n := 0
	for _, list := range substitutes {
		n += len(list)
	}
	if n < 60 {
		t.Fatalf("substitute table has %d rows, want at least 60", n)
	}
}

func TestSubstitutesOfByNameInBothLanguages(t *testing.T) {
	for _, name := range []string{"Sour cream", "saure sahne"} {
		subs := SubstitutesOf(name)
		if len(subs) == 0 || subs[0].To != "greek_yogurt" || subs[0].Ratio != 1 {
			t.Errorf("SubstitutesOf(%q) = %+v, want Greek yoghurt 1:1 first", name, subs)
		}
	}
	if SubstitutesOf("unobtainium") != nil {
		t.Error("an unknown food has no substitutes")
	}
}

func TestIsSynonym(t *testing.T) {
	if !IsSynonym("ghee", "Ghee") || IsSynonym("ghee", "Butter") {
		t.Error("IsSynonym must match the entry's own names only")
	}
}

// A row whose two foods share a name would swap a food for itself.
func TestSubstituteRowsAreDistinctFoods(t *testing.T) {
	for from, list := range substitutes {
		for _, s := range list {
			for n := range synonyms.byID[s.To].names {
				if _, same := synonyms.byID[from].names[n]; same {
					t.Errorf("%s -> %s share the name %q", from, s.To, n)
				}
			}
		}
	}
}
