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
}

func (r *fakeRepo) Create(_ context.Context, _ *food.Food) error { return nil }
func (r *fakeRepo) Get(_ context.Context, _ int64) (*food.Food, error) {
	if r.getFood != nil {
		return r.getFood, nil
	}
	return &food.Food{Kind: food.KindLeaf}, nil
}
func (r *fakeRepo) Update(_ context.Context, _ *food.Food) error { return nil }
func (r *fakeRepo) Delete(_ context.Context, _ int64) error      { return nil }
func (r *fakeRepo) List(_ context.Context, q food.ListQuery) (*food.ListResult, error) {
	r.lastListQuery = q
	return &food.ListResult{}, nil
}

func (r *fakeRepo) Reachable(_ context.Context, _ int64) (map[int64]struct{}, error) {
	return map[int64]struct{}{}, nil
}

func (r *fakeRepo) LookupByIDs(_ context.Context, _ []int64) (map[int64]*food.Food, error) {
	return map[int64]*food.Food{}, nil
}

func (r *fakeRepo) ListChildren(_ context.Context, _ int64) ([]food.FoodComponent, error) {
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
func (r *fakeRepo) MarkCooked(_ context.Context, _ int64, _ time.Time) error { return nil }
func (r *fakeRepo) Insights(_ context.Context, _ time.Time, _, _ int) (food.Insights, error) {
	return food.Insights{}, nil
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
	_, err := svc.List(context.Background(), food.ListQuery{Limit: 999})
	require.NoError(t, err)
	assert.Equal(t, 200, repo.lastListQuery.Limit)
}
