package theme_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/flavor"
	"github.com/jaltszeimer/plantry/backend/internal/domain/theme"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
)

var start = time.Date(2026, 11, 28, 0, 0, 0, 0, time.UTC)

type env struct {
	svc     *theme.Service
	now     time.Time
	chefOn  bool
	library int
	found   []string
}

func newEnv(t *testing.T) *env {
	t.Helper()
	e := &env{now: start.AddDate(0, 0, -3), chefOn: true}
	e.svc = theme.NewService(theme.Deps{
		Store:  sqlite.NewThemeRepo(testhelper.NewTestDB(t)),
		ChefOn: func(context.Context) (bool, error) { return e.chefOn, nil },
		Library: func(context.Context, theme.Theme) (int, error) {
			return e.library, nil
		},
		Find: func(_ context.Context, v string, n int) error {
			for range n {
				e.found = append(e.found, v)
			}
			return nil
		},
		Cuisines: func(context.Context) ([]string, error) { return []string{"Japanese", "thai"}, nil },
		Now:      func() time.Time { return e.now },
	})
	return e
}

// A theme starts the finds the library lacks the same day and says how many
// (THEME-3, FIND-1d); at most two per window (THEME-2); the window after
// offers it again (THEME-5).
func TestSet_FindsLimitAndAgain(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := context.Background()

	r, err := e.svc.Set(ctx, start, theme.Theme{Kind: theme.KindCuisine, Value: " Japanese ", Source: theme.SourceUser})
	require.NoError(t, err)
	assert.Equal(t, "japanese", r.Theme.Value)
	assert.Equal(t, 0, r.Library)
	assert.Equal(t, 2, r.Finding, "No Japanese recipes yet. I'll find 2.")
	assert.Equal(t, []string{"japanese", "japanese"}, e.found)

	again, err := e.svc.Set(ctx, start, theme.Theme{Kind: theme.KindCuisine, Value: "japanese", Source: theme.SourceUser})
	require.NoError(t, err)
	assert.Equal(t, r.Theme.ID, again.Theme.ID, "the same theme twice is one")
	assert.Len(t, e.found, 2, "no second search")

	e.library = 5
	r2, err := e.svc.Set(ctx, start, theme.Theme{Kind: theme.KindMood, Value: "one-pot", Source: theme.SourceChat})
	require.NoError(t, err)
	assert.Zero(t, r2.Finding, "the library supplies it")
	_, err = e.svc.Set(ctx, start, theme.Theme{Kind: theme.KindIngredient, Value: "pumpkin", Source: theme.SourceUser})
	require.ErrorIs(t, err, domain.ErrThemeLimit)
	_, err = e.svc.Set(ctx, start, theme.Theme{Kind: "colour", Value: "red"})
	require.ErrorIs(t, err, domain.ErrInvalidInput)

	n, err := e.svc.NewDishes(ctx, start)
	require.NoError(t, err)
	assert.Equal(t, 0, n, "library 5 for both themes")

	next := start.AddDate(0, 0, 7)
	w, err := e.svc.Window(ctx, next)
	require.NoError(t, err)
	assert.Empty(t, w.Themes, "a theme ends with its window")
	require.Len(t, w.Again, 2)
	assert.Nil(t, w.Suggestion, "again? is the window's one offer")

	e.now = next.AddDate(0, 0, 7)
	_, err = e.svc.Set(ctx, next, theme.Theme{Kind: theme.KindCuisine, Value: "thai"})
	require.ErrorIs(t, err, domain.ErrInvalidInput, "an ended window takes no theme")
}

// With the Chef off a theme starts no finds and adds no new dishes.
func TestSet_ChefOff(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	e.chefOn = false
	ctx := context.Background()
	r, err := e.svc.Set(ctx, start, theme.Theme{Kind: theme.KindCuisine, Value: "japanese", Source: theme.SourceUser})
	require.NoError(t, err)
	assert.Zero(t, r.Finding)
	assert.Empty(t, e.found)
	n, err := e.svc.NewDishes(ctx, start)
	require.NoError(t, err)
	assert.Zero(t, n)
	w, err := e.svc.Window(ctx, start.AddDate(0, 0, 7))
	require.NoError(t, err)
	assert.Nil(t, w.Suggestion)
}

// At most one suggestion per window, never applied by itself; two refusals
// in a row pause suggestions for 4 weeks (THEME-4).
func TestSuggestion_OnePerWindowAndPause(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := context.Background()

	w, err := e.svc.Window(ctx, start)
	require.NoError(t, err)
	require.NotNil(t, w.Suggestion)
	assert.Equal(t, theme.Suggestion{Kind: theme.KindCuisine, Value: "japanese", State: theme.StateOpen}, *w.Suggestion)
	assert.Empty(t, w.Themes, "never applied by itself")
	w2, err := e.svc.Window(ctx, start)
	require.NoError(t, err)
	assert.Equal(t, w.Suggestion, w2.Suggestion, "the same one on every read")

	require.NoError(t, e.svc.Decline(ctx, start))
	w, err = e.svc.Window(ctx, start)
	require.NoError(t, err)
	assert.Nil(t, w.Suggestion, "one per window")
	require.ErrorIs(t, e.svc.Decline(ctx, start), domain.ErrNotFound)

	second := start.AddDate(0, 0, 7)
	e.now = second.AddDate(0, 0, -1)
	w, err = e.svc.Window(ctx, second)
	require.NoError(t, err)
	require.NotNil(t, w.Suggestion)
	assert.Equal(t, "thai", w.Suggestion.Value, "a recent value is not suggested again")
	require.NoError(t, e.svc.Decline(ctx, second))

	for i := 2; i <= 4; i++ { // paused for 4 weeks after the second refusal
		ws := start.AddDate(0, 0, 7*i)
		e.now = second.AddDate(0, 0, 7*(i-1)-1)
		w, err = e.svc.Window(ctx, ws)
		require.NoError(t, err)
		assert.Nilf(t, w.Suggestion, "window %d paused", i)
	}
	ws := start.AddDate(0, 0, 7*5)
	e.now = ws.AddDate(0, 0, -1)
	w, err = e.svc.Window(ctx, ws)
	require.NoError(t, err)
	require.NotNil(t, w.Suggestion, "the pause ends after 4 weeks")

	r, err := e.svc.Accept(ctx, ws)
	require.NoError(t, err)
	assert.Equal(t, theme.SourceSuggestion, r.Theme.Source)
	w, err = e.svc.Window(ctx, ws)
	require.NoError(t, err)
	assert.Len(t, w.Themes, 1)
	assert.Nil(t, w.Suggestion)
}

// A theme fits a recipe by its kind's attributes or tags.
func TestMatches(t *testing.T) {
	t.Parallel()
	a := flavor.Attributes{Cuisine: "middle_eastern", Heroes: []string{"chickpea"}, Format: "bowl"}
	assert.True(t, theme.Theme{Kind: theme.KindCuisine, Value: "middle eastern"}.Matches(a, nil))
	assert.False(t, theme.Theme{Kind: theme.KindCuisine, Value: "japanese"}.Matches(a, nil))
	assert.True(t, theme.Theme{Kind: theme.KindIngredient, Value: "chickpea"}.Matches(a, nil))
	assert.True(t, theme.Theme{Kind: theme.KindIngredient, Value: "autumn"}.Matches(a, []string{"Autumn"}))
	assert.True(t, theme.Theme{Kind: theme.KindMood, Value: "bowl"}.Matches(a, nil))
	assert.False(t, theme.Theme{Kind: theme.KindMood, Value: "chickpea"}.Matches(a, nil))
}
