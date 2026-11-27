package main

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
)

func list(v ...string) func(context.Context) ([]string, error) {
	return func(context.Context) ([]string, error) { return v, nil }
}

// FIND-15: the household's cuisines are its library's, its loves, Cuisines
// to explore and the locale's home cuisine.
func TestHouseholdCuisines_LibraryLovesExploreLocale(t *testing.T) {
	prof := func(context.Context) (*profile.Profile, error) { return &profile.Profile{Locale: "de-DE"}, nil }
	got, err := householdCuisines(prof, list("italian", "indian"), list("thai"), list())(context.Background())
	require.NoError(t, err)
	assert.Equal(t, []string{"german", "indian", "italian", "thai"}, got)
}

// FIND-15, COLD-4: a planning find's dish rotates through Cuisines to explore
// once the household has cooked; without any it falls back to the
// household's cuisines (CP3 dry run: an empty list gave no dish and a 400),
// and week one stays within the household's cuisines.
func TestPlanningDish_Scope(t *testing.T) {
	day := time.Date(2026, 11, 7, 0, 0, 0, 0, time.UTC)
	cooked := func(v bool) func(context.Context) (bool, error) {
		return func(context.Context) (bool, error) { return v, nil }
	}
	tests := []struct {
		name    string
		cooked  bool
		house   []string
		explore []string
		want    string
	}{
		{"cooked, explore set", true, []string{"italian"}, []string{"thai"}, "thai"},
		{"cooked, no explore", true, []string{"italian"}, nil, "italian"},
		{"week one", false, []string{"italian"}, []string{"thai"}, "italian"},
		{"nothing in scope", true, nil, nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := planningDish(cooked(tt.cooked), list(tt.house...), list(tt.explore...))(context.Background(), day)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
