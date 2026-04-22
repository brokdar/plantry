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

func (r *fakeRepo) LookupForNutrition(_ context.Context, ids []int64) (map[int64]*ingredient.Ingredient, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := make(map[int64]*ingredient.Ingredient, len(ids))
	for _, id := range ids {
		if i, ok := r.items[id]; ok {
			clone := *i
			result[id] = &clone
		}
	}
	return result, nil
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

type fakeImageDeleter struct {
	calls []string
}

func (f *fakeImageDeleter) Delete(category string, id int64) error {
	f.calls = append(f.calls, fmt.Sprintf("%s/%d", category, id))
	return nil
}

func TestDelete_CleansUpImage(t *testing.T) {
	repo := newFakeRepo()
	deleter := &fakeImageDeleter{}
	svc := ingredient.NewService(repo).WithImageStore(deleter)

	require.NoError(t, svc.Create(context.Background(), &ingredient.Ingredient{Name: "Rice"}))
	require.NoError(t, svc.Delete(context.Background(), 1))

	assert.Equal(t, []string{"ingredients/1"}, deleter.calls)
}

func TestDelete_WithoutImageStore_NoPanic(t *testing.T) {
	repo := newFakeRepo()
	svc := ingredient.NewService(repo)
	require.NoError(t, svc.Create(context.Background(), &ingredient.Ingredient{Name: "Rice"}))
	require.NoError(t, svc.Delete(context.Background(), 1))
}

func TestDelete_RepoErrorSkipsImageDelete(t *testing.T) {
	repo := newFakeRepo()
	deleter := &fakeImageDeleter{}
	svc := ingredient.NewService(repo).WithImageStore(deleter)

	err := svc.Delete(context.Background(), 9999)
	assert.ErrorIs(t, err, domain.ErrNotFound)
	assert.Empty(t, deleter.calls)
}

// --- portion sync tests ---

type fakePortionProvider struct {
	portions []ingredient.FoodPortion
	err      error
	calls    int
}

func (f *fakePortionProvider) GetFoodPortions(_ context.Context, _ int) ([]ingredient.FoodPortion, error) {
	f.calls++
	return f.portions, f.err
}

func TestSyncPortionsFromFDC_Happy(t *testing.T) {
	repo := newFakeRepo()
	provider := &fakePortionProvider{
		portions: []ingredient.FoodPortion{
			{RawUnit: "cup", GramWeight: 339},
			{RawUnit: "undetermined", Modifier: "tbsp", GramWeight: 21},
			{RawUnit: "undetermined", Modifier: "tsp", GramWeight: 7},
		},
	}
	svc := ingredient.NewService(repo).WithPortionProvider(provider)
	fdcID := "169640"
	i := &ingredient.Ingredient{Name: "Honey", Source: ingredient.SourceFDC, FdcID: &fdcID}
	require.NoError(t, svc.Create(context.Background(), i))

	count, err := svc.SyncPortionsFromFDC(context.Background(), i.ID)
	require.NoError(t, err)
	assert.Equal(t, 3, count)

	portions, err := svc.ListPortions(context.Background(), i.ID)
	require.NoError(t, err)
	require.Len(t, portions, 3)

	byUnit := map[string]float64{}
	for _, p := range portions {
		byUnit[p.Unit] = p.Grams
	}
	assert.Equal(t, 339.0, byUnit["cup"])
	assert.Equal(t, 21.0, byUnit["tbsp"])
	assert.Equal(t, 7.0, byUnit["tsp"])
}

func TestSyncPortionsFromFDC_MedianOfDuplicates(t *testing.T) {
	repo := newFakeRepo()
	// Eggs: small 38g, medium 44g, large 50g all fold to "piece"; median 44g.
	provider := &fakePortionProvider{
		portions: []ingredient.FoodPortion{
			{RawUnit: "undetermined", Modifier: "small", GramWeight: 38},
			{RawUnit: "undetermined", Modifier: "medium", GramWeight: 44},
			{RawUnit: "undetermined", Modifier: "large", GramWeight: 50},
		},
	}
	svc := ingredient.NewService(repo).WithPortionProvider(provider)
	fdcID := "100"
	i := &ingredient.Ingredient{Name: "Egg", Source: ingredient.SourceFDC, FdcID: &fdcID}
	require.NoError(t, svc.Create(context.Background(), i))
	count, err := svc.SyncPortionsFromFDC(context.Background(), i.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	portions, err := svc.ListPortions(context.Background(), i.ID)
	require.NoError(t, err)
	require.Len(t, portions, 1)
	assert.Equal(t, "piece", portions[0].Unit)
	assert.Equal(t, 44.0, portions[0].Grams)
}

func TestSyncPortionsFromFDC_SimplifiesVerboseFDCLabels(t *testing.T) {
	repo := newFakeRepo()
	// Banana-style: "cup, sliced" + "cup, mashed" both map to "cup" (median);
	// "large (8 inches)" folds to "piece" with its own gram weight.
	provider := &fakePortionProvider{
		portions: []ingredient.FoodPortion{
			{RawUnit: "cup, sliced", GramWeight: 150},
			{RawUnit: "cup, mashed", GramWeight: 225},
			{RawUnit: "large (8 inches or longer)", GramWeight: 136},
			{RawUnit: "NLEA serving", GramWeight: 126},
		},
	}
	svc := ingredient.NewService(repo).WithPortionProvider(provider)
	fdcID := "173944"
	i := &ingredient.Ingredient{Name: "Banana", Source: ingredient.SourceFDC, FdcID: &fdcID}
	require.NoError(t, svc.Create(context.Background(), i))
	count, err := svc.SyncPortionsFromFDC(context.Background(), i.ID)
	require.NoError(t, err)
	assert.Equal(t, 3, count)

	portions, err := svc.ListPortions(context.Background(), i.ID)
	require.NoError(t, err)
	byUnit := map[string]float64{}
	for _, p := range portions {
		byUnit[p.Unit] = p.Grams
	}
	assert.Equal(t, 187.5, byUnit["cup"]) // (150+225)/2
	assert.Equal(t, 136.0, byUnit["piece"])
	assert.Equal(t, 126.0, byUnit["serving"])
}

func TestSyncPortionsFromFDC_NoFdcID(t *testing.T) {
	repo := newFakeRepo()
	provider := &fakePortionProvider{}
	svc := ingredient.NewService(repo).WithPortionProvider(provider)
	i := &ingredient.Ingredient{Name: "Manual"}
	require.NoError(t, svc.Create(context.Background(), i))
	_, err := svc.SyncPortionsFromFDC(context.Background(), i.ID)
	assert.ErrorIs(t, err, ingredient.ErrNoFdcID)
	assert.Equal(t, 0, provider.calls, "provider must not be called when no fdc_id")
}

func TestSyncPortionsFromFDC_NoProvider(t *testing.T) {
	repo := newFakeRepo()
	svc := ingredient.NewService(repo)
	fdcID := "100"
	i := &ingredient.Ingredient{Name: "Apple", Source: ingredient.SourceFDC, FdcID: &fdcID}
	require.NoError(t, svc.Create(context.Background(), i))
	_, err := svc.SyncPortionsFromFDC(context.Background(), i.ID)
	assert.ErrorIs(t, err, domain.ErrLookupFailed)
}

func TestSyncPortionsFromFDC_IngredientNotFound(t *testing.T) {
	svc := ingredient.NewService(newFakeRepo()).WithPortionProvider(&fakePortionProvider{})
	_, err := svc.SyncPortionsFromFDC(context.Background(), 999)
	assert.ErrorIs(t, err, domain.ErrNotFound)
}

func TestSyncPortionsFromFDC_SkipsZeroGrams(t *testing.T) {
	repo := newFakeRepo()
	provider := &fakePortionProvider{
		portions: []ingredient.FoodPortion{
			{RawUnit: "cup", GramWeight: 0},   // dropped
			{RawUnit: "tbsp", GramWeight: -5}, // dropped
			{RawUnit: "tbsp", GramWeight: 15}, // kept
		},
	}
	svc := ingredient.NewService(repo).WithPortionProvider(provider)
	fdcID := "100"
	i := &ingredient.Ingredient{Name: "Something", Source: ingredient.SourceFDC, FdcID: &fdcID}
	require.NoError(t, svc.Create(context.Background(), i))
	count, err := svc.SyncPortionsFromFDC(context.Background(), i.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
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
