package plate_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
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

// fakeSlots and fakeFoods satisfy the SlotChecker / FoodChecker interfaces.
type fakeSlots struct{}

func (fakeSlots) Exists(_ context.Context, _ int64) (bool, error) { return true, nil }

type fakeFoods struct{}

func (fakeFoods) Exists(_ context.Context, _ int64) (bool, error) { return true, nil }

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
