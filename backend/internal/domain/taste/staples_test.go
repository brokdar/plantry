package taste_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/feedback"
	"github.com/jaltszeimer/plantry/backend/internal/domain/taste"
)

// cook adds a confirmed cook session of the plate's recipe on date and links
// the plate to it, as the plan does.
func (r *rig) cook(foodID, plateID int64, date string) {
	r.t.Helper()
	res, err := r.db.Exec(`INSERT INTO cook_sessions (food_id, cook_date, cook_slot_id, portions, state) VALUES (?, ?, ?, 1, 'confirmed')`, foodID, date, r.dinner)
	require.NoError(r.t, err)
	id, err := res.LastInsertId()
	require.NoError(r.t, err)
	_, err = r.db.Exec(`UPDATE plate_components SET cook_session_id = ? WHERE plate_id = ?`, id, plateID)
	require.NoError(r.t, err)
}

func (r *rig) isStaple(id int64) bool {
	r.t.Helper()
	set, err := r.svc.Staples(r.ctx, r.now)
	require.NoError(r.t, err)
	return set[id]
}

// THEME-1: a staple has at least 3 cook sessions in 16 weeks, a positive
// signal and no dislike in 8 weeks; the favourite star pins one on its own.
func TestStaples_Rule(t *testing.T) {
	r := newRig(t, seed(t))
	a, b, c := r.plate(r.ctx, 3, r.curry), r.plate(r.ctx, 4, r.curry), r.plate(r.ctx, 5, r.curry)
	r.cook(r.curry, a.ID, "2026-02-23")
	r.cook(r.curry, b.ID, "2026-02-09")
	assert.False(t, r.isStaple(r.curry), "two sessions")

	r.cook(r.curry, c.ID, "2026-01-12")
	assert.True(t, r.isStaple(r.curry), "three sessions in 16 weeks and a positive signal (the plate added by hand)")

	_, err := r.fb.RecordFeedback(r.ctx, a.ID, feedback.StatusDisliked, nil)
	require.NoError(t, err)
	assert.False(t, r.isStaple(r.curry), "a dislike in the last 8 weeks")

	s := r.plate(r.ctx, 6, r.salmon)
	r.cook(r.salmon, s.ID, "2025-09-01") // older than 16 weeks
	r.cook(r.salmon, s.ID, "2025-09-08")
	r.cook(r.salmon, s.ID, "2025-09-15")
	assert.False(t, r.isStaple(r.salmon), "sessions older than 16 weeks do not count")
	_, err = r.foods.SetFavorite(r.ctx, r.salmon, true)
	require.NoError(t, err)
	assert.True(t, r.isStaple(r.salmon), "the favourite star pins a staple")

	list, err := r.svc.StapleList(r.ctx)
	require.NoError(t, err)
	assert.Equal(t, []taste.Staple{{ID: r.salmon, Name: "Salmon bake"}}, list)
}

// TASTE-8: a confirmed statement is permanent; the chat's window item goes.
func TestConfirmStatement(t *testing.T) {
	r := newRig(t, seed(t))
	ref := taste.Ref{Level: taste.LevelHero, Key: "coriander"}
	week := r.now.Truncate(24 * 60 * 60 * 1e9)
	_, _, err := r.svc.State(r.ctx, ref, taste.RatherNot, taste.ScopeWindow, week)
	require.NoError(t, err)

	it, undo, err := r.svc.ConfirmStatement(r.ctx, ref, taste.RatherNot, taste.ScopeAlways, week)
	require.NoError(t, err)
	assert.Equal(t, taste.ScopeAlways, it.Scope)
	_, ok, err := r.store.ItemByRef(r.ctx, ref, taste.ScopeWindow, week)
	require.NoError(t, err)
	assert.False(t, ok, "the window item is replaced")
	always, ok := r.item(taste.LevelHero, "coriander")
	require.True(t, ok)
	assert.Equal(t, taste.SourceStatement, always.Source)

	_, err = r.events.Revert(r.ctx, undo)
	require.NoError(t, err)
	_, ok = r.item(taste.LevelHero, "coriander")
	assert.False(t, ok, "Undo takes the permanent item back")
	_, ok, _ = r.store.ItemByRef(r.ctx, ref, taste.ScopeWindow, week)
	assert.True(t, ok, "and restores the window item")
}
