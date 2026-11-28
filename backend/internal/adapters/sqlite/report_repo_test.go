package sqlite_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/report"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
)

func reportNow(t *testing.T) time.Time {
	t.Helper()
	now, err := time.Parse("2006-01-02 15:04:05", testhelper.ReportNow)
	require.NoError(t, err)
	return now
}

func collect(t *testing.T, seed bool) report.Data {
	t.Helper()
	db := testhelper.NewTestDB(t)
	if seed {
		testhelper.SeedReport(t, db)
	}
	d, err := sqlite.NewReportRepo(db).Collect(context.Background(), sqlite.ReportOpts{Now: reportNow(t), Days: 30, Version: "v1", DBBytes: 2_000_000})
	require.NoError(t, err)
	return d
}

func kind(d report.Data, name string) report.KindStat {
	for _, k := range d.AI.Kinds {
		if k.Kind == name {
			return k
		}
	}
	return report.KindStat{}
}

func TestReportCollect_AI(t *testing.T) {
	t.Parallel()
	d := collect(t, true)

	help := kind(d, "chef_help")
	assert.Equal(t, 4, help.Runs, "the run before the window is left out")
	assert.Equal(t, 1, help.Errors)
	assert.Equal(t, []report.KeyCount{{Key: "error.ai.provider_error", N: 1}}, help.TopErrors)
	// Nearest rank over 1000, 2000, 4000, 12000.
	assert.Equal(t, 4, help.Timed)
	assert.EqualValues(t, 2000, help.P50)
	assert.EqualValues(t, 12000, help.P90)
	assert.InDelta(t, 0.04, help.USD, 1e-9)

	assert.Equal(t, 1, d.AI.Untracked)
	require.Len(t, d.AI.Spend, 1)
	assert.Equal(t, "2026-11", d.AI.Spend[0].Month)
	assert.InDelta(t, 4.2+0.04+0.04, d.AI.Spend[0].USD, 1e-9, "capped kinds only: find, help and import checks")
	assert.InDelta(t, 5.0, d.AI.Cap, 0, "the default cap while the setting is unset")
	assert.GreaterOrEqual(t, d.Build.SchemaVersion, 76, "the schema version is read from goose")
}

func TestReportCollect_ChefOutcomes(t *testing.T) {
	t.Parallel()
	c := collect(t, true).Chef

	assert.Equal(t, 1, c.HelpHelped)
	assert.Equal(t, 1, c.HelpNotHelped)
	assert.Equal(t, 1, c.HelpNoVerdict, "the failed run is not an answer and the old run is outside the window")
	assert.ElementsMatch(t, []report.KeyCount{{Key: "in_library/accepted", N: 2}, {Key: "rejected", N: 1}}, c.FindStates)
	assert.Equal(t, []report.KeyCount{{Key: "too_long", N: 1}}, c.FindRejectReasons)
	assert.Equal(t, report.Funnel{Accepted: 2, InLibrary: 2, Cooked: 1, Rated: 1}, c.Funnel)
	assert.Equal(t, report.Trial{Total: 2, Archived: 1, Cooked: 1}, c.Trial)
	assert.ElementsMatch(t, []report.KeyCount{{Key: "applied", N: 1}, {Key: "dismissed", N: 1}}, c.Tips)
	assert.ElementsMatch(t, []report.KeyCount{{Key: "chef_find/failed", N: 3}, {Key: "chef_attributes/done", N: 1}}, c.Jobs)

	im := c.Import
	assert.Equal(t, 2, im.Checks)
	assert.Equal(t, 1, im.Saved)
	assert.Equal(t, []report.KeyCount{{Key: "keep", N: 1}, {Key: "times", N: 1}}, im.Kept)
	assert.Equal(t, []report.KeyCount{{Key: "role", N: 1}}, im.Undone)
	assert.Equal(t, 1, im.TouchAdded)
	assert.Equal(t, 2, im.FlagsIgnored)
}

func TestReportCollect_PlanningAndRatings(t *testing.T) {
	t.Parallel()
	d := collect(t, true)

	p := d.Planning
	assert.Equal(t, 1, p.Generate)
	assert.Equal(t, 1, p.Regenerate)
	assert.Equal(t, 1, p.Heuristic)
	assert.Equal(t, 1, p.LLM)
	assert.Equal(t, 1, p.Evaluated, "the regenerate run stored no evaluation")
	assert.Equal(t, []report.KeyCount{{Key: "kcal_below_floor", N: 2}, {Key: "too_many", N: 1}}, p.Violations,
		"the closing evaluation counts, not the one before the change")

	assert.Equal(t, report.RatingCounts{Loved: 1}, d.Ratings.Chef)
	assert.Equal(t, report.RatingCounts{Disliked: 1}, d.Ratings.Other)
}

func TestReportCollect_EmptyDatabase(t *testing.T) {
	t.Parallel()
	d := collect(t, false)
	assert.Empty(t, d.AI.Kinds)
	assert.Zero(t, d.AI.Untracked)
	assert.Empty(t, d.AI.Spend)
	assert.Zero(t, d.Chef.Funnel)
	assert.Zero(t, d.Chef.Trial)
	assert.Zero(t, d.Planning.Generate)
	assert.Zero(t, d.Ratings)
	assert.Empty(t, report.Problems(d))
}
