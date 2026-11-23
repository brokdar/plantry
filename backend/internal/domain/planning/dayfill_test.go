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
	t.Run("a cold plate beside the one cooked dinner", func(t *testing.T) {
		s, tgt, p := emptyLunchDay()
		coldFoods(&s)
		got, notes := planning.FillEmptyMeals(planning.ChooseInput{State: s, Target: tgt}, p)
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
		s, tgt, p := emptyLunchDay()
		got, notes := planning.FillEmptyMeals(planning.ChooseInput{State: s, Target: tgt}, p)
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
		s, _, p := emptyLunchDay()
		week := targetCells(cell(mon, lunch), cell(tue, lunch), cell(tue, dinner), cell(wed, lunch), cell(thu, lunch), cell(fri, lunch))
		got, notes := planning.FillEmptyMeals(planning.ChooseInput{State: s, Target: week}, p)
		assert.Equal(t, p, got)
		assert.Empty(t, notes)
	})
}

// regenDay replans one day of a spec chat household with the heuristic.
func regenDay(t *testing.T, dbName string, date time.Time) *planning.RunView {
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
	for _, c := range []struct {
		db   string
		date time.Time
	}{{"chat-newuser", time.Date(2026, 11, 22, 0, 0, 0, 0, time.UTC)}, {"chat-veg1700", time.Date(2026, 11, 26, 0, 0, 0, 0, time.UTC)}} {
		t.Run(c.db+"/"+c.date.Format("Mon"), func(t *testing.T) {
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
	s, tgt, p := emptyLunchDay()
	why := planning.EmptyDayMeals(planning.Merge(s, tgt, p), tgt, day(tue))
	assert.Contains(t, why, "would stay empty")
	filled, _ := planning.FillEmptyMeals(planning.ChooseInput{State: s, Target: tgt}, p)
	assert.Empty(t, planning.EmptyDayMeals(planning.Merge(s, tgt, filled), tgt, day(tue)))
}
