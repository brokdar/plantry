package importer_test

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/jsonld"
	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/importer"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
)

type finalizeEnv struct {
	foods    *food.Service
	importer *importer.Service
}

func newFinalizeEnv(t *testing.T) finalizeEnv {
	t.Helper()
	db := testhelper.NewTestDB(t)
	foods := food.NewService(sqlite.NewFoodRepo(db))
	return finalizeEnv{
		foods:    foods,
		importer: importer.NewService(nil, jsonld.Extractor{}, nil, nil, foods),
	}
}

func (e finalizeEnv) leaf(t *testing.T, name string) int64 {
	t.Helper()
	src := food.SourceManual
	f := &food.Food{Kind: food.KindLeaf, Name: name, Source: &src}
	require.NoError(t, e.foods.Create(context.Background(), f))
	return f.ID
}

func (e finalizeEnv) portions(t *testing.T, id int64) []food.Portion {
	t.Helper()
	ps, err := e.foods.ListPortions(context.Background(), id)
	require.NoError(t, err)
	return ps
}

// importChili runs the whole import of the chefkoch fixture the way the
// wizard does: extract, link every row to a leaf by name, finalize, create.
func (e finalizeEnv) importChili(t *testing.T, leaves map[string]int64) *food.Food {
	t.Helper()
	ctx := context.Background()
	html, err := os.ReadFile("testdata/chefkoch_chili.html")
	require.NoError(t, err)
	draft, err := e.importer.Extract(ctx, importer.ExtractInput{HTML: string(html)})
	require.NoError(t, err)

	in := importer.FinalizeInput{Name: draft.Name, Role: "main", ReferencePortions: draft.ReferencePortions}
	for _, ing := range draft.Ingredients {
		id, ok := leaves[ing.Name]
		require.True(t, ok, "no leaf for %q", ing.Name)
		in.Children = append(in.Children, importer.FinalizedChild{
			Resolution: importer.ResolutionExisting, FoodID: id, Amount: ing.Amount, Unit: ing.Unit,
		})
	}
	out, err := e.importer.Finalize(ctx, in)
	require.NoError(t, err)

	f := &food.Food{Kind: food.KindComposed, Name: out.Name, ReferencePortions: &out.ReferencePortions}
	role := food.Role(out.Role)
	f.Role = &role
	for _, ch := range out.Children {
		f.Children = append(f.Children, food.FoodComponent{
			ChildID: ch.ChildID, Amount: ch.Amount, Unit: ch.Unit, Grams: ch.Grams, SortOrder: ch.SortOrder,
		})
	}
	require.NoError(t, e.foods.Create(ctx, f))
	got, err := e.foods.Get(ctx, f.ID)
	require.NoError(t, err)
	return got
}

// B5-AC3
func TestImport_CountUnitsResolveThroughPortions(t *testing.T) {
	env := newFinalizeEnv(t)
	ctx := context.Background()
	beans := env.leaf(t, "Kidneybohnen")
	onion := env.leaf(t, "Zwiebel")
	garlic := env.leaf(t, "Knoblauch")
	// Garlic already knows its clove weight; onion has no portions.
	require.NoError(t, env.foods.UpsertPortion(ctx, &food.Portion{FoodID: garlic, Unit: "clove", Grams: 4}))
	leaves := map[string]int64{"Kidneybohnen": beans, "Zwiebel": onion, "Knoblauch": garlic}

	chili := env.importChili(t, leaves)

	byChild := map[int64]food.FoodComponent{}
	for _, c := range chili.Children {
		byChild[c.ChildID] = c
	}
	assert.Equal(t, "g", byChild[beans].Unit)
	assert.InDelta(t, 400, byChild[beans].Grams, 1e-9)
	assert.Equal(t, "piece", byChild[onion].Unit)
	assert.InDelta(t, 1, byChild[onion].Amount, 1e-9)
	assert.InDelta(t, 120, byChild[onion].Grams, 1e-9, "1 piece × the units.CountGrams default")
	assert.Equal(t, "clove", byChild[garlic].Unit)
	assert.InDelta(t, 2, byChild[garlic].Amount, 1e-9)
	assert.InDelta(t, 8, byChild[garlic].Grams, 1e-9, "2 cloves × the leaf's own 4 g portion")

	assert.Equal(t, []food.Portion{{FoodID: onion, Unit: "piece", Grams: 120}}, env.portions(t, onion),
		"the onion lacked a piece portion, so the import creates it")
	assert.Equal(t, []food.Portion{{FoodID: garlic, Unit: "clove", Grams: 4}}, env.portions(t, garlic),
		"the garlic portion is kept as it was")
	assert.Empty(t, env.portions(t, beans))

	// Importing the same recipe again creates no duplicate portions.
	env.importChili(t, leaves)
	assert.Len(t, env.portions(t, onion), 1)
	assert.Len(t, env.portions(t, garlic), 1)
}

func TestFinalize_CountUnitPortionGrams(t *testing.T) {
	ctx := context.Background()
	grams := func(v float64) *float64 { return &v }

	tests := []struct {
		name         string
		existing     *food.Portion
		portionGrams *float64
		unit         string
		wantGrams    float64
		wantPortions []food.Portion
		wantErr      error
	}{
		{
			name:         "edited estimate creates the portion",
			portionGrams: grams(150),
			unit:         "piece",
			wantGrams:    300,
			wantPortions: []food.Portion{{Unit: "piece", Grams: 150}},
		},
		{
			name:         "edited grams update an existing portion",
			existing:     &food.Portion{Unit: "piece", Grams: 100},
			portionGrams: grams(90),
			unit:         "piece",
			wantGrams:    180,
			wantPortions: []food.Portion{{Unit: "piece", Grams: 90}},
		},
		{
			name:         "a portion stored under an alias is matched, not duplicated",
			existing:     &food.Portion{Unit: "Stück", Grams: 110},
			unit:         "piece",
			wantGrams:    220,
			wantPortions: []food.Portion{{Unit: "Stück", Grams: 110}},
		},
		{
			name:         "unit alias in the request is normalised",
			unit:         "Zehen",
			wantGrams:    10,
			wantPortions: []food.Portion{{Unit: "clove", Grams: 5}},
		},
		{
			name:         "non-positive portion grams are rejected",
			portionGrams: grams(0),
			unit:         "piece",
			wantErr:      domain.ErrImportInvalidResolution,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := newFinalizeEnv(t)
			id := env.leaf(t, "Ei")
			if tt.existing != nil {
				p := *tt.existing
				p.FoodID = id
				require.NoError(t, env.foods.UpsertPortion(ctx, &p))
			}
			out, err := env.importer.Finalize(ctx, importer.FinalizeInput{
				Name: "Omelett", Role: "main", ReferencePortions: 1,
				Children: []importer.FinalizedChild{{
					Resolution: importer.ResolutionExisting, FoodID: id, Amount: 2,
					Unit: tt.unit, PortionGrams: tt.portionGrams,
				}},
			})
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				assert.Empty(t, env.portions(t, id))
				return
			}
			require.NoError(t, err)
			require.Len(t, out.Children, 1)
			assert.InDelta(t, tt.wantGrams, out.Children[0].Grams, 1e-9)
			assert.Equal(t, food.GramsSourcePortion, out.Children[0].GramsSource)
			for i := range tt.wantPortions {
				tt.wantPortions[i].FoodID = id
			}
			assert.Equal(t, tt.wantPortions, env.portions(t, id))
		})
	}
}

func TestFinalize_MassAndVolumeUnchanged(t *testing.T) {
	env := newFinalizeEnv(t)
	ctx := context.Background()
	flour := env.leaf(t, "Mehl")
	milk := env.leaf(t, "Milch")

	out, err := env.importer.Finalize(ctx, importer.FinalizeInput{
		Name: "Teig", Role: "main", ReferencePortions: 2,
		Children: []importer.FinalizedChild{
			{Resolution: importer.ResolutionExisting, FoodID: flour, Amount: 200, Unit: "g"},
			{Resolution: importer.ResolutionExisting, FoodID: milk, Amount: 250, Unit: "ml"},
		},
	})
	require.NoError(t, err)
	require.Len(t, out.Children, 2)
	assert.InDelta(t, 200, out.Children[0].Grams, 1e-9)
	assert.Equal(t, food.GramsSourceDirect, out.Children[0].GramsSource)
	assert.InDelta(t, 250, out.Children[1].Grams, 1e-9)
	assert.Equal(t, food.GramsSourceFallback, out.Children[1].GramsSource)
	assert.Empty(t, env.portions(t, flour))
	assert.Empty(t, env.portions(t, milk))
}
