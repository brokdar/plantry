package diet_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaltszeimer/plantry/backend/internal/domain/diet"
)

// Leaf names from a real import run (USDA-backed leaves of twelve German and
// English recipes). Rice compounds and dried herbs and spices were Unknown,
// which made every recipe using them Unknown too. Herbs and spices are
// neutral: they classify as the plant foods they are and never raise a
// recipe's class.
var realWorldCases = []struct {
	name string
	want diet.Class
}{
	{"Langkornreis, roh", vgn},
	{"Basmatireis", vgn},
	{"Risottoreis", vgn},
	{"Jasminreis", vgn},
	{"Rundkornreis", vgn},
	{"Milchreis", vgt}, // the milk still decides
	{"long-grain rice", vgn},
	{"Lorbeerblatt", vgn},
	{"bay leaf", vgn},
	{"Oregano, getrocknet", vgn},
	{"dried oregano", vgn},
	{"Thymian", vgn},
	{"thyme", vgn},
	{"Rosmarin", vgn},
	{"rosemary", vgn},
	{"Majoran", vgn},
	{"marjoram", vgn},
	{"Salbei", vgn},
	{"Bohnenkraut", vgn},
	{"Muskatnuss, gerieben", vgn},
	{"nutmeg", vgn},
	{"Kardamom", vgn},
	{"cardamom", vgn},
	{"Garam Masala", vgn},
	{"Paprikapulver", vgn},
	{"Currypulver", vgn},
	{"Kreuzkümmel", vgn},
	{"Kurkuma", vgn},
	{"chilli", vgn}, // the British spelling
	{"1 red chilli deseeded and finely chopped", vgn},
	{"coriander", vgn},
	{"small bunch coriander stalks and leaves chopped separately", vgn},
	// Unchanged neighbours.
	{"Rinderhackfleisch (Faschiertes)", omn},
	{"Hähnchenbrust", omn},
	{"Speck (Bacon)", omn},
	{"Lachsfilet", psc},
	{"Eier", vgt},
	{"Parmesan, gerieben", vgt},
	{"Crème fraîche (Näherung: Sauerrahm)", vgt},
	{"Spaghetti (trocken)", vgn},
	{"Fantasiezutat", unk},
}

func TestClassify_RealWorld(t *testing.T) {
	for _, tc := range realWorldCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, diet.Classify(tc.name))
		})
	}
}

// A spice is neutral: it does not turn a vegetarian recipe vegan nor an
// unknown one known, and Max over a recipe keeps working.
func TestClassify_SpicesAreNeutralInARecipe(t *testing.T) {
	got := diet.Max(
		diet.Classify("Langkornreis, roh"),
		diet.Classify("Hähnchenbrust"),
		diet.Classify("Lorbeerblatt"),
		diet.Classify("Oregano, getrocknet"),
	)
	assert.Equal(t, omn, got)
}
