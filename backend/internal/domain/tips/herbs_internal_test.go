package tips

import (
	"slices"
	"testing"

	"github.com/jaltszeimer/plantry/backend/internal/domain/flavor"
	"github.com/jaltszeimer/plantry/backend/internal/domain/platefit"
)

// TestHerbChoice: the herb a plate is offered fits its cuisine and main,
// from a library holding every herb; chives stay the neutral fallback.
func TestHerbChoice(t *testing.T) {
	t.Parallel()
	all := []string{"Chives", "Parsley", "Coriander (fresh)", "Basil", "Mint", "Dill", "Chervil", "Tarragon"}
	tests := []struct {
		name, cuisine, main string
		want                []string // best first
	}{
		{"indian", "indian", "Chicken curry", []string{"Coriander (fresh)", "Mint"}},
		{"thai", "thai", "Green curry", []string{"Coriander (fresh)", "Basil"}},
		{"mexican", "mexican", "Tacos", []string{"Coriander (fresh)"}},
		{"chinese", "chinese", "Stir-fry", []string{"Coriander (fresh)"}},
		{"middle eastern", "middle_eastern", "Falafel", []string{"Coriander (fresh)", "Mint", "Parsley"}},
		{"italian", "italian", "Risotto", []string{"Parsley", "Basil"}},
		{"italian tomato", "italian", "Spaghetti al pomodoro (Tomate)", []string{"Basil", "Parsley"}},
		{"mediterranean", "mediterranean", "Lamb stew", []string{"Parsley", "Basil"}},
		{"french", "french", "Coq au vin", []string{"Parsley", "Chervil", "Tarragon"}},
		{"german potato", "german", "Kartoffelgratin", []string{"Chives", "Parsley"}},
		{"neutral", "", "Omelette", []string{"Chives", "Parsley"}},
		{"german fish", "german", "Baked cod with potatoes", []string{"Dill", "Chives", "Parsley"}},
		{"neutral fish", "neutral", "Lachs vom Blech", []string{"Dill", "Chives"}},
		{"fish curry stays coriander", "indian", "Prawn coconut curry", []string{"Coriander (fresh)", "Mint"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			prefs := herbPrefs(platefit.Component{Name: tc.main, Attrs: flavor.Attributes{Cuisine: tc.cuisine}})
			ranked := slices.Clone(all)
			slices.SortStableFunc(ranked, func(a, b string) int {
				return herbRank(prefs, platefit.Component{Name: a}) - herbRank(prefs, platefit.Component{Name: b})
			})
			if got := ranked[:len(tc.want)]; !slices.Equal(got, tc.want) {
				t.Errorf("herbs %v, want %v", got, tc.want)
			}
		})
	}
}

// TestHerbRank_FallbackAnyHerb: with the preferred herbs missing from the
// library, any other herb still ranks (after the preferred ones).
func TestHerbRank_FallbackAnyHerb(t *testing.T) {
	t.Parallel()
	prefs := herbPrefs(platefit.Component{Attrs: flavor.Attributes{Cuisine: "indian"}})
	if r := herbRank(prefs, platefit.Component{Name: "Chives"}); r != len(prefs) {
		t.Errorf("rank %d, want %d", r, len(prefs))
	}
}
