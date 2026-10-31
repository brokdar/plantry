package food_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
)

// fakeRepo is a minimal in-memory stub for food.Repository.
type fakeRepo struct {
	getFood       *food.Food // returned by Get; defaults to a leaf food when nil
	lastListQuery food.ListQuery
	listItems     []food.Food
	history       map[int64]food.History
	usedIn        map[int64]int
	historyIDs    []int64
	usedInIDs     []int64
}

func (r *fakeRepo) Create(_ context.Context, _ *food.Food) error { return nil }
func (r *fakeRepo) Get(_ context.Context, _ int64) (*food.Food, error) {
	if r.getFood != nil {
		return r.getFood, nil
	}
	return &food.Food{Kind: food.KindLeaf}, nil
}

func (r *fakeRepo) LeafIDByName(_ context.Context, _ string) (int64, error) {
	return 0, domain.ErrNotFound
}

func (r *fakeRepo) Update(_ context.Context, _ *food.Food) error { return nil }
func (r *fakeRepo) Delete(_ context.Context, _ int64) error      { return nil }
func (r *fakeRepo) List(_ context.Context, q food.ListQuery) (*food.ListResult, error) {
	r.lastListQuery = q
	return &food.ListResult{Items: r.listItems, Total: len(r.listItems)}, nil
}

func (r *fakeRepo) Reachable(_ context.Context, _ int64) (map[int64]struct{}, error) {
	return map[int64]struct{}{}, nil
}

func (r *fakeRepo) LookupByIDs(_ context.Context, _ []int64) (map[int64]*food.Food, error) {
	return map[int64]*food.Food{}, nil
}

func (r *fakeRepo) ListChildrenByParents(_ context.Context, _ []int64) (map[int64][]food.FoodComponent, error) {
	return nil, nil
}

func (r *fakeRepo) ListPortions(_ context.Context, _ int64) ([]food.Portion, error) {
	return nil, nil
}
func (r *fakeRepo) UpsertPortion(_ context.Context, _ *food.Portion) error   { return nil }
func (r *fakeRepo) DeletePortion(_ context.Context, _ int64, _ string) error { return nil }
func (r *fakeRepo) CreateVariantGroup(_ context.Context, _ string) (int64, error) {
	return 1, nil
}

func (r *fakeRepo) Siblings(_ context.Context, _ int64, _ int64) ([]food.Food, error) {
	return nil, nil
}

func (r *fakeRepo) History(_ context.Context, ids []int64, _ time.Time) (map[int64]food.History, error) {
	r.historyIDs = ids
	return r.history, nil
}

func (r *fakeRepo) UsedInCounts(_ context.Context, ids []int64) (map[int64]int, error) {
	r.usedInIDs = ids
	return r.usedIn, nil
}

func (r *fakeRepo) SetFavorite(_ context.Context, _ int64, _ bool) (*food.Food, error) {
	return &food.Food{}, nil
}

func ptrF64(v float64) *float64      { return &v }
func ptrRole(r food.Role) *food.Role { return &r }

// ── Create: leaf ──────────────────────────────────────────────────────────────

func TestService_Create_Leaf_OK(t *testing.T) {
	svc := food.NewService(&fakeRepo{})
	err := svc.Create(context.Background(), &food.Food{
		Name: "Chicken Breast",
		Kind: food.KindLeaf,
	})
	require.NoError(t, err)
}

func TestService_Create_MissingName(t *testing.T) {
	svc := food.NewService(&fakeRepo{})
	err := svc.Create(context.Background(), &food.Food{Kind: food.KindLeaf})
	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrInvalidInput))
}

func TestService_Create_Leaf_NegativeNutrition(t *testing.T) {
	svc := food.NewService(&fakeRepo{})
	err := svc.Create(context.Background(), &food.Food{
		Name:     "Bad Food",
		Kind:     food.KindLeaf,
		Kcal100g: ptrF64(-1),
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrInvalidInput))
}

func TestService_Create_Leaf_RoleSet(t *testing.T) {
	svc := food.NewService(&fakeRepo{})
	err := svc.Create(context.Background(), &food.Food{
		Name: "Apple",
		Kind: food.KindLeaf,
		Role: ptrRole(food.RoleMain),
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrInvalidInput))
}

func TestService_Create_Leaf_HasChildren(t *testing.T) {
	svc := food.NewService(&fakeRepo{})
	err := svc.Create(context.Background(), &food.Food{
		Name: "Apple",
		Kind: food.KindLeaf,
		Children: []food.FoodComponent{
			{ChildID: 1, Amount: 100, Unit: "g"},
		},
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrInvalidInput))
}

func TestService_Create_InvalidKind(t *testing.T) {
	svc := food.NewService(&fakeRepo{})
	err := svc.Create(context.Background(), &food.Food{
		Name: "Weird",
		Kind: food.Kind("unknown"),
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrInvalidInput))
}

// ── Create: composed ──────────────────────────────────────────────────────────

func TestService_Create_Composed_OK(t *testing.T) {
	svc := food.NewService(&fakeRepo{})
	err := svc.Create(context.Background(), &food.Food{
		Name: "Pasta Carbonara",
		Kind: food.KindComposed,
		Role: ptrRole(food.RoleMain),
		Children: []food.FoodComponent{
			{ChildID: 1, Amount: 200, Unit: "g"},
		},
	})
	require.NoError(t, err)
}

func TestService_Create_Composed_MissingRole(t *testing.T) {
	svc := food.NewService(&fakeRepo{})
	err := svc.Create(context.Background(), &food.Food{
		Name: "Pasta Carbonara",
		Kind: food.KindComposed,
		Children: []food.FoodComponent{
			{ChildID: 1, Amount: 200, Unit: "g"},
		},
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrInvalidInput))
}

func TestService_Create_Composed_NoChildren(t *testing.T) {
	svc := food.NewService(&fakeRepo{})
	err := svc.Create(context.Background(), &food.Food{
		Name: "Empty Dish",
		Kind: food.KindComposed,
		Role: ptrRole(food.RoleMain),
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrInvalidInput))
}

func TestService_Create_Composed_SelfLoop(t *testing.T) {
	svc := food.NewService(&fakeRepo{})
	// ID must be non-zero for the self-loop check to trigger.
	err := svc.Create(context.Background(), &food.Food{
		ID:   5,
		Name: "Recursive Dish",
		Kind: food.KindComposed,
		Role: ptrRole(food.RoleMain),
		Children: []food.FoodComponent{
			{ChildID: 5, Amount: 100, Unit: "g"},
		},
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrInvalidInput))
}

func TestService_Create_Composed_InvalidRole(t *testing.T) {
	svc := food.NewService(&fakeRepo{})
	bad := food.Role("snack")
	err := svc.Create(context.Background(), &food.Food{
		Name: "Snack",
		Kind: food.KindComposed,
		Role: &bad,
		Children: []food.FoodComponent{
			{ChildID: 1, Amount: 100, Unit: "g"},
		},
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrInvalidInput))
}

// ── Portions ──────────────────────────────────────────────────────────────────

func TestService_UpsertPortion_EmptyUnit(t *testing.T) {
	svc := food.NewService(&fakeRepo{})
	err := svc.UpsertPortion(context.Background(), &food.Portion{
		FoodID: 1, Unit: "", Grams: 100,
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrInvalidInput))
}

func TestService_UpsertPortion_NonPositiveGrams(t *testing.T) {
	svc := food.NewService(&fakeRepo{})
	err := svc.UpsertPortion(context.Background(), &food.Portion{
		FoodID: 1, Unit: "cup", Grams: 0,
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrInvalidInput))
}

func TestService_UpsertPortion_ComposedFood(t *testing.T) {
	repo := &fakeRepo{getFood: &food.Food{Kind: food.KindComposed}}
	svc := food.NewService(repo)
	err := svc.UpsertPortion(context.Background(), &food.Portion{
		FoodID: 1, Unit: "cup", Grams: 240,
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrInvalidInput))
}

// ── List defaults ─────────────────────────────────────────────────────────────

func TestService_List_DefaultLimit(t *testing.T) {
	repo := &fakeRepo{}
	svc := food.NewService(repo)
	_, err := svc.List(context.Background(), food.ListQuery{Limit: 0})
	require.NoError(t, err)
	assert.Equal(t, 50, repo.lastListQuery.Limit)
}

func TestService_List_ClampLimit(t *testing.T) {
	repo := &fakeRepo{}
	svc := food.NewService(repo)
	_, err := svc.List(context.Background(), food.ListQuery{Limit: 99999})
	require.NoError(t, err)
	assert.Equal(t, 10000, repo.lastListQuery.Limit)
}

func TestService_List_RejectsNegativeFilters(t *testing.T) {
	for name, q := range map[string]food.ListQuery{
		"max_minutes":         {MaxMinutes: -1},
		"not_eaten_weeks":     {NotEatenWeeks: -1},
		"created_within_days": {CreatedWithinDays: -1},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := food.NewService(&fakeRepo{}).List(context.Background(), q)
			assert.ErrorIs(t, err, domain.ErrInvalidInput)
		})
	}
}

func TestService_Derive_HistoryAndUsedInCountPerKind(t *testing.T) {
	eaten := time.Date(2026, 10, 27, 0, 0, 0, 0, time.UTC)
	repo := &fakeRepo{
		listItems: []food.Food{
			{ID: 1, Kind: food.KindComposed},
			{ID: 2, Kind: food.KindLeaf},
			{ID: 3, Kind: food.KindComposed},
		},
		history: map[int64]food.History{1: {LastEaten: &eaten, TimesCooked: 1}},
		usedIn:  map[int64]int{2: 4},
	}
	svc := food.NewService(repo)
	res, err := svc.List(context.Background(), food.ListQuery{})
	require.NoError(t, err)
	assert.Nil(t, repo.historyIDs, "List does not derive; callers call Derive once")
	assert.Nil(t, repo.usedInIDs)

	foods := []*food.Food{&res.Items[0], &res.Items[1], &res.Items[2]}
	require.NoError(t, svc.Derive(context.Background(), time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC), foods...))
	assert.Equal(t, []int64{1, 3}, repo.historyIDs, "one history query for the recipes")
	assert.Equal(t, []int64{2}, repo.usedInIDs, "one usage query for the leaves")
	assert.Equal(t, food.History{LastEaten: &eaten, TimesCooked: 1}, res.Items[0].History)
	assert.Equal(t, 4, res.Items[1].UsedInCount)
	assert.Equal(t, food.History{}, res.Items[2].History, "never eaten")
}
