package main

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/flavor"
	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
)

func list(v ...string) func(context.Context) ([]string, error) {
	return func(context.Context) ([]string, error) { return v, nil }
}

// FIND-15, TASTE, CP3 run 3 item 6: the household's cuisines come best
// first: its loves, its staples' cuisines, Cuisines to explore, its library's
// (most recipes first) and the locale's home cuisine last, each once.
func TestHouseholdCuisines_LovesFirstLocaleLast(t *testing.T) {
	prof := func(context.Context) (*profile.Profile, error) { return &profile.Profile{Locale: "de-DE"}, nil }
	got, err := householdCuisines(prof, list("indian"), list("italian", "indian"), list("thai"), list("german", "italian", "french"))(context.Background())
	require.NoError(t, err)
	assert.Equal(t, []string{"indian", "italian", "thai", "german", "french"}, got)
}

// THEME-1: the staples' cuisines, the most staples first, neutral left out.
func TestStapleCuisines_MostStaplesFirst(t *testing.T) {
	staples := func(context.Context, time.Time) (map[int64]bool, error) {
		return map[int64]bool{1: true, 2: true, 3: true, 4: true}, nil
	}
	attrs := func(_ context.Context, ids []int64) (map[int64]flavor.Record, error) {
		require.Equal(t, []int64{1, 2, 3, 4}, ids)
		c := map[int64]string{1: "italian", 2: "mexican", 3: "mexican", 4: "neutral"}
		out := map[int64]flavor.Record{}
		for id, v := range c {
			out[id] = flavor.Record{Values: flavor.Attributes{Cuisine: v}}
		}
		return out, nil
	}
	got, err := stapleCuisines(staples, attrs, time.Now)(context.Background())
	require.NoError(t, err)
	assert.Equal(t, []string{"mexican", "italian"}, got)
}
