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

func TestSettingsSpec_WeekStartsOn(t *testing.T) {
	svc := newSpecTestService(t)
	ctx := context.Background()

	valid := []string{"monday", "sunday", "saturday"}
	for _, v := range valid {
		require.NoErrorf(t, svc.Set(ctx, settings.KeyPlanWeekStartsOn, v), "expected %q to be accepted", v)
	}

	invalid := []string{"tuesday", "wednesday", "MONDAY", "Mon", "", "0"}
	for _, v := range invalid {
		err := svc.Set(ctx, settings.KeyPlanWeekStartsOn, v)
		assert.ErrorIsf(t, err, settings.ErrInvalidKind, "expected %q to be rejected", v)
	}
}

func TestSettingsSpec_Anchor(t *testing.T) {
	svc := newSpecTestService(t)
	ctx := context.Background()

	valid := []string{"today", "next_shopping_day", "fixed_weekday"}
	for _, v := range valid {
		require.NoErrorf(t, svc.Set(ctx, settings.KeyPlanAnchor, v), "expected %q to be accepted", v)
	}

	invalid := []string{"tomorrow", "week_start", "Today", "", "0"}
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

func TestSettingsSpec_WindowDays(t *testing.T) {
	svc := newSpecTestService(t)
	ctx := context.Background()

	// boundary values — 5 and 14 must be accepted
	for _, v := range []string{"5", "7", "10", "14"} {
		require.NoErrorf(t, svc.Set(ctx, settings.KeyPlanWindowDays, v), "expected %q to be accepted", v)
	}

	// below minimum
	for _, v := range []string{"4", "1", "0"} {
		err := svc.Set(ctx, settings.KeyPlanWindowDays, v)
		assert.ErrorIsf(t, err, settings.ErrInvalidKind, "expected %q to be rejected (below min)", v)
	}

	// above maximum
	for _, v := range []string{"15", "20", "100"} {
		err := svc.Set(ctx, settings.KeyPlanWindowDays, v)
		assert.ErrorIsf(t, err, settings.ErrInvalidKind, "expected %q to be rejected (above max)", v)
	}
}
