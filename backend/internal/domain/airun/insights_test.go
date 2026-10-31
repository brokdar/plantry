package airun

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// memInsights serves fixed rows; ListRuns applies the query like the SQL does.
type memInsights struct {
	rows    []RunRow // newest first
	corr    map[int64]Correction
	corrErr error
	queries []RunQuery
}

func (m *memInsights) ListRuns(_ context.Context, q RunQuery) ([]RunRow, error) {
	m.queries = append(m.queries, q)
	var out []RunRow
	for _, r := range m.rows {
		if q.Kind != "" && r.Kind != q.Kind ||
			q.From != nil && r.StartedAt.Before(*q.From) ||
			q.To != nil && !r.StartedAt.Before(*q.To) ||
			q.BeforeID > 0 && r.ID >= q.BeforeID {
			continue
		}
		if q.Limit >= 0 && len(out) == q.Limit {
			break
		}
		out = append(out, r)
	}
	return out, nil
}

func (m *memInsights) GetRun(context.Context, int64) (RunDetailRow, error) {
	return RunDetailRow{}, errors.New("unused")
}

func (m *memInsights) ListSteps(context.Context, int64) ([]StepView, error) { return nil, nil }

func (m *memInsights) Corrections(_ context.Context, minID, maxID int64) (map[int64]Correction, error) {
	out := map[int64]Correction{}
	for id, c := range m.corr {
		if id >= minID && id <= maxID {
			out[id] = c
		}
	}
	return out, m.corrErr
}

func noOverrides(context.Context) (map[string]Price, error) { return nil, nil }

func at(month time.Month, day int) time.Time {
	return time.Date(2026, month, day, 9, 0, 0, 0, time.UTC)
}

func TestInsights_RunsPagesByCursor(t *testing.T) {
	store := &memInsights{
		rows: []RunRow{
			{ID: 5, Kind: KindChat, StartedAt: at(10, 5), Model: "claude-sonnet-4-6", Provider: "anthropic", Tokens: Tokens{Input: 1_000_000}},
			{ID: 4, Kind: KindGenerate, StartedAt: at(10, 4), Model: "mystery", Provider: "fake", Tokens: Tokens{Input: 10}},
			{ID: 3, Kind: KindGenerate, StartedAt: at(10, 3), Chooser: ChooserHeuristic},
		},
		corr: map[int64]Correction{4: {Generated: 4, Corrected: 1}},
	}
	svc := NewInsights(store, noOverrides)

	first, err := svc.Runs(context.Background(), RunQuery{Limit: 2})
	require.NoError(t, err)
	require.Len(t, first.Items, 2)
	require.NotNil(t, first.NextCursor)
	assert.Equal(t, int64(4), *first.NextCursor)
	require.NotNil(t, first.Items[0].CostUSD)
	assert.InDelta(t, 3.0, *first.Items[0].CostUSD, 1e-9)
	assert.Nil(t, first.Items[0].CorrectionRate)
	assert.Nil(t, first.Items[1].CostUSD, "unpriced model")
	require.NotNil(t, first.Items[1].CorrectionRate)
	assert.InDelta(t, 0.25, *first.Items[1].CorrectionRate, 1e-9)
	assert.Equal(t, 3, store.queries[0].Limit, "reads one extra row to detect the next page")

	second, err := svc.Runs(context.Background(), RunQuery{Limit: 2, BeforeID: *first.NextCursor})
	require.NoError(t, err)
	require.Len(t, second.Items, 1)
	assert.Nil(t, second.NextCursor)
	require.NotNil(t, second.Items[0].CostUSD, "a heuristic run has no tokens and costs 0")
	assert.Zero(t, *second.Items[0].CostUSD)
}

func TestInsights_RunsOverridesReprice(t *testing.T) {
	store := &memInsights{rows: []RunRow{{ID: 1, Model: "mystery", Provider: "openai", Tokens: Tokens{Output: 1_000_000}}}}
	svc := NewInsights(store, func(context.Context) (map[string]Price, error) {
		return map[string]Price{"mystery": {In: 1, Out: 2}}, nil
	})
	page, err := svc.Runs(context.Background(), RunQuery{Limit: 50})
	require.NoError(t, err)
	require.NotNil(t, page.Items[0].CostUSD)
	assert.InDelta(t, 2.0, *page.Items[0].CostUSD, 1e-9)
}

func TestInsights_WeeklyBuckets(t *testing.T) {
	// 2026-10-05 and 2026-10-12 are Mondays.
	store := &memInsights{
		rows: []RunRow{
			{ID: 6, Kind: KindChat, StartedAt: at(10, 25), Model: "mystery", Tokens: Tokens{Input: 5}}, // outside the range
			{
				ID: 5, Kind: KindGenerate, StartedAt: at(10, 13), Model: "claude-sonnet-4-6", Provider: "anthropic",
				Tokens: Tokens{Input: 1_000_000}, Outcome: "discarded", PromptVersion: "p1",
				ScoreJSON: `{"coverage":0.5,"variety":{"distinct":4}}`, ViolationsJSON: `[{"rule":"H2"},{"rule":"H8"}]`,
			},
			{
				ID: 4, Kind: KindGenerate, StartedAt: at(10, 12), Model: "claude-sonnet-4-6", Provider: "anthropic",
				Tokens: Tokens{Input: 1_000_000}, Outcome: "accepted", PromptVersion: "p1",
				ScoreJSON: `{"coverage":1,"variety":{"distinct":6},"per_day":[1,2]}`, ViolationsJSON: `[{"rule":"H2"}]`,
			},
			{ID: 3, Kind: KindChat, StartedAt: at(10, 5), Model: "mystery", Provider: "fake", Tokens: Tokens{Input: 5}}, // Monday before from
			{ID: 2, Kind: KindChat, StartedAt: at(10, 6), Model: "mystery", Provider: "fake", Tokens: Tokens{Input: 5}},
			{ID: 1, Kind: KindGenerate, StartedAt: at(10, 7), Chooser: ChooserHeuristic, Outcome: "partially_accepted"},
		},
		corr: map[int64]Correction{1: {Generated: 4, Corrected: 1}, 4: {Generated: 2, Corrected: 1}, 5: {Generated: 2}},
	}
	svc := NewInsights(store, noOverrides)

	rep, err := svc.Weekly(context.Background(), time.Date(2026, 10, 6, 0, 0, 0, 0, time.UTC), time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	assert.Equal(t, PricesAsOf, rep.PricesAsOf)
	require.Len(t, rep.Weeks, 3, "Oct 5, 12 and 19 (empty)")

	w1 := rep.Weeks[0]
	assert.Equal(t, time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC), w1.Start)
	assert.Equal(t, 3, w1.Runs)
	assert.Equal(t, 1, w1.Accepted)
	assert.Equal(t, 2, w1.UnpricedRuns, "the two unpriced chats; the heuristic run costs 0")
	assert.Zero(t, w1.CostUSD)
	require.NotNil(t, w1.CostPerAccepted)
	assert.Zero(t, *w1.CostPerAccepted)
	require.NotNil(t, w1.CorrectionRate)
	assert.InDelta(t, 0.25, *w1.CorrectionRate, 1e-9)

	w2 := rep.Weeks[1]
	assert.Equal(t, 2, w2.Runs)
	assert.Equal(t, 1, w2.Accepted)
	assert.Equal(t, 1, w2.Discarded)
	assert.Zero(t, w2.UnpricedRuns)
	assert.InDelta(t, 6.0, w2.CostUSD, 1e-9)
	assert.InDelta(t, 6.0, *w2.CostPerAccepted, 1e-9)
	assert.InDelta(t, 0.25, *w2.CorrectionRate, 1e-9)

	w3 := rep.Weeks[2]
	assert.Zero(t, w3.Runs)
	assert.Nil(t, w3.CorrectionRate)
	assert.Nil(t, w3.CostPerAccepted)

	require.Len(t, rep.ByVersion, 3)
	assert.Equal(t, KindChat, rep.ByVersion[0].Kind)
	assert.Equal(t, 2, rep.ByVersion[0].Runs)
	assert.Empty(t, rep.ByVersion[0].ScoreAvg)
	gen := rep.ByVersion[2]
	assert.Equal(t, VersionGroup{
		Kind: KindGenerate, PromptVersion: "p1", Model: "claude-sonnet-4-6", Runs: 2,
		ScoreAvg:   map[string]float64{"coverage": 0.75, "variety.distinct": 5},
		Violations: map[string]int{"H2": 2, "H8": 1},
	}, gen)

	q := store.queries[0]
	assert.Equal(t, -1, q.Limit)
	assert.Equal(t, time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC), *q.From, "from snaps to its Monday")
	assert.Equal(t, time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC), *q.To, "to is inclusive")
}

func TestInsights_Price(t *testing.T) {
	svc := NewInsights(&memInsights{}, func(context.Context) (map[string]Price, error) {
		return map[string]Price{"mine": {In: 1, Out: 1}}, nil
	})
	ctx := context.Background()
	for _, tt := range []struct {
		model string
		src   PriceSource
	}{{"mine", PriceOverride}, {"claude-opus-5-5", PriceBuiltin}, {"llama", PriceNone}} {
		info, err := svc.Price(ctx, tt.model)
		require.NoError(t, err)
		assert.Equal(t, tt.src, info.Source, tt.model)
		assert.Equal(t, tt.src != PriceNone, info.Price != nil, tt.model)
		assert.Equal(t, PricesAsOf, info.AsOf)
	}
}
