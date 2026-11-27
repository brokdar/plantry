package agent_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain/agent"
	"github.com/jaltszeimer/plantry/backend/internal/domain/airun"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
	"github.com/jaltszeimer/plantry/backend/internal/domain/slot"
	"github.com/jaltszeimer/plantry/backend/internal/domain/window"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper/testsvc"
)

func migrateCopy(t *testing.T, conn *sql.DB) {
	t.Helper()
	testhelper.Migrate(t, conn)
}

// replayEnv wires the tool set over a COPY of a start database.
func replayEnv(t *testing.T, src string) *planToolEnv {
	return replayEnvAt(t, src, time.Date(2026, 11, 21, 0, 0, 0, 0, time.UTC))
}

// replayEnvAt is replayEnv with the chat window starting on start.
func replayEnvAt(t *testing.T, src string, start time.Time) *planToolEnv {
	data, err := os.ReadFile(src)
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "r.db")
	require.NoError(t, os.WriteFile(path, data, 0o600))
	db, err := sql.Open("sqlite", testhelper.DSN(path))
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	// The fixture may predate the newest migrations: bring the copy up.
	migrateCopy(t, db)
	t.Cleanup(func() { _ = db.Close() })
	today := time.Date(2026, 11, 17, 0, 0, 0, 0, time.UTC)
	e := &planToolEnv{t: t, db: db}
	set := testsvc.Settings(db)
	e.deps = testsvc.PlanningDeps(db)
	e.rec = airun.NewRecorder(sqlite.NewAIRunRepo(db), slog.New(slog.DiscardHandler), time.Now)
	e.deps.RunLog, e.deps.Runs = planning.RecorderLog{Recorder: e.rec}, sqlite.NewAIRunRepo(db)
	e.deps.Go = func(f func()) { f() }
	e.planner = planning.NewService(e.deps)
	e.plates, e.avail = testsvc.PlanAt(db, nil, func() time.Time { return today })
	e.changes = agent.NewChanges(sqlite.NewAIChangeRepo(db), e.plates, e.avail, sqlite.NewPlanLightRepo(db), sqlite.NewTxRunner(db), func() time.Time { return today })
	foods := sqlite.NewFoodRepo(db)
	e.ts, err = agent.NewToolSet(agent.Services{
		Foods: food.NewService(foods), NutritionResolver: food.NewNutritionResolver(foods),
		Plates: e.plates, Profile: profile.NewService(sqlite.NewProfileRepo(db)),
		Slots: slot.NewService(sqlite.NewSlotRepo(db)), Settings: set,
		PlanView: testsvc.PlanView(db), Planner: e.planner, Cells: e.avail, Availability: e.avail,
		Changes: sqlite.NewAIChangeRepo(db),
		State:   testsvc.PlanState(db), Tx: sqlite.NewTxRunner(db), Light: sqlite.NewPlanLightRepo(db), Undo: e.changes,
	})
	require.NoError(t, err)
	e.ctx = agent.WithWindow(agent.WithToday(context.Background(), today), window.Window{Start: start, End: start.AddDate(0, 0, 6)})
	return e
}

// chatStartDB is the chat start database of the a1 to e5x rounds, slimmed to
// the plan, the library and the settings (no run data): the replays below
// start from a copy of it, so they run in every gate.
const chatStartDB = "testdata/chat_start.db"

// TestReplayA1 replays three request shapes of the a1 round on a copy of the
// start database: each result carries one recommended option.
func TestReplayA1(t *testing.T) {
	t.Parallel()
	testhelper.Slow(t, "replays a recorded chat turn by turn over the A1 fixture database, every tool call against the real planner; one goroutine, nothing for -race to find")
	src := chatStartDB
	run := func(e *planToolEnv, ctx context.Context, name, in string) map[string]any {
		out, _, _, err := e.ts.Execute(ctx, name, json.RawMessage(in))
		require.NoError(t, err)
		var m map[string]any
		require.NoError(t, json.Unmarshal(out, &m))
		return m
	}
	t.Run("lower day under the minimum", func(t *testing.T) {
		e := replayEnv(t, src)
		ctx, _ := e.chat()
		res := run(e, ctx, "find_ways", `{"goal":"lower_day","date":"2026-11-24","target_kcal":900}`)
		o := relMaps(res["offer"])[0]
		assert.Equal(t, true, o["recommended"])
		got := run(e, ctx, "apply_changes", mustMarshal(t, map[string]any{"ops": o["ops"], "goal": res["goal"], "light": o["light"], "dry_run": true}))
		kcal, _, _ := dayOf(t, got)
		assert.InDelta(t, o["after"].(map[string]any)["day_kcal"], kcal, 1)
		assert.GreaterOrEqual(t, kcal, res["minimum_kcal"].(float64))
	})
	t.Run("move creates a repeat", func(t *testing.T) {
		e := replayEnv(t, src)
		ctx, _ := e.chat()
		res := run(e, ctx, "apply_changes", `{"ops":[{"op":"move","plate_id":32,"to":{"date":"2026-11-27","slot_id":3},"occupant":"swap"}]}`)
		offers := relMaps(res["offers"])
		// The fixture holds a variant pair (Chili con carne / sin carne, one group = one dish for H8):
		// their two cooks are a repeat, and taking the other variant's batch removes it.
		require.Len(t, offers, 2)
		assert.Equal(t, true, offers[0]["recommended"])
		assert.Equal(t, false, offers[1]["recommended"])
		fri := offers[1] // the swap on the moved plate's day
		eff := fri["effect"].(map[string]any)
		require.Equal(t, "Fri 27 Nov", eff["day"])
		got := run(e, ctx, "apply_changes", mustMarshal(t, map[string]any{"ops": fri["ops"], "dry_run": true}))
		after := relMaps(got["after"].(map[string]any)["days"])
		var kcal float64
		for _, d := range after {
			if d["date"] == "2026-11-27" {
				kcal = d["kcal"].(float64)
			}
		}
		assert.InDelta(t, eff["day_kcal_after"], kcal, 1, "promise equals the dry run")
	})
	t.Run("vegetarian swap leaves the day under its range", func(t *testing.T) {
		e := replayEnv(t, src)
		ctx, _ := e.chat()
		res := run(e, ctx, "apply_changes", `{"ops":[{"op":"swap","plate_component_id":39,"food_id":111,"portions":1}]}`)
		issues := res["after"].(map[string]any)["issues"].(map[string]any)
		fix := relMaps(relMaps(issues["day_fix"])[0]["options"])
		require.Len(t, fix, 1)
		assert.Equal(t, true, fix[0]["recommended"])
		for _, n := range relMaps(issues["new"]) {
			assert.Nil(t, n["options"])
		}
	})
}

func mustMarshal(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return string(b)
}
