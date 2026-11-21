package planning_test

import (
	"context"
	"database/sql"
	"log/slog"
	"os"
	"path/filepath"
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

// KR7 (v1 OQ6, v1.2): a day replan whose day would repeat its main saves nothing
// only when no fallback (another main, a cold plate) fixes the day. These
// households (the spec's chat fixtures) all have a bread, a cucumber or tomato
// and a spread or cheese, so a cold plate is possible: the replan must save one,
// or end in error naming a hard rule that is really open.
func TestKR7_ColdPlateIsTheFallbackWhenItIsPossible(t *testing.T) {
	chatWeek := time.Date(2026, 11, 21, 0, 0, 0, 0, time.UTC)
	today := time.Date(2026, 11, 17, 0, 0, 0, 0, time.UTC)
	for _, c := range []struct {
		db  string
		day int
	}{{"chat-newuser", 4}, {"chat-veg1700", 2}, {"chat-veg1700", 4}} {
		date := chatWeek.AddDate(0, 0, c.day)
		t.Run(c.db+"/"+date.Format("01-02"), func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join("..", "..", "spectest", "testdata", c.db+".db"))
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
				Start: chatWeek, Today: today,
				Scope: planning.Scope{Kind: planning.ScopeDay, Date: date}, Chooser: airun.ChooserHeuristic,
			})
			require.NoError(t, err)
			v, err := svc.Run(context.Background(), id)
			require.NoError(t, err)
			if v.Status == airun.StatusError {
				assert.NotEqual(t, "error.plan.day_same_dish", v.ErrorKey, "a cold plate is possible here, yet: %s", v.Notes)
				return
			}
			assert.Contains(t, []airun.Status{airun.StatusOK, airun.StatusPartial}, v.Status)
		})
	}
}
