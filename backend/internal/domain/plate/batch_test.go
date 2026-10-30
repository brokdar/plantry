package plate_test

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

// Slots: lunch (id 1, sort 1), dinner (id 2, sort 2), breakfast (id 3, sort 0).
const (
	lunch     int64 = 1
	dinner    int64 = 2
	breakfast int64 = 3
)

var order = plate.SlotOrder{lunch: 1, dinner: 2, breakfast: 0}

func day(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

func cell(date string, slot int64) plate.CellKey {
	return plate.CellKey{Date: day(date), SlotID: slot}
}

// sunDinner is a session cooked Sun 2026-11-01 dinner.
func sunDinner(portions, frozen float64) plate.CookSession {
	return plate.CookSession{ID: 7, FoodID: 42, CookDate: day("2026-11-01"), CookSlotID: dinner, Portions: portions, FrozenPortions: frozen}
}

func TestCompareAndBefore(t *testing.T) {
	tests := []struct {
		name string
		a, b plate.CellKey
		want int
	}{
		{"earlier date", cell("2026-11-01", dinner), cell("2026-11-02", breakfast), -1},
		{"later date", cell("2026-11-03", breakfast), cell("2026-11-02", dinner), 1},
		{"same date, slot sort order wins over id", cell("2026-11-01", breakfast), cell("2026-11-01", lunch), -1},
		{"same date, lunch before dinner", cell("2026-11-01", lunch), cell("2026-11-01", dinner), -1},
		{"same cell", cell("2026-11-01", dinner), cell("2026-11-01", dinner), 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, plate.Compare(tt.a, tt.b, order))
			assert.Equal(t, tt.want < 0, plate.Before(tt.a, tt.b, order))
		})
	}
}

func TestCompare_EqualSortOrderFallsBackToSlotID(t *testing.T) {
	o := plate.SlotOrder{5: 0, 4: 0}
	assert.True(t, plate.Before(plate.CellKey{Date: day("2026-11-01"), SlotID: 4}, plate.CellKey{Date: day("2026-11-01"), SlotID: 5}, o))
}

func TestUseByAndClassify(t *testing.T) {
	s := sunDinner(6, 0)
	assert.Equal(t, day("2026-11-04"), plate.UseBy(s, 3))

	tests := []struct {
		name string
		c    plate.CellKey
		want plate.LinkKind
	}{
		{"cook cell is fresh", cell("2026-11-01", dinner), plate.KindFresh},
		{"next day is a leftover", cell("2026-11-02", lunch), plate.KindLeftover},
		{"use-by day is a leftover", cell("2026-11-04", dinner), plate.KindLeftover},
		{"after use-by is frozen", cell("2026-11-05", breakfast), plate.KindFrozen},
		{"before the cook cell is a leftover (with a warning)", cell("2026-11-01", lunch), plate.KindLeftover},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, plate.Classify(tt.c, s, 3))
		})
	}
}

func link(plateID int64, c plate.CellKey, perEater, eaters float64) plate.Link {
	return plate.Link{PlateID: plateID, ComponentID: plateID * 10, FoodID: 42, Cell: c, PerEater: perEater, Eaters: eaters}
}

func TestOrderLinksAndConsumption(t *testing.T) {
	s := sunDinner(8, 2)
	links := []plate.Link{
		link(3, cell("2026-11-08", dinner), 1, 2), // frozen
		link(2, cell("2026-11-02", dinner), 1, 2),
		link(1, cell("2026-11-01", dinner), 1.5, 2),
	}
	got := plate.OrderLinks(s, links, 3, order)
	require.Len(t, got, 3)
	assert.Equal(t, []int64{1, 2, 3}, []int64{got[0].PlateID, got[1].PlateID, got[2].PlateID})
	assert.Equal(t, []int{1, 2, 3}, []int{got[0].Position, got[1].Position, got[2].Position})
	assert.Equal(t, []plate.LinkKind{plate.KindFresh, plate.KindLeftover, plate.KindFrozen}, []plate.LinkKind{got[0].Kind, got[1].Kind, got[2].Kind})
	assert.InDelta(t, 3.0, got[0].Portions, 1e-9)

	use := plate.Consumption(s, got, 3)
	assert.InDelta(t, 7.0, use.Total, 1e-9)
	assert.InDelta(t, 2.0, use.Frozen, 1e-9)
	assert.Equal(t, int64(3), links[0].PlateID, "input slice is not reordered")
}

func TestSync(t *testing.T) {
	tests := []struct {
		name               string
		before, after      plate.Usage
		portions, frozen   float64
		wantPort, wantFroz float64
	}{
		{"exact grows", plate.Usage{Total: 2}, plate.Usage{Total: 6}, 2, 0, 6, 0},
		{"exact shrinks and clamps frozen", plate.Usage{Total: 6}, plate.Usage{Total: 4}, 6, 5, 4, 4},
		{"surplus keeps", plate.Usage{Total: 2}, plate.Usage{Total: 3}, 5, 1, 5, 1},
		{"deficit keeps", plate.Usage{Total: 6}, plate.Usage{Total: 4}, 5, 0, 5, 0},
		{"float noise counts as exact", plate.Usage{Total: 0.1 + 0.2}, plate.Usage{Total: 1}, 0.3, 0, 1, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, f := plate.Sync(tt.before, tt.after, tt.portions, tt.frozen)
			assert.InDelta(t, tt.wantPort, p, 1e-9)
			assert.InDelta(t, tt.wantFroz, f, 1e-9)
		})
	}
}

func TestCheckLink(t *testing.T) {
	s := sunDinner(2, 0)
	off := plate.BatchSettings{DefaultKeepDays: 3, FreezingEnabled: false, FreezerKeepDays: 90}
	on := plate.BatchSettings{DefaultKeepDays: 3, FreezingEnabled: true, FreezerKeepDays: 30}
	tests := []struct {
		name string
		c    plate.CellKey
		food int64
		set  plate.BatchSettings
		want error
	}{
		{"fresh on the cook cell", cell("2026-11-01", dinner), 42, off, nil},
		{"leftover next day", cell("2026-11-02", lunch), 42, off, nil},
		{"leftover on use-by", cell("2026-11-04", dinner), 42, off, nil},
		{"earlier slot same day", cell("2026-11-01", lunch), 42, off, domain.ErrSessionBeforeCook},
		{"earlier day", cell("2026-10-31", dinner), 42, off, domain.ErrSessionBeforeCook},
		{"past use-by, freezing off", cell("2026-11-05", lunch), 42, off, domain.ErrSessionPastKeepDays},
		{"past use-by, freezing on", cell("2026-11-05", lunch), 42, on, nil},
		{"freezer limit day", cell("2026-12-01", dinner), 42, on, nil},
		{"past freezer days", cell("2026-12-02", lunch), 42, on, domain.ErrSessionPastFreezerDays},
		{"other food", cell("2026-11-02", lunch), 43, off, domain.ErrSessionFoodMismatch},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := plate.CheckLink(tt.c, tt.food, s, 3, tt.set, order)
			if tt.want == nil {
				assert.NoError(t, err)
				return
			}
			assert.True(t, errors.Is(err, tt.want), "got %v", err)
		})
	}
}

func TestWarnings(t *testing.T) {
	off := plate.BatchSettings{DefaultKeepDays: 3, FreezerKeepDays: 90}
	on := plate.BatchSettings{DefaultKeepDays: 3, FreezingEnabled: true, FreezerKeepDays: 90}
	codes := func(ws []plate.Warning) []string {
		out := []string{}
		for _, w := range ws {
			out = append(out, w.Code)
		}
		return out
	}

	t.Run("exact batch has none", func(t *testing.T) {
		s := sunDinner(6, 0)
		ls := plate.OrderLinks(s, []plate.Link{
			link(1, cell("2026-11-01", dinner), 1, 2),
			link(2, cell("2026-11-02", lunch), 1, 2),
			link(3, cell("2026-11-02", dinner), 1, 2),
		}, 3, order)
		assert.Empty(t, plate.Warnings(s, ls, 3, off, order))
	})
	t.Run("moved cook cell leaves leftovers before it", func(t *testing.T) {
		s := sunDinner(6, 0)
		s.CookDate = day("2026-11-03")
		ls := plate.OrderLinks(s, []plate.Link{
			link(1, cell("2026-11-03", dinner), 1, 2),
			link(2, cell("2026-11-02", lunch), 1, 2),
		}, 3, order)
		ws := plate.Warnings(s, ls, 3, off, order)
		require.Len(t, ws, 1)
		assert.Equal(t, plate.WarnLeftoverBeforeCook, ws[0].Code)
		require.NotNil(t, ws[0].PlateID)
		assert.Equal(t, int64(2), *ws[0].PlateID)
	})
	t.Run("past keep days without freezing, deficit", func(t *testing.T) {
		s := sunDinner(2, 0)
		ls := plate.OrderLinks(s, []plate.Link{
			link(1, cell("2026-11-01", dinner), 1, 2),
			link(2, cell("2026-11-06", lunch), 1, 1),
		}, 3, order)
		assert.Equal(t, []string{plate.WarnPastKeepDays, plate.WarnDeficit}, codes(plate.Warnings(s, ls, 3, off, order)))
	})
	t.Run("frozen deficit with freezing on", func(t *testing.T) {
		s := sunDinner(4, 1)
		ls := plate.OrderLinks(s, []plate.Link{
			link(1, cell("2026-11-01", dinner), 1, 2),
			link(2, cell("2026-11-06", lunch), 1, 2),
		}, 3, order)
		assert.Equal(t, []string{plate.WarnFrozenDeficit}, codes(plate.Warnings(s, ls, 3, on, order)))
	})
}

func TestBatchSettingsKeepDays(t *testing.T) {
	set := plate.BatchSettings{DefaultKeepDays: 3}
	assert.Equal(t, 3, set.KeepDays(nil))
	five := 5
	assert.Equal(t, 5, set.KeepDays(&five))
}
