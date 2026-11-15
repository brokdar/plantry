package jsonld_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/jsonld"
)

// Some sites double-escape their JSON-LD strings, so the decoded value still
// holds HTML entities ("Gemüse &amp; Hähnchen"). Every text field is decoded.
func TestExtractRecipe_DecodesHTMLEntities(t *testing.T) {
	html := `<html><head><script type="application/ld+json">{
  "@type": "Recipe",
  "name": "Gebratener Reis mit Ei, Gemüse &amp; Hähnchen",
  "description": "Schnell &amp; einfach &quot;wie vom Wok&quot;",
  "recipeIngredient": ["100 g Erbsen &amp; Möhren", "1 EL Sojasoße"],
  "recipeInstructions": [{"@type": "HowToStep", "text": "Rühren &amp; braten."}],
  "keywords": "Reis &amp; Ei, Wok"
}</script></head></html>`
	r, err := jsonld.ExtractRecipe(html)
	require.NoError(t, err)
	assert.Equal(t, "Gebratener Reis mit Ei, Gemüse & Hähnchen", r.Name)
	assert.Equal(t, `Schnell & einfach "wie vom Wok"`, r.Description)
	assert.Equal(t, []string{"100 g Erbsen & Möhren", "1 EL Sojasoße"}, r.RecipeIngredient)
	assert.Equal(t, []string{"Rühren & braten."}, r.RecipeInstructions)
	assert.Contains(t, r.Keywords, "Reis & Ei")
}
