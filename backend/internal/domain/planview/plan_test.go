package planview

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/availability"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/settings"
)

var ctx = context.Background()

func TestPlan_AC1Window(t *testing.T) {
	f := newFixture()
	sid := f.ac1()
	start := date("2026-10-31")

	p, err := f.service().Plan(ctx, &start, date("2026-11-04"))
	require.NoError(t, err)

	assert.Equal(t, date("2026-10-31"), p.Window.Start)
	assert.Equal(t, date("2026-11-06"), p.Window.End)
	assert.Equal(t, date("2026-10-24"), p.Window.Prev())
	assert.Equal(t, date("2026-11-07"), p.Window.Next())
	assert.True(t, p.IsCurrent)
	assert.Equal(t, 5, p.ShoppingDay)
	require.Len(t, p.Slots, 3, "inactive slots are left out")
	assert.Equal(t, []int64{breakfast, lunch, dinner}, []int64{p.Slots[0].ID, p.Slots[1].ID, p.Slots[2].ID})

	// B4-AC3
	assert.Len(t, p.Cells, 21)
	require.Len(t, p.Sessions, 1)
	v := p.Sessions[0]
	assert.Equal(t, sid, v.ID)
	assert.Equal(t, "Chili", v.FoodName)
	assert.Len(t, v.Links, 3)
	assert.Equal(t, 3, v.Meals)
	assert.InDelta(t, 6, v.Consumed, 1e-9)
	assert.Equal(t, []plate.LinkKind{plate.KindFresh, plate.KindLeftover, plate.KindLeftover},
		[]plate.LinkKind{v.Links[0].Kind, v.Links[1].Kind, v.Links[2].Kind})

	require.Len(t, p.Days, 7)
	monday := p.Days[2]
	assert.Equal(t, date("2026-11-02"), monday.Date)
	assert.InDelta(t, 2*chiliKcal, monday.Macros.Kcal, 1e-9, "per eater: plates have 2 eaters")
	assert.True(t, monday.Complete)
	assert.Empty(t, monday.Missing)
	assert.InDelta(t, chiliKcal, p.Days[1].Macros.Kcal, 1e-9)
	assert.Zero(t, p.Days[0].Macros.Kcal)
	assert.True(t, p.Days[0].Complete)
	assert.NotNil(t, p.Days[0].Missing)

	assert.Nil(t, p.Targets, "no kcal target")
	assert.Equal(t, Stats{Plates: 3, CookDays: 1, Warnings: 0}, p.Stats)

	sunDinner := p.Cells[1*3+2]
	assert.Equal(t, date("2026-11-01"), sunDinner.Date)
	assert.Equal(t, dinner, sunDinner.SlotID)
	require.NotNil(t, sunDinner.Plate)
	assert.Equal(t, "Chili", sunDinner.Plate.Components[0].FoodName)
	assert.Nil(t, p.Cells[0].Plate)
	assert.Equal(t, availability.ModePlan, p.Cells[0].Mode)
}

func TestPlan_NotCurrent(t *testing.T) {
	f := newFixture()
	start := date("2026-11-07")
	p, err := f.service().Plan(ctx, &start, date("2026-11-04"))
	require.NoError(t, err)
	assert.False(t, p.IsCurrent)
	assert.Equal(t, date("2026-11-04"), p.Today)
}

// B4-AC2 (domain half)
func TestPlan_StartMustBeShoppingDay(t *testing.T) {
	f := newFixture()
	start := date("2026-11-01") // Sunday
	_, err := f.service().Plan(ctx, &start, date("2026-11-04"))
	require.ErrorIs(t, err, domain.ErrInvalidPlanStart)
}

func TestPlan_AnchoredWithoutStart(t *testing.T) {
	tests := []struct {
		anchor, today, start string
	}{
		{"next_shopping_day", "2026-11-05", "2026-11-07"},
		{"next_shopping_day", "2026-11-07", "2026-11-07"},
		{"today", "2026-11-05", "2026-10-31"},
	}
	for _, tt := range tests {
		t.Run(tt.anchor+" "+tt.today, func(t *testing.T) {
			f := newFixture()
			f.settings[settings.KeyPlanAnchor] = tt.anchor
			p, err := f.service().Plan(ctx, nil, date(tt.today))
			require.NoError(t, err)
			assert.Equal(t, date(tt.start), p.Window.Start)
		})
	}
}

func TestPlan_Targets(t *testing.T) {
	f := newFixture()
	f.profile.p.KcalTarget = ptr(2000.0)
	f.profile.p.ProteinPct = ptr(30.0)
	f.profile.p.FatPct = ptr(27.0)
	start := date("2026-10-31")
	p, err := f.service().Plan(ctx, &start, start)
	require.NoError(t, err)
	require.NotNil(t, p.Targets)
	assert.InDelta(t, 2000, p.Targets.Kcal, 1e-9)
	assert.InDelta(t, 150, p.Targets.ProteinG, 1e-9)
	assert.InDelta(t, 60, p.Targets.FatG, 1e-9)
	assert.Zero(t, p.Targets.CarbsG, "a missing percentage gives 0 g")
}

// B4-AC4
func TestPlan_LeafWithMissingNutrition(t *testing.T) {
	f := newFixture()
	f.leaf("2026-11-03", lunch, mysteryID, "Mystery sauce", 50)
	f.leaf("2026-11-03", dinner, appleID, "Apple", 150)
	start := date("2026-10-31")

	p, err := f.service().Plan(ctx, &start, start)
	require.NoError(t, err)

	tue := p.Days[3]
	assert.Equal(t, date("2026-11-03"), tue.Date)
	assert.False(t, tue.Complete)
	assert.Equal(t, []string{"Mystery sauce"}, tue.Missing)
	assert.InDelta(t, 200*0.5+52*1.5, tue.Macros.Kcal, 1e-9, "leaf macros are per 100 g × grams / 100")
	assert.True(t, p.Days[2].Complete)
}

// B4-AC9: a 7-day window with 3 slots and 21 plates loads the plates with one
// call for the whole range and resolves each food once.
func TestPlan_BoundedQueries(t *testing.T) {
	f := newFixture()
	sid := f.session(chiliID, "2026-10-31", breakfast, 42)
	for d := 0; d < 7; d++ {
		day := date("2026-10-31").AddDate(0, 0, d).Format(time.DateOnly)
		f.composed(day, breakfast, chiliID, sid)
		f.composed(day, lunch, chiliID, sid)
		f.leaf(day, dinner, appleID, "Apple", 100)
	}
	require.Len(t, f.plates, 21)
	f.settings[settings.KeyPlanDefaultKeepDays] = "7"
	start := date("2026-10-31")

	p, err := f.service().Plan(ctx, &start, start)
	require.NoError(t, err)

	assert.Equal(t, [][2]time.Time{{date("2026-10-31"), date("2026-11-06")}}, f.plateCalls)
	assert.Equal(t, 1, f.sessionCalls)
	assert.Equal(t, map[int64]int{chiliID: 1, appleID: 1}, f.nutritionCalls)
	assert.Equal(t, 21, p.Stats.Plates)
	require.Len(t, p.Sessions, 1)
	assert.Equal(t, 14, p.Sessions[0].Meals)
}

func TestPlan_SessionsFromEarlierWindowAndAvailability(t *testing.T) {
	f := newFixture()
	// Cooked Friday of the previous window, a leftover on Saturday.
	sid := f.session(chiliID, "2026-10-30", dinner, 4)
	f.composed("2026-10-30", dinner, chiliID, sid)
	f.composed("2026-10-31", lunch, chiliID, sid)
	// Cooked in this window: one Monday session plus one too many portions.
	other := f.session(saladID, "2026-11-02", lunch, 1)
	f.composed("2026-11-02", lunch, saladID, other)
	f.overrides[cellID(date("2026-11-03"), dinner)] = availability.ModeNone
	start := date("2026-10-31")

	p, err := f.service().Plan(ctx, &start, start)
	require.NoError(t, err)

	require.Len(t, p.Sessions, 2)
	assert.Equal(t, sid, p.Sessions[0].ID, "cook cell order")
	assert.Equal(t, other, p.Sessions[1].ID)
	assert.Equal(t, 1, p.Stats.CookDays, "only sessions cooking in the window count")
	assert.Equal(t, 1, p.Stats.Warnings, "the salad session is short by one portion")
	none := p.Cells[3*3+2]
	assert.Equal(t, availability.ModeNone, none.Mode)
	assert.Equal(t, availability.SourceOverride, none.Source)
}

// TestReadModels_ReadInOneTx: each read model reads in one transaction, so a
// concurrent write cannot produce a torn view.
func TestReadModels_ReadInOneTx(t *testing.T) {
	f := newFixture()
	f.ac1()
	start := date("2026-10-31")

	_, err := f.service().Plan(ctx, &start, date("2026-11-04"))
	require.NoError(t, err)
	assert.Equal(t, 1, f.tx.calls, "Plan")

	_, err = f.service().Today(ctx, date("2026-11-04"))
	require.NoError(t, err)
	assert.Equal(t, 2, f.tx.calls, "Today")
	assert.Zero(t, f.readsOutsideTx)
}
