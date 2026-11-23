package planning_test

import (
	"context"
	"database/sql"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain/airun"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper/testsvc"
)

// emptyLunchDay is a Tuesday replan that cooks Main 4 at dinner and leaves
// lunch empty (the day may cook once).
func emptyLunchDay() (planning.State, planning.Target, planning.Proposal) {
	s := batchCooker()
	tgt := targetCells(cell(tue, lunch), cell(tue, dinner))
	p := planning.Proposal{
		Sessions: []planning.ProposedSession{session("s1", 104, tue, dinner, 1)},
		Plates:   []planning.ProposedPlate{pplate(tue, dinner, onKey(104, "s1"))},
	}
	return s, tgt, p
}

func plateAt(p planning.Proposal, date string, slot int64) (planning.ProposedPlate, bool) {
	for _, pl := range p.Plates {
		if pl.Date.Equal(day(date)) && pl.SlotID == slot {
			return pl, true
		}
	}
	return planning.ProposedPlate{}, false
}

// KR7 (tune-v2c g2-replan-01): a meal the day's one cooking time leaves empty
// takes a cold plate first, else the day's dish again; never stays empty.
func TestFillEmptyMeals_KR7Fallbacks(t *testing.T) {
	t.Parallel()
	t.Run("a cold plate beside the one cooked dinner", func(t *testing.T) {
		t.Parallel()
		s, tgt, p := emptyLunchDay()
		coldFoods(&s)
		got, notes := planning.FillEmptyMeals(planning.ChooseInput{State: s, Target: tgt}, p, nil)
		pl, ok := plateAt(got, tue, lunch)
		require.True(t, ok, "lunch is filled")
		for _, c := range pl.Components {
			assert.Zero(t, c.Portions, "no recipe on a cold plate")
			assert.Positive(t, c.Grams)
		}
		require.Len(t, notes, 1)
		assert.Contains(t, notes[0], "is a cold plate")
		saved := strings.Join(planning.SavedPlanNotes(planning.ChooseInput{State: s, Target: tgt}, p, got), "\n")
		assert.Contains(t, saved, "is a cold plate, nothing to cook")
		for _, v := range violations(check(s, tgt, got)) {
			assert.NotEqual(t, planning.CodeMissingPlate, v.Code)
			assert.NotEqual(t, planning.CodeTooManyCookOccasions, v.Code)
		}
	})
	t.Run("without cold foods lunch eats the day's dish, cooked at lunch", func(t *testing.T) {
		t.Parallel()
		s, tgt, p := emptyLunchDay()
		got, notes := planning.FillEmptyMeals(planning.ChooseInput{State: s, Target: tgt}, p, nil)
		pl, ok := plateAt(got, tue, lunch)
		require.True(t, ok, "lunch is filled")
		assert.Equal(t, "s1", pl.Components[0].SessionKey)
		require.Len(t, got.Sessions, 1)
		assert.EqualValues(t, lunch, got.Sessions[0].CookSlotID, "the dish cooks at the earlier meal")
		require.Len(t, notes, 1)
		assert.Contains(t, notes[0], "eats the day's cooked dish again")
		for _, v := range violations(check(s, tgt, got)) {
			assert.NotEqual(t, planning.CodeMissingPlate, v.Code)
			assert.NotEqual(t, planning.CodeTooManyCookOccasions, v.Code)
		}
	})
	t.Run("a week run is the chooser's alone", func(t *testing.T) {
		t.Parallel()
		s, _, p := emptyLunchDay()
		week := targetCells(cell(mon, lunch), cell(tue, lunch), cell(tue, dinner), cell(wed, lunch), cell(thu, lunch), cell(fri, lunch))
		got, notes := planning.FillEmptyMeals(planning.ChooseInput{State: s, Target: week}, p, nil)
		assert.Equal(t, p, got)
		assert.Empty(t, notes)
	})
}

// regenDay replans one day of a spec chat household with the heuristic.
func regenDay(t *testing.T, dbName string, date time.Time, setup ...string) *planning.RunView {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "spectest", "testdata", dbName+".db"))
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "r.db")
	require.NoError(t, os.WriteFile(path, data, 0o600))
	db, err := sql.Open("sqlite", testhelper.DSN(path))
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	testhelper.Migrate(t, db)
	for _, q := range setup {
		_, err := db.Exec(q)
		require.NoError(t, err)
	}
	deps := testsvc.PlanningDeps(db)
	deps.RunLog, deps.Runs = planning.RecorderLog{Recorder: airun.NewRecorder(sqlite.NewAIRunRepo(db), slog.New(slog.DiscardHandler), time.Now)}, sqlite.NewAIRunRepo(db)
	deps.Go = func(f func()) { f() }
	svc := planning.NewService(deps)
	id, err := svc.Regenerate(context.Background(), planning.Request{
		Start: time.Date(2026, 11, 21, 0, 0, 0, 0, time.UTC), Today: time.Date(2026, 11, 17, 0, 0, 0, 0, time.UTC),
		Scope: planning.Scope{Kind: planning.ScopeDay, Date: date}, Chooser: airun.ChooserHeuristic,
	})
	require.NoError(t, err)
	v, err := svc.Run(context.Background(), id)
	require.NoError(t, err)
	return v
}

// B-3/KR7 (tune-v2c g2-replan-01): Sunday's lunch ate Saturday's batch; the
// replan avoids that dish and cooks once at dinner. It used to save Sunday
// with lunch empty (partial, a missing plate); now lunch is a cold plate.
func TestDayReplan_NeverSavesAnEmptyMeal(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		db   string
		date time.Time
	}{{"chat-newuser", time.Date(2026, 11, 22, 0, 0, 0, 0, time.UTC)}, {"chat-veg1700", time.Date(2026, 11, 26, 0, 0, 0, 0, time.UTC)}} {
		t.Run(c.db+"/"+c.date.Format("Mon"), func(t *testing.T) {
			t.Parallel()
			v := regenDay(t, c.db, c.date)
			require.Equal(t, airun.StatusOK, v.Status, "%s %s", v.ErrorKey, v.Notes)
			for _, is := range v.Issues {
				if is.Severity == planning.SeverityViolation {
					assert.NotEqual(t, planning.CodeMissingPlate, is.Code, "no meal left empty")
				}
			}
			assert.Contains(t, v.Notes, "is a cold plate, nothing to cook")
		})
	}
}

// A day replan that no fallback fills names what stays open, so it saves nothing.
func TestEmptyDayMeals_NamesTheEmptyMeal(t *testing.T) {
	t.Parallel()
	s, tgt, p := emptyLunchDay()
	why := planning.EmptyDayMeals(planning.Merge(s, tgt, p), tgt, day(tue), nil, true)
	assert.Contains(t, why, "would stay empty")
	filled, _ := planning.FillEmptyMeals(planning.ChooseInput{State: s, Target: tgt}, p, nil)
	assert.Empty(t, planning.EmptyDayMeals(planning.Merge(s, tgt, filled), tgt, day(tue), nil, true))
}

// F1 (review of #183): a day under its minimum fails a fill unless the day was
// under it at the start of the run (SF1 a). A cold plate too small to close
// the floor is refused and the day's dish again takes the meal; with the floor
// open at the start the day may stay under it, so the cold plate is taken.
func TestFillEmptyMeals_FloorFailsAFillUnlessOpenAtTheStart(t *testing.T) {
	t.Parallel()
	s, tgt, p := emptyLunchDay()
	coldFoods(&s)
	s.Profile.KcalTarget = f64(3200)
	in := planning.ChooseInput{State: s, Target: tgt}
	tue := planning.Issue{Rule: "H9", Code: planning.CodeKcalBelowFloor, Severity: planning.SeverityViolation, Date: "2026-11-10"}

	got, _ := planning.FillEmptyMeals(in, p, nil)
	pl, ok := plateAt(got, "2026-11-10", lunch)
	require.True(t, ok, "lunch is filled")
	assert.Equal(t, "s1", pl.Components[0].SessionKey, "the too-small cold plate is refused; the day's dish again")

	got, notes := planning.FillEmptyMeals(in, p, []planning.Issue{tue})
	pl, ok = plateAt(got, "2026-11-10", lunch)
	require.True(t, ok)
	assert.Zero(t, pl.Components[0].Portions, "the cold plate is taken")
	require.NotEmpty(t, notes)
	assert.Contains(t, notes[0], "is a cold plate")
}

// The service path of keyDayMealEmpty (F2): with no no-cook food a fill cannot
// hold. A day the run newly leaves under its minimum saves nothing; a day whose
// hard issues were open at the start (SF1 a) saves what the run found.
func TestDayReplan_KeyDayMealEmptyAndTheStartOpenExemption(t *testing.T) {
	t.Parallel()
	const noCold = "UPDATE foods SET no_cook = 0"
	t.Run("a day newly under its minimum saves nothing", func(t *testing.T) {
		t.Parallel()
		v := regenDay(t, "chat-newuser", time.Date(2026, 11, 21, 0, 0, 0, 0, time.UTC), noCold)
		assert.Equal(t, airun.StatusError, v.Status)
		assert.Equal(t, "error.plan.day_meal_empty", v.ErrorKey)
		assert.Contains(t, v.Notes, "Nothing was saved: Saturday would stay under its minimum")
		assert.NotContains(t, v.Notes, "cooks once")
	})
	t.Run("an issue open at the start does not refuse the day", func(t *testing.T) {
		t.Parallel()
		v := regenDay(t, "chat-newuser", time.Date(2026, 11, 22, 0, 0, 0, 0, time.UTC), noCold)
		assert.Equal(t, airun.StatusPartial, v.Status, "saved with its open issue: %s %s", v.ErrorKey, v.Notes)
		assert.Empty(t, v.ErrorKey)
	})
}

// F3: the refusal says the real reason. The cooking cap is named only when it
// is used; "neither a cold plate nor a second serving" only when both ran.
func TestEmptyDayMeals_SaysTheRealReason(t *testing.T) {
	t.Parallel()
	s, tgt, p := emptyLunchDay()
	m := planning.Merge(s, tgt, p)
	why := planning.EmptyDayMeals(m, tgt, day(tue), nil, true)
	assert.Contains(t, why, "no cooking time is left that day, and neither a cold plate nor a second serving")
	why = planning.EmptyDayMeals(m, tgt, day(tue), nil, false)
	assert.Contains(t, why, "no cooking time is left that day")
	assert.NotContains(t, why, "neither a cold plate")
	m.Settings.MaxCookOccasionsPerDay = 3
	why = planning.EmptyDayMeals(m, tgt, day(tue), nil, true)
	assert.Contains(t, why, "no dish fits that meal")
	assert.NotContains(t, why, "cooking time")
	// Open at the start, by issue id, the empty meal is no reason (SF1 a).
	for _, is := range planning.OpenHard(m, tgt) {
		if is.Code == planning.CodeMissingPlate {
			assert.Empty(t, planning.EmptyDayMeals(m, tgt, day(tue), []planning.Issue{is}, true))
		}
	}
}
