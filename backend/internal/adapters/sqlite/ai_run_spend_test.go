package sqlite_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/airun"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
)

var chefKinds = []airun.Kind{
	airun.KindChefAttributes, airun.KindChefMatch, airun.KindChefImportCheck,
	airun.KindChefFind, airun.KindChefHelp, airun.KindChefLibraryReview,
}

func TestAIRunRepo_ChefKindsInsert(t *testing.T) {
	ctx := context.Background()
	db := testhelper.NewTestDB(t)
	repo := sqlite.NewAIRunRepo(db)
	for _, k := range chefKinds {
		id, err := repo.CreateRun(ctx, airun.RunStart{Kind: k, StartedAt: time.Now()})
		require.NoError(t, err, k)
		var got string
		require.NoError(t, db.QueryRow(`SELECT kind FROM ai_runs WHERE id = ?`, id).Scan(&got))
		assert.Equal(t, string(k), got)
	}
}

func TestAIRunRepo_UnknownKindRejected(t *testing.T) {
	ctx := context.Background()
	db := testhelper.NewTestDB(t)
	repo := sqlite.NewAIRunRepo(db)
	_, err := repo.CreateRun(ctx, airun.RunStart{Kind: "chef_bogus", StartedAt: time.Now()})
	require.ErrorIs(t, err, domain.ErrInvalidInput)
	var n int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM ai_runs`).Scan(&n))
	assert.Zero(t, n)
}

func TestAIRunRepo_FinishRunStoresCost(t *testing.T) {
	ctx := context.Background()
	db := testhelper.NewTestDB(t)
	repo := sqlite.NewAIRunRepo(db)
	cost := 0.0123
	for _, c := range []*float64{&cost, nil} {
		id, err := repo.CreateRun(ctx, airun.RunStart{Kind: airun.KindChefFind, StartedAt: time.Now()})
		require.NoError(t, err)
		require.NoError(t, repo.FinishRun(ctx, airun.RunEnd{ID: id, Status: airun.StatusOK, FinishedAt: time.Now(), CostUSD: c}))
		var got sql.NullFloat64
		require.NoError(t, db.QueryRow(`SELECT cost_usd FROM ai_runs WHERE id = ?`, id).Scan(&got))
		if c == nil {
			assert.False(t, got.Valid, "unpriced is NULL")
			continue
		}
		assert.InDelta(t, cost, got.Float64, 1e-12)
	}
}

// Spend sums the stored costs of a calendar month, per kind list; unpriced
// and running runs and other months add nothing.
func TestSpend_SumsStoredCosts(t *testing.T) {
	ctx := context.Background()
	db := testhelper.NewTestDB(t)
	repo := sqlite.NewAIRunRepo(db)
	nov := func(day, hour int) time.Time { return time.Date(2026, 11, day, hour, 0, 0, 0, time.UTC) }
	f := func(v float64) *float64 { return &v }
	runs := []struct {
		kind    airun.Kind
		started time.Time
		cost    *float64
		running bool
	}{
		{airun.KindChefFind, nov(1, 0), f(0.50), false},
		{airun.KindChefHelp, nov(15, 12), f(0.25), false},
		{airun.KindChefMatch, nov(30, 23), nil, false},                                     // unpriced
		{airun.KindChefFind, nov(20, 8), nil, true},                                        // running
		{airun.KindChat, nov(10, 9), f(1.00), false},                                       // not capped
		{airun.KindChefFind, time.Date(2026, 10, 31, 23, 59, 0, 0, time.UTC), f(9), false}, // October
		{airun.KindChefFind, time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC), f(9), false},    // December
	}
	for _, r := range runs {
		id, err := repo.CreateRun(ctx, airun.RunStart{Kind: r.kind, StartedAt: r.started})
		require.NoError(t, err)
		if !r.running {
			require.NoError(t, repo.FinishRun(ctx, airun.RunEnd{ID: id, Status: airun.StatusOK, FinishedAt: r.started, CostUSD: r.cost}))
		}
	}

	month := nov(18, 0)
	got, err := airun.Spend(ctx, repo, month, chefKinds)
	require.NoError(t, err)
	assert.InDelta(t, 0.75, got, 1e-9)

	got, err = airun.Spend(ctx, repo, month, []airun.Kind{airun.KindChefHelp})
	require.NoError(t, err)
	assert.InDelta(t, 0.25, got, 1e-9)

	got, err = airun.Spend(ctx, repo, month, nil)
	require.NoError(t, err)
	assert.InDelta(t, 1.75, got, 1e-9, "no kinds: every kind")

	got, err = airun.Spend(ctx, repo, time.Date(2026, 9, 5, 0, 0, 0, 0, time.UTC), nil)
	require.NoError(t, err)
	assert.Zero(t, got, "a month without runs")
}
