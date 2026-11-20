//go:build spectest && cc22

package spectest

// CC22 contract tests (spec v1.1 SB1). They need the band API, so they build
// only with `-tags spectest,cc22`; the product worker enables the tag when the
// API exists. The contract these tests expect:
//
//	DB:     time_slots.band_min_pct REAL NULL, time_slots.band_max_pct REAL NULL
//	        (both NULL = the default band of SB1; both set otherwise)
//	slot:   TimeSlot gains BandMinPct, BandMaxPct *float64
//	        type Band struct{ SlotID int64; MinPct, MaxPct float64 }  // effective band
//	        (s *Service) Bands(ctx) ([]Band, error)         // active slots, sort order
//	        (s *Service) SetBands(ctx, []Band) error        // atomic; validates the whole
//	                                                        // effective set of active slots
//	        (s *Service) ApplyPreset(ctx, name string) error // "lunch_biggest", "dinner_biggest"
//	domain: ErrInvalidSlotBands, message key "error.slot.invalid_bands"; its text names the
//	        numbers: "minimums add up to 110 % (at most 100), maximums to 90 % (at least 100)"
//
// Creating a slot is never refused for its band; the effective set may be
// invalid until SetBands is called, and SetBands refuses an invalid result.

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/domain/slot"
)

func bandsByName(t *testing.T, e *env) map[string]slot.Band {
	t.Helper()
	bs, err := e.slots.Bands(context.Background())
	require.NoError(t, err)
	out := map[string]slot.Band{}
	for _, b := range bs {
		ts, err := e.slots.Get(context.Background(), b.SlotID)
		require.NoError(t, err)
		out[strings.TrimPrefix(ts.NameKey, "slot.")] = b
	}
	return out
}

func slotID(t *testing.T, e *env, name string) int64 {
	t.Helper()
	ts, err := e.slots.List(context.Background(), true)
	require.NoError(t, err)
	for _, s := range ts {
		if strings.TrimPrefix(s.NameKey, "slot.") == name {
			return s.ID
		}
	}
	t.Fatalf("no slot %s", name)
	return 0
}

// SB1: the defaults when nothing is stored.
func TestSB1cc22_DefaultBands(t *testing.T) {
	e := newEnv(t, "chat-dev", chatWeek)
	got := bandsByName(t, e)
	assert.InDelta(t, 20, got["breakfast"].MinPct, 1e-9)
	assert.InDelta(t, 30, got["breakfast"].MaxPct, 1e-9)
	assert.InDelta(t, 30, got["lunch"].MinPct, 1e-9)
	assert.InDelta(t, 40, got["lunch"].MaxPct, 1e-9)
	assert.InDelta(t, 25, got["dinner"].MinPct, 1e-9)
	assert.InDelta(t, 40, got["dinner"].MaxPct, 1e-9)
}

// SB1: an invalid set is refused with the numbers; nothing is stored, lowered or normalised.
func TestSB1cc22_InvalidSetRefusedWithNumbers(t *testing.T) {
	cases := []struct {
		name  string
		set   map[string][2]float64
		shows string // a number the message must name
	}{
		{"mins above 100", map[string][2]float64{"breakfast": {20, 30}, "lunch": {45, 55}, "dinner": {45, 55}}, "110"},
		{"maxes below 100", map[string][2]float64{"breakfast": {20, 30}, "lunch": {20, 30}, "dinner": {20, 30}}, "90"},
		{"min above max", map[string][2]float64{"breakfast": {30, 20}, "lunch": {30, 40}, "dinner": {25, 40}}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := newEnv(t, "chat-dev", chatWeek)
			before := bandsByName(t, e)
			var set []slot.Band
			for n, b := range c.set {
				set = append(set, slot.Band{SlotID: slotID(t, e, n), MinPct: b[0], MaxPct: b[1]})
			}
			err := e.slots.SetBands(context.Background(), set)
			require.Error(t, err)
			assert.True(t, errors.Is(err, domain.ErrInvalidSlotBands))
			key, _ := domain.MessageKey(err, "slot")
			assert.Equal(t, "error.slot.invalid_bands", key)
			if c.shows != "" {
				assert.Contains(t, err.Error(), c.shows)
				assert.Contains(t, err.Error(), "100")
			}
			assert.Equal(t, before, bandsByName(t, e), "a refused set must change nothing (no auto-lowering, no normalising)")
		})
	}
}

// SB1: any number of slots is accepted when the set is valid, and the values
// read back exactly as given.
func TestSB1cc22_AnySlotCountAcceptedWhenValid(t *testing.T) {
	e := newEnv(t, "chat-dev", chatWeek)
	ctx := context.Background()
	for i, n := range []string{"slot.brunch", "slot.tea", "slot.supper"} {
		ts := &slot.TimeSlot{NameKey: n, Icon: "x", SortOrder: 10 + i, Active: true, Kind: slot.KindMeal}
		require.NoError(t, e.slots.Create(ctx, ts), "creating a slot is never refused for its band")
	}
	all, err := e.slots.List(ctx, true)
	require.NoError(t, err)
	require.Len(t, all, 6)
	// the defaults of six slots (three of them 25-40) are invalid: the first SetBands that
	// leaves the set invalid is refused and names the numbers
	err = e.slots.SetBands(ctx, []slot.Band{{SlotID: all[0].ID, MinPct: 20, MaxPct: 30}})
	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrInvalidSlotBands))
	// the user distributes the bands: valid, accepted, stored unchanged
	var set []slot.Band
	for _, s := range all {
		set = append(set, slot.Band{SlotID: s.ID, MinPct: 10, MaxPct: 20})
	}
	require.NoError(t, e.slots.SetBands(ctx, set))
	got, err := e.slots.Bands(ctx)
	require.NoError(t, err)
	require.Len(t, got, 6)
	for _, b := range got {
		assert.InDelta(t, 10, b.MinPct, 1e-9)
		assert.InDelta(t, 20, b.MaxPct, 1e-9)
	}
}

// SB1: exactly two presets; extra snack slots get their band by kind.
func TestSB1cc22_Presets(t *testing.T) {
	for _, p := range presets {
		t.Run(p.name, func(t *testing.T) {
			e := newEnv(t, "chat-dev", chatWeek)
			name := map[string]string{"Lunch is the biggest meal": "lunch_biggest", "Dinner is the biggest meal": "dinner_biggest"}[p.name]
			require.NoError(t, e.slots.ApplyPreset(context.Background(), name))
			got := bandsByName(t, e)
			for _, n := range []string{"breakfast", "lunch", "dinner"} {
				assert.InDelta(t, p.bands[n].Min, got[n].MinPct, 1e-9, n)
				assert.InDelta(t, p.bands[n].Max, got[n].MaxPct, 1e-9, n)
			}
		})
	}
	e := newEnv(t, "chat-dev", chatWeek)
	assert.Error(t, e.slots.ApplyPreset(context.Background(), "even"), "Even is not a preset")
}

// SB1/SB2/SB3 end to end, per preset on the dev household: the light cap of
// the biggest meal and the away proration of the other main meal on one day.
func TestSB1cc22_PresetDrivesLightCapAndAwayProration(t *testing.T) {
	for _, p := range presets {
		t.Run(p.name, func(t *testing.T) {
			e := newEnv(t, "chat-dev", chatWeek)
			name := map[string]string{"Lunch is the biggest meal": "lunch_biggest", "Dinner is the biggest meal": "dinner_biggest"}[p.name]
			require.NoError(t, e.slots.ApplyPreset(context.Background(), name))
			sh := presetShares(p, []string{"breakfast", "lunch", "dinner"})
			s := e.st()
			T := fullTarget(s)
			lunch, dinner := mealPlates(s)
			big, bigName := dinner, "dinner"
			if sh["lunch"] > sh["dinner"] {
				big, bigName = lunch, "lunch"
			}
			pl, ok := pick(big, 1)
			require.True(t, ok)
			rs, _ := e.turn("make it light", tc{"find_ways", map[string]any{"goal": "lighter", "plate_id": pl.ID, "to_limit": true}})
			require.Len(t, rs, 1)
			// SB5: the cap is 75 % of the slot's expected kcal, shifted when a routine meal misses its own share
			byID := map[int64]float64{}
			for id, name := range s.SlotNames {
				byID[id] = sh[strings.TrimPrefix(name, "slot.")]
			}
			exp, _ := slotExpectedWith(s, planning.Evaluate(s, planning.WindowTarget(s)), pl.Date.Format("2006-01-02"), byID)
			assert.InDelta(t, 0.75*exp[pl.SlotID], num(rs[0].Out["max_kcal"]), 1.0, "light cap = 75 %% of the %s's shifted expected kcal (SB5)", bigName)
			// away on one day: the other main meal
			other := "lunch"
			if bigName == "lunch" {
				other = "dinner"
			}
			e2 := newEnv(t, "chat-dev", chatWeek)
			require.NoError(t, e2.slots.ApplyPreset(context.Background(), name))
			date := chatWeek.AddDate(0, 0, 2) // a day with a routine breakfast and both meals planned
			rs2, _ := e2.turn("away", tc{"apply_changes", map[string]any{"ops": []any{map[string]any{"op": "away", "date": date.Format("2006-01-02"), "slot_id": slotID(t, e2, other)}}}})
			require.Len(t, rs2, 1)
			days, _ := rs2[0].Out["after"].(map[string]any)["days"].([]any)
			var day map[string]any
			for _, d := range days {
				if m, ok := d.(map[string]any); ok && m["date"] == date.Format("2006-01-02") {
					day = m
				}
			}
			require.NotNil(t, day)
			assert.InDelta(t, T*sh[other], num(day["away_kcal"]), 1.0)
			assert.InDelta(t, T*(1-sh[other]), num(day["planned_target_kcal"]), 1.0)
		})
	}
}
