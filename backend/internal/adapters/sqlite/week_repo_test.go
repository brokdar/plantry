package sqlite_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planner"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
)

func newWeekRepo(t *testing.T) *sqlite.WeekRepo {
	t.Helper()
	return sqlite.NewWeekRepo(testhelper.NewTestDB(t))
}

func TestWeekRepo_RoundTrip(t *testing.T) {
	repo := newWeekRepo(t)
	ctx := context.Background()

	w := &planner.Week{Year: 2026, WeekNumber: 16}
	require.NoError(t, repo.Create(ctx, w))
	assert.NotZero(t, w.ID)
	assert.False(t, w.CreatedAt.IsZero())

	got, err := repo.Get(ctx, w.ID)
	require.NoError(t, err)
	assert.Equal(t, 2026, got.Year)
	assert.Equal(t, 16, got.WeekNumber)
}

func TestWeekRepo_GetByYearAndNumber_NotFound(t *testing.T) {
	repo := newWeekRepo(t)
	_, err := repo.GetByYearAndNumber(context.Background(), 2026, 50)
	assert.True(t, errors.Is(err, domain.ErrNotFound))
}

func TestWeekRepo_GetByYearAndNumber_Found(t *testing.T) {
	repo := newWeekRepo(t)
	ctx := context.Background()
	w := &planner.Week{Year: 2026, WeekNumber: 30}
	require.NoError(t, repo.Create(ctx, w))

	got, err := repo.GetByYearAndNumber(ctx, 2026, 30)
	require.NoError(t, err)
	assert.Equal(t, w.ID, got.ID)
}

func TestWeekRepo_DuplicateYearWeek(t *testing.T) {
	repo := newWeekRepo(t)
	ctx := context.Background()
	require.NoError(t, repo.Create(ctx, &planner.Week{Year: 2026, WeekNumber: 1}))
	err := repo.Create(ctx, &planner.Week{Year: 2026, WeekNumber: 1})
	assert.Error(t, err)
}

func TestWeekRepo_List(t *testing.T) {
	repo := newWeekRepo(t)
	ctx := context.Background()
	for w := 1; w <= 5; w++ {
		require.NoError(t, repo.Create(ctx, &planner.Week{Year: 2026, WeekNumber: w}))
	}
	rows, total, err := repo.List(ctx, 3, 0)
	require.NoError(t, err)
	assert.Equal(t, int64(5), total)
	assert.Len(t, rows, 3)
}
