package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
)

func seedComposed(t *testing.T, db *sql.DB, name string) int64 {
	t.Helper()
	res, err := db.Exec(`INSERT INTO foods (name, kind, role, reference_portions) VALUES (?, 'composed', 'main', 4)`, name)
	require.NoError(t, err)
	id, err := res.LastInsertId()
	require.NoError(t, err)
	return id
}

func TestCookSessionRepo_RoundTrip(t *testing.T) {
	db := testhelper.NewTestDB(t)
	ctx := context.Background()
	slotID := seedSlot(t, db)
	chili := seedComposed(t, db, "Chili")
	repo := NewCookSessionRepo(db)
	sun := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)

	s := &plate.CookSession{State: plate.StateConfirmed, FoodID: chili, CookDate: sun, CookSlotID: slotID, Portions: 6}
	require.NoError(t, repo.CreateSession(ctx, s))
	require.NotZero(t, s.ID)
	assert.False(t, s.CreatedAt.IsZero())

	got, err := repo.GetSession(ctx, s.ID)
	require.NoError(t, err)
	assert.Equal(t, sun, got.CookDate)
	assert.InDelta(t, 6.0, got.Portions, 1e-9)
	assert.Zero(t, got.FrozenPortions)
	assert.Nil(t, got.CookedAt)
	assert.Nil(t, got.Note)

	cooked := time.Date(2026, 11, 1, 18, 30, 0, 0, time.UTC)
	note := "double garlic"
	got.Portions, got.FrozenPortions, got.CookedAt, got.Note = 8, 2, &cooked, &note
	got.CookDate = sun.AddDate(0, 0, 1)
	require.NoError(t, repo.UpdateSession(ctx, got))
	got, err = repo.GetSession(ctx, s.ID)
	require.NoError(t, err)
	assert.InDelta(t, 8.0, got.Portions, 1e-9)
	assert.InDelta(t, 2.0, got.FrozenPortions, 1e-9)
	require.NotNil(t, got.CookedAt)
	assert.True(t, cooked.Equal(*got.CookedAt))
	assert.Equal(t, "double garlic", *got.Note)
	assert.Equal(t, sun.AddDate(0, 0, 1), got.CookDate)

	got.CookedAt, got.Note = nil, nil
	require.NoError(t, repo.UpdateSession(ctx, got))
	got, err = repo.GetSession(ctx, s.ID)
	require.NoError(t, err)
	assert.Nil(t, got.CookedAt, "nullable round trip")
	assert.Nil(t, got.Note)

	require.NoError(t, repo.DeleteSession(ctx, s.ID))
	_, err = repo.GetSession(ctx, s.ID)
	assert.ErrorIs(t, err, domain.ErrNotFound)
	assert.ErrorIs(t, repo.DeleteSession(ctx, s.ID), domain.ErrNotFound)
	missing := &plate.CookSession{State: plate.StateConfirmed, ID: 999, FoodID: chili, CookDate: sun, CookSlotID: slotID, Portions: 1}
	assert.ErrorIs(t, repo.UpdateSession(ctx, missing), domain.ErrNotFound)
}

func TestCookSessionRepo_LinksAndRestrict(t *testing.T) {
	db := testhelper.NewTestDB(t)
	ctx := context.Background()
	slotID := seedSlot(t, db)
	chili := seedComposed(t, db, "Chili")
	sessions := NewCookSessionRepo(db)
	plates := NewPlateRepo(db)
	sun := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)

	s := &plate.CookSession{State: plate.StateConfirmed, FoodID: chili, CookDate: sun, CookSlotID: slotID, Portions: 5}
	require.NoError(t, sessions.CreateSession(ctx, s))
	one, half := 1.0, 1.5
	p1 := &plate.Plate{State: plate.StateConfirmed, Date: sun, SlotID: slotID, Eaters: 2, Components: []plate.PlateComponent{{FoodID: chili, Portions: &one, CookSessionID: &s.ID}}}
	p2 := &plate.Plate{State: plate.StateConfirmed, Date: sun.AddDate(0, 0, 1), SlotID: slotID, Eaters: 2, Components: []plate.PlateComponent{{FoodID: chili, Portions: &half, CookSessionID: &s.ID}}}
	require.NoError(t, plates.Create(ctx, p1))
	require.NoError(t, plates.Create(ctx, p2))

	links, err := sessions.ListLinks(ctx, s.ID)
	require.NoError(t, err)
	require.Len(t, links, 2)
	assert.Equal(t, plate.Link{
		PlateID: p1.ID, ComponentID: p1.Components[0].ID, FoodID: chili,
		Cell: plate.CellKey{Date: sun, SlotID: slotID}, PerEater: 1, Eaters: 2,
	}, links[0])
	assert.InDelta(t, 1.5, links[1].PerEater, 1e-9)

	assert.ErrorIs(t, sessions.DeleteSession(ctx, s.ID), domain.ErrInUse, "ON DELETE RESTRICT while linked")

	empty, err := sessions.ListLinks(ctx, 999)
	require.NoError(t, err)
	assert.Empty(t, empty)

	bad := &plate.CookSession{State: plate.StateConfirmed, FoodID: 999, CookDate: sun, CookSlotID: slotID, Portions: 1}
	assert.ErrorIs(t, sessions.CreateSession(ctx, bad), domain.ErrInvalidInput)
}

func TestCookSessionRepo_ListInRange(t *testing.T) {
	db := testhelper.NewTestDB(t)
	ctx := context.Background()
	slotID := seedSlot(t, db)
	chili := seedComposed(t, db, "Chili")
	sessions := NewCookSessionRepo(db)
	plates := NewPlateRepo(db)
	day := func(s string) time.Time {
		d, err := time.Parse(time.DateOnly, s)
		require.NoError(t, err)
		return d
	}
	one := 1.0
	session := func(cook string, eaten ...string) *plate.CookSession {
		s := &plate.CookSession{State: plate.StateConfirmed, FoodID: chili, CookDate: day(cook), CookSlotID: slotID, Portions: float64(len(eaten) + 1)}
		require.NoError(t, sessions.CreateSession(ctx, s))
		for _, d := range eaten {
			p := &plate.Plate{State: plate.StateConfirmed, Date: day(d), SlotID: slotID, Eaters: 1, Components: []plate.PlateComponent{{FoodID: chili, Portions: &one, CookSessionID: &s.ID}}}
			require.NoError(t, plates.Create(ctx, p))
		}
		return s
	}
	cooksIn := session("2026-11-01", "2026-11-01", "2026-11-05")
	linkedIn := session("2026-10-25", "2026-10-26", "2026-11-02")
	cooksInUnlinked := session("2026-11-04")
	session("2026-11-20", "2026-11-20")
	session("2026-10-20", "2026-10-21")

	got, links, err := sessions.ListInRange(ctx, day("2026-11-01"), day("2026-11-04"))
	require.NoError(t, err)
	ids := make([]int64, len(got))
	for i, s := range got {
		ids[i] = s.ID
	}
	assert.Equal(t, []int64{linkedIn.ID, cooksIn.ID, cooksInUnlinked.ID}, ids, "cook date order")
	require.Len(t, links[cooksIn.ID], 2, "links outside the range come along")
	assert.Equal(t, day("2026-11-05"), links[cooksIn.ID][1].Cell.Date)
	assert.InDelta(t, 1.0, links[cooksIn.ID][1].PerEater, 1e-9)
	assert.InDelta(t, 1.0, links[cooksIn.ID][1].Eaters, 1e-9)
	assert.Len(t, links[linkedIn.ID], 2)
	assert.Empty(t, links[cooksInUnlinked.ID])
	assert.Len(t, links, 2, "only links of the listed sessions")
}

// TestRepos_CorruptDatesFail: a date or timestamp column that does not parse
// is an error naming the row, never a silent 0001-01-01.
func TestRepos_CorruptDatesFail(t *testing.T) {
	sun := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	one := 1.0
	setup := func(t *testing.T) (*sql.DB, *plate.CookSession, *plate.Plate) {
		t.Helper()
		db := testhelper.NewTestDB(t)
		ctx := context.Background()
		slotID := seedSlot(t, db)
		chili := seedComposed(t, db, "Chili")
		s := &plate.CookSession{State: plate.StateConfirmed, FoodID: chili, CookDate: sun, CookSlotID: slotID, Portions: 2}
		require.NoError(t, NewCookSessionRepo(db).CreateSession(ctx, s))
		p := &plate.Plate{State: plate.StateConfirmed, Date: sun, SlotID: slotID, Eaters: 1, Components: []plate.PlateComponent{{FoodID: chili, Portions: &one, CookSessionID: &s.ID}}}
		require.NoError(t, NewPlateRepo(db).Create(ctx, p))
		require.NoError(t, NewPlateRepo(db).RestoreFeedback(ctx, p.ID, plate.Feedback{Status: "loved", RatedAt: sun}))
		return db, s, p
	}
	ctx := context.Background()
	tests := []struct {
		name   string
		update string
		read   func(db *sql.DB, s *plate.CookSession, p *plate.Plate) error
		want   func(s *plate.CookSession, p *plate.Plate) string
	}{
		{
			"plate date", `UPDATE plates SET date = 'garbage'`,
			func(db *sql.DB, _ *plate.CookSession, p *plate.Plate) error {
				_, err := NewPlateRepo(db).Get(ctx, p.ID)
				return err
			},
			func(_ *plate.CookSession, p *plate.Plate) string { return fmt.Sprintf("plate %d", p.ID) },
		},
		{
			"plate created_at in range", `UPDATE plates SET created_at = 'garbage'`,
			func(db *sql.DB, _ *plate.CookSession, _ *plate.Plate) error {
				_, err := NewPlateRepo(db).ListByDateRange(ctx, sun, sun)
				return err
			},
			func(_ *plate.CookSession, p *plate.Plate) string { return fmt.Sprintf("plate %d", p.ID) },
		},
		{
			"feedback rated_at", `UPDATE plate_feedback SET rated_at = 'garbage'`,
			func(db *sql.DB, _ *plate.CookSession, p *plate.Plate) error {
				_, err := NewPlateRepo(db).Get(ctx, p.ID)
				return err
			},
			func(_ *plate.CookSession, p *plate.Plate) string { return fmt.Sprintf("plate %d feedback", p.ID) },
		},
		{
			"session cook_date", `UPDATE cook_sessions SET cook_date = 'garbage'`,
			func(db *sql.DB, s *plate.CookSession, _ *plate.Plate) error {
				_, err := NewCookSessionRepo(db).GetSession(ctx, s.ID)
				return err
			},
			func(s *plate.CookSession, _ *plate.Plate) string { return fmt.Sprintf("cook session %d", s.ID) },
		},
		{
			"session cooked_at", `UPDATE cook_sessions SET cooked_at = 'garbage'`,
			func(db *sql.DB, s *plate.CookSession, _ *plate.Plate) error {
				_, err := NewCookSessionRepo(db).GetSession(ctx, s.ID)
				return err
			},
			func(s *plate.CookSession, _ *plate.Plate) string { return fmt.Sprintf("cook session %d", s.ID) },
		},
		{
			"link date", `UPDATE plates SET date = 'garbage'`,
			func(db *sql.DB, s *plate.CookSession, _ *plate.Plate) error {
				_, err := NewCookSessionRepo(db).ListLinks(ctx, s.ID)
				return err
			},
			func(_ *plate.CookSession, p *plate.Plate) string { return fmt.Sprintf("plate %d", p.ID) },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, s, p := setup(t)
			_, err := db.Exec(tt.update)
			require.NoError(t, err)
			err = tt.read(db, s, p)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want(s, p))
		})
	}
}

func TestPlateRepo_EatersSessionsFeedbackAndFoodFacts(t *testing.T) {
	db := testhelper.NewTestDB(t)
	ctx := context.Background()
	slotID := seedSlot(t, db)
	chili := seedComposed(t, db, "Chili")
	sessions := NewCookSessionRepo(db)
	repo := NewPlateRepo(db)
	sun := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)

	s := &plate.CookSession{State: plate.StateConfirmed, FoodID: chili, CookDate: sun, CookSlotID: slotID, Portions: 3.5}
	require.NoError(t, sessions.CreateSession(ctx, s))
	portions := 1.0
	note := "late"
	p := &plate.Plate{State: plate.StateConfirmed, Date: sun, SlotID: slotID, Eaters: 3.5, Note: &note, Components: []plate.PlateComponent{
		{FoodID: chili, Portions: &portions, CookSessionID: &s.ID},
	}}
	require.NoError(t, repo.Create(ctx, p))
	assert.False(t, p.UpdatedAt.IsZero(), "updated_at is set on create")

	rated := time.Date(2026, 11, 2, 8, 0, 0, 0, time.UTC)
	fbNote := "great"
	require.NoError(t, repo.RestoreFeedback(ctx, p.ID, plate.Feedback{Status: "loved", Note: &fbNote, RatedAt: rated}))

	check := func(t *testing.T, got *plate.Plate) {
		t.Helper()
		assert.InDelta(t, 3.5, got.Eaters, 1e-9)
		require.Len(t, got.Components, 1)
		c := got.Components[0]
		require.NotNil(t, c.CookSessionID)
		assert.Equal(t, s.ID, *c.CookSessionID)
		assert.InDelta(t, 1.0, *c.Portions, 1e-9)
		assert.Equal(t, "Chili", c.FoodName)
		assert.Equal(t, "composed", c.FoodKind)
		require.NotNil(t, c.FoodRole)
		assert.Equal(t, "main", *c.FoodRole)
		require.NotNil(t, got.Feedback)
		assert.Equal(t, "loved", got.Feedback.Status)
		assert.Equal(t, "great", *got.Feedback.Note)
		assert.True(t, rated.Equal(got.Feedback.RatedAt), "rated_at is kept")
	}
	got, err := repo.Get(ctx, p.ID)
	require.NoError(t, err)
	check(t, got)

	listed, err := repo.ListByDateRange(ctx, sun, sun)
	require.NoError(t, err)
	require.Len(t, listed, 1)
	check(t, &listed[0])

	got.Eaters = 2
	require.NoError(t, repo.Update(ctx, got))
	require.NoError(t, repo.Touch(ctx, got.ID))
	got, err = repo.Get(ctx, p.ID)
	require.NoError(t, err)
	assert.InDelta(t, 2.0, got.Eaters, 1e-9)

	// The component's session can be changed (swap) and cleared for a leaf.
	other := &plate.CookSession{State: plate.StateConfirmed, FoodID: chili, CookDate: sun, CookSlotID: slotID, Portions: 2}
	require.NoError(t, sessions.CreateSession(ctx, other))
	c := got.Components[0]
	c.CookSessionID = &other.ID
	require.NoError(t, repo.UpdateComponent(ctx, &c))
	stored, err := repo.GetComponent(ctx, c.ID)
	require.NoError(t, err)
	assert.Equal(t, other.ID, *stored.CookSessionID)
}
