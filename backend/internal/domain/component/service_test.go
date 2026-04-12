package component_test

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/component"
	"github.com/jaltszeimer/plantry/backend/internal/domain/ingredient"
)

// --- fake repository ---

type fakeRepo struct {
	mu    sync.Mutex
	items map[int64]*component.Component
	seq   int64
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{items: make(map[int64]*component.Component)}
}

func (r *fakeRepo) Create(_ context.Context, c *component.Component) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.seq++
	c.ID = r.seq
	clone := *c
	clone.Ingredients = append([]component.ComponentIngredient(nil), c.Ingredients...)
	clone.Instructions = append([]component.Instruction(nil), c.Instructions...)
	clone.Tags = append([]string(nil), c.Tags...)
	r.items[c.ID] = &clone
	return nil
}

func (r *fakeRepo) Get(_ context.Context, id int64) (*component.Component, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.items[id]
	if !ok {
		return nil, fmt.Errorf("%w: id %d", domain.ErrNotFound, id)
	}
	clone := *c
	return &clone, nil
}

func (r *fakeRepo) Update(_ context.Context, c *component.Component) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[c.ID]; !ok {
		return fmt.Errorf("%w: id %d", domain.ErrNotFound, c.ID)
	}
	clone := *c
	clone.Ingredients = append([]component.ComponentIngredient(nil), c.Ingredients...)
	clone.Instructions = append([]component.Instruction(nil), c.Instructions...)
	clone.Tags = append([]string(nil), c.Tags...)
	r.items[c.ID] = &clone
	return nil
}

func (r *fakeRepo) Delete(_ context.Context, id int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[id]; !ok {
		return fmt.Errorf("%w: id %d", domain.ErrNotFound, id)
	}
	delete(r.items, id)
	return nil
}

func (r *fakeRepo) List(_ context.Context, q component.ListQuery) (*component.ListResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var filtered []component.Component
	for _, c := range r.items {
		if q.Search != "" && !strings.Contains(strings.ToLower(c.Name), strings.ToLower(q.Search)) {
			continue
		}
		if q.Role != "" && string(c.Role) != q.Role {
			continue
		}
		filtered = append(filtered, *c)
	}
	total := len(filtered)
	start := q.Offset
	if start > total {
		start = total
	}
	end := start + q.Limit
	if end > total {
		end = total
	}
	return &component.ListResult{Items: filtered[start:end], Total: total}, nil
}

// --- fake portion lookup ---

type fakePortionLookup struct {
	portions map[int64][]ingredient.Portion
}

func newFakePortionLookup() *fakePortionLookup {
	return &fakePortionLookup{portions: make(map[int64][]ingredient.Portion)}
}

func (f *fakePortionLookup) ListPortions(_ context.Context, ingredientID int64) ([]ingredient.Portion, error) {
	return f.portions[ingredientID], nil
}

// --- fake nutrition lookup ---

type fakeNutritionLookup struct {
	ingredients map[int64]*ingredient.Ingredient
}

func newFakeNutritionLookup() *fakeNutritionLookup {
	return &fakeNutritionLookup{ingredients: make(map[int64]*ingredient.Ingredient)}
}

func (f *fakeNutritionLookup) LookupForNutrition(_ context.Context, ids []int64) (map[int64]*ingredient.Ingredient, error) {
	result := make(map[int64]*ingredient.Ingredient, len(ids))
	for _, id := range ids {
		if ing, ok := f.ingredients[id]; ok {
			result[id] = ing
		}
	}
	return result, nil
}

// --- helpers ---

func newService() (*component.Service, *fakeRepo, *fakePortionLookup, *fakeNutritionLookup) {
	repo := newFakeRepo()
	pl := newFakePortionLookup()
	nl := newFakeNutritionLookup()
	return component.NewService(repo, pl, nl), repo, pl, nl
}

func validComponent() *component.Component {
	return &component.Component{
		Name:              "Chicken Curry",
		Role:              component.RoleMain,
		ReferencePortions: 2,
		Ingredients: []component.ComponentIngredient{
			{IngredientID: 1, Amount: 300, Unit: "g"},
		},
		Instructions: []component.Instruction{
			{StepNumber: 1, Text: "Cook chicken"},
		},
		Tags: []string{"spicy"},
	}
}

// --- tests ---

func TestCreate_AssignsID(t *testing.T) {
	svc, _, _, _ := newService()
	c := validComponent()
	require.NoError(t, svc.Create(context.Background(), c))
	assert.NotZero(t, c.ID)
}

func TestCreate_DefaultReferencePortions(t *testing.T) {
	svc, _, _, _ := newService()
	c := validComponent()
	c.ReferencePortions = 0 // should default to 1
	require.NoError(t, svc.Create(context.Background(), c))
	assert.Equal(t, 1.0, c.ReferencePortions)
}

func TestCreate_EmptyName(t *testing.T) {
	svc, _, _, _ := newService()
	c := validComponent()
	c.Name = ""
	err := svc.Create(context.Background(), c)
	assert.ErrorIs(t, err, domain.ErrInvalidInput)
}

func TestCreate_InvalidRole(t *testing.T) {
	svc, _, _, _ := newService()
	c := validComponent()
	c.Role = "appetizer"
	err := svc.Create(context.Background(), c)
	assert.ErrorIs(t, err, domain.ErrInvalidInput)
}

func TestCreate_EmptyRole(t *testing.T) {
	svc, _, _, _ := newService()
	c := validComponent()
	c.Role = ""
	err := svc.Create(context.Background(), c)
	assert.ErrorIs(t, err, domain.ErrInvalidInput)
}

func TestCreate_NegativeReferencePortions(t *testing.T) {
	svc, _, _, _ := newService()
	c := validComponent()
	c.ReferencePortions = -1
	err := svc.Create(context.Background(), c)
	assert.ErrorIs(t, err, domain.ErrInvalidInput)
}

func TestCreate_ResolvesPortionUnit(t *testing.T) {
	svc, _, pl, _ := newService()
	pl.portions[10] = []ingredient.Portion{{IngredientID: 10, Unit: "cup", Grams: 185}}

	c := validComponent()
	c.Ingredients = []component.ComponentIngredient{
		{IngredientID: 10, Amount: 2, Unit: "cup"},
	}
	require.NoError(t, svc.Create(context.Background(), c))
	assert.Equal(t, 370.0, c.Ingredients[0].Grams) // 2 * 185
}

func TestCreate_GramsUnitSkipsLookup(t *testing.T) {
	svc, _, _, _ := newService()
	c := validComponent()
	c.Ingredients = []component.ComponentIngredient{
		{IngredientID: 1, Amount: 250, Unit: "g"},
	}
	require.NoError(t, svc.Create(context.Background(), c))
	assert.Equal(t, 250.0, c.Ingredients[0].Grams)
}

func TestCreate_MlUnitSkipsLookup(t *testing.T) {
	svc, _, _, _ := newService()
	c := validComponent()
	c.Ingredients = []component.ComponentIngredient{
		{IngredientID: 1, Amount: 100, Unit: "ml"},
	}
	require.NoError(t, svc.Create(context.Background(), c))
	assert.Equal(t, 100.0, c.Ingredients[0].Grams)
}

func TestCreate_UnknownPortionUnit(t *testing.T) {
	svc, _, _, _ := newService()
	c := validComponent()
	c.Ingredients = []component.ComponentIngredient{
		{IngredientID: 1, Amount: 2, Unit: "bushel"},
	}
	err := svc.Create(context.Background(), c)
	assert.ErrorIs(t, err, domain.ErrInvalidInput)
}

func TestCreate_IngredientZeroAmount(t *testing.T) {
	svc, _, _, _ := newService()
	c := validComponent()
	c.Ingredients = []component.ComponentIngredient{
		{IngredientID: 1, Amount: 0, Unit: "g"},
	}
	err := svc.Create(context.Background(), c)
	assert.ErrorIs(t, err, domain.ErrInvalidInput)
}

func TestCreate_InstructionEmptyText(t *testing.T) {
	svc, _, _, _ := newService()
	c := validComponent()
	c.Instructions = []component.Instruction{{StepNumber: 1, Text: ""}}
	err := svc.Create(context.Background(), c)
	assert.ErrorIs(t, err, domain.ErrInvalidInput)
}

func TestUpdate_ReplacesChildren(t *testing.T) {
	svc, repo, _, _ := newService()
	c := validComponent()
	require.NoError(t, svc.Create(context.Background(), c))

	c.Ingredients = []component.ComponentIngredient{
		{IngredientID: 2, Amount: 500, Unit: "g"},
	}
	c.Instructions = []component.Instruction{
		{StepNumber: 1, Text: "New step"},
	}
	c.Tags = []string{"new-tag"}
	require.NoError(t, svc.Update(context.Background(), c))

	got, err := repo.Get(context.Background(), c.ID)
	require.NoError(t, err)
	assert.Len(t, got.Ingredients, 1)
	assert.Equal(t, int64(2), got.Ingredients[0].IngredientID)
	assert.Len(t, got.Instructions, 1)
	assert.Equal(t, "New step", got.Instructions[0].Text)
	assert.Equal(t, []string{"new-tag"}, got.Tags)
}

func TestUpdate_NotFound(t *testing.T) {
	svc, _, _, _ := newService()
	c := validComponent()
	c.ID = 999
	err := svc.Update(context.Background(), c)
	assert.ErrorIs(t, err, domain.ErrNotFound)
}

func TestDelete_NotFound(t *testing.T) {
	svc, _, _, _ := newService()
	err := svc.Delete(context.Background(), 999)
	assert.ErrorIs(t, err, domain.ErrNotFound)
}

func TestGet_NotFound(t *testing.T) {
	svc, _, _, _ := newService()
	_, err := svc.Get(context.Background(), 999)
	assert.ErrorIs(t, err, domain.ErrNotFound)
}

func TestList_DefaultLimit(t *testing.T) {
	svc, _, _, _ := newService()
	ctx := context.Background()
	for i := 0; i < 60; i++ {
		c := &component.Component{
			Name:              fmt.Sprintf("Comp%d", i),
			Role:              component.RoleMain,
			ReferencePortions: 1,
		}
		require.NoError(t, svc.Create(ctx, c))
	}

	result, err := svc.List(ctx, component.ListQuery{})
	require.NoError(t, err)
	assert.Len(t, result.Items, 50)
	assert.Equal(t, 60, result.Total)
}

func TestList_MaxLimitCap(t *testing.T) {
	svc, _, _, _ := newService()
	result, err := svc.List(context.Background(), component.ListQuery{Limit: 500})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestList_RoleFilter(t *testing.T) {
	svc, _, _, _ := newService()
	ctx := context.Background()
	require.NoError(t, svc.Create(ctx, &component.Component{
		Name: "Main dish", Role: component.RoleMain, ReferencePortions: 1,
	}))
	require.NoError(t, svc.Create(ctx, &component.Component{
		Name: "Side dish", Role: component.RoleSideVeg, ReferencePortions: 1,
	}))

	result, err := svc.List(ctx, component.ListQuery{Role: string(component.RoleMain)})
	require.NoError(t, err)
	assert.Equal(t, 1, result.Total)
	assert.Equal(t, "Main dish", result.Items[0].Name)
}

func TestCreate_NoIngredientsAllowed(t *testing.T) {
	svc, _, _, _ := newService()
	c := &component.Component{
		Name:              "Simple sauce",
		Role:              component.RoleSauce,
		ReferencePortions: 1,
		Ingredients:       nil,
		Instructions:      nil,
	}
	require.NoError(t, svc.Create(context.Background(), c))
	assert.NotZero(t, c.ID)
}

func TestCreate_AllValidRoles(t *testing.T) {
	svc, _, _, _ := newService()
	roles := []string{"main", "side_starch", "side_veg", "side_protein", "sauce", "drink", "dessert", "standalone"}
	for _, role := range roles {
		c := &component.Component{
			Name:              fmt.Sprintf("Test %s", role),
			Role:              component.Role(role),
			ReferencePortions: 1,
		}
		require.NoError(t, svc.Create(context.Background(), c), "role %s should be valid", role)
	}
}

func TestNutrition_PerPortion(t *testing.T) {
	svc, _, _, nl := newService()
	ctx := context.Background()

	// Seed ingredient nutrition data.
	nl.ingredients[1] = &ingredient.Ingredient{
		ID: 1, Kcal100g: 165, Protein100g: 31, Fat100g: 3.6,
	}

	c := &component.Component{
		Name:              "Chicken dish",
		Role:              component.RoleMain,
		ReferencePortions: 2,
		Ingredients: []component.ComponentIngredient{
			{IngredientID: 1, Amount: 200, Unit: "g", Grams: 200},
		},
	}
	require.NoError(t, svc.Create(ctx, c))

	macros, err := svc.Nutrition(ctx, c.ID)
	require.NoError(t, err)
	// 200g chicken: 330 kcal total / 2 portions = 165
	assert.InDelta(t, 165.0, macros.Kcal, 0.01)
	// 200g: 62g protein total / 2 portions = 31
	assert.InDelta(t, 31.0, macros.Protein, 0.01)
}

func TestNutrition_NotFound(t *testing.T) {
	svc, _, _, _ := newService()
	_, err := svc.Nutrition(context.Background(), 999)
	assert.ErrorIs(t, err, domain.ErrNotFound)
}
