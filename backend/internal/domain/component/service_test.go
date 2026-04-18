package component_test

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/component"
	"github.com/jaltszeimer/plantry/backend/internal/domain/ingredient"
)

// --- fake repository ---

type fakeRepo struct {
	mu       sync.Mutex
	items    map[int64]*component.Component
	seq      int64
	groupSeq int64
	groups   map[int64]string // groupID -> name

	lastInsightsCutoff  time.Time
	lastForgottenLimit  int
	lastMostCookedLimit int
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		items:  make(map[int64]*component.Component),
		groups: make(map[int64]string),
	}
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

func (r *fakeRepo) CreateVariantGroup(_ context.Context, name string) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.groupSeq++
	r.groups[r.groupSeq] = name
	return r.groupSeq, nil
}

func (r *fakeRepo) MarkCooked(_ context.Context, id int64, at time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.items[id]
	if !ok {
		return fmt.Errorf("%w: id %d", domain.ErrNotFound, id)
	}
	c.CookCount++
	t := at
	c.LastCookedAt = &t
	return nil
}

func (r *fakeRepo) Insights(_ context.Context, cutoff time.Time, forgottenLimit, mostCookedLimit int) (component.Insights, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lastInsightsCutoff = cutoff
	r.lastForgottenLimit = forgottenLimit
	r.lastMostCookedLimit = mostCookedLimit
	return component.Insights{}, nil
}

func (r *fakeRepo) Siblings(_ context.Context, variantGroupID int64, excludeID int64) ([]component.Component, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var result []component.Component
	for _, c := range r.items {
		if c.VariantGroupID != nil && *c.VariantGroupID == variantGroupID && c.ID != excludeID {
			result = append(result, *c)
		}
	}
	return result, nil
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

type fakeImageDeleter struct {
	calls []string
}

func (f *fakeImageDeleter) Delete(category string, id int64) error {
	f.calls = append(f.calls, fmt.Sprintf("%s/%d", category, id))
	return nil
}

func TestDelete_CleansUpImage(t *testing.T) {
	svc, _, _, _ := newService()
	deleter := &fakeImageDeleter{}
	svc.WithImageStore(deleter)

	c := validComponent()
	require.NoError(t, svc.Create(context.Background(), c))
	require.NoError(t, svc.Delete(context.Background(), c.ID))

	assert.Equal(t, []string{fmt.Sprintf("components/%d", c.ID)}, deleter.calls)
}

func TestDelete_WithoutImageStore_NoPanic(t *testing.T) {
	svc, _, _, _ := newService()
	c := validComponent()
	require.NoError(t, svc.Create(context.Background(), c))
	require.NoError(t, svc.Delete(context.Background(), c.ID))
}

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

// --- variant tests ---

func TestCreateVariant_FirstVariant_CreatesGroup(t *testing.T) {
	svc, repo, _, _ := newService()
	ctx := context.Background()

	parent := validComponent()
	require.NoError(t, svc.Create(ctx, parent))
	assert.Nil(t, parent.VariantGroupID)

	variant, err := svc.CreateVariant(ctx, parent.ID)
	require.NoError(t, err)
	assert.NotZero(t, variant.ID)
	assert.NotEqual(t, parent.ID, variant.ID)

	// Parent should now have a variant_group_id.
	updatedParent, err := repo.Get(ctx, parent.ID)
	require.NoError(t, err)
	require.NotNil(t, updatedParent.VariantGroupID)

	// Variant should share the same group.
	require.NotNil(t, variant.VariantGroupID)
	assert.Equal(t, *updatedParent.VariantGroupID, *variant.VariantGroupID)

	// Variant inherits role and reference_portions.
	assert.Equal(t, parent.Role, variant.Role)
	assert.Equal(t, parent.ReferencePortions, variant.ReferencePortions)

	// Variant name includes "(variant)".
	assert.Contains(t, variant.Name, "(variant)")
}

func TestCreateVariant_SubsequentVariant_JoinsGroup(t *testing.T) {
	svc, repo, _, _ := newService()
	ctx := context.Background()

	parent := validComponent()
	require.NoError(t, svc.Create(ctx, parent))

	v1, err := svc.CreateVariant(ctx, parent.ID)
	require.NoError(t, err)

	v2, err := svc.CreateVariant(ctx, parent.ID)
	require.NoError(t, err)

	// All three should share the same group.
	updatedParent, _ := repo.Get(ctx, parent.ID)
	require.NotNil(t, updatedParent.VariantGroupID)
	require.NotNil(t, v1.VariantGroupID)
	require.NotNil(t, v2.VariantGroupID)
	assert.Equal(t, *updatedParent.VariantGroupID, *v1.VariantGroupID)
	assert.Equal(t, *updatedParent.VariantGroupID, *v2.VariantGroupID)

	// No new group created for v2 — group count should be 1.
	assert.Len(t, repo.groups, 1)
}

func TestCreateVariant_NotFound(t *testing.T) {
	svc, _, _, _ := newService()
	_, err := svc.CreateVariant(context.Background(), 999)
	assert.ErrorIs(t, err, domain.ErrNotFound)
}

func TestCreateVariant_FromVariant_JoinsSameGroup(t *testing.T) {
	svc, repo, _, _ := newService()
	ctx := context.Background()

	parent := validComponent()
	require.NoError(t, svc.Create(ctx, parent))

	v1, err := svc.CreateVariant(ctx, parent.ID)
	require.NoError(t, err)

	// Create a variant from v1 (not from parent).
	v2, err := svc.CreateVariant(ctx, v1.ID)
	require.NoError(t, err)

	// All three should share the same group.
	updatedParent, _ := repo.Get(ctx, parent.ID)
	require.NotNil(t, v2.VariantGroupID)
	assert.Equal(t, *updatedParent.VariantGroupID, *v2.VariantGroupID)

	// No new group created.
	assert.Len(t, repo.groups, 1)
}

func TestListVariants_ReturnsSiblings(t *testing.T) {
	svc, _, _, _ := newService()
	ctx := context.Background()

	parent := validComponent()
	require.NoError(t, svc.Create(ctx, parent))

	v1, err := svc.CreateVariant(ctx, parent.ID)
	require.NoError(t, err)

	v2, err := svc.CreateVariant(ctx, parent.ID)
	require.NoError(t, err)

	// From parent's perspective: siblings are v1 and v2.
	siblings, err := svc.ListVariants(ctx, parent.ID)
	require.NoError(t, err)
	assert.Len(t, siblings, 2)

	// From v1's perspective: siblings are parent and v2.
	siblings, err = svc.ListVariants(ctx, v1.ID)
	require.NoError(t, err)
	assert.Len(t, siblings, 2)

	// From v2's perspective: siblings are parent and v1.
	siblings, err = svc.ListVariants(ctx, v2.ID)
	require.NoError(t, err)
	assert.Len(t, siblings, 2)
}

func TestListVariants_NoGroup_ReturnsEmpty(t *testing.T) {
	svc, _, _, _ := newService()
	ctx := context.Background()

	c := validComponent()
	require.NoError(t, svc.Create(ctx, c))

	siblings, err := svc.ListVariants(ctx, c.ID)
	require.NoError(t, err)
	assert.Empty(t, siblings)
}

func TestListVariants_NotFound(t *testing.T) {
	svc, _, _, _ := newService()
	_, err := svc.ListVariants(context.Background(), 999)
	assert.ErrorIs(t, err, domain.ErrNotFound)
}

// --- insights tests ---

func TestInsights_AppliesDefaults(t *testing.T) {
	svc, repo, _, _ := newService()
	before := time.Now().UTC()
	_, err := svc.Insights(context.Background(), component.InsightsQuery{})
	require.NoError(t, err)

	assert.Equal(t, 10, repo.lastForgottenLimit)
	assert.Equal(t, 5, repo.lastMostCookedLimit)
	// Default is 4 weeks back from now.
	expected := before.AddDate(0, 0, -28)
	assert.WithinDuration(t, expected, repo.lastInsightsCutoff, 2*time.Second)
}

func TestInsights_ClampsHighLimits(t *testing.T) {
	svc, repo, _, _ := newService()
	_, err := svc.Insights(context.Background(), component.InsightsQuery{
		ForgottenWeeks:  500,
		ForgottenLimit:  1000,
		MostCookedLimit: 1000,
	})
	require.NoError(t, err)

	assert.Equal(t, 50, repo.lastForgottenLimit)
	assert.Equal(t, 50, repo.lastMostCookedLimit)
	// Clamped to 52 weeks back.
	expected := time.Now().UTC().AddDate(0, 0, -52*7)
	assert.WithinDuration(t, expected, repo.lastInsightsCutoff, 2*time.Second)
}

func TestInsights_CustomForgottenWeeks(t *testing.T) {
	svc, repo, _, _ := newService()
	_, err := svc.Insights(context.Background(), component.InsightsQuery{
		ForgottenWeeks: 8,
	})
	require.NoError(t, err)

	expected := time.Now().UTC().AddDate(0, 0, -56)
	assert.WithinDuration(t, expected, repo.lastInsightsCutoff, 2*time.Second)
}
