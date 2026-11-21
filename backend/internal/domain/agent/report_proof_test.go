package agent_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain/agent"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
	"github.com/jaltszeimer/plantry/backend/internal/domain/slot"
	"github.com/jaltszeimer/plantry/backend/internal/domain/window"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper/testsvc"
)

// Real-data proof of the change report: set PLANTRY_PROOF_DB to a chat-start
// database and run with -v. It copies the file, calls apply_changes as a
// check (no model) for real requests and prints each report. Skipped without
// the variable.
func TestChangeReport_RealData(t *testing.T) {
	t.Parallel()
	src := os.Getenv("PLANTRY_PROOF_DB")
	if src == "" {
		t.Skip("PLANTRY_PROOF_DB not set")
	}
	raw, err := os.ReadFile(src)
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "proof.db")
	require.NoError(t, os.WriteFile(path, raw, 0o600))
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)")
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	today := time.Date(2026, 11, 17, 0, 0, 0, 0, time.UTC)
	start := time.Date(2026, 11, 21, 0, 0, 0, 0, time.UTC)
	plates, avail := testsvc.PlanAt(db, nil, func() time.Time { return today })
	foods := sqlite.NewFoodRepo(db)
	ts, err := agent.NewToolSet(agent.Services{
		Foods: food.NewService(foods), NutritionResolver: food.NewNutritionResolver(foods),
		Plates: plates, Profile: profile.NewService(sqlite.NewProfileRepo(db)),
		Slots: slot.NewService(sqlite.NewSlotRepo(db)), Settings: testsvc.Settings(db),
		PlanView: testsvc.PlanView(db), Cells: avail, Availability: avail,
		Changes: sqlite.NewAIChangeRepo(db), State: testsvc.PlanState(db), Tx: sqlite.NewTxRunner(db), Light: sqlite.NewPlanLightRepo(db),
	})
	require.NoError(t, err)
	ctx := agent.WithWindow(agent.WithToday(context.Background(), today), window.Window{Start: start, End: start.AddDate(0, 0, 6)})
	one := func(q string, args ...any) int64 {
		var id int64
		require.NoError(t, db.QueryRow(q, args...).Scan(&id), q)
		return id
	}
	run := func(name, input string) {
		out, _, _, err := ts.Execute(ctx, "apply_changes", json.RawMessage(input))
		if err != nil {
			fmt.Printf("== %s\nERROR %v\n", name, err)
			return
		}
		var m map[string]any
		require.NoError(t, json.Unmarshal(out, &m))
		rep, _ := json.MarshalIndent(m["report"], "", "  ")
		fmt.Printf("== %s\n%s\n", name, rep)
	}
	comp := func(plateID int64, food string) int64 {
		return one(`SELECT pc.id FROM plate_components pc JOIN foods f ON f.id=pc.food_id WHERE pc.plate_id=? AND f.name LIKE ?`, plateID, food)
	}
	plateAt := func(date string, slotID int64) int64 {
		return one(`SELECT id FROM plates WHERE date=? AND slot_id=?`, date, slotID)
	}
	// Tuesday dinner salmon 1 -> 0.5 portions.
	run("Tuesday dinner salmon 0.5", fmt.Sprintf(`{"dry_run":true,"ops":[{"op":"set_quantity","plate_component_id":%d,"portions":0.5}]}`, comp(plateAt("2026-11-24", 3), "Lachs%")))
	// Wednesday dinner: swap the beef ragu for the chili con carne batch (cooked Friday 27 Nov dinner is later: use fresh swap to the chili).
	chili := one(`SELECT id FROM foods WHERE name = 'Chili sin carne'`)
	run("Wednesday dinner to chili", fmt.Sprintf(`{"dry_run":true,"ops":[{"op":"swap","plate_component_id":%d,"food_id":%d}]}`, comp(plateAt("2026-11-25", 3), "Beef%"), chili))
	// Away on Wednesday.
	run("away Wednesday", `{"dry_run":true,"ops":[{"op":"away","date":"2026-11-25"}]}`)
	// Remove the Greek yogurt from all dinners.
	var ops string
	rows, err := db.Query(`SELECT pc.id FROM plate_components pc JOIN foods f ON f.id=pc.food_id JOIN plates p ON p.id=pc.plate_id WHERE f.name='Greek yogurt' AND p.slot_id=3 AND p.state='draft' ORDER BY p.date`)
	require.NoError(t, err)
	for rows.Next() {
		var id int64
		require.NoError(t, rows.Scan(&id))
		if ops != "" {
			ops += ","
		}
		ops += fmt.Sprintf(`{"op":"remove","plate_component_id":%d}`, id)
	}
	require.NoError(t, rows.Close())
	run("remove Greek yogurt from dinners", `{"dry_run":true,"ops":[`+ops+`]}`)
}
