package planning_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/airun"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/domain/window"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper/testsvc"
)

// Real-data proof of E022: set PLANTRY_PROOF_ROUND to a loop round directory
// and run with -v. On a copy of the round's chat start DB it (1) regenerates
// Thursday with the heuristic chooser through the service and (2) puts the
// plan the model submitted in the paid replan-thu run through the same finishing
// step the service applies, printing lunch and dinner mains and the week's meat.
// No model is called. Skipped without the variable.
func TestE022_RealData(t *testing.T) {
	round := os.Getenv("PLANTRY_PROOF_ROUND")
	if round == "" {
		t.Skip("PLANTRY_PROOF_ROUND not set")
	}
	thu := time.Date(2026, 11, 26, 0, 0, 0, 0, time.UTC)
	names := func(s planning.State, id int64) string { return s.Foods[id].Name }
	mains := func(s planning.State, plates []planning.ProposedPlate) string {
		var out []string
		for _, pl := range plates {
			if pl.Date.Equal(thu) {
				var cs []string
				for _, c := range pl.Components {
					cs = append(cs, fmt.Sprintf("%s x%.2f", names(s, c.FoodID), c.Portions))
				}
				out = append(out, fmt.Sprintf("slot %d: %s", pl.SlotID, strings.Join(cs, " + ")))
			}
		}
		return strings.Join(out, " | ")
	}
	meatOf := func(s planning.State, tgt planning.Target, p planning.Proposal) float64 {
		return planning.WeekEffectOf(planning.Merge(s, tgt, p)).MeatG
	}

	// (2) the model's submitted plan through the finishing step.
	t.Run("model plan", func(t *testing.T) {
		db := copyDB(t, filepath.Join(round, "start-chat.db"))
		var in planning.ChooseInput
		deps := testsvc.PlanningDeps(db)
		runs := newFakeRuns(t, db)
		deps.RunLog, deps.Runs = runs, runs
		deps.Go = func(f func()) { f() }
		deps.Choosers = map[airun.Chooser]planning.Chooser{airun.ChooserHeuristic: captureChooser{&in}}
		svc := planning.NewService(deps)
		_, err := svc.Regenerate(context.Background(), planning.Request{
			Start: time.Date(2026, 11, 21, 0, 0, 0, 0, time.UTC), Today: time.Date(2026, 11, 17, 0, 0, 0, 0, time.UTC),
			Scope: planning.Scope{Kind: planning.ScopeDay, Date: thu},
		})
		require.NoError(t, err)
		svc.Wait()
		require.NotEmpty(t, in.Target.Cells)

		if os.Getenv("PLANTRY_PROOF_SIDES") != "" {
			raw, _ := lastSubmit(t, filepath.Join(round, "runs", "replan-thu-r1", "run.db"))
			old := planning.SetPairInPlanner(false)
			_, b, _ := planning.PlannerView(in, raw[len(raw)-1], true)
			planning.SetPairInPlanner(true)
			_, a, _ := planning.PlannerView(in, raw[len(raw)-1], true)
			planning.SetPairInPlanner(old)
			for i := range a.Plates {
				if a.Plates[i].Date.Equal(thu) {
					fmt.Printf("THU slot %d\n   before: %s\n   after:  %s\n", a.Plates[i].SlotID, sidesOf(in, b.Plates[i]), sidesOf(in, a.Plates[i]))
				}
			}
		}
		if os.Getenv("PLANTRY_PROOF_NOCOOK") != "" {
			for _, id := range in.State.NoCook {
				f := in.State.Foods[id]
				fmt.Printf("NOCOOK %d %-28s composed=%v role=%q leaf=%q kcal=%.0f prot=%.1f serv=%.0f sweet=%v diet=%s\n", id, f.Name, f.Composed(), f.Role, f.LeafRole, f.Macros.Kcal, f.Macros.Protein, f.ServingG, f.Sweet, f.DietClass)
			}
		}
		calls, _ := lastSubmit(t, filepath.Join(round, "runs", "replan-thu-r1", "run.db"))
		raw := calls[len(calls)-1]
		before, err := planning.LegacyView(in, raw)
		require.NoError(t, err)
		view, done, err := planning.PlannerView(in, raw, false)
		require.NoError(t, err)
		prop, err := planning.ParseForProof(raw, in)
		require.NoError(t, err)
		fmt.Printf("E022 model plan\n  before: %s\n  after:  %s\n  week meat g: %.0f -> %.0f (limit 300 x days/7)\n  notes: %v\n",
			mains(in.State, prop.Plates), mains(in.State, done.Plates), meatOf(in.State, in.Target, prop), meatOf(in.State, in.Target, done), view["code_changes"])
		_ = before
		raw2, _ := json.Marshal(view)
		var v struct {
			Days   []map[string]any `json:"days"`
			Issues []map[string]any `json:"issues"`
			Week   map[string]any   `json:"week"`
		}
		_ = json.Unmarshal(raw2, &v)
		for _, d := range v.Days {
			if d["date"] == "2026-11-26" {
				fmt.Printf("  Thursday kcal %v, band %v-%v, target %v, protein %v of %v\n", d["kcal"], d["band_low"], d["band_high"], d["target_kcal"], d["protein_g"], d["protein_target_g"])
			}
		}
		for _, is := range v.Issues {
			fmt.Printf("  issue %v/%v %v %v\n", is["rule"], is["code"], is["severity"], is["date"])
		}
		for _, pl := range done.Plates {
			if pl.Date.Equal(thu) {
				for _, c := range pl.Components {
					f := in.State.Foods[c.FoodID]
					k := f.Macros.Kcal * c.Portions
					if c.Grams > 0 {
						k = f.Macros.Kcal * c.Grams / 100
					}
					fmt.Printf("  slot %d: %s portions %.2f grams %.0f kcal %.0f\n", pl.SlotID, f.Name, c.Portions, c.Grams, k)
				}
			}
		}
		fmt.Printf("  week: %v\n", v.Week)
	})

	// (1) the heuristic regenerate, saved through the service.
	t.Run("heuristic regenerate", func(t *testing.T) {
		db := copyDB(t, filepath.Join(round, "start-chat.db"))
		deps := testsvc.PlanningDeps(db)
		runs := newFakeRuns(t, db)
		deps.RunLog, deps.Runs = runs, runs
		deps.Go = func(f func()) { f() }
		svc := planning.NewService(deps)
		id, err := svc.Regenerate(context.Background(), planning.Request{
			Start: time.Date(2026, 11, 21, 0, 0, 0, 0, time.UTC), Today: time.Date(2026, 11, 17, 0, 0, 0, 0, time.UTC),
			Scope: planning.Scope{Kind: planning.ScopeDay, Date: thu}, Chooser: airun.ChooserHeuristic,
		})
		require.NoError(t, err)
		svc.Wait()
		rows, err := db.Query(`SELECT p.slot_id, f.name FROM plates p JOIN plate_components pc ON pc.plate_id=p.id JOIN foods f ON f.id=pc.food_id WHERE p.date='2026-11-26' AND pc.position='main' ORDER BY p.slot_id`)
		require.NoError(t, err)
		defer rows.Close()
		var got []string
		for rows.Next() {
			var slot int64
			var name string
			require.NoError(t, rows.Scan(&slot, &name))
			got = append(got, fmt.Sprintf("slot %d: %s", slot, name))
		}
		var meat float64
		_ = db.QueryRow(`SELECT 0`).Scan(&meat)
		notes := runs.end(id).Notes
		st, err := testsvc.PlanState(db)(context.Background(), window.Window{Start: time.Date(2026, 11, 21, 0, 0, 0, 0, time.UTC), End: time.Date(2026, 11, 27, 0, 0, 0, 0, time.UTC)}, time.Date(2026, 11, 17, 0, 0, 0, 0, time.UTC))
		require.NoError(t, err)
		for _, l := range strings.Split(planning.RenderPlanSummary(st, nil), "\n") {
			if strings.HasPrefix(l, "Thursday 2026-11-26") {
				fmt.Println("  SUMMARY:", l)
			}
		}
		fmt.Printf("E022 heuristic regenerate (run %d)\n  mains: %s\n  notes: %q\n", id, strings.Join(got, " | "), notes)
		_ = json.Marshal
	})
}

// lastSubmit returns the tool inputs of the evaluate_plan and submit_plan calls of the last
// generate or regenerate run of a loop run.db.
func lastSubmit(t *testing.T, path string) ([]json.RawMessage, int64) {
	db := copyDB(t, path)
	var rid int64
	require.NoError(t, db.QueryRow(`SELECT max(id) FROM ai_runs WHERE kind IN ('generate','regenerate')`).Scan(&rid))
	rows, err := db.Query(`SELECT response_json FROM ai_run_steps WHERE run_id=? AND type='llm_call' ORDER BY seq`, rid)
	require.NoError(t, err)
	defer rows.Close()
	var calls []json.RawMessage
	for rows.Next() {
		var raw string
		require.NoError(t, rows.Scan(&raw))
		var r struct {
			Content []struct {
				Type  string
				Input json.RawMessage `json:"tool_use_input"`
				Name  string          `json:"tool_use_name"`
			}
		}
		if json.Unmarshal([]byte(raw), &r) != nil {
			continue
		}
		for _, b := range r.Content {
			if b.Type == "tool_use" && (b.Name == "evaluate_plan" || b.Name == "submit_plan") {
				calls = append(calls, b.Input)
			}
		}
	}
	require.NotEmpty(t, calls)
	return calls, rid
}
