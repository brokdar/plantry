package feedback_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/feedback"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/foodevent"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper/testsvc"
)

// NEW-2/NEW-4, AC-15: a disliked first cooking archives a trial recipe and
// names it for the reason; a trial ends two days after a good eating, a
// "not eaten" plate keeps it running, and a disliked later cooking archives
// nothing.
func TestRecordFeedback_TrialRecipes(t *testing.T) {
	ctx := context.Background()
	db := testhelper.NewTestDB(t)
	now := time.Date(2026, 3, 2, 18, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	var slot int64
	for _, q := range []string{
		`INSERT INTO time_slots (name_key, icon, sort_order, active) VALUES ('slot.dinner', 'moon', 2, 1)`,
		`INSERT INTO foods (name, kind, role, reference_portions, trial) VALUES ('Soup', 'composed', 'main', 1, 1)`,
		`INSERT INTO foods (name, kind, role, reference_portions, trial) VALUES ('Stew', 'composed', 'main', 1, 1)`,
		`INSERT INTO foods (name, kind, role, reference_portions, trial) VALUES ('Dal', 'composed', 'main', 1, 1)`,
		`INSERT INTO foods (name, kind, role, reference_portions) VALUES ('Pasta', 'composed', 'main', 1)`,
	} {
		_, err := db.Exec(q)
		require.NoError(t, err, q)
	}
	require.NoError(t, db.QueryRow(`SELECT id FROM time_slots`).Scan(&slot))
	id := func(name string) int64 {
		var v int64
		require.NoError(t, db.QueryRow(`SELECT id FROM foods WHERE name = ?`, name).Scan(&v))
		return v
	}
	restorers := map[foodevent.Kind]foodevent.Restorer{}
	events := foodevent.NewService(sqlite.NewTxRunner(db), sqlite.NewFoodEventRepo(db), restorers, clock)
	foods := food.NewService(sqlite.NewFoodRepo(db)).WithEvents(events)
	restorers[foodevent.KindRecipe] = foods
	plates, _ := testsvc.PlanAt(db, nil, clock)
	fb := testsvc.Feedback(db).WithTrials(foods)

	eat := func(day int, ids ...int64) int64 {
		in := plate.PlateInput{Date: time.Date(2026, 3, day, 0, 0, 0, 0, time.UTC), SlotID: slot}
		for _, id := range ids {
			one := 1.0
			in.Components = append(in.Components, plate.PlateComponent{FoodID: id, Portions: &one})
		}
		p, err := plates.Create(ctx, in)
		require.NoError(t, err)
		_, err = db.Exec(`UPDATE plates SET state = 'confirmed' WHERE id = ?`, p.ID)
		require.NoError(t, err)
		return p.ID
	}
	onTrial := func(id int64) bool {
		f, err := foods.Get(ctx, id)
		require.NoError(t, err)
		require.NoError(t, foods.Derive(ctx, now, f))
		return f.OnTrial()
	}

	// A disliked first cooking archives the trial recipe, not the other one.
	got, err := fb.RecordFeedback(ctx, eat(1, id("Soup"), id("Pasta")), feedback.StatusDisliked, nil)
	require.NoError(t, err)
	require.Len(t, got.ArchivedTrials, 1)
	assert.Equal(t, "Soup", got.ArchivedTrials[0].Name)
	soup, err := foods.Get(ctx, id("Soup"))
	require.NoError(t, err)
	assert.True(t, soup.Archived)
	pasta, err := foods.Get(ctx, id("Pasta"))
	require.NoError(t, err)
	assert.False(t, pasta.Archived)

	// Eaten and liked: the trial runs for two days, then ends.
	eat(3, id("Stew"))
	now = time.Date(2026, 3, 4, 12, 0, 0, 0, time.UTC)
	assert.True(t, onTrial(id("Stew")))
	now = time.Date(2026, 3, 5, 12, 0, 0, 0, time.UTC)
	assert.False(t, onTrial(id("Stew")), "eaten two days ago without a bad rating")

	// Not eaten keeps the trial running.
	_, err = fb.RecordFeedback(ctx, eat(2, id("Dal")), feedback.StatusNotEaten, nil)
	require.NoError(t, err)
	assert.True(t, onTrial(id("Dal")))

	// A later cooking disliked after the trial archives nothing.
	now = time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	got, err = fb.RecordFeedback(ctx, eat(9, id("Stew")), feedback.StatusDisliked, nil)
	require.NoError(t, err)
	assert.Empty(t, got.ArchivedTrials)
}

type failingArchiver struct{}

func (failingArchiver) ArchiveDisliked(context.Context, []int64, time.Time) ([]food.ArchivedTrial, error) {
	return nil, errors.New("archive failed")
}

// NEW-4: the feedback and the archiving of a disliked trial share one
// transaction: a failed archive leaves no feedback row.
func TestRecordFeedback_FailedArchiveRollsBack(t *testing.T) {
	ctx := context.Background()
	db := testhelper.NewTestDB(t)
	clock := func() time.Time { return time.Date(2026, 3, 2, 18, 0, 0, 0, time.UTC) }
	for _, q := range []string{
		`INSERT INTO time_slots (name_key, icon, sort_order, active) VALUES ('slot.dinner', 'moon', 2, 1)`,
		`INSERT INTO foods (name, kind, role, reference_portions, trial) VALUES ('Soup', 'composed', 'main', 1, 1)`,
	} {
		_, err := db.Exec(q)
		require.NoError(t, err, q)
	}
	var slot, soup int64
	require.NoError(t, db.QueryRow(`SELECT id FROM time_slots`).Scan(&slot))
	require.NoError(t, db.QueryRow(`SELECT id FROM foods WHERE name = 'Soup'`).Scan(&soup))
	plates, _ := testsvc.PlanAt(db, nil, clock)
	one := 1.0
	p, err := plates.Create(ctx, plate.PlateInput{
		Date: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC), SlotID: slot,
		Components: []plate.PlateComponent{{FoodID: soup, Portions: &one}},
	})
	require.NoError(t, err)

	fb := testsvc.Feedback(db).WithTrials(failingArchiver{})
	_, err = fb.RecordFeedback(ctx, p.ID, feedback.StatusDisliked, nil)
	require.Error(t, err)
	_, err = sqlite.NewFeedbackRepo(db).Get(ctx, p.ID)
	assert.ErrorIs(t, err, domain.ErrNotFound)
}
