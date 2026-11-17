package sqlite_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
)

func TestPlanLightRepo_ReplaceListPrune(t *testing.T) {
	ctx := context.Background()
	repo := sqlite.NewPlanLightRepo(testhelper.NewTestDB(t))
	d := func(s string) time.Time { x, _ := time.Parse(time.DateOnly, s); return x }
	today := d("2026-11-10")

	require.NoError(t, repo.Replace(ctx, today, []string{"2026-11-09", "2026-11-11", "2026-11-12"}, []planning.LightIntent{
		{Date: "2026-11-09", Scope: "day"},
		{Date: "2026-11-11", Scope: "meal", Slot: "dinner"},
		{Date: "2026-11-12", Scope: "day"},
	}))
	got, err := repo.List(ctx, d("2026-11-01"), d("2026-11-30"))
	require.NoError(t, err)
	assert.Equal(t, []planning.LightIntent{
		{Date: "2026-11-11", Scope: "meal", Slot: "dinner"},
		{Date: "2026-11-12", Scope: "day"},
	}, got, "a past date is pruned on write")

	// Replacing a date overwrites it; a date without intents is cleared.
	require.NoError(t, repo.Replace(ctx, today, []string{"2026-11-11", "2026-11-12"}, []planning.LightIntent{{Date: "2026-11-11", Scope: "day"}}))
	got, err = repo.List(ctx, d("2026-11-11"), d("2026-11-12"))
	require.NoError(t, err)
	assert.Equal(t, []planning.LightIntent{{Date: "2026-11-11", Scope: "day"}}, got)
}

func TestPlanLightRepo_Level(t *testing.T) {
	ctx := context.Background()
	repo := sqlite.NewPlanLightRepo(testhelper.NewTestDB(t))
	d := func(s string) time.Time { x, _ := time.Parse(time.DateOnly, s); return x }
	in := []planning.LightIntent{
		{Date: "2026-11-11", Scope: "meal", Slot: "dinner", Level: planning.LevelHearty},
		{Date: "2026-11-11", Scope: "meal", Slot: "lunch"},
	}
	require.NoError(t, repo.Replace(ctx, d("2026-11-10"), []string{"2026-11-11"}, in))
	got, err := repo.List(ctx, d("2026-11-11"), d("2026-11-11"))
	require.NoError(t, err)
	assert.ElementsMatch(t, in, got, "hearty round-trips, light reads back as the empty default")
}
