package sqlite_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/component"
	"github.com/jaltszeimer/plantry/backend/internal/domain/ingredient"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
)

func intPtr(v int) *int { return &v }

func seedIngredient(t *testing.T, repo *sqlite.IngredientRepo, name string) *ingredient.Ingredient {
	t.Helper()
	i := &ingredient.Ingredient{Name: name, Source: "manual", Kcal100g: 100, Protein100g: 10}
	require.NoError(t, repo.Create(context.Background(), i))
	return i
}

func TestComponentRepo_CreateAndGet(t *testing.T) {
	db := testhelper.NewTestDB(t)
	iRepo := sqlite.NewIngredientRepo(db)
	repo := sqlite.NewComponentRepo(db)
	ctx := context.Background()

	ing := seedIngredient(t, iRepo, "Chicken")

	c := &component.Component{
		Name:              "Chicken Curry",
		Role:              component.RoleMain,
		ReferencePortions: 2,
		PrepMinutes:       intPtr(10),
		CookMinutes:       intPtr(30),
		Ingredients: []component.ComponentIngredient{
			{IngredientID: ing.ID, Amount: 300, Unit: "g", Grams: 300, SortOrder: 0},
		},
		Instructions: []component.Instruction{
			{StepNumber: 1, Text: "Cook chicken"},
			{StepNumber: 2, Text: "Add curry paste"},
		},
		Tags: []string{"spicy", "thai"},
	}

	require.NoError(t, repo.Create(ctx, c))
	assert.NotZero(t, c.ID)
	assert.False(t, c.CreatedAt.IsZero())
	assert.False(t, c.UpdatedAt.IsZero())

	got, err := repo.Get(ctx, c.ID)
	require.NoError(t, err)
	assert.Equal(t, "Chicken Curry", got.Name)
	assert.Equal(t, component.RoleMain, got.Role)
	assert.Equal(t, 2.0, got.ReferencePortions)
	require.NotNil(t, got.PrepMinutes)
	assert.Equal(t, 10, *got.PrepMinutes)
	require.NotNil(t, got.CookMinutes)
	assert.Equal(t, 30, *got.CookMinutes)

	require.Len(t, got.Ingredients, 1)
	assert.Equal(t, ing.ID, got.Ingredients[0].IngredientID)
	assert.Equal(t, 300.0, got.Ingredients[0].Grams)

	require.Len(t, got.Instructions, 2)
	assert.Equal(t, "Cook chicken", got.Instructions[0].Text)
	assert.Equal(t, "Add curry paste", got.Instructions[1].Text)

	assert.Equal(t, []string{"spicy", "thai"}, got.Tags)
}

func TestComponentRepo_UpdateReplacesChildren(t *testing.T) {
	db := testhelper.NewTestDB(t)
	iRepo := sqlite.NewIngredientRepo(db)
	repo := sqlite.NewComponentRepo(db)
	ctx := context.Background()

	ing1 := seedIngredient(t, iRepo, "Chicken")
	ing2 := seedIngredient(t, iRepo, "Tofu")

	c := &component.Component{
		Name:              "Test Component",
		Role:              component.RoleMain,
		ReferencePortions: 1,
		Ingredients: []component.ComponentIngredient{
			{IngredientID: ing1.ID, Amount: 300, Unit: "g", Grams: 300},
		},
		Instructions: []component.Instruction{
			{StepNumber: 1, Text: "Step one"},
		},
		Tags: []string{"original"},
	}
	require.NoError(t, repo.Create(ctx, c))

	// Replace all children.
	c.Ingredients = []component.ComponentIngredient{
		{IngredientID: ing2.ID, Amount: 200, Unit: "g", Grams: 200},
	}
	c.Instructions = []component.Instruction{
		{StepNumber: 1, Text: "New step"},
	}
	c.Tags = []string{"updated", "vegan"}
	require.NoError(t, repo.Update(ctx, c))

	got, err := repo.Get(ctx, c.ID)
	require.NoError(t, err)
	require.Len(t, got.Ingredients, 1)
	assert.Equal(t, ing2.ID, got.Ingredients[0].IngredientID)
	require.Len(t, got.Instructions, 1)
	assert.Equal(t, "New step", got.Instructions[0].Text)
	assert.Equal(t, []string{"updated", "vegan"}, got.Tags)
}

func TestComponentRepo_DeleteNotFound(t *testing.T) {
	db := testhelper.NewTestDB(t)
	repo := sqlite.NewComponentRepo(db)
	err := repo.Delete(context.Background(), 999)
	assert.ErrorIs(t, err, domain.ErrNotFound)
}

func TestComponentRepo_GetNotFound(t *testing.T) {
	db := testhelper.NewTestDB(t)
	repo := sqlite.NewComponentRepo(db)
	_, err := repo.Get(context.Background(), 999)
	assert.ErrorIs(t, err, domain.ErrNotFound)
}

func TestComponentRepo_FTSSearch(t *testing.T) {
	db := testhelper.NewTestDB(t)
	repo := sqlite.NewComponentRepo(db)
	ctx := context.Background()

	require.NoError(t, repo.Create(ctx, &component.Component{Name: "Chicken Curry", Role: component.RoleMain, ReferencePortions: 1}))
	require.NoError(t, repo.Create(ctx, &component.Component{Name: "Tofu Stir Fry", Role: component.RoleMain, ReferencePortions: 1}))
	require.NoError(t, repo.Create(ctx, &component.Component{Name: "Chicken Salad", Role: component.RoleSideVeg, ReferencePortions: 1}))

	result, err := repo.List(ctx, component.ListQuery{Search: "chicken", Limit: 50})
	require.NoError(t, err)
	assert.Equal(t, 2, result.Total)
}

func TestComponentRepo_ListRoleFilter(t *testing.T) {
	db := testhelper.NewTestDB(t)
	repo := sqlite.NewComponentRepo(db)
	ctx := context.Background()

	require.NoError(t, repo.Create(ctx, &component.Component{Name: "Main 1", Role: component.RoleMain, ReferencePortions: 1}))
	require.NoError(t, repo.Create(ctx, &component.Component{Name: "Side 1", Role: component.RoleSideVeg, ReferencePortions: 1}))

	result, err := repo.List(ctx, component.ListQuery{Role: string(component.RoleMain), Limit: 50})
	require.NoError(t, err)
	assert.Equal(t, 1, result.Total)
	assert.Equal(t, "Main 1", result.Items[0].Name)
}

func TestComponentRepo_ListTagFilter(t *testing.T) {
	db := testhelper.NewTestDB(t)
	repo := sqlite.NewComponentRepo(db)
	ctx := context.Background()

	c1 := &component.Component{Name: "Spicy Bowl", Role: component.RoleMain, ReferencePortions: 1, Tags: []string{"spicy", "quick"}}
	c2 := &component.Component{Name: "Mild Bowl", Role: component.RoleMain, ReferencePortions: 1, Tags: []string{"mild"}}
	require.NoError(t, repo.Create(ctx, c1))
	require.NoError(t, repo.Create(ctx, c2))

	result, err := repo.List(ctx, component.ListQuery{Tag: "spicy", Limit: 50})
	require.NoError(t, err)
	assert.Equal(t, 1, result.Total)
	assert.Equal(t, "Spicy Bowl", result.Items[0].Name)
}

func TestComponentRepo_NullableFieldsRoundTrip(t *testing.T) {
	db := testhelper.NewTestDB(t)
	repo := sqlite.NewComponentRepo(db)
	ctx := context.Background()

	notes := "Some notes"
	c := &component.Component{
		Name:              "Nullable Test",
		Role:              component.RoleMain,
		ReferencePortions: 1,
		Notes:             &notes,
	}
	require.NoError(t, repo.Create(ctx, c))

	got, _ := repo.Get(ctx, c.ID)
	require.NotNil(t, got.Notes)
	assert.Equal(t, "Some notes", *got.Notes)
	assert.Nil(t, got.ImagePath)
	assert.Nil(t, got.VariantGroupID)

	// Update to null.
	c.Notes = nil
	require.NoError(t, repo.Update(ctx, c))

	got, _ = repo.Get(ctx, c.ID)
	assert.Nil(t, got.Notes)
}

func TestComponentRepo_IngredientDeleteRestricted(t *testing.T) {
	db := testhelper.NewTestDB(t)
	iRepo := sqlite.NewIngredientRepo(db)
	cRepo := sqlite.NewComponentRepo(db)
	ctx := context.Background()

	ing := seedIngredient(t, iRepo, "Used Ingredient")
	c := &component.Component{
		Name:              "Uses Ingredient",
		Role:              component.RoleMain,
		ReferencePortions: 1,
		Ingredients: []component.ComponentIngredient{
			{IngredientID: ing.ID, Amount: 100, Unit: "g", Grams: 100},
		},
	}
	require.NoError(t, cRepo.Create(ctx, c))

	// Attempting to delete the ingredient should fail with ErrInUse.
	err := iRepo.Delete(ctx, ing.ID)
	assert.ErrorIs(t, err, domain.ErrInUse)
}

func TestComponentRepo_DeleteCascadesChildren(t *testing.T) {
	db := testhelper.NewTestDB(t)
	iRepo := sqlite.NewIngredientRepo(db)
	repo := sqlite.NewComponentRepo(db)
	ctx := context.Background()

	ing := seedIngredient(t, iRepo, "Temp Ingredient")
	c := &component.Component{
		Name:              "To Delete",
		Role:              component.RoleMain,
		ReferencePortions: 1,
		Ingredients: []component.ComponentIngredient{
			{IngredientID: ing.ID, Amount: 100, Unit: "g", Grams: 100},
		},
		Instructions: []component.Instruction{{StepNumber: 1, Text: "Do it"}},
		Tags:         []string{"temp"},
	}
	require.NoError(t, repo.Create(ctx, c))
	require.NoError(t, repo.Delete(ctx, c.ID))

	_, err := repo.Get(ctx, c.ID)
	assert.ErrorIs(t, err, domain.ErrNotFound)
}

func TestComponentRepo_FTSSearchSanitization(t *testing.T) {
	db := testhelper.NewTestDB(t)
	repo := sqlite.NewComponentRepo(db)
	ctx := context.Background()

	// Seed one component so searches execute against a non-empty table.
	require.NoError(t, repo.Create(ctx, &component.Component{
		Name: "Chicken Curry", Role: component.RoleMain, ReferencePortions: 1,
	}))

	// Each search must not cause an FTS5 syntax error.
	tests := []struct {
		name   string
		search string
	}{
		{"reserved AND", "AND"},
		{"reserved OR", "OR"},
		{"reserved NOT", "NOT"},
		{"reserved NEAR", "NEAR"},
		{"wildcard star", "ch*ken"},
		{"caret prefix", "^chicken"},
		{"embedded quote", `ch"ken`},
		{"unicode", "poulet rôti"},
		{"mixed operators", "chicken AND NOT OR"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := repo.List(ctx, component.ListQuery{Search: tt.search, Limit: 50})
			assert.NoError(t, err, "search %q should not cause FTS5 error", tt.search)
		})
	}
}

func TestComponentRepo_CreateVariantGroup(t *testing.T) {
	db := testhelper.NewTestDB(t)
	repo := sqlite.NewComponentRepo(db)
	ctx := context.Background()

	groupID, err := repo.CreateVariantGroup(ctx, "Curry family")
	require.NoError(t, err)
	assert.NotZero(t, groupID)

	// A second group gets a different ID.
	groupID2, err := repo.CreateVariantGroup(ctx, "Bowl family")
	require.NoError(t, err)
	assert.NotEqual(t, groupID, groupID2)
}

func TestComponentRepo_Siblings(t *testing.T) {
	db := testhelper.NewTestDB(t)
	iRepo := sqlite.NewIngredientRepo(db)
	repo := sqlite.NewComponentRepo(db)
	ctx := context.Background()

	ing := seedIngredient(t, iRepo, "Chicken")

	groupID, err := repo.CreateVariantGroup(ctx, "Curry family")
	require.NoError(t, err)

	parent := &component.Component{
		Name:              "Chicken Curry",
		Role:              component.RoleMain,
		ReferencePortions: 2,
		VariantGroupID:    &groupID,
		Ingredients: []component.ComponentIngredient{
			{IngredientID: ing.ID, Amount: 300, Unit: "g", Grams: 300, SortOrder: 0},
		},
		Instructions: []component.Instruction{
			{StepNumber: 1, Text: "Cook chicken"},
		},
		Tags: []string{"spicy"},
	}
	require.NoError(t, repo.Create(ctx, parent))

	variant := &component.Component{
		Name:              "Tofu Curry",
		Role:              component.RoleMain,
		ReferencePortions: 2,
		VariantGroupID:    &groupID,
	}
	require.NoError(t, repo.Create(ctx, variant))

	// Siblings of the variant should return the parent (excluding the variant).
	siblings, err := repo.Siblings(ctx, groupID, variant.ID)
	require.NoError(t, err)
	require.Len(t, siblings, 1)
	assert.Equal(t, parent.ID, siblings[0].ID)
	assert.Equal(t, "Chicken Curry", siblings[0].Name)

	// Children must be loaded (not empty).
	require.Len(t, siblings[0].Ingredients, 1)
	assert.Equal(t, ing.ID, siblings[0].Ingredients[0].IngredientID)
	require.Len(t, siblings[0].Instructions, 1)
	assert.Equal(t, "Cook chicken", siblings[0].Instructions[0].Text)
	assert.Equal(t, []string{"spicy"}, siblings[0].Tags)
}

func TestComponentRepo_SiblingsEmpty(t *testing.T) {
	db := testhelper.NewTestDB(t)
	repo := sqlite.NewComponentRepo(db)
	ctx := context.Background()

	groupID, err := repo.CreateVariantGroup(ctx, "Solo group")
	require.NoError(t, err)

	solo := &component.Component{
		Name:              "Solo Component",
		Role:              component.RoleMain,
		ReferencePortions: 1,
		VariantGroupID:    &groupID,
	}
	require.NoError(t, repo.Create(ctx, solo))

	// Only member in group — siblings should be empty.
	siblings, err := repo.Siblings(ctx, groupID, solo.ID)
	require.NoError(t, err)
	assert.Empty(t, siblings)
}

func TestComponentRepo_SiblingsExcludesCorrectID(t *testing.T) {
	db := testhelper.NewTestDB(t)
	repo := sqlite.NewComponentRepo(db)
	ctx := context.Background()

	groupID, err := repo.CreateVariantGroup(ctx, "Triple group")
	require.NoError(t, err)

	a := &component.Component{Name: "A", Role: component.RoleMain, ReferencePortions: 1, VariantGroupID: &groupID}
	b := &component.Component{Name: "B", Role: component.RoleMain, ReferencePortions: 1, VariantGroupID: &groupID}
	c := &component.Component{Name: "C", Role: component.RoleMain, ReferencePortions: 1, VariantGroupID: &groupID}
	require.NoError(t, repo.Create(ctx, a))
	require.NoError(t, repo.Create(ctx, b))
	require.NoError(t, repo.Create(ctx, c))

	siblings, err := repo.Siblings(ctx, groupID, b.ID)
	require.NoError(t, err)
	require.Len(t, siblings, 2)

	names := []string{siblings[0].Name, siblings[1].Name}
	assert.Contains(t, names, "A")
	assert.Contains(t, names, "C")
}

func TestComponentRepo_MarkCooked(t *testing.T) {
	db := testhelper.NewTestDB(t)
	iRepo := sqlite.NewIngredientRepo(db)
	repo := sqlite.NewComponentRepo(db)
	ctx := context.Background()

	ing := seedIngredient(t, iRepo, "Rice")
	c := &component.Component{
		Name:              "Rice bowl",
		Role:              component.RoleMain,
		ReferencePortions: 1,
		Ingredients: []component.ComponentIngredient{
			{IngredientID: ing.ID, Amount: 100, Unit: "g", Grams: 100, SortOrder: 0},
		},
	}
	require.NoError(t, repo.Create(ctx, c))

	before, err := repo.Get(ctx, c.ID)
	require.NoError(t, err)
	assert.Equal(t, 0, before.CookCount)
	assert.Nil(t, before.LastCookedAt)

	at := time.Now().UTC().Truncate(time.Second)
	require.NoError(t, repo.MarkCooked(ctx, c.ID, at))

	after, err := repo.Get(ctx, c.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, after.CookCount)
	require.NotNil(t, after.LastCookedAt)
	assert.WithinDuration(t, at, *after.LastCookedAt, time.Second)

	require.NoError(t, repo.MarkCooked(ctx, c.ID, at.Add(time.Hour)))
	after2, err := repo.Get(ctx, c.ID)
	require.NoError(t, err)
	assert.Equal(t, 2, after2.CookCount)
}

func TestIngredientRepo_LookupForNutrition(t *testing.T) {
	db := testhelper.NewTestDB(t)
	repo := sqlite.NewIngredientRepo(db)
	ctx := context.Background()

	i1 := &ingredient.Ingredient{Name: "A", Source: "manual", Kcal100g: 100, Protein100g: 10}
	i2 := &ingredient.Ingredient{Name: "B", Source: "manual", Kcal100g: 200, Protein100g: 20}
	require.NoError(t, repo.Create(ctx, i1))
	require.NoError(t, repo.Create(ctx, i2))

	result, err := repo.LookupForNutrition(ctx, []int64{i1.ID, i2.ID})
	require.NoError(t, err)
	assert.Len(t, result, 2)
	assert.Equal(t, 100.0, result[i1.ID].Kcal100g)
	assert.Equal(t, 200.0, result[i2.ID].Kcal100g)
}

func TestIngredientRepo_LookupForNutrition_Empty(t *testing.T) {
	db := testhelper.NewTestDB(t)
	repo := sqlite.NewIngredientRepo(db)
	result, err := repo.LookupForNutrition(context.Background(), nil)
	require.NoError(t, err)
	assert.Empty(t, result)
}
