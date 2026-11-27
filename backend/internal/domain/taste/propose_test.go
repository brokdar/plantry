package taste_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaltszeimer/plantry/backend/internal/domain/taste"
)

// COLD-1: a free-text answer becomes statements to confirm: loves, rather
// nots (a negative word in the clause), a health word after "allergic".
func TestPropose(t *testing.T) {
	got := taste.Propose("We love thai and pasta, but no coriander. Our kid is allergic to peanuts; mom is celiac")
	want := []taste.Proposal{
		{Kind: "taste", Level: taste.LevelCuisine, Key: "thai", Polarity: taste.Love},
		{Kind: "taste", Level: taste.LevelHero, Key: "pasta", Polarity: taste.Love},
		{Kind: "taste", Level: taste.LevelHero, Key: "coriander", Polarity: taste.RatherNot},
		{Kind: "health", Word: "peanuts"},
		{Kind: "health", Word: "gluten"},
	}
	assert.Equal(t, want, got)
	assert.Empty(t, taste.Propose("  everything, really "))
}
