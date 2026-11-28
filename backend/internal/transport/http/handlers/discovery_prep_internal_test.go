package handlers

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaltszeimer/plantry/backend/internal/domain/discovery"
	"github.com/jaltszeimer/plantry/backend/internal/domain/importer"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planview"
)

// TestToSuggestionResponse_PrepAhead: a find whose steps marinate for hours
// carries the derived prep_ahead mark; one without such a step carries none.
func TestToSuggestionResponse_PrepAhead(t *testing.T) {
	s := &discovery.Suggestion{Draft: importer.Draft{
		Name:         "Miso Chicken",
		Ingredients:  []importer.DraftIngredient{{Name: "chicken thighs"}},
		Instructions: []string{"Marinate for at least 4 hours.", "Roast."},
	}}
	assert.Equal(t, &planview.PrepAhead{Kind: "marinate", Hours: 4}, toSuggestionResponse(s).PrepAhead)

	s.Draft.Instructions = []string{"Roast."}
	assert.Nil(t, toSuggestionResponse(s).PrepAhead)
}
