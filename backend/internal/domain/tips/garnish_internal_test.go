package tips

import (
	"testing"

	"github.com/jaltszeimer/plantry/backend/internal/domain/platefit"
)

// TestGarnishKind: a garnish's kind comes from what the food is, not from a
// fresh flag (CP1: cucumber was offered as fresh herbs 26 times).
func TestGarnishKind(t *testing.T) {
	t.Parallel()
	tests := map[string]Kind{
		"Frische Petersilie": KindHerbs, "Schnittlauch": KindHerbs, "Fresh coriander": KindHerbs, "Basilikumblätter": KindHerbs,
		"Frühlingszwiebel-Ringe": KindHerbs, "Kresse": KindHerbs,
		"Gurke": KindCool, "Salatgurke": KindCool, "Cucumber": KindCool, "Naturjoghurt": KindCool, "Gurken-Raita": KindCool,
		"Gewürzgurke": KindPickle, "Rote Zwiebeln, eingelegt": KindPickle, "Kimchi": KindPickle,
		"Zitronenspalten": KindAcid, "Limette": KindAcid, "Balsamico-Essig": KindAcid,
		"Geröstete Erdnüsse": KindCrunch, "Sesam geröstet": KindCrunch, "Kürbiskerne": KindCrunch, "Röstzwiebeln": KindCrunch, "Walnüsse": KindCrunch,
		"Tomate": "", "Karotte": "", "Paprika": "", "Haferflocken": "",
		"Nutmeg": "", "Peanut butter": "", "Peanut oil": "", "Sesame oil": "", "Ground almonds": "", "Almond butter": "",
		"Basil (dried)": "", "Basil pesto": "", "Caraway seeds": "", "Light coconut milk": "", "Kürbiskernöl": "",
	}
	for name, want := range tests {
		if got := garnishKind(platefit.Component{Name: name}); got != want {
			t.Errorf("%s: kind %q, want %q", name, got, want)
		}
	}
}

// TestGarnishGrams: every garnish kind has an amount per eater as a cook
// portions it.
func TestGarnishGrams(t *testing.T) {
	t.Parallel()
	ranges := map[Kind][2]float64{
		KindHerbs: {3, 5}, KindCrunch: {10, 15}, KindPickle: {25, 35}, KindCool: {50, 80}, KindAcid: {5, 15},
	}
	for k, r := range ranges {
		if g := garnishGrams[k]; g < r[0] || g > r[1] {
			t.Errorf("%s: %g g per eater, want %g-%g", k, g, r[0], r[1])
		}
	}
	if len(garnishGrams) != len(ranges) {
		t.Errorf("garnishGrams has %d kinds, want %d", len(garnishGrams), len(ranges))
	}
}
