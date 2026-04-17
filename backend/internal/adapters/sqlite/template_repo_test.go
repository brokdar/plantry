package sqlite_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/component"
	"github.com/jaltszeimer/plantry/backend/internal/domain/ingredient"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/template"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
)

type templateFixture struct {
	db         *sql.DB
	templates  *sqlite.TemplateRepo
	component  *component.Component
	component2 *component.Component
}

func setupTemplateFixture(t *testing.T) *templateFixture {
	t.Helper()
	db := testhelper.NewTestDB(t)
	templateRepo := sqlite.NewTemplateRepo(db)
	ingRepo := sqlite.NewIngredientRepo(db)
	compRepo := sqlite.NewComponentRepo(db)

	ctx := context.Background()
	ing := &ingredient.Ingredient{Name: "Chicken", Source: "manual", Kcal100g: 100}
	require.NoError(t, ingRepo.Create(ctx, ing))
	c := &component.Component{
		Name: "Curry", Role: component.RoleMain, ReferencePortions: 2,
		Ingredients: []component.ComponentIngredient{
			{IngredientID: ing.ID, Amount: 100, Unit: "g", Grams: 100},
		},
	}
	require.NoError(t, compRepo.Create(ctx, c))

	ing2 := &ingredient.Ingredient{Name: "Rice", Source: "manual", Kcal100g: 130}
	require.NoError(t, ingRepo.Create(ctx, ing2))
	c2 := &component.Component{
		Name: "Rice", Role: component.RoleSideStarch, ReferencePortions: 2,
		Ingredients: []component.ComponentIngredient{
			{IngredientID: ing2.ID, Amount: 100, Unit: "g", Grams: 100},
		},
	}
	require.NoError(t, compRepo.Create(ctx, c2))

	return &templateFixture{db: db, templates: templateRepo, component: c, component2: c2}
}

func TestTemplateRepo_RoundTrip(t *testing.T) {
	f := setupTemplateFixture(t)
	ctx := context.Background()

	tpl := &template.Template{Name: "Curry Night", Components: []template.TemplateComponent{
		{ComponentID: f.component.ID, Portions: 2, SortOrder: 0},
		{ComponentID: f.component2.ID, Portions: 1, SortOrder: 1},
	}}
	require.NoError(t, f.templates.Create(ctx, tpl))
	assert.NotZero(t, tpl.ID)
	assert.False(t, tpl.CreatedAt.IsZero())
	require.Len(t, tpl.Components, 2)
	assert.NotZero(t, tpl.Components[0].ID)

	got, err := f.templates.Get(ctx, tpl.ID)
	require.NoError(t, err)
	assert.Equal(t, "Curry Night", got.Name)
	require.Len(t, got.Components, 2)

	updated, err := f.templates.UpdateName(ctx, tpl.ID, "New Name")
	require.NoError(t, err)
	assert.Equal(t, "New Name", updated.Name)

	require.NoError(t, f.templates.Delete(ctx, tpl.ID))
	_, err = f.templates.Get(ctx, tpl.ID)
	assert.True(t, errors.Is(err, domain.ErrNotFound))
}

func TestTemplateRepo_DeleteCascadesComponents(t *testing.T) {
	f := setupTemplateFixture(t)
	ctx := context.Background()

	tpl := &template.Template{Name: "X", Components: []template.TemplateComponent{
		{ComponentID: f.component.ID, Portions: 1, SortOrder: 0},
	}}
	require.NoError(t, f.templates.Create(ctx, tpl))

	require.NoError(t, f.templates.Delete(ctx, tpl.ID))

	// listing children should return an empty slice — cascade worked.
	children, err := f.templates.ListComponentsByTemplate(ctx, tpl.ID)
	require.NoError(t, err)
	assert.Empty(t, children)
}

func TestTemplateRepo_ReplaceComponents_IsAtomic(t *testing.T) {
	f := setupTemplateFixture(t)
	ctx := context.Background()

	tpl := &template.Template{Name: "X", Components: []template.TemplateComponent{
		{ComponentID: f.component.ID, Portions: 1, SortOrder: 0},
	}}
	require.NoError(t, f.templates.Create(ctx, tpl))

	require.NoError(t, f.templates.ReplaceComponents(ctx, tpl.ID, []template.TemplateComponent{
		{ComponentID: f.component.ID, Portions: 3, SortOrder: 0},
		{ComponentID: f.component2.ID, Portions: 2, SortOrder: 1},
	}))

	got, err := f.templates.ListComponentsByTemplate(ctx, tpl.ID)
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.InDelta(t, 3.0, got[0].Portions, 1e-9)
	assert.Equal(t, f.component2.ID, got[1].ComponentID)
}

func TestTemplateRepo_CountUsingComponent(t *testing.T) {
	f := setupTemplateFixture(t)
	ctx := context.Background()

	tpl1 := &template.Template{Name: "A", Components: []template.TemplateComponent{
		{ComponentID: f.component.ID, Portions: 1, SortOrder: 0},
	}}
	tpl2 := &template.Template{Name: "B", Components: []template.TemplateComponent{
		{ComponentID: f.component.ID, Portions: 2, SortOrder: 0},
	}}
	require.NoError(t, f.templates.Create(ctx, tpl1))
	require.NoError(t, f.templates.Create(ctx, tpl2))

	n, err := f.templates.CountUsingComponent(ctx, f.component.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(2), n)
}

func TestTemplateRepo_List(t *testing.T) {
	f := setupTemplateFixture(t)
	ctx := context.Background()

	require.NoError(t, f.templates.Create(ctx, &template.Template{Name: "Z"}))
	require.NoError(t, f.templates.Create(ctx, &template.Template{Name: "A", Components: []template.TemplateComponent{
		{ComponentID: f.component.ID, Portions: 1, SortOrder: 0},
	}}))

	got, err := f.templates.List(ctx)
	require.NoError(t, err)
	require.Len(t, got, 2)
	// ORDER BY name asc.
	assert.Equal(t, "A", got[0].Name)
	require.Len(t, got[0].Components, 1)
}

func TestTemplateRepo_ComponentFKRestrict(t *testing.T) {
	f := setupTemplateFixture(t)
	ctx := context.Background()

	tpl := &template.Template{Name: "X", Components: []template.TemplateComponent{
		{ComponentID: f.component.ID, Portions: 1, SortOrder: 0},
	}}
	require.NoError(t, f.templates.Create(ctx, tpl))

	compRepo := sqlite.NewComponentRepo(f.db)
	err := compRepo.Delete(ctx, f.component.ID)
	// component is referenced only by a template; delete must still be blocked.
	require.Error(t, err)
}

func TestTxRunner_RunInTemplateTx(t *testing.T) {
	f := setupTemplateFixture(t)
	ctx := context.Background()
	runner := sqlite.NewTxRunner(f.db)

	err := runner.RunInTemplateTx(ctx, func(tr template.Repository, _ plate.Repository) error {
		return tr.Create(ctx, &template.Template{Name: "Tx-scoped"})
	})
	require.NoError(t, err)

	got, err := f.templates.List(ctx)
	require.NoError(t, err)
	assert.NotEmpty(t, got)
}
