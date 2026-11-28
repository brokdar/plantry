package finds

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/importer"
)

// CP4: a find swapped lime for vinegar while step 1 still said "squeeze the
// lime".
func TestRemovedMentions_ChipotleLime(t *testing.T) {
	t.Parallel()
	base := &importer.Draft{Ingredients: []importer.DraftIngredient{
		{RawText: "2 chipotle peppers in adobo", Name: "chipotle pepper"},
		{RawText: "juice of 2 limes", Name: "lime juice"},
	}}
	adapted := func(steps ...string) adaptation {
		return adaptation{
			draft: &importer.Draft{
				Ingredients:  []importer.DraftIngredient{base.Ingredients[0], {RawText: "2 tbsp white vinegar", Name: "white vinegar"}},
				Instructions: steps,
			},
			changes: []Change{{Kind: "swap", From: "juice of 2 limes", To: "2 tbsp white vinegar"}},
		}
	}
	got := removedMentions(base, adapted("Blend the chipotles.", "Squeeze the lime over the top."))
	require.Len(t, got, 1)
	assert.Equal(t, CodeStepMentionsRemoved, got[0].Code)
	assert.Equal(t, 1, got[0].Step)
	assert.Empty(t, removedMentions(base, adapted("Blend the chipotles.", "Stir in the vinegar.")))

	// The food is still in another line: its steps stay right.
	a := adapted("Squeeze the lime over the top.")
	a.draft.Ingredients = append(a.draft.Ingredients, importer.DraftIngredient{RawText: "1 lime, in wedges", Name: "lime"})
	assert.Empty(t, removedMentions(base, a))
}

func TestTooSimilar(t *testing.T) {
	t.Parallel()
	library := []string{"Speedy lentil coconut curry", "Chili", "Pasta Bolognese", "Chicken Ramen Soup"}
	for name, want := range map[string]bool{
		"Creamy Coconut Curry Lentils with Spinach": true,
		"Easy Chili":                     true,
		"Ramen":                          false,
		"Pasta":                          false,
		"Lentil":                         false,
		"Chicken Curry":                  false,
		"Coconut Rice":                   false,
		"Spaghetti Bolognese":            false,
		"Pasta Bolognese with Mushrooms": true,
	} {
		assert.Equal(t, want, tooSimilar(name, library), name)
	}
}
