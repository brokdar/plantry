package sqlite_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/component"
	"github.com/jaltszeimer/plantry/backend/internal/domain/feedback"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planner"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
)

func seedPlateFor(t *testing.T, f *plateFixture, day int) *plate.Plate {
	t.Helper()
	p := &plate.Plate{WeekID: f.week.ID, Day: day, SlotID: f.slot.ID}
	require.NoError(t, f.plates.Create(context.Background(), p))
	return p
}

func TestFeedbackRepo_UpsertCreatesThenReplaces(t *testing.T) {
	f := setupPlateFixture(t)
	repo := sqlite.NewFeedbackRepo(f.db)
	ctx := context.Background()

	p := seedPlateFor(t, f, 0)
	note := "too spicy"
	fb := &feedback.PlateFeedback{PlateID: p.ID, Status: feedback.StatusLoved, Note: &note}
	require.NoError(t, repo.Upsert(ctx, fb))
	assert.False(t, fb.RatedAt.IsZero())

	got, err := repo.Get(ctx, p.ID)
	require.NoError(t, err)
	assert.Equal(t, feedback.StatusLoved, got.Status)
	require.NotNil(t, got.Note)
	assert.Equal(t, "too spicy", *got.Note)

	// Upsert the same plate_id with a different status — row is replaced, not duplicated.
	fb2 := &feedback.PlateFeedback{PlateID: p.ID, Status: feedback.StatusSkipped}
	require.NoError(t, repo.Upsert(ctx, fb2))

	got2, err := repo.Get(ctx, p.ID)
	require.NoError(t, err)
	assert.Equal(t, feedback.StatusSkipped, got2.Status)
	assert.Nil(t, got2.Note, "upsert clears note when not provided")

	rows, err := repo.ListByWeek(ctx, f.week.ID)
	require.NoError(t, err)
	assert.Len(t, rows, 1, "plate_id is PK — no duplicates")
}

func TestFeedbackRepo_UpsertUnknownPlateReturnsNotFound(t *testing.T) {
	f := setupPlateFixture(t)
	repo := sqlite.NewFeedbackRepo(f.db)
	ctx := context.Background()

	err := repo.Upsert(ctx, &feedback.PlateFeedback{PlateID: 9999, Status: feedback.StatusCooked})
	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrNotFound))
}

func TestFeedbackRepo_GetMissing(t *testing.T) {
	f := setupPlateFixture(t)
	repo := sqlite.NewFeedbackRepo(f.db)
	ctx := context.Background()

	p := seedPlateFor(t, f, 0)
	_, err := repo.Get(ctx, p.ID)
	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrNotFound))
}

func TestFeedbackRepo_DeleteMissingReturnsNotFound(t *testing.T) {
	f := setupPlateFixture(t)
	repo := sqlite.NewFeedbackRepo(f.db)

	err := repo.Delete(context.Background(), 9999)
	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrNotFound))
}

func TestFeedbackRepo_DeleteExisting(t *testing.T) {
	f := setupPlateFixture(t)
	repo := sqlite.NewFeedbackRepo(f.db)
	ctx := context.Background()

	p := seedPlateFor(t, f, 0)
	require.NoError(t, repo.Upsert(ctx, &feedback.PlateFeedback{PlateID: p.ID, Status: feedback.StatusCooked}))
	require.NoError(t, repo.Delete(ctx, p.ID))

	_, err := repo.Get(ctx, p.ID)
	assert.True(t, errors.Is(err, domain.ErrNotFound))
}

func TestFeedbackRepo_CascadesOnPlateDelete(t *testing.T) {
	f := setupPlateFixture(t)
	repo := sqlite.NewFeedbackRepo(f.db)
	ctx := context.Background()

	p := seedPlateFor(t, f, 0)
	require.NoError(t, repo.Upsert(ctx, &feedback.PlateFeedback{PlateID: p.ID, Status: feedback.StatusCooked}))

	require.NoError(t, f.plates.Delete(ctx, p.ID))

	_, err := repo.Get(ctx, p.ID)
	assert.True(t, errors.Is(err, domain.ErrNotFound), "feedback row should cascade delete")
}

func TestFeedbackRepo_ListByWeekScopesToWeek(t *testing.T) {
	f := setupPlateFixture(t)
	repo := sqlite.NewFeedbackRepo(f.db)
	ctx := context.Background()

	p1 := seedPlateFor(t, f, 0)
	p2 := seedPlateFor(t, f, 1)

	// A second week with one plate — feedback on it must NOT leak into week-1 list.
	weekRepo := sqlite.NewWeekRepo(f.db)
	w2 := &planner.Week{Year: 2027, WeekNumber: 5}
	require.NoError(t, weekRepo.Create(ctx, w2))
	p3 := &plate.Plate{WeekID: w2.ID, Day: 0, SlotID: f.slot.ID}
	require.NoError(t, f.plates.Create(ctx, p3))

	require.NoError(t, repo.Upsert(ctx, &feedback.PlateFeedback{PlateID: p1.ID, Status: feedback.StatusCooked}))
	require.NoError(t, repo.Upsert(ctx, &feedback.PlateFeedback{PlateID: p2.ID, Status: feedback.StatusLoved}))
	require.NoError(t, repo.Upsert(ctx, &feedback.PlateFeedback{PlateID: p3.ID, Status: feedback.StatusSkipped}))

	rows, err := repo.ListByWeek(ctx, f.week.ID)
	require.NoError(t, err)
	assert.Len(t, rows, 2)
	ids := []int64{rows[0].PlateID, rows[1].PlateID}
	assert.Contains(t, ids, p1.ID)
	assert.Contains(t, ids, p2.ID)
	assert.NotContains(t, ids, p3.ID)
}

// TestTxRunner_FeedbackRollsBackOnClosureError verifies the real SQLite
// transaction boundary inside RunInFeedbackTx. A closure that upserts
// feedback and then returns an error must NOT persist the feedback row.
// This covers the rollback path that the in-memory fake in the domain
// service_test.go cannot exercise.
func TestTxRunner_FeedbackRollsBackOnClosureError(t *testing.T) {
	f := setupPlateFixture(t)
	ctx := context.Background()
	p := seedPlateFor(t, f, 3)

	tx := sqlite.NewTxRunner(f.db)
	boom := errors.New("simulated failure")
	err := tx.RunInFeedbackTx(ctx, func(fb feedback.Repository, _ component.Repository, _ profile.Repository) error {
		if err := fb.Upsert(ctx, &feedback.PlateFeedback{PlateID: p.ID, Status: feedback.StatusLoved}); err != nil {
			return err
		}
		return boom
	})
	require.ErrorIs(t, err, boom)

	// Feedback must not be visible outside the aborted tx.
	repo := sqlite.NewFeedbackRepo(f.db)
	_, getErr := repo.Get(ctx, p.ID)
	assert.True(t, errors.Is(getErr, domain.ErrNotFound), "tx rollback must discard feedback upsert")
}

// TestTxRunner_FeedbackCommitsOnSuccess verifies the happy path commits.
func TestTxRunner_FeedbackCommitsOnSuccess(t *testing.T) {
	f := setupPlateFixture(t)
	ctx := context.Background()
	p := seedPlateFor(t, f, 4)

	tx := sqlite.NewTxRunner(f.db)
	err := tx.RunInFeedbackTx(ctx, func(fb feedback.Repository, _ component.Repository, _ profile.Repository) error {
		return fb.Upsert(ctx, &feedback.PlateFeedback{PlateID: p.ID, Status: feedback.StatusCooked})
	})
	require.NoError(t, err)

	repo := sqlite.NewFeedbackRepo(f.db)
	got, err := repo.Get(ctx, p.ID)
	require.NoError(t, err)
	assert.Equal(t, feedback.StatusCooked, got.Status)
}
