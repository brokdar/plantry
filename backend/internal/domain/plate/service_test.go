package plate_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeRepo is a minimal Repository stub. Only ListByDateRange is exercised by Range/Day.
type fakeRepo struct {
	rangeResult []plate.Plate
	rangeErr    error
}

func (r *fakeRepo) Create(_ context.Context, _ *plate.Plate) error                   { return nil }
func (r *fakeRepo) Get(_ context.Context, _ int64) (*plate.Plate, error)             { return nil, nil }
func (r *fakeRepo) Update(_ context.Context, _ *plate.Plate) error                   { return nil }
func (r *fakeRepo) Delete(_ context.Context, _ int64) error                          { return nil }
func (r *fakeRepo) CreateComponent(_ context.Context, _ *plate.PlateComponent) error { return nil }
func (r *fakeRepo) GetComponent(_ context.Context, _ int64) (*plate.PlateComponent, error) {
	return nil, nil
}
func (r *fakeRepo) UpdateComponent(_ context.Context, _ *plate.PlateComponent) error { return nil }
func (r *fakeRepo) DeleteComponent(_ context.Context, _ int64) error                 { return nil }
func (r *fakeRepo) ListComponentsByPlate(_ context.Context, _ int64) ([]plate.PlateComponent, error) {
	return nil, nil
}
func (r *fakeRepo) CountUsingFood(_ context.Context, _ int64) (int64, error)     { return 0, nil }
func (r *fakeRepo) CountUsingTimeSlot(_ context.Context, _ int64) (int64, error) { return 0, nil }
func (r *fakeRepo) SetSkipped(_ context.Context, _ int64, _ bool, _ *string) (*plate.Plate, error) {
	return nil, nil
}

func (r *fakeRepo) ListByDateRange(_ context.Context, _, _ time.Time) ([]plate.Plate, error) {
	return r.rangeResult, r.rangeErr
}

// fakeSlots and fakeFoods satisfy the SlotChecker / FoodLookup interfaces.
type fakeSlots struct{}

func (fakeSlots) Exists(_ context.Context, _ int64) (bool, error) { return true, nil }

// fakeFoods returns a configurable food per id and any portion overrides.
// Empty maps mean "every id resolves to a default composed food, no portions".
type fakeFoods struct {
	byID     map[int64]*food.Food
	portions map[int64][]food.Portion
}

func (f fakeFoods) Get(_ context.Context, id int64) (*food.Food, error) {
	if got, ok := f.byID[id]; ok {
		return got, nil
	}
	role := food.RoleMain
	ref := float64(1)
	return &food.Food{ID: id, Kind: food.KindComposed, Role: &role, ReferencePortions: &ref}, nil
}

func (f fakeFoods) ListPortions(_ context.Context, id int64) ([]food.Portion, error) {
	return f.portions[id], nil
}

func newService(repo *fakeRepo) *plate.Service {
	return plate.NewService(repo, fakeSlots{}, fakeFoods{})
}

func TestPlateService_Range_HappyPath(t *testing.T) {
	from := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2025, 6, 7, 0, 0, 0, 0, time.UTC)

	want := []plate.Plate{
		{ID: 1, Date: from},
		{ID: 2, Date: to},
	}
	repo := &fakeRepo{rangeResult: want}
	svc := newService(repo)

	got, err := svc.Range(context.Background(), from, to)

	require.NoError(t, err)
	require.Equal(t, want, got)
}

func TestPlateService_Range_FromAfterTo(t *testing.T) {
	from := time.Date(2025, 6, 2, 0, 0, 0, 0, time.UTC)
	to := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)

	svc := newService(&fakeRepo{})

	_, err := svc.Range(context.Background(), from, to)

	require.Error(t, err)
	require.True(t, errors.Is(err, domain.ErrInvalidInput))
}

func TestPlateService_Range_SpanTooLong(t *testing.T) {
	from := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 2, 2, 0, 0, 0, 0, time.UTC) // > 366 days

	svc := newService(&fakeRepo{})

	_, err := svc.Range(context.Background(), from, to)

	require.Error(t, err)
	require.True(t, errors.Is(err, domain.ErrInvalidInput))
}

func TestPlateService_Day_HappyPath(t *testing.T) {
	date := time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC)
	want := []plate.Plate{{ID: 3, Date: date}}

	repo := &fakeRepo{rangeResult: want}
	svc := newService(repo)

	got, err := svc.Day(context.Background(), date)

	require.NoError(t, err)
	require.Equal(t, want, got)
}

func TestPlateService_Update_ZeroDate(t *testing.T) {
	svc := newService(&fakeRepo{})

	err := svc.Update(context.Background(), &plate.Plate{SlotID: 1})

	require.Error(t, err)
	require.True(t, errors.Is(err, domain.ErrInvalidInput))
}

func TestPlateService_Range_DST(t *testing.T) {
	t.Setenv("TZ", "America/New_York")

	from := time.Date(2025, 3, 8, 0, 0, 0, 0, time.UTC) // day before US DST spring-forward
	to := time.Date(2025, 3, 10, 0, 0, 0, 0, time.UTC)  // day after

	svc := newService(&fakeRepo{rangeResult: []plate.Plate{}})

	_, err := svc.Range(context.Background(), from, to)

	require.NoError(t, err)
}

// ── AddComponent — kind-aware quantity validation + grams resolution ─────────

// componentRepo records the writes and serves component lookups so the plate
// service's AddComponent can run end-to-end against an in-memory fake.
type componentRepo struct {
	fakeRepo
	components []plate.PlateComponent
	plates     map[int64]*plate.Plate
}

func (r *componentRepo) Get(_ context.Context, id int64) (*plate.Plate, error) {
	if p, ok := r.plates[id]; ok {
		return p, nil
	}
	return &plate.Plate{ID: id}, nil
}

func (r *componentRepo) ListComponentsByPlate(_ context.Context, plateID int64) ([]plate.PlateComponent, error) {
	out := []plate.PlateComponent{}
	for _, pc := range r.components {
		if pc.PlateID == plateID {
			out = append(out, pc)
		}
	}
	return out, nil
}

func (r *componentRepo) CreateComponent(_ context.Context, pc *plate.PlateComponent) error {
	pc.ID = int64(len(r.components) + 1)
	r.components = append(r.components, *pc)
	return nil
}

func (r *componentRepo) GetComponent(_ context.Context, id int64) (*plate.PlateComponent, error) {
	for i := range r.components {
		if r.components[i].ID == id {
			c := r.components[i]
			return &c, nil
		}
	}
	return nil, errors.New("not found")
}

func (r *componentRepo) UpdateComponent(_ context.Context, pc *plate.PlateComponent) error {
	for i := range r.components {
		if r.components[i].ID == pc.ID {
			r.components[i] = *pc
			return nil
		}
	}
	return errors.New("not found")
}

func leafFood(id int64, name string) *food.Food {
	src := food.SourceManual
	return &food.Food{ID: id, Name: name, Kind: food.KindLeaf, Source: &src}
}

func composedFood(id int64, name string) *food.Food {
	role := food.RoleMain
	ref := float64(1)
	return &food.Food{ID: id, Name: name, Kind: food.KindComposed, Role: &role, ReferencePortions: &ref}
}

func newComponentService(t *testing.T, foods fakeFoods) (*plate.Service, *componentRepo) {
	t.Helper()
	repo := &componentRepo{plates: map[int64]*plate.Plate{1: {ID: 1}}}
	svc := plate.NewService(repo, fakeSlots{}, foods)
	return svc, repo
}

func intPtr(v int) *int             { return &v }
func float64Ptr(v float64) *float64 { return &v }
func strPtr(v string) *string       { return &v }

func TestAddComponent_Composed_Portions_OK(t *testing.T) {
	foods := fakeFoods{byID: map[int64]*food.Food{10: composedFood(10, "Bolognese")}}
	svc, _ := newComponentService(t, foods)

	pc, err := svc.AddComponent(context.Background(), 1, &plate.PlateComponent{
		FoodID:   10,
		Portions: intPtr(2),
	})
	require.NoError(t, err)
	require.NotNil(t, pc.Portions)
	assert.Equal(t, 2, *pc.Portions)
	assert.Nil(t, pc.Amount)
	assert.Nil(t, pc.Unit)
	assert.Nil(t, pc.Grams)
	assert.Nil(t, pc.GramsSource)
}

func TestAddComponent_Composed_Rejects_Amount(t *testing.T) {
	foods := fakeFoods{byID: map[int64]*food.Food{10: composedFood(10, "Bolognese")}}
	svc, _ := newComponentService(t, foods)

	_, err := svc.AddComponent(context.Background(), 1, &plate.PlateComponent{
		FoodID: 10,
		Amount: float64Ptr(100),
		Unit:   strPtr("g"),
	})
	require.Error(t, err)
	require.True(t, errors.Is(err, domain.ErrInvalidInput))
	require.True(t, errors.Is(err, plate.ErrInvalidQuantityForComposed))
}

func TestAddComponent_Composed_Rejects_Zero_Portions(t *testing.T) {
	foods := fakeFoods{byID: map[int64]*food.Food{10: composedFood(10, "Bolognese")}}
	svc, _ := newComponentService(t, foods)

	_, err := svc.AddComponent(context.Background(), 1, &plate.PlateComponent{
		FoodID:   10,
		Portions: intPtr(0),
	})
	require.Error(t, err)
	require.True(t, errors.Is(err, domain.ErrInvalidInput))
}

func TestAddComponent_Leaf_Mass_Direct(t *testing.T) {
	foods := fakeFoods{byID: map[int64]*food.Food{20: leafFood(20, "Rice")}}
	svc, _ := newComponentService(t, foods)

	pc, err := svc.AddComponent(context.Background(), 1, &plate.PlateComponent{
		FoodID: 20,
		Amount: float64Ptr(200),
		Unit:   strPtr("g"),
	})
	require.NoError(t, err)
	require.NotNil(t, pc.Grams)
	assert.InDelta(t, 200.0, *pc.Grams, 0.01)
	require.NotNil(t, pc.GramsSource)
	assert.Equal(t, "direct", *pc.GramsSource)
	assert.Nil(t, pc.Portions)
}

func TestAddComponent_Leaf_Mass_Default(t *testing.T) {
	foods := fakeFoods{byID: map[int64]*food.Food{20: leafFood(20, "Rice")}}
	svc, _ := newComponentService(t, foods)

	pc, err := svc.AddComponent(context.Background(), 1, &plate.PlateComponent{
		FoodID: 20,
		Amount: float64Ptr(7),
		Unit:   strPtr("oz"),
	})
	require.NoError(t, err)
	require.NotNil(t, pc.Grams)
	assert.InDelta(t, 198.4465, *pc.Grams, 0.1)
	assert.Equal(t, "default", *pc.GramsSource)
}

func TestAddComponent_Leaf_Volume_Fallback(t *testing.T) {
	foods := fakeFoods{byID: map[int64]*food.Food{20: leafFood(20, "Rice")}}
	svc, _ := newComponentService(t, foods)

	pc, err := svc.AddComponent(context.Background(), 1, &plate.PlateComponent{
		FoodID: 20,
		Amount: float64Ptr(200),
		Unit:   strPtr("ml"),
	})
	require.NoError(t, err)
	require.NotNil(t, pc.Grams)
	assert.InDelta(t, 200.0, *pc.Grams, 0.01)
	assert.Equal(t, "fallback", *pc.GramsSource)
}

func TestAddComponent_Leaf_Portion_Override(t *testing.T) {
	foods := fakeFoods{
		byID: map[int64]*food.Food{30: leafFood(30, "Apple")},
		portions: map[int64][]food.Portion{
			30: {{FoodID: 30, Unit: "apple", Grams: 180}},
		},
	}
	svc, _ := newComponentService(t, foods)

	pc, err := svc.AddComponent(context.Background(), 1, &plate.PlateComponent{
		FoodID: 30,
		Amount: float64Ptr(1),
		Unit:   strPtr("apple"),
	})
	require.NoError(t, err)
	require.NotNil(t, pc.Grams)
	assert.InDelta(t, 180.0, *pc.Grams, 0.01)
	assert.Equal(t, "portion", *pc.GramsSource)
}

func TestAddComponent_Leaf_Count_Without_Portion_Errors(t *testing.T) {
	foods := fakeFoods{byID: map[int64]*food.Food{20: leafFood(20, "Rice")}}
	svc, _ := newComponentService(t, foods)

	_, err := svc.AddComponent(context.Background(), 1, &plate.PlateComponent{
		FoodID: 20,
		Amount: float64Ptr(1),
		Unit:   strPtr("slice"),
	})
	require.Error(t, err)
	require.True(t, errors.Is(err, domain.ErrInvalidInput))
	require.True(t, errors.Is(err, plate.ErrUnitRequiresPortion))
}

func TestAddComponent_Leaf_Rejects_Portions(t *testing.T) {
	foods := fakeFoods{byID: map[int64]*food.Food{20: leafFood(20, "Rice")}}
	svc, _ := newComponentService(t, foods)

	_, err := svc.AddComponent(context.Background(), 1, &plate.PlateComponent{
		FoodID:   20,
		Portions: intPtr(1),
	})
	require.Error(t, err)
	require.True(t, errors.Is(err, plate.ErrInvalidQuantityForLeaf))
}

func TestAddComponent_Rejects_Both_Shapes(t *testing.T) {
	foods := fakeFoods{byID: map[int64]*food.Food{20: leafFood(20, "Rice")}}
	svc, _ := newComponentService(t, foods)

	_, err := svc.AddComponent(context.Background(), 1, &plate.PlateComponent{
		FoodID:   20,
		Portions: intPtr(1),
		Amount:   float64Ptr(100),
		Unit:     strPtr("g"),
	})
	require.Error(t, err)
	require.True(t, errors.Is(err, plate.ErrInvalidQuantityShape))
}

func TestAddComponent_Rejects_Neither_Shape(t *testing.T) {
	foods := fakeFoods{byID: map[int64]*food.Food{20: leafFood(20, "Rice")}}
	svc, _ := newComponentService(t, foods)

	_, err := svc.AddComponent(context.Background(), 1, &plate.PlateComponent{FoodID: 20})
	require.Error(t, err)
	require.True(t, errors.Is(err, plate.ErrInvalidQuantityShape))
}

func TestUpdateComponentQuantity_Reresolves_Grams(t *testing.T) {
	foods := fakeFoods{
		byID: map[int64]*food.Food{30: leafFood(30, "Apple")},
		portions: map[int64][]food.Portion{
			30: {{FoodID: 30, Unit: "apple", Grams: 180}},
		},
	}
	svc, repo := newComponentService(t, foods)

	pc, err := svc.AddComponent(context.Background(), 1, &plate.PlateComponent{
		FoodID: 30, Amount: float64Ptr(200), Unit: strPtr("g"),
	})
	require.NoError(t, err)
	assert.Equal(t, "direct", *pc.GramsSource)

	updated, err := svc.UpdateComponentQuantity(context.Background(), pc.ID, plate.PlateComponent{
		Amount: float64Ptr(2),
		Unit:   strPtr("apple"),
	})
	require.NoError(t, err)
	require.NotNil(t, updated.Grams)
	assert.InDelta(t, 360.0, *updated.Grams, 0.01)
	assert.Equal(t, "portion", *updated.GramsSource)

	// Repo state reflects the swap.
	stored, err := repo.GetComponent(context.Background(), pc.ID)
	require.NoError(t, err)
	require.NotNil(t, stored.Grams)
	assert.InDelta(t, 360.0, *stored.Grams, 0.01)
}
