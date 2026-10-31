package settings_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/settings"
)

func newSpecTestService(t *testing.T) *settings.Service {
	t.Helper()
	return settings.NewService(newMemRepo(), settings.NewEnvSnapshot(nil), newTestCipher(t))
}

func TestSettingsSpec_WeekStartsOnIsUnknown(t *testing.T) {
	svc := newSpecTestService(t)
	err := svc.Set(context.Background(), "plan.week_starts_on", "monday")
	assert.ErrorIs(t, err, settings.ErrUnknownKey)
}

func TestSettingsSpec_Anchor(t *testing.T) {
	svc := newSpecTestService(t)
	ctx := context.Background()

	valid := []string{"today", "next_shopping_day"}
	for _, v := range valid {
		require.NoErrorf(t, svc.Set(ctx, settings.KeyPlanAnchor, v), "expected %q to be accepted", v)
	}

	invalid := []string{"fixed_weekday", "tomorrow", "week_start", "Today", "", "0"}
	for _, v := range invalid {
		err := svc.Set(ctx, settings.KeyPlanAnchor, v)
		assert.ErrorIsf(t, err, settings.ErrInvalidKind, "expected %q to be rejected", v)
	}
}

func TestSettingsSpec_ShoppingDay(t *testing.T) {
	svc := newSpecTestService(t)
	ctx := context.Background()

	// boundary values — 0 (Monday) and 6 (Sunday) must be accepted
	for _, v := range []string{"0", "1", "2", "3", "4", "5", "6"} {
		require.NoErrorf(t, svc.Set(ctx, settings.KeyPlanShoppingDay, v), "expected %q to be accepted", v)
	}

	// out-of-range
	for _, v := range []string{"-1", "7", "10", "100"} {
		err := svc.Set(ctx, settings.KeyPlanShoppingDay, v)
		assert.ErrorIsf(t, err, settings.ErrInvalidKind, "expected %q to be rejected", v)
	}
}

func TestSettingsSpec_Phase1Defaults(t *testing.T) {
	svc := newSpecTestService(t)
	ctx := context.Background()

	defaults := map[string]string{
		settings.KeySetupCompleted:             "false",
		settings.KeyPlanMaxCookOccasionsPerDay: "1",
		settings.KeyPlanBatchCooking:           "true",
		settings.KeyPlanDefaultKeepDays:        "3",
		settings.KeyPlanFreezingEnabled:        "false",
		settings.KeyPlanFreezerKeepDays:        "90",
		settings.KeyPlanVarietyWeeks:           "3",
		settings.KeyPlanExplorePerWeek:         "0",
		settings.KeyDiscoveryDailyEnabled:      "true",
		settings.KeyAIRunRetentionDays:         "180",
		settings.KeyPlanAnchor:                 "next_shopping_day",
	}
	for key, want := range defaults {
		t.Run(key, func(t *testing.T) {
			v, err := svc.Get(ctx, key)
			require.NoError(t, err)
			assert.Equal(t, want, v.Raw)
			assert.Equal(t, settings.SourceDefault, v.Source)
		})
	}
}

func TestSettingsSpec_Phase1IntRanges(t *testing.T) {
	cases := []struct {
		key      string
		accepted []string
		rejected []string
	}{
		{settings.KeyPlanMaxCookOccasionsPerDay, []string{"1", "3"}, []string{"0", "4"}},
		{settings.KeyPlanDefaultKeepDays, []string{"1", "7"}, []string{"0", "8"}},
		{settings.KeyPlanFreezerKeepDays, []string{"1", "365"}, []string{"0", "366"}},
		{settings.KeyPlanVarietyWeeks, []string{"0", "8"}, []string{"-1", "9"}},
		{settings.KeyPlanExplorePerWeek, []string{"0", "3"}, []string{"-1", "4"}},
		{settings.KeyAIRunRetentionDays, []string{"7", "3650"}, []string{"6", "3651"}},
	}
	for _, tc := range cases {
		t.Run(tc.key, func(t *testing.T) {
			svc := newSpecTestService(t)
			ctx := context.Background()
			for _, v := range tc.accepted {
				require.NoErrorf(t, svc.Set(ctx, tc.key, v), "expected %q to be accepted", v)
			}
			for _, v := range append(tc.rejected, "abc", "") {
				assert.ErrorIsf(t, svc.Set(ctx, tc.key, v), settings.ErrInvalidKind, "expected %q to be rejected", v)
			}
		})
	}
}

func TestSettingsSpec_Phase1Bools(t *testing.T) {
	keys := []string{
		settings.KeySetupCompleted,
		settings.KeyPlanBatchCooking,
		settings.KeyPlanFreezingEnabled,
		settings.KeyDiscoveryDailyEnabled,
	}
	for _, key := range keys {
		t.Run(key, func(t *testing.T) {
			svc := newSpecTestService(t)
			ctx := context.Background()
			require.NoError(t, svc.Set(ctx, key, "true"))
			require.NoError(t, svc.Set(ctx, key, "false"))
			for _, v := range []string{"yes", "maybe", ""} {
				assert.ErrorIsf(t, svc.Set(ctx, key, v), settings.ErrInvalidKind, "expected %q to be rejected", v)
			}
		})
	}
}

func TestSettingsSpec_PriceOverrides(t *testing.T) {
	svc := newSpecTestService(t)
	ctx := context.Background()

	v, err := svc.Get(ctx, settings.KeyAIPriceOverrides)
	require.NoError(t, err)
	assert.Equal(t, "{}", v.Raw)

	assert.ErrorIs(t, svc.Set(ctx, settings.KeyAIPriceOverrides, `[]`), settings.ErrInvalidKind)
	assert.ErrorIs(t, svc.Set(ctx, settings.KeyAIPriceOverrides, `{"m":{"in":-1,"out":1}}`), settings.ErrInvalidKind)
	require.NoError(t, svc.Set(ctx, settings.KeyAIPriceOverrides, `{"m":{"in":1,"out":2}}`))
}

func TestSettingsSpec_RemovedAndDeferredKeysUnknown(t *testing.T) {
	svc := newSpecTestService(t)
	ctx := context.Background()

	for _, key := range []string{"plan.window_days", "discovery.cuisines"} {
		_, err := svc.Get(ctx, key)
		assert.ErrorIsf(t, err, settings.ErrUnknownKey, "Get(%q)", key)
		assert.ErrorIsf(t, svc.Set(ctx, key, "7"), settings.ErrUnknownKey, "Set(%q)", key)
	}
}
