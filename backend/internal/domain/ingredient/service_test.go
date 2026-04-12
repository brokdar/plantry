package ingredient_test

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/ingredient"
)

// --- fake in-memory repo ---

type portionKey struct {
	ingredientID int64
	unit         string
}

type fakeRepo struct {
	mu       sync.Mutex
	items    map[int64]*ingredient.Ingredient
	portions map[portionKey]*ingredient.Portion
	seq      int64
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		items:    make(map[int64]*ingredient.Ingredient),
		portions: make(map[portionKey]*ingredient.Portion),
	}
}

func (r *fakeRepo) Create(_ context.Context, i *ingredient.Ingredient) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, existing := range r.items {
		if existing.Name == i.Name {
			return fmt.Errorf("%w: %s", domain.ErrDuplicateName, i.Name)
		}
	}
	r.seq++
	i.ID = r.seq
	clone := *i
	r.items[i.ID] = &clone
	return nil
}

func (r *fakeRepo) Get(_ context.Context, id int64) (*ingredient.Ingredient, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	i, ok := r.items[id]
	if !ok {
		return nil, fmt.Errorf("%w: id %d", domain.ErrNotFound, id)
	}
	clone := *i
	return &clone, nil
}

func (r *fakeRepo) Update(_ context.Context, i *ingredient.Ingredient) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[i.ID]; !ok {
		return fmt.Errorf("%w: id %d", domain.ErrNotFound, i.ID)
	}
	for _, existing := range r.items {
		if existing.Name == i.Name && existing.ID != i.ID {
			return fmt.Errorf("%w: %s", domain.ErrDuplicateName, i.Name)
		}
	}
	clone := *i
	r.items[i.ID] = &clone
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

func (r *fakeRepo) List(_ context.Context, q ingredient.ListQuery) (*ingredient.ListResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var filtered []ingredient.Ingredient
	for _, i := range r.items {
		if q.Search != "" && !strings.Contains(strings.ToLower(i.Name), strings.ToLower(q.Search)) {
			continue
		}
		filtered = append(filtered, *i)
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
	return &ingredient.ListResult{Items: filtered[start:end], Total: total}, nil
}

func (r *fakeRepo) ListPortions(_ context.Context, ingredientID int64) ([]ingredient.Portion, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var result []ingredient.Portion
	for k, p := range r.portions {
		if k.ingredientID == ingredientID {
			result = append(result, *p)
		}
	}
	return result, nil
}

func (r *fakeRepo) UpsertPortion(_ context.Context, p *ingredient.Portion) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := portionKey{ingredientID: p.IngredientID, unit: p.Unit}
	clone := *p
	r.portions[key] = &clone
	return nil
}

func (r *fakeRepo) DeletePortion(_ context.Context, ingredientID int64, unit string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := portionKey{ingredientID: ingredientID, unit: unit}
	if _, ok := r.portions[key]; !ok {
		return fmt.Errorf("%w: portion %d/%s", domain.ErrNotFound, ingredientID, unit)
	}
	delete(r.portions, key)
	return nil
}

// --- tests ---

func TestCreate_AssignsID(t *testing.T) {
	svc := ingredient.NewService(newFakeRepo())
	i := &ingredient.Ingredient{Name: "Chicken Breast"}
	err := svc.Create(context.Background(), i)
	require.NoError(t, err)
	assert.NotZero(t, i.ID)
}

func TestCreate_DuplicateName(t *testing.T) {
	svc := ingredient.NewService(newFakeRepo())
	_ = svc.Create(context.Background(), &ingredient.Ingredient{Name: "Tofu"})
	err := svc.Create(context.Background(), &ingredient.Ingredient{Name: "Tofu"})
	assert.ErrorIs(t, err, domain.ErrDuplicateName)
}

func TestCreate_DefaultSource(t *testing.T) {
	svc := ingredient.NewService(newFakeRepo())
	i := &ingredient.Ingredient{Name: "Rice"}
	_ = svc.Create(context.Background(), i)
	assert.Equal(t, "manual", i.Source)
}

func TestCreate_EmptyName(t *testing.T) {
	svc := ingredient.NewService(newFakeRepo())
	err := svc.Create(context.Background(), &ingredient.Ingredient{Name: ""})
	assert.ErrorIs(t, err, domain.ErrInvalidInput)
}

func TestGet_NotFound(t *testing.T) {
	svc := ingredient.NewService(newFakeRepo())
	_, err := svc.Get(context.Background(), 999)
	assert.ErrorIs(t, err, domain.ErrNotFound)
}

func TestUpdate_NotFound(t *testing.T) {
	svc := ingredient.NewService(newFakeRepo())
	err := svc.Update(context.Background(), &ingredient.Ingredient{ID: 999, Name: "X"})
	assert.ErrorIs(t, err, domain.ErrNotFound)
}

func TestDelete_NotFound(t *testing.T) {
	svc := ingredient.NewService(newFakeRepo())
	err := svc.Delete(context.Background(), 999)
	assert.ErrorIs(t, err, domain.ErrNotFound)
}

func TestList_RespectsSearch(t *testing.T) {
	repo := newFakeRepo()
	svc := ingredient.NewService(repo)
	_ = svc.Create(context.Background(), &ingredient.Ingredient{Name: "Chicken Breast"})
	_ = svc.Create(context.Background(), &ingredient.Ingredient{Name: "Chicken Thigh"})
	_ = svc.Create(context.Background(), &ingredient.Ingredient{Name: "Tofu"})

	result, err := svc.List(context.Background(), ingredient.ListQuery{Search: "chicken"})
	require.NoError(t, err)
	assert.Equal(t, 2, result.Total)
	assert.Len(t, result.Items, 2)
}

func TestList_DefaultLimit(t *testing.T) {
	repo := newFakeRepo()
	svc := ingredient.NewService(repo)
	ctx := context.Background()

	for i := 0; i < 60; i++ {
		require.NoError(t, svc.Create(ctx, &ingredient.Ingredient{Name: fmt.Sprintf("Item%d", i)}))
	}

	result, err := svc.List(ctx, ingredient.ListQuery{})
	require.NoError(t, err)
	assert.Len(t, result.Items, 50, "default limit should cap results at 50")
}

func TestUpdate_EmptyName(t *testing.T) {
	repo := newFakeRepo()
	svc := ingredient.NewService(repo)
	i := &ingredient.Ingredient{Name: "Rice"}
	require.NoError(t, svc.Create(context.Background(), i))

	i.Name = ""
	err := svc.Update(context.Background(), i)
	assert.ErrorIs(t, err, domain.ErrInvalidInput)
}

func TestUpdate_DuplicateName(t *testing.T) {
	repo := newFakeRepo()
	svc := ingredient.NewService(repo)
	require.NoError(t, svc.Create(context.Background(), &ingredient.Ingredient{Name: "Alpha"}))
	b := &ingredient.Ingredient{Name: "Beta"}
	require.NoError(t, svc.Create(context.Background(), b))

	b.Name = "Alpha"
	err := svc.Update(context.Background(), b)
	assert.ErrorIs(t, err, domain.ErrDuplicateName)
}

func TestCreate_InvalidSource(t *testing.T) {
	svc := ingredient.NewService(newFakeRepo())
	err := svc.Create(context.Background(), &ingredient.Ingredient{Name: "X", Source: "bogus"})
	assert.ErrorIs(t, err, domain.ErrInvalidInput)
}

func TestUpdate_InvalidSource(t *testing.T) {
	repo := newFakeRepo()
	svc := ingredient.NewService(repo)
	i := &ingredient.Ingredient{Name: "Rice"}
	require.NoError(t, svc.Create(context.Background(), i))

	i.Source = "bogus"
	err := svc.Update(context.Background(), i)
	assert.ErrorIs(t, err, domain.ErrInvalidInput)
}

func TestCreate_NegativeNutrition(t *testing.T) {
	svc := ingredient.NewService(newFakeRepo())
	err := svc.Create(context.Background(), &ingredient.Ingredient{Name: "X", Kcal100g: -1})
	assert.ErrorIs(t, err, domain.ErrInvalidInput)
}

func TestUpdate_NegativeNutrition(t *testing.T) {
	repo := newFakeRepo()
	svc := ingredient.NewService(repo)
	i := &ingredient.Ingredient{Name: "Rice"}
	require.NoError(t, svc.Create(context.Background(), i))

	i.Fat100g = -0.5
	err := svc.Update(context.Background(), i)
	assert.ErrorIs(t, err, domain.ErrInvalidInput)
}

func TestList_NegativeLimit(t *testing.T) {
	svc := ingredient.NewService(newFakeRepo())
	result, err := svc.List(context.Background(), ingredient.ListQuery{Limit: -1})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestList_MaxLimitCap(t *testing.T) {
	repo := newFakeRepo()
	svc := ingredient.NewService(repo)
	ctx := context.Background()

	for i := 0; i < 250; i++ {
		require.NoError(t, svc.Create(ctx, &ingredient.Ingredient{Name: fmt.Sprintf("Cap%d", i)}))
	}

	result, err := svc.List(ctx, ingredient.ListQuery{Limit: 500})
	require.NoError(t, err)
	assert.Equal(t, 250, result.Total)
	assert.Len(t, result.Items, 200, "limit 500 should be clamped to max 200")
}

func TestUpsertPortion_Valid(t *testing.T) {
	repo := newFakeRepo()
	svc := ingredient.NewService(repo)
	require.NoError(t, svc.Create(context.Background(), &ingredient.Ingredient{Name: "Rice"}))

	err := svc.UpsertPortion(context.Background(), &ingredient.Portion{
		IngredientID: 1, Unit: "cup", Grams: 185,
	})
	assert.NoError(t, err)
}

func TestUpsertPortion_EmptyUnit(t *testing.T) {
	repo := newFakeRepo()
	svc := ingredient.NewService(repo)
	require.NoError(t, svc.Create(context.Background(), &ingredient.Ingredient{Name: "Rice"}))

	err := svc.UpsertPortion(context.Background(), &ingredient.Portion{
		IngredientID: 1, Unit: "", Grams: 185,
	})
	assert.ErrorIs(t, err, domain.ErrInvalidInput)
}

func TestUpsertPortion_ZeroGrams(t *testing.T) {
	repo := newFakeRepo()
	svc := ingredient.NewService(repo)
	require.NoError(t, svc.Create(context.Background(), &ingredient.Ingredient{Name: "Rice"}))

	err := svc.UpsertPortion(context.Background(), &ingredient.Portion{
		IngredientID: 1, Unit: "cup", Grams: 0,
	})
	assert.ErrorIs(t, err, domain.ErrInvalidInput)
}

func TestUpsertPortion_NegativeGrams(t *testing.T) {
	repo := newFakeRepo()
	svc := ingredient.NewService(repo)
	require.NoError(t, svc.Create(context.Background(), &ingredient.Ingredient{Name: "Rice"}))

	err := svc.UpsertPortion(context.Background(), &ingredient.Portion{
		IngredientID: 1, Unit: "cup", Grams: -10,
	})
	assert.ErrorIs(t, err, domain.ErrInvalidInput)
}

func TestUpsertPortion_IngredientNotFound(t *testing.T) {
	svc := ingredient.NewService(newFakeRepo())
	err := svc.UpsertPortion(context.Background(), &ingredient.Portion{
		IngredientID: 999, Unit: "cup", Grams: 185,
	})
	assert.ErrorIs(t, err, domain.ErrNotFound)
}

func TestListPortions_IngredientNotFound(t *testing.T) {
	svc := ingredient.NewService(newFakeRepo())
	_, err := svc.ListPortions(context.Background(), 999)
	assert.ErrorIs(t, err, domain.ErrNotFound)
}

func TestDeletePortion_NotFound(t *testing.T) {
	svc := ingredient.NewService(newFakeRepo())
	err := svc.DeletePortion(context.Background(), 999, "cup")
	assert.ErrorIs(t, err, domain.ErrNotFound)
}

func TestUpsertPortion_OverwritesExisting(t *testing.T) {
	repo := newFakeRepo()
	svc := ingredient.NewService(repo)
	require.NoError(t, svc.Create(context.Background(), &ingredient.Ingredient{Name: "Rice"}))

	require.NoError(t, svc.UpsertPortion(context.Background(), &ingredient.Portion{
		IngredientID: 1, Unit: "cup", Grams: 185,
	}))
	require.NoError(t, svc.UpsertPortion(context.Background(), &ingredient.Portion{
		IngredientID: 1, Unit: "cup", Grams: 200,
	}))

	portions, err := svc.ListPortions(context.Background(), 1)
	require.NoError(t, err)
	require.Len(t, portions, 1)
	assert.Equal(t, 200.0, portions[0].Grams)
}
