package planview

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/availability"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/settings"
	"github.com/jaltszeimer/plantry/backend/internal/domain/setup"
)

// B4-AC5
func TestToday_Verdicts(t *testing.T) {
	t.Run("Sunday of AC-1 cooks the Chili", func(t *testing.T) {
		f := newFixture()
		sid := f.ac1()
		td, err := f.service().Today(ctx, date("2026-11-01"))
		require.NoError(t, err)
		assert.Equal(t, Verdict{Kind: VerdictCook, SessionID: sid, FoodName: "Chili", Portions: 6, SlotID: dinner}, td.Verdict)
		require.Len(t, td.Cook, 1)
		assert.Equal(t, sid, td.Cook[0].Session.ID)
		require.NotNil(t, td.Cook[0].Minutes)
		assert.Equal(t, 60, *td.Cook[0].Minutes)
	})

	t.Run("Monday of AC-1 has nothing to cook", func(t *testing.T) {
		f := newFixture()
		f.ac1()
		td, err := f.service().Today(ctx, date("2026-11-02"))
		require.NoError(t, err)
		assert.Equal(t, Verdict{Kind: VerdictNothingToCook}, td.Verdict)
		assert.Empty(t, td.Cook)
	})

	t.Run("a 15-minute omelette is a quick cook", func(t *testing.T) {
		f := newFixture()
		f.ac1()
		sid := f.session(omeletID, "2026-11-03", breakfast, 2)
		f.composed("2026-11-03", breakfast, omeletID, sid)
		td, err := f.service().Today(ctx, date("2026-11-03"))
		require.NoError(t, err)
		assert.Equal(t, VerdictQuickCook, td.Verdict.Kind)
		assert.Equal(t, "Omelette", td.Verdict.FoodName)
	})

	t.Run("zero recorded minutes are unknown, not quick", func(t *testing.T) {
		f := newFixture()
		f.store.foods[chiliID].PrepMinutes = ptr(0)
		f.store.foods[chiliID].CookMinutes = ptr(0)
		sid := f.ac1()
		td, err := f.service().Today(ctx, date("2026-11-01"))
		require.NoError(t, err)
		assert.Equal(t, Verdict{Kind: VerdictCook, SessionID: sid, FoodName: "Chili", Portions: 6, SlotID: dinner}, td.Verdict)
	})

	t.Run("an empty day in a planned window has no plates", func(t *testing.T) {
		f := newFixture()
		f.ac1()
		td, err := f.service().Today(ctx, date("2026-11-04"))
		require.NoError(t, err)
		assert.Equal(t, Verdict{Kind: VerdictNoPlates}, td.Verdict)
	})

	t.Run("an empty window has no plan", func(t *testing.T) {
		f := newFixture()
		f.ac1()
		td, err := f.service().Today(ctx, date("2026-11-08"))
		require.NoError(t, err)
		assert.Equal(t, Verdict{Kind: VerdictNoPlan}, td.Verdict)
	})

	t.Run("the main beats a bigger side", func(t *testing.T) {
		f := newFixture()
		side := f.session(saladID, "2026-11-03", lunch, 8)
		f.composed("2026-11-03", lunch, saladID, side)
		main := f.session(chiliID, "2026-11-03", dinner, 2)
		f.composed("2026-11-03", dinner, chiliID, main)
		td, err := f.service().Today(ctx, date("2026-11-03"))
		require.NoError(t, err)
		assert.Equal(t, main, td.Verdict.SessionID)
		assert.Equal(t, VerdictCook, td.Verdict.Kind)
		require.Len(t, td.Cook, 2)
		assert.Equal(t, side, td.Cook[0].Session.ID, "cook list in cell order")
		assert.Nil(t, td.Cook[0].Minutes, "no prep or cook minutes")
	})

	t.Run("the most portions among mains", func(t *testing.T) {
		f := newFixture()
		small := f.session(omeletID, "2026-11-03", breakfast, 2)
		f.composed("2026-11-03", breakfast, omeletID, small)
		big := f.session(chiliID, "2026-11-03", dinner, 6)
		f.composed("2026-11-03", dinner, chiliID, big)
		td, err := f.service().Today(ctx, date("2026-11-03"))
		require.NoError(t, err)
		assert.Equal(t, big, td.Verdict.SessionID)
	})

	t.Run("a cooked session yields to the uncooked one", func(t *testing.T) {
		f := newFixture()
		big := f.session(chiliID, "2026-11-03", dinner, 6)
		f.composed("2026-11-03", dinner, chiliID, big)
		small := f.session(omeletID, "2026-11-03", breakfast, 2)
		f.composed("2026-11-03", breakfast, omeletID, small)
		cs := f.sessions[big]
		cs.CookedAt = ptr(date("2026-11-03"))
		f.sessions[big] = cs
		td, err := f.service().Today(ctx, date("2026-11-03"))
		require.NoError(t, err)
		assert.Equal(t, small, td.Verdict.SessionID)
		require.Len(t, td.Cook, 2, "cooked sessions stay in the cook list")
	})

	t.Run("every session cooked has nothing left to cook", func(t *testing.T) {
		f := newFixture()
		sid := f.ac1()
		cs := f.sessions[sid]
		cs.CookedAt = ptr(date("2026-11-01"))
		f.sessions[sid] = cs
		td, err := f.service().Today(ctx, date("2026-11-01"))
		require.NoError(t, err)
		assert.Equal(t, Verdict{Kind: VerdictNothingToCook}, td.Verdict)
		require.Len(t, td.Cook, 1)
	})
}

func TestToday_MealsAndSetup(t *testing.T) {
	f := newFixture()
	f.ac1()
	f.overrides[cellID(date("2026-11-02"), breakfast)] = availability.ModeNone

	td, err := f.service().Today(ctx, date("2026-11-02"))
	require.NoError(t, err)

	assert.Equal(t, date("2026-11-02"), td.Date)
	assert.Equal(t, date("2026-10-31"), td.WindowStart)
	require.Len(t, td.Meals, 3)
	assert.Equal(t, breakfast, td.Meals[0].SlotID)
	assert.Equal(t, availability.ModeNone, td.Meals[0].Mode)
	assert.Nil(t, td.Meals[0].Plate)
	assert.Nil(t, td.Meals[0].Link)
	lunchMeal := td.Meals[1]
	require.NotNil(t, lunchMeal.Plate)
	require.NotNil(t, lunchMeal.Link)
	assert.Equal(t, 2, lunchMeal.Link.Position)
	assert.Equal(t, plate.KindLeftover, lunchMeal.Link.Kind)
	assert.Equal(t, lunchMeal.Plate.Components[0].ID, lunchMeal.Link.ComponentID)
	assert.Equal(t, 3, td.Meals[2].Link.Position)
	assert.Equal(t, Setup{RecipeCount: 3, RecipeTarget: setup.RecipeTarget}, td.Setup)
}

// B4-AC6
func TestToday_Reminders(t *testing.T) {
	newBatch := func() (*fixture, int64) {
		f := newFixture()
		f.settings[settings.KeyPlanFreezingEnabled] = "true"
		sid := f.session(chiliID, "2026-11-01", dinner, 8)
		f.composed("2026-11-01", dinner, chiliID, sid)
		f.composed("2026-11-04", lunch, chiliID, sid)  // leftover on use-by (Sun + 3)
		f.composed("2026-11-06", dinner, chiliID, sid) // frozen
		return f, sid
	}

	t.Run("defrost the day before a frozen link", func(t *testing.T) {
		f, sid := newBatch()
		td, err := f.service().Today(ctx, date("2026-11-05"))
		require.NoError(t, err)
		assert.Equal(t, []Reminder{{Kind: ReminderDefrost, SessionID: sid, FoodName: "Chili", Date: date("2026-11-06"), SlotID: dinner}}, td.Reminders)
	})

	t.Run("use-by on the leftover's last good day", func(t *testing.T) {
		f, sid := newBatch()
		td, err := f.service().Today(ctx, date("2026-11-04"))
		require.NoError(t, err)
		assert.Equal(t, []Reminder{{Kind: ReminderUseBy, SessionID: sid, FoodName: "Chili", Date: date("2026-11-04"), SlotID: lunch}}, td.Reminders)
	})

	t.Run("no reminders otherwise", func(t *testing.T) {
		f, _ := newBatch()
		for _, d := range []string{"2026-11-01", "2026-11-03", "2026-11-06"} {
			td, err := f.service().Today(ctx, date(d))
			require.NoError(t, err)
			assert.Empty(t, td.Reminders, d)
		}
	})
}

// B4-AC7
func TestToday_Rate(t *testing.T) {
	f := newFixture()
	sid := f.session(chiliID, "2026-11-01", dinner, 8)
	f.composed("2026-11-01", dinner, chiliID, sid)
	monLunch := f.composed("2026-11-02", lunch, chiliID, sid)
	monDinner := f.composed("2026-11-02", dinner, chiliID, sid)
	f.setFeedback(monDinner)
	f.leaf("2026-11-03", lunch, appleID, "Apple", 150)
	tueDinner := f.session(saladID, "2026-11-04", dinner, 2)
	f.composed("2026-11-04", dinner, saladID, tueDinner) // on the date itself

	td, err := f.service().Today(ctx, date("2026-11-04"))
	require.NoError(t, err)
	require.NotNil(t, td.Rate)
	assert.Equal(t, Rate{PlateID: monLunch, Date: date("2026-11-02"), SlotID: lunch, FoodName: "Chili"}, *td.Rate)

	td, err = f.service().Today(ctx, date("2026-11-08"))
	require.NoError(t, err)
	assert.Nil(t, td.Rate, "nothing composed and unrated in the 3 days before")
}

// B4-AC8
func TestToday_PlanBanner(t *testing.T) {
	f := newFixture()
	f.ac1()
	svc := f.service()

	check := func(day string, due bool, next string) {
		t.Helper()
		td, err := svc.Today(ctx, date(day))
		require.NoError(t, err)
		assert.Equal(t, PlanBanner{Due: due, NextStart: date(next)}, td.PlanBanner, day)
	}
	check("2026-11-05", false, "2026-11-07") // two days before
	check("2026-11-06", true, "2026-11-07")  // the day before
	check("2026-11-07", true, "2026-11-07")  // the shopping day itself, still unplanned
	check("2026-11-08", false, "2026-11-14")

	f.leaf("2026-11-09", lunch, appleID, "Apple", 100)
	check("2026-11-06", false, "2026-11-07")
	check("2026-11-07", false, "2026-11-07")
}
