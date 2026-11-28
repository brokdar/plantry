package libreview_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/libreview"
)

type memStore struct {
	state    libreview.State
	recipes  []libreview.Recipe
	replaced map[int64]int
	gaps     []libreview.Gap
	grouped  [][2]int64
	computed int
}

func (m *memStore) State(context.Context) (libreview.State, error) { return m.state, nil }
func (m *memStore) SaveState(_ context.Context, s libreview.State) error {
	m.state = s
	return nil
}

func (m *memStore) Recipes(context.Context) ([]libreview.Recipe, error) {
	m.computed++
	return m.recipes, nil
}

func (m *memStore) Replaced(context.Context, time.Time) (map[int64]int, error) {
	return m.replaced, nil
}

func (m *memStore) QueueGap(_ context.Context, g libreview.Gap, _ time.Time) error {
	m.gaps = append(m.gaps, g)
	return nil
}

func (m *memStore) GroupVariants(_ context.Context, a, b int64) error {
	m.grouped = append(m.grouped, [2]int64{a, b})
	return nil
}

type passTx struct{}

func (passTx) InTx(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) }

type rig struct {
	*memStore
	svc *libreview.Service
	now time.Time
	on  bool
}

func newRig(recipes []libreview.Recipe, replaced map[int64]int) *rig {
	r := &rig{memStore: &memStore{recipes: recipes, replaced: replaced}, now: time.Date(2026, 11, 28, 9, 0, 0, 0, time.UTC), on: true}
	r.svc = libreview.NewService(libreview.Deps{
		Store: r.memStore, Tx: passTx{}, Enabled: func(context.Context) (bool, error) { return r.on, nil }, Now: func() time.Time { return r.now },
	})
	return r
}

func rec(id int64, name string, ingredients ...int64) libreview.Recipe {
	return libreview.Recipe{ID: id, Name: name, Role: "main", Prep: 20, Cook: 20, Steps: 3, Ingredients: ingredients}
}

func kinds(fs []libreview.Finding) []libreview.Kind {
	var out []libreview.Kind
	for _, f := range fs {
		out = append(out, f.Kind)
	}
	return out
}

// Each finding kind comes from its rule: same words in the name or nearly the
// same ingredients are duplicates (not when already variants), a recipe taken
// off plans three times is replaced, missing times or steps are data.
func TestFindings(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	get := func(t *testing.T, recipes []libreview.Recipe, replaced map[int64]int) []libreview.Finding {
		t.Helper()
		card, err := newRig(recipes, replaced).svc.Get(ctx)
		require.NoError(t, err)
		if card == nil {
			return nil
		}
		return card.Findings
	}

	t.Run("duplicates", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name    string
			recipes []libreview.Recipe
			want    bool
		}{
			{"same name different case and punctuation", []libreview.Recipe{rec(1, "Chili con Carne"), rec(2, "chili con  carne!")}, true},
			{"words in another order", []libreview.Recipe{rec(1, "Bolognese Spaghetti"), rec(2, "Spaghetti Bolognese")}, true},
			{"name one word apart of four", []libreview.Recipe{rec(1, "Creamy Mushroom Pasta Bake"), rec(2, "Creamy Mushroom Pasta Bakes")}, false},
			{"name shares three of four words", []libreview.Recipe{rec(1, "Creamy Mushroom Pasta Bake"), rec(2, "Creamy Mushroom Pasta")}, true},
			{"near-identical ingredients, five of six shared", []libreview.Recipe{rec(1, "Soup", 1, 2, 3, 4, 5), rec(2, "Stew", 1, 2, 3, 4, 5, 6)}, true},
			{"half the ingredients shared", []libreview.Recipe{rec(1, "Soup", 1, 2, 3, 4), rec(2, "Stew", 1, 2, 5, 6)}, false},
			{"identical ingredients", []libreview.Recipe{rec(1, "Soup", 1, 2, 3, 4), rec(2, "Stew", 1, 2, 3, 4)}, true},
			{"different recipes", []libreview.Recipe{rec(1, "Soup", 1, 2, 3), rec(2, "Stew", 4, 5, 6)}, false},
			{"too few ingredients to compare", []libreview.Recipe{rec(1, "Soup", 1, 2), rec(2, "Stew", 1, 2)}, false},
			{"already variants", []libreview.Recipe{withGroup(rec(1, "Curry"), 5), withGroup(rec(2, "Curry"), 5)}, false},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				fs := get(t, tt.recipes, nil)
				if !tt.want {
					for _, f := range fs {
						assert.NotEqual(t, libreview.KindDuplicates, f.Kind)
					}
					return
				}
				require.NotEmpty(t, fs)
				assert.Equal(t, libreview.KindDuplicates, fs[0].Kind)
				assert.Equal(t, "dup:1-2", fs[0].Key)
				assert.Equal(t, []int64{1, 2}, []int64{fs[0].Recipes[0].ID, fs[0].Recipes[1].ID})
			})
		}
	})

	t.Run("replaced needs three recent removals", func(t *testing.T) {
		t.Parallel()
		recipes := []libreview.Recipe{rec(1, "Pizza"), rec(2, "Soup", 1), rec(3, "Stew", 2)}
		fs := get(t, recipes, map[int64]int{1: 4, 2: 2, 3: 3})
		require.Len(t, fs, 2)
		assert.Equal(t, "replaced:1", fs[0].Key, "the most often first")
		assert.Equal(t, 4, fs[0].Count)
		assert.Equal(t, "replaced:3", fs[1].Key)
		assert.Empty(t, get(t, recipes, map[int64]int{1: 2}))
	})

	t.Run("data lists recipes lacking times or steps", func(t *testing.T) {
		t.Parallel()
		noTimes, noSteps, neither := rec(1, "A"), rec(2, "B"), rec(3, "C")
		noTimes.Prep, noTimes.Cook = 0, 0
		noSteps.Steps = 0
		neither.Prep, neither.Cook, neither.Steps = 0, 0, 0
		fs := get(t, []libreview.Recipe{noTimes, noSteps, neither, rec(4, "D")}, nil)
		require.Len(t, fs, 1)
		assert.Equal(t, libreview.KindData, fs[0].Kind)
		assert.Equal(t, 3, fs[0].Count)
		assert.Equal(t, []libreview.RecipeRef{
			{ID: 3, Name: "C", Missing: []string{"times", "steps"}},
			{ID: 1, Name: "A", Missing: []string{"times"}},
			{ID: 2, Name: "B", Missing: []string{"steps"}},
		}, fs[0].Recipes)
	})

	t.Run("nothing to say, no card", func(t *testing.T) {
		t.Parallel()
		assert.Empty(t, get(t, []libreview.Recipe{rec(1, "A"), rec(2, "B", 1)}, nil))
	})
}

func withGroup(r libreview.Recipe, g int64) libreview.Recipe { r.VariantGroup = g; return r }

// A card holds at most three findings, one of each kind in turn.
func TestPick_AtMostThree(t *testing.T) {
	t.Parallel()
	mk := func(k libreview.Kind, n int) []libreview.Finding {
		var out []libreview.Finding
		for i := 0; i < n; i++ {
			out = append(out, libreview.Finding{Key: string(k) + string(rune('a'+i)), Kind: k})
		}
		return out
	}
	assert.Equal(t, []libreview.Kind{"duplicates", "replaced", "data"}, kinds(libreview.Pick(mk("duplicates", 3), mk("replaced", 3), mk("data", 1))))
	assert.Equal(t, []libreview.Kind{"duplicates", "data", "duplicates"}, kinds(libreview.Pick(mk("duplicates", 3), nil, mk("data", 1))))
	assert.Equal(t, []libreview.Kind{"replaced", "replaced", "replaced"}, kinds(libreview.Pick(nil, mk("replaced", 5), nil)))
	assert.Empty(t, libreview.Pick())
}

// The card shows once a month: a later look in the month reads the stored
// findings, the next month computes anew; off shows nothing and computes
// nothing.
func TestService_MonthlyLimit(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	r := newRig([]libreview.Recipe{rec(1, "Pizza"), rec(2, "Soup", 1)}, map[int64]int{1: 3})

	r.on = false
	card, err := r.svc.Get(ctx)
	require.NoError(t, err)
	assert.Nil(t, card)
	assert.Zero(t, r.computed, "off: no rules run")

	r.on = true
	card, err = r.svc.Get(ctx)
	require.NoError(t, err)
	require.NotNil(t, card)
	assert.Equal(t, "2026-11", card.Month)
	require.Len(t, card.Findings, 1)

	r.recipes = nil // the library changed: this month's card stays as computed
	r.now = r.now.AddDate(0, 0, 1)
	card, err = r.svc.Get(ctx)
	require.NoError(t, err)
	require.NotNil(t, card)
	assert.Equal(t, 1, r.computed)

	require.NoError(t, r.svc.Dismiss(ctx, ""))
	card, err = r.svc.Get(ctx)
	require.NoError(t, err)
	assert.Nil(t, card, "a dismissed card stays away for the month")
	assert.Equal(t, 1, r.computed)

	r.now = time.Date(2026, 12, 1, 8, 0, 0, 0, time.UTC)
	r.recipes = []libreview.Recipe{rec(1, "Pizza"), rec(2, "Soup", 1)}
	card, err = r.svc.Get(ctx)
	require.NoError(t, err)
	require.NotNil(t, card, "next month: a new card")
	assert.Equal(t, "2026-12", card.Month)
	assert.Equal(t, 2, r.computed)
}

// Dismissing every finding takes the card away; make-variants groups the two
// recipes and drops its finding; an unknown or wrong-kind key is not found.
func TestService_DismissAndVariants(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	r := newRig([]libreview.Recipe{rec(1, "Curry"), rec(2, "curry"), rec(3, "Pizza"), rec(4, "Soup", 1)}, map[int64]int{3: 5})
	card, err := r.svc.Get(ctx)
	require.NoError(t, err)
	require.Len(t, card.Findings, 2)

	require.ErrorIs(t, r.svc.Dismiss(ctx, "nope"), domain.ErrNotFound)
	require.True(t, errors.Is(r.svc.MakeVariants(ctx, "replaced:3"), domain.ErrNotFound), "only duplicates make variants")
	require.NoError(t, r.svc.MakeVariants(ctx, "dup:1-2"))
	assert.Equal(t, [][2]int64{{1, 2}}, r.grouped)
	card, _ = r.svc.Get(ctx)
	require.Len(t, card.Findings, 1)
	assert.Equal(t, "replaced:3", card.Findings[0].Key)

	require.NoError(t, r.svc.Dismiss(ctx, "replaced:3"))
	card, err = r.svc.Get(ctx)
	require.NoError(t, err)
	assert.Nil(t, card)
}

// Gaps the assistant can fill are queued when the card is computed: no quick
// main, fewer than two mains; a small library is the cold start.
func TestGapsQueued(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	slow := func(id int64, role string) libreview.Recipe {
		r := rec(id, "Dish "+string(rune('A'+id)), id*10)
		r.Role = role
		return r
	}
	quick := rec(9, "Quick", 90)
	quick.Prep, quick.Cook = 10, 15

	tests := []struct {
		name    string
		recipes []libreview.Recipe
		want    []string
	}{
		{"small library: cold start, no gap", []libreview.Recipe{slow(1, "main")}, nil},
		{"one slow main", []libreview.Recipe{slow(1, "main"), slow(2, "side_veg"), slow(3, "side_veg"), slow(4, "sauce"), slow(5, "dessert")}, []string{"few_mains", "quick_main"}},
		{"two slow mains", []libreview.Recipe{slow(1, "main"), slow(2, "standalone"), slow(3, "side_veg"), slow(4, "sauce"), slow(5, "dessert")}, []string{"quick_main"}},
		{"a quick main and another", []libreview.Recipe{quick, slow(2, "main"), slow(3, "side_veg"), slow(4, "sauce"), slow(5, "dessert")}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			r := newRig(tt.recipes, nil)
			_, err := r.svc.Get(ctx)
			require.NoError(t, err)
			var got []string
			for _, g := range r.gaps {
				got = append(got, g.Kind)
				assert.NotEmpty(t, g.Dish)
			}
			assert.Equal(t, tt.want, got)
		})
	}

	t.Run("not queued when off", func(t *testing.T) {
		t.Parallel()
		r := newRig([]libreview.Recipe{slow(1, "main")}, nil)
		r.on = false
		_, err := r.svc.Get(ctx)
		require.NoError(t, err)
		assert.Empty(t, r.gaps)
	})
}
