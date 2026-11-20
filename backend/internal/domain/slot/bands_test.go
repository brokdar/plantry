package slot_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/slot"
)

func threeSlots(t *testing.T) (*slot.Service, []int64) {
	t.Helper()
	svc := slot.NewService(newFakeRepo())
	var ids []int64
	for _, n := range []string{"breakfast", "lunch", "dinner"} {
		kind := slot.KindMeal
		if n == "breakfast" {
			kind = slot.KindLight
		}
		ts := &slot.TimeSlot{NameKey: "slot." + n, Icon: "x", Active: true, Kind: kind}
		require.NoError(t, svc.Create(context.Background(), ts))
		ids = append(ids, ts.ID)
	}
	return svc, ids
}

// SB1/SB2: defaults by name, shares are midpoints over the sum of midpoints.
func TestBands_DefaultsAndShares(t *testing.T) {
	svc, ids := threeSlots(t)
	bands, err := svc.Bands(context.Background())
	require.NoError(t, err)
	assert.Equal(t, []slot.Band{{ids[0], 20, 30}, {ids[1], 30, 40}, {ids[2], 25, 40}}, bands)
	sh := slot.Shares(bands)
	assert.InDelta(t, 0.270, sh[ids[0]], 0.001)
	assert.InDelta(t, 0.378, sh[ids[1]], 0.001)
	assert.InDelta(t, 0.351, sh[ids[2]], 0.001)
}

func TestDefaultBand_ByNameThenKind(t *testing.T) {
	lo, hi := slot.DefaultBand("slot.snack", slot.KindLight)
	assert.Equal(t, [2]float64{5, 15}, [2]float64{lo, hi})
	lo, hi = slot.DefaultBand("slot.brunch", slot.KindMeal)
	assert.Equal(t, [2]float64{25, 40}, [2]float64{lo, hi})
	lo, hi = slot.DefaultBand("slot.tea", slot.KindLight)
	assert.Equal(t, [2]float64{5, 15}, [2]float64{lo, hi})
}

// SetBands is atomic and validates the whole effective set, naming the numbers.
func TestBands_InvalidSetIsRefusedAndChangesNothing(t *testing.T) {
	svc, ids := threeSlots(t)
	ctx := context.Background()
	err := svc.SetBands(ctx, []slot.Band{{ids[0], 40, 50}, {ids[1], 40, 50}, {ids[2], 30, 40}}) // minimums 110
	require.ErrorIs(t, err, domain.ErrInvalidSlotBands)
	assert.Contains(t, err.Error(), "minimums add up to 110 % (at most 100)")
	err = svc.SetBands(ctx, []slot.Band{{ids[0], 10, 20}, {ids[1], 10, 20}, {ids[2], 10, 20}}) // maximums 60
	require.ErrorIs(t, err, domain.ErrInvalidSlotBands)
	assert.Contains(t, err.Error(), "maximums to 60 % (at least 100)")
	assert.True(t, errors.Is(svc.SetBands(ctx, []slot.Band{{ids[0], 40, 30}}), domain.ErrInvalidSlotBands), "min above max")
	assert.True(t, errors.Is(svc.SetBands(ctx, []slot.Band{{ids[0], -5, 30}}), domain.ErrInvalidSlotBands), "outside 0-100")
	bands, err := svc.Bands(ctx)
	require.NoError(t, err)
	assert.Equal(t, slot.Band{SlotID: ids[1], MinPct: 30, MaxPct: 40}, bands[1], "nothing changed")
	key, _ := domain.MessageKey(err, "slot")
	_ = key
	_, sentinel := domain.MessageKey(domain.ErrInvalidSlotBands, "slot")
	assert.Equal(t, domain.ErrInvalidSlotBands, sentinel)
}

func TestBands_PresetsAndUnknownName(t *testing.T) {
	svc, ids := threeSlots(t)
	ctx := context.Background()
	require.NoError(t, svc.ApplyPreset(ctx, "lunch_biggest"))
	bands, _ := svc.Bands(ctx)
	assert.Equal(t, []slot.Band{{ids[0], 20, 30}, {ids[1], 35, 45}, {ids[2], 20, 30}}, bands)
	require.NoError(t, svc.ApplyPreset(ctx, "dinner_biggest"))
	bands, _ = svc.Bands(ctx)
	assert.Equal(t, []slot.Band{{ids[0], 20, 30}, {ids[1], 20, 30}, {ids[2], 35, 45}}, bands)
	assert.Error(t, svc.ApplyPreset(ctx, "even"))
	assert.Error(t, svc.ApplyPreset(ctx, ""))
}

// Creating a slot is never refused for its band.
func TestBands_CreateIsNeverRefusedForTheBand(t *testing.T) {
	svc, _ := threeSlots(t)
	ctx := context.Background()
	for i := 0; i < 4; i++ {
		require.NoError(t, svc.Create(ctx, &slot.TimeSlot{NameKey: "slot.snack", Icon: "x", Active: true, Kind: slot.KindLight}))
	}
	_, err := svc.Bands(ctx)
	require.NoError(t, err)
}
