package planning_test

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/domain/window"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper/testsvc"
)

// B-2 (l1r7-tune g2-protein-06, "mehr Eiweiß am Sonntag"): on chat-veg1700's
// Sunday (1701 kcal, range 1530 to 1785) every protein food that adds 10 g
// takes the day over its range top or puts a third dish on one cooking time
// (KR2), and none breaks a hard rule. The empty search used to say "without
// breaking a hard rule"; it names the checks that really dropped them.
func TestWays_MoreProteinNamesTheCheckThatBlocked(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile(filepath.Join("..", "..", "spectest", "testdata", "chat-veg1700.db"))
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "r.db")
	require.NoError(t, os.WriteFile(path, data, 0o600))
	db, err := sql.Open("sqlite", testhelper.DSN(path))
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	testhelper.Migrate(t, db)
	w := window.Containing(time.Date(2026, 11, 21, 0, 0, 0, 0, time.UTC), 5)
	st, err := testsvc.PlanningDeps(db).Builder.Build(context.Background(), w, time.Date(2026, 11, 17, 0, 0, 0, 0, time.UTC), w.Start, w.End)
	require.NoError(t, err)

	res, err := planning.Ways(st, planning.WindowTarget(st), planning.WayRequest{Date: "2026-11-22", Goal: planning.GoalMoreProtein})
	require.NoError(t, err)
	require.Empty(t, res.Ways)
	assert.Equal(t, "No change adds 10 g of protein or more without taking the day (1701 kcal) over its range top of 1785 kcal or putting more than 2 dishes on one cooking time.", res.EmptyWhy)
}
