package chef_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/airun"
	"github.com/jaltszeimer/plantry/backend/internal/domain/chef"
	"github.com/jaltszeimer/plantry/backend/internal/domain/settings"
)

type fakeSettings struct {
	on     bool
	capUSD float64
	ai     settings.AIConfig
}

func (f fakeSettings) ChefEnabled(context.Context) (bool, error)       { return f.on, nil }
func (f fakeSettings) ChefMonthlyCap(context.Context) (float64, error) { return f.capUSD, nil }
func (f fakeSettings) EffectiveAI(context.Context) (settings.AIConfig, error) {
	return f.ai, nil
}

type fakeSpend struct {
	usd   float64
	kinds []airun.Kind
	from  time.Time
}

func (f *fakeSpend) SpendUSD(_ context.Context, from, _ time.Time, kinds []airun.Kind) (float64, error) {
	f.from, f.kinds = from, kinds
	return f.usd, nil
}

var (
	priced   = settings.AIConfig{Provider: "anthropic", Model: "claude-sonnet-5-5", APIKey: "k"}
	unpriced = settings.AIConfig{Provider: "anthropic", Model: "mystery-model", APIKey: "k"}
	noPrices = func(context.Context) (map[string]airun.Price, error) { return nil, nil }
	nov25    = func() time.Time { return time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC) }
)

// AC-17 skeleton and COST-2/3: at the cap autonomous work stops, requested
// work warns, help always runs; an unpriced model does no autonomous work;
// the switch off or no provider stops everything but help.
func TestGuard_Check(t *testing.T) {
	cases := []struct {
		name      string
		s         fakeSettings
		spent     float64
		kind      airun.Kind
		requested bool
		want      chef.Decision
	}{
		{"under cap autonomous", fakeSettings{true, 5, priced}, 1, airun.KindChefFind, false, chef.Decision{Run: true}},
		{"at cap autonomous stops", fakeSettings{true, 5, priced}, 5, airun.KindChefFind, false, chef.Decision{Reason: chef.ReasonCap}},
		{"at cap requested warns", fakeSettings{true, 5, priced}, 5.2, airun.KindChefFind, true, chef.Decision{Run: true, Warn: true, Reason: chef.ReasonCap}},
		{"at cap help runs", fakeSettings{true, 5, priced}, 9, airun.KindChefHelp, true, chef.Decision{Run: true}},
		{"switch off help runs", fakeSettings{false, 5, priced}, 0, airun.KindChefHelp, true, chef.Decision{Run: true}},
		{"switch off autonomous", fakeSettings{false, 5, priced}, 0, airun.KindChefAttributes, false, chef.Decision{Reason: chef.ReasonOff}},
		{"switch off requested", fakeSettings{false, 5, priced}, 0, airun.KindChefFind, true, chef.Decision{Reason: chef.ReasonOff}},
		{"unpriced autonomous", fakeSettings{true, 5, unpriced}, 0, airun.KindChefFind, false, chef.Decision{Reason: chef.ReasonUnpriced}},
		{"unpriced requested runs", fakeSettings{true, 5, unpriced}, 0, airun.KindChefFind, true, chef.Decision{Run: true}},
		{"no provider", fakeSettings{true, 5, settings.AIConfig{}}, 0, airun.KindChefFind, true, chef.Decision{Reason: chef.ReasonNoProvider}},
		{"no key", fakeSettings{true, 5, settings.AIConfig{Provider: "openai", Model: "gpt-5"}}, 0, airun.KindChefFind, false, chef.Decision{Reason: chef.ReasonNoProvider}},
		{"cap zero stops autonomous", fakeSettings{true, 0, priced}, 0, airun.KindChefMatch, false, chef.Decision{Reason: chef.ReasonCap}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			spend := &fakeSpend{usd: tc.spent}
			g := &chef.Guard{Settings: tc.s, Spend: spend, Prices: noPrices, Now: nov25}
			got, err := g.Check(context.Background(), tc.kind, tc.requested)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestGuard_PriceOverrideMakesModelPriced(t *testing.T) {
	overrides := func(context.Context) (map[string]airun.Price, error) {
		return map[string]airun.Price{"mystery-model": {In: 1, Out: 2}}, nil
	}
	g := &chef.Guard{Settings: fakeSettings{true, 5, unpriced}, Spend: &fakeSpend{}, Prices: overrides, Now: nov25}
	got, err := g.Check(context.Background(), airun.KindChefFind, false)
	require.NoError(t, err)
	assert.True(t, got.Run)
}

func TestGuard_SpentCountsCappedKindsThisMonth(t *testing.T) {
	spend := &fakeSpend{usd: 2.5}
	g := &chef.Guard{Settings: fakeSettings{true, 5, priced}, Spend: spend, Prices: noPrices, Now: nov25}
	got, err := g.Spent(context.Background())
	require.NoError(t, err)
	assert.Equal(t, chef.MonthSpend{Month: "2026-11", SpentUSD: 2.5, CapUSD: 5}, got)
	assert.Equal(t, chef.CappedKinds, spend.kinds, "only capped kinds count (COST-1)")
	assert.Equal(t, time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC), spend.from)
}

func TestGuard_RejectsUncappedKind(t *testing.T) {
	g := &chef.Guard{Settings: fakeSettings{true, 5, priced}, Spend: &fakeSpend{}, Prices: noPrices, Now: nov25}
	_, err := g.Check(context.Background(), airun.KindChat, false)
	assert.Error(t, err)
}
