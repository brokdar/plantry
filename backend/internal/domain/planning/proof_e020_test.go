package planning_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/airun"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper/testsvc"
)

// captureChooser records what the service hands a chooser and stops the run.
type captureChooser struct{ in *planning.ChooseInput }

func (c captureChooser) Choose(_ context.Context, in planning.ChooseInput) (planning.Proposal, error) {
	*c.in = in
	return planning.Proposal{}, errors.New("captured")
}

// Real-data proof of E020: set PLANTRY_PROOF_ROUND to a loop round directory
// (rounds/e014b-a1d24d2) and run with -v. For each stored generate run it
// rebuilds the planner's input from the round's start DB, takes the model's
// first evaluate_plan call and prints what it returns before and after
// E020. No model is called. Skipped without the variable.
func TestE020_RealData(t *testing.T) {
	round := os.Getenv("PLANTRY_PROOF_ROUND")
	if round == "" {
		t.Skip("PLANTRY_PROOF_ROUND not set")
	}
	only := os.Getenv("PLANTRY_PROOF_ONLY")
	runs, err := filepath.Glob(filepath.Join(round, "runs", "gen-*-r1"))
	require.NoError(t, err)
	for _, dir := range runs {
		label := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(dir), "gen-"), "-r1")
		if only != "" && !strings.Contains(","+only+",", ","+label+",") {
			continue
		}
		t.Run(label, func(t *testing.T) { proofOne(t, round, dir, label) })
	}
}

var proofInstruction = map[string]string{
	"light": "something light on weekdays", "plain": "", "veg": "a vegetarian week", "protein": "high protein this week",
	"guests": "two guests on Sunday", "short": "we're away on Tuesday and Wednesday", "busy": "a busy week, I can only cook on two days",
	"confirmed": "fill in the rest of the week", "inject": "use the quick tomato soup and the herb omelette this week",
}

func copyDB(t *testing.T, src string) *sql.DB {
	raw, err := os.ReadFile(src)
	require.NoError(t, err)
	p := filepath.Join(t.TempDir(), "p.db")
	require.NoError(t, os.WriteFile(p, raw, 0o600))
	db, err := sql.Open("sqlite", "file:"+p+"?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)")
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func proofInput(t *testing.T, round, dir, label string) planning.ChooseInput {
	start := filepath.Join(round, "start-gen.db")
	if _, err := os.Stat(filepath.Join(dir, "setup.db")); err == nil {
		start = filepath.Join(dir, "setup.db")
	}
	db := copyDB(t, start)
	var in planning.ChooseInput
	deps := testsvc.PlanningDeps(db)
	runs := newFakeRuns(t, db)
	deps.RunLog, deps.Runs = runs, runs
	deps.Go = func(f func()) { f() }
	deps.Choosers = map[airun.Chooser]planning.Chooser{airun.ChooserHeuristic: captureChooser{&in}}
	svc := planning.NewService(deps)
	req := planning.Request{
		Start: time.Date(2026, 11, 21, 0, 0, 0, 0, time.UTC), Today: time.Date(2026, 11, 17, 0, 0, 0, 0, time.UTC),
		Instruction: proofInstruction[label],
	}
	if label == "guests" {
		req.EatersByDate = map[string]float64{"2026-11-22": 3}
	}
	_, err := svc.Generate(context.Background(), req)
	require.NoError(t, err)
	svc.Wait()
	require.NotEmpty(t, in.Target.Cells, "chooser was not reached")
	return in
}

// firstEvaluate is the input of the model's first evaluate_plan call.
func firstEvaluate(t *testing.T, dir string) ([]json.RawMessage, int) {
	db := copyDB(t, filepath.Join(dir, "run.db"))
	var rid int64
	require.NoError(t, db.QueryRow(`SELECT max(id) FROM ai_runs WHERE kind='generate'`).Scan(&rid))
	var stored int
	require.NoError(t, db.QueryRow(`SELECT length(result_json) FROM ai_run_steps WHERE run_id=? AND name='plan.evaluation' ORDER BY seq LIMIT 1`, rid).Scan(&stored))
	var calls []json.RawMessage
	rows, err := db.Query(`SELECT response_json FROM ai_run_steps WHERE run_id=? AND type='llm_call' ORDER BY seq`, rid)
	require.NoError(t, err)
	defer rows.Close()
	for rows.Next() {
		var raw string
		require.NoError(t, rows.Scan(&raw))
		var r struct {
			Content []struct {
				Type, Name string
				Input      json.RawMessage `json:"tool_use_input"`
				N          string          `json:"tool_use_name"`
			}
		}
		if json.Unmarshal([]byte(raw), &r) != nil {
			continue
		}
		for _, b := range r.Content {
			if b.Type == "tool_use" && (b.N == "evaluate_plan" || b.N == "submit_plan") {
				calls = append(calls, b.Input)
			}
		}
	}
	require.NotEmpty(t, calls, "no evaluate_plan call stored")
	return calls, stored
}

type openItem struct {
	Rule, Code, Severity string
	Options              int
}

func proofOne(t *testing.T, round, dir, label string) {
	in := proofInput(t, round, dir, label)
	calls, stored := firstEvaluate(t, dir)
	raw, ordinal := calls[0], 1
	for i, c := range calls {
		// A first call that is a partial draft (cells without a plate) is not a draft yet.
		v, _, err := planning.PlannerView(in, c, false)
		require.NoError(t, err)
		b, _ := json.Marshal(v)
		if !strings.Contains(string(b), `"missing_plate"`) {
			raw, ordinal = c, i+1
			break
		}
	}
	before, err := planning.LegacyView(in, raw)
	require.NoError(t, err)
	after, done, err := planning.PlannerView(in, raw, false)
	require.NoError(t, err)
	bb, _ := json.Marshal(before)
	ab, _ := json.Marshal(after)
	var items []string
	act := 0
	if is, ok := after["issues"].([]any); ok {
		_ = is
	}
	var parsed struct {
		Issues []struct {
			Rule     string           `json:"rule"`
			Code     string           `json:"code"`
			Severity string           `json:"severity"`
			Date     string           `json:"date"`
			Options  []map[string]any `json:"options"`
		} `json:"issues"`
		Levers  []string `json:"open_levers"`
		Repeats any      `json:"repeats"`
		Changes []string `json:"code_changes"`
	}
	require.NoError(t, json.Unmarshal(ab, &parsed))
	for _, is := range parsed.Issues {
		items = append(items, fmt.Sprintf("%s/%s/%s%s opts=%d", is.Rule, is.Code, is.Severity, map[bool]string{true: "@" + is.Date, false: ""}[is.Date != ""], len(is.Options)))
		if is.Severity != "soft" || len(is.Options) > 0 {
			act++
		}
	}
	act += len(parsed.Levers)
	sort.Strings(items)
	fmt.Printf("PROOF %s (call #%d): before=%d chars (stored first eval %d) after=%d chars; must-act=%d\n  issues: %s\n  levers: %v\n  repeats: %v\n  code_changes: %d\n",
		label, ordinal, len(bb), stored, len(ab), act, strings.Join(items, "; "), parsed.Levers, parsed.Repeats, len(parsed.Changes))
	if again, _, err := planning.PlannerView(in, raw, true); err == nil {
		ab2, _ := json.Marshal(again)
		var v2 struct {
			Issues  []struct{ Rule, Code, Severity string }
			Levers  []string `json:"open_levers"`
			Accept  string   `json:"accepted_shortfall"`
			Changes []string `json:"code_changes"`
		}
		_ = json.Unmarshal(ab2, &v2)
		fmt.Printf("  second evaluation: issues=%d levers=%d accepted=%q\n", len(v2.Issues), len(v2.Levers), v2.Accept)
	}
	{
		words := regexp.MustCompile(`(?i)\b(bands?|floors?|caps?|hearty|occasions?|add-ons?|soft|hard|sessions?|violations?)\b`)
		var v any
		_ = json.Unmarshal(planning.KitchenJSON(ab), &v)
		counts := map[string]int{}
		var walk func(v any)
		walk = func(v any) {
			switch x := v.(type) {
			case string:
				if strings.Contains(x, " ") {
					for _, m := range words.FindAllString(x, -1) {
						counts[strings.ToLower(m)]++
					}
				}
			case []any:
				for _, e := range x {
					walk(e)
				}
			case map[string]any:
				for _, e := range x {
					walk(e)
				}
			}
		}
		walk(v)
		fmt.Printf("  WORDS in the evaluate_plan result prose: %v\n", counts)
	}
	if h, ok := after["variety_hints"]; ok {
		fmt.Printf("  VARIETY: %v\n", h)
	}
	if h, ok := after["cooking_hints"]; ok {
		fmt.Printf("  HINTS: %v\n", h)
	}
	if os.Getenv("PLANTRY_PROOF_DUMP") != "" {
		var v struct {
			Days []map[string]any `json:"days"`
		}
		_ = json.Unmarshal(ab, &v)
		for _, d := range v.Days {
			fmt.Printf("  day %v kcal=%v target=%v band=%v-%v cook=%v\n", d["date"], d["kcal"], d["target_kcal"], d["band_low"], d["band_high"], d["cook_occasions"])
		}
		for _, c := range parsed.Changes {
			fmt.Println("  change:", c)
		}
		for _, pl := range done.Plates {
			var cs []string
			for _, c := range pl.Components {
				cs = append(cs, fmt.Sprintf("%d x%.2f g%.0f", c.FoodID, c.Portions, c.Grams))
			}
			fmt.Printf("  plate %v/%v: %s\n", pl.Date.Format("01-02"), pl.SlotID, strings.Join(cs, ", "))
		}
	}
	_ = reflect.TypeOf
}

// replayChooser answers a run with a proposal taken from a stored run.
type replayChooser struct {
	raw json.RawMessage
	in  *planning.ChooseInput
}

func (r replayChooser) Choose(_ context.Context, in planning.ChooseInput) (planning.Proposal, error) {
	*r.in = in
	return planning.ParseForProof(r.raw, in)
}

// savedNotes runs the service with the model's last stored plan and returns the saved note.
func savedNotes(t *testing.T, round, dir, label string) string {
	start := filepath.Join(round, "start-gen.db")
	if _, err := os.Stat(filepath.Join(dir, "setup.db")); err == nil {
		start = filepath.Join(dir, "setup.db")
	}
	db := copyDB(t, start)
	calls, _ := firstEvaluate(t, dir)
	var in planning.ChooseInput
	deps := testsvc.PlanningDeps(db)
	runs := newFakeRuns(t, db)
	deps.RunLog, deps.Runs = runs, runs
	deps.Go = func(f func()) { f() }
	deps.Choosers = map[airun.Chooser]planning.Chooser{airun.ChooserHeuristic: planning.Heuristic{}, airun.ChooserLLM: replayChooser{calls[len(calls)-1], &in}}
	svc := planning.NewService(deps)
	req := planning.Request{
		Start: time.Date(2026, 11, 21, 0, 0, 0, 0, time.UTC), Today: time.Date(2026, 11, 17, 0, 0, 0, 0, time.UTC),
		Instruction: proofInstruction[label], Chooser: airun.ChooserLLM,
	}
	if label == "guests" {
		req.EatersByDate = map[string]float64{"2026-11-22": 3}
	}
	id, err := svc.Generate(context.Background(), req)
	require.NoError(t, err)
	svc.Wait()
	return runs.end(id).Notes
}

func TestE028_SavedNotes(t *testing.T) {
	round := os.Getenv("PLANTRY_PROOF_ROUND")
	if round == "" {
		t.Skip("PLANTRY_PROOF_ROUND not set")
	}
	for _, label := range strings.Split(os.Getenv("PLANTRY_PROOF_ONLY"), ",") {
		if label == "" {
			continue
		}
		dir := filepath.Join(round, "runs", "gen-"+label+"-r1")
		fmt.Printf("NOTE %s:\n%s\n", label, savedNotes(t, round, dir, label))
	}
}

func TestPromptCost(t *testing.T) {
	round := os.Getenv("PLANTRY_PROOF_ROUND")
	if round == "" {
		t.Skip("PLANTRY_PROOF_ROUND not set")
	}
	dir := filepath.Join(round, "runs", "gen-plain-r1")
	in := proofInput(t, round, dir, "plain")
	for _, r := range planning.PromptSections(in) {
		fmt.Printf("PROMPT %-40v %v\n", r[0], r[1])
	}
	cols := planning.CandidateColumns(in)
	keys := make([]string, 0, len(cols))
	for k := range cols {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return cols[keys[i]] > cols[keys[j]] })
	for _, k := range keys {
		fmt.Printf("COLUMN %-20s %d\n", k, cols[k])
	}
}

func sidesOf(in planning.ChooseInput, pl planning.ProposedPlate) string {
	main := int64(0)
	for _, c := range pl.Components {
		if f := in.State.Foods[c.FoodID]; f.Composed() && f.Role == "main" {
			main = c.FoodID
			break
		}
	}
	var out []string
	for _, c := range pl.Components {
		if c.FoodID == main {
			continue
		}
		f := in.State.Foods[c.FoodID]
		q := fmt.Sprintf("%.0f g", c.Grams)
		if c.Grams == 0 {
			q = fmt.Sprintf("x%.2g", c.Portions)
		}
		out = append(out, fmt.Sprintf("%s %s", f.Name, q))
	}
	return strings.Join(out, ", ")
}

// TestE034_Sides prints, per plate, the sides the balancing pass chose with and without the pairing rules.
func TestE034_Sides(t *testing.T) {
	round := os.Getenv("PLANTRY_PROOF_ROUND")
	if round == "" {
		t.Skip("PLANTRY_PROOF_ROUND not set")
	}
	for _, label := range strings.Split(os.Getenv("PLANTRY_PROOF_ONLY"), ",") {
		dir := filepath.Join(round, "runs", "gen-"+label+"-r1")
		in := proofInput(t, round, dir, label)
		calls, _ := firstEvaluate(t, dir)
		raw := calls[0]
		old := planning.SetPairInPlanner(false)
		_, before, err := planning.PlannerView(in, raw, true)
		require.NoError(t, err)
		planning.SetPairInPlanner(true)
		_, after, err := planning.PlannerView(in, raw, true)
		require.NoError(t, err)
		planning.SetPairInPlanner(old)
		for i := range after.Plates {
			b, a := sidesOf(in, before.Plates[i]), sidesOf(in, after.Plates[i])
			if a != b {
				fmt.Printf("SIDES %s %s %s/%d:\n   before: %s\n   after:  %s\n", label, after.Plates[i].Date.Format("Mon 02"), "slot", after.Plates[i].SlotID, b, a)
			}
		}
	}
}

// TestE038_LightMeals shows, per light meal, its kcal in the model's draft and after the finishing step.
func TestE038_LightMeals(t *testing.T) {
	round := os.Getenv("PLANTRY_PROOF_ROUND")
	if round == "" {
		t.Skip("PLANTRY_PROOF_ROUND not set")
	}
	dir := filepath.Join(round, "runs", "gen-light-r1")
	in := proofInput(t, round, dir, "light")
	calls, _ := firstEvaluate(t, dir)
	for k, raw := range []json.RawMessage{calls[0], calls[len(calls)-1]} {
		orig, err := planning.ParseForProof(raw, in)
		require.NoError(t, err)
		final, _ := planning.FinishProposal(in, orig)
		evB := planning.Evaluate(planning.Merge(in.State, in.Target, orig), in.Target)
		evA := planning.Evaluate(planning.Merge(in.State, in.Target, final), in.Target)
		find := func(p planning.Proposal, date string, slot int64) planning.ProposedPlate {
			for _, pl := range p.Plates {
				if pl.Date.Format(time.DateOnly) == date && pl.SlotID == slot {
					return pl
				}
			}
			return planning.ProposedPlate{}
		}
		for i, pa := range evA.Plates {
			if pa.Intent == "" {
				continue
			}
			var pb planning.PlateEval
			for _, e := range evB.Plates {
				if e.Date == pa.Date && e.SlotID == pa.SlotID {
					pb = e
				}
			}
			_ = i
			fmt.Printf("LIGHT call%d %s slot %d: raw %.0f -> final %.0f (max %.0f)\n   raw:   %s\n   final: %s\n", k, pa.Date, pa.SlotID, pb.Kcal, pa.Kcal, pa.MaxKcal, sidesOfAll(in, find(orig, pa.Date, pa.SlotID)), sidesOfAll(in, find(final, pa.Date, pa.SlotID)))
		}
	}
}

func sidesOfAll(in planning.ChooseInput, pl planning.ProposedPlate) string {
	var out []string
	for _, c := range pl.Components {
		f := in.State.Foods[c.FoodID]
		q := fmt.Sprintf("%.0fg", c.Grams)
		if c.Grams == 0 {
			q = fmt.Sprintf("x%.2g", c.Portions)
		}
		out = append(out, f.Name+" "+q)
	}
	return strings.Join(out, ", ")
}
