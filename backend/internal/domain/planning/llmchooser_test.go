package planning_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/fake"
	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/airun"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper/testsvc"
)

// The LLM chooser tests run the whole pipeline over SQLite (the batch
// cooker fixture of service_test.go) with the real run recorder and the
// metered fake client, so the ai_runs row and its steps are the ones
// production writes.

type llmEnv struct {
	*svcEnv
	client *fake.Client
	// script is the fake client's script: set its turns before a run.
	script *fake.Script
}

func newLLMEnv(t *testing.T) *llmEnv {
	t.Helper()
	script := &fake.Script{}
	client := fake.NewFromScript(script)
	e := newSvcEnv(t)
	repo := sqlite.NewAIRunRepo(e.db)
	rec := airun.NewRecorder(repo, slog.New(slog.DiscardHandler), time.Now)
	t.Cleanup(rec.Wait)
	e.deps.RunLog, e.deps.Runs = planning.RecorderLog{Recorder: rec}, repo
	e.deps.Choosers = map[airun.Chooser]planning.Chooser{
		airun.ChooserHeuristic: planning.Heuristic{},
		airun.ChooserLLM:       planning.NewLLMChooser(llm.StaticResolver(rec.Meter(client, "fake"), "fake-model")),
	}
	e.svc = planning.NewService(e.deps)
	return &llmEnv{svcEnv: e, client: client, script: script}
}

// generate runs a window generation with the default chooser.
func (e *llmEnv) gen(turns ...fake.Turn) int64 {
	e.t.Helper()
	e.script.Turns = turns
	return e.generate(e.req())
}

// runRow is the ai_runs row of a generation.
type runRow struct {
	Status, Chooser, PromptVersion, ErrorKey, Violations, Notes string
	RepairAttempts, InputTokens, OutputTokens, LLMSteps         int
}

func (e *llmEnv) run(id int64) runRow {
	e.t.Helper()
	var r runRow
	err := e.db.QueryRow(`SELECT status, COALESCE(chooser, ''), COALESCE(prompt_version, ''), COALESCE(error_key, ''),
		COALESCE(violations_json, ''), COALESCE(notes, ''), COALESCE(repair_attempts, 0), input_tokens, output_tokens,
		(SELECT COUNT(*) FROM ai_run_steps WHERE run_id = ai_runs.id AND type = 'llm_call')
		FROM ai_runs WHERE id = ?`, id).Scan(&r.Status, &r.Chooser, &r.PromptVersion, &r.ErrorKey, &r.Violations, &r.Notes,
		&r.RepairAttempts, &r.InputTokens, &r.OutputTokens, &r.LLMSteps)
	require.NoError(e.t, err)
	return r
}

// rows counts the plates and sessions run id wrote.
func (e *llmEnv) rows(id int64) (plates, sessions int) {
	e.t.Helper()
	require.NoError(e.t, e.db.QueryRow(`SELECT (SELECT COUNT(*) FROM plates WHERE ai_run_id = ?), (SELECT COUNT(*) FROM cook_sessions WHERE ai_run_id = ?)`, id, id).Scan(&plates, &sessions))
	return plates, sessions
}

// toolResult is the text of the tool result a repair request ends with.
func toolResult(t *testing.T, req llm.Request) string {
	t.Helper()
	last := req.Messages[len(req.Messages)-1]
	require.Equal(t, llm.RoleUser, last.Role)
	require.Len(t, last.Content, 1)
	b := last.Content[0]
	require.Equal(t, llm.ContentTypeToolResult, b.Type)
	assert.True(t, b.ToolResultIsError)
	var s string
	require.NoError(t, json.Unmarshal(b.ToolResultContent, &s))
	return s
}

// submit is a fake turn calling submit_plan with input.
func submit(t *testing.T, id string, input any) fake.Turn {
	t.Helper()
	raw, err := json.Marshal(input)
	require.NoError(t, err)
	return submitRaw(id, raw)
}

func submitRaw(id string, raw json.RawMessage) fake.Turn {
	return fake.Turn{
		StopReason: "tool_use",
		Message:    fake.ScriptedMessage{Content: []fake.ScriptedBlock{{Type: "tool_use", ToolUseID: id, ToolName: "submit_plan", ToolInput: raw}}},
		Usage:      fake.ScriptedUsage{InputTokens: 1000, OutputTokens: 200},
	}
}

type jsonPlan struct {
	Sessions []map[string]any `json:"sessions"`
	Plates   []map[string]any `json:"plates"`
	Notes    string           `json:"notes"`
}

func (p *jsonPlan) session(key string, foodID int64, date string, slot int64, portions float64) {
	p.Sessions = append(p.Sessions, map[string]any{
		"key": key, "food_id": foodID, "cook_date": date, "cook_slot_id": slot, "portions": portions, "reason": "Cooked for " + date,
	})
}

func (p *jsonPlan) plate(date string, slot, foodID int64, key string) {
	p.Plates = append(p.Plates, map[string]any{
		"date": date, "slot_id": slot, "reason": fmt.Sprintf("Plate %s/%d", date, slot),
		"components": []map[string]any{{"food_id": foodID, "portions": 1, "session_key": key}},
	})
}

// validPlan fills the batch cooker's 16 plan cells without a violation: one
// batch a day of a main not eaten recently, cooked at the day's first plan
// cell (Sat and Sun breakfast, Mon–Fri lunch; the weekday breakfasts are
// routine).
func (e *llmEnv) validPlan() *jsonPlan {
	p := &jsonPlan{Notes: "One batch a day."}
	for i := range 7 {
		date := e.start.AddDate(0, 0, i).Format(time.DateOnly)
		key, main := fmt.Sprintf("s%d", i), e.mains[3+i]
		slots := []int64{e.lunch, e.dinner}
		if i < 2 {
			slots = []int64{e.breakfast, e.lunch, e.dinner}
		}
		p.session(key, main, date, slots[0], float64(len(slots)))
		for _, sl := range slots {
			p.plate(date, sl, main, key)
		}
	}
	return p
}

// h2Plan is validPlan with Sunday split into two cooking occasions: the
// main at breakfast and a side cooked at lunch for lunch and dinner.
func (e *llmEnv) h2Plan() *jsonPlan {
	p := e.validPlan()
	p.Notes = "Two occasions on Sunday."
	p.Sessions[1]["portions"] = 1
	p.session("side", e.sides[0], sun, e.lunch, 2)
	for _, pl := range p.Plates {
		if pl["date"] == sun && pl["slot_id"] != e.breakfast {
			pl["components"] = []map[string]any{{"food_id": e.sides[0], "portions": 1, "session_key": "side"}}
		}
	}
	return p
}

// unknownFoodPlan is validPlan with Saturday's batch of a food that does
// not exist.
func (e *llmEnv) unknownFoodPlan() *jsonPlan {
	p := e.validPlan()
	p.Sessions[0]["food_id"] = 999999
	for _, pl := range p.Plates {
		if pl["date"] == sat {
			pl["components"] = []map[string]any{{"food_id": 999999, "portions": 1, "session_key": "s0"}}
		}
	}
	return p
}

// B7-AC1 (AC-4): the forced call's shape and what its prompt holds.
func TestLLMChooser_RequestAndPrompt(t *testing.T) {
	e := newLLMEnv(t)
	e.exec(`UPDATE user_profile SET system_prompt = 'No coriander, ever.', dietary_restrictions = '["nuts"]' WHERE id = 1`)

	id := e.gen(submit(t, "tu_1", e.validPlan()))

	r := e.run(id)
	assert.Equal(t, "ok", r.Status, "violations %s, error %s", r.Violations, r.ErrorKey)
	assert.Equal(t, "llm", r.Chooser)
	assert.Equal(t, planning.PromptVersion, r.PromptVersion)
	assert.Equal(t, 0, r.RepairAttempts)
	assert.Equal(t, 1, r.LLMSteps)
	assert.Equal(t, "One batch a day.", r.Notes)
	var reasons int
	require.NoError(t, e.db.QueryRow(`SELECT COUNT(*) FROM plates WHERE ai_run_id = ? AND reason LIKE 'Plate %'`, id).Scan(&reasons))
	assert.Equal(t, 16, reasons)

	reqs := e.client.Requests()
	require.Len(t, reqs, 1)
	req := reqs[0]
	require.NotNil(t, req.ToolChoice)
	assert.Equal(t, "submit_plan", req.ToolChoice.Name)
	assert.InDelta(t, 0.4, req.Temperature, 1e-9)
	assert.Equal(t, 16000, req.MaxTokens)
	assert.Equal(t, "fake-model", req.Model)
	require.Len(t, req.Tools, 1)
	assert.Equal(t, "submit_plan", req.Tools[0].Name)
	var schema map[string]any
	require.NoError(t, json.Unmarshal(req.Tools[0].Schema, &schema))
	assert.Equal(t, "object", schema["type"])
	for _, k := range []string{"anyOf", "oneOf", "allOf"} {
		assert.NotContains(t, schema, k)
	}

	sys := req.System
	assert.Contains(t, sys, "Today is Friday 2026-11-06.")
	for i, wd := range []string{"Saturday", "Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday"} {
		assert.Contains(t, sys, "- "+wd+" "+e.start.AddDate(0, 0, i).Format(time.DateOnly)+"\n")
	}
	cells, err := e.avail.Resolve(e.ctx, e.start, e.start.AddDate(0, 0, 6))
	require.NoError(t, err)
	plan := 0
	for _, c := range cells {
		if c.Mode == "plan" {
			plan++
			assert.Contains(t, sys, fmt.Sprintf("%s %s | %d | plan | fill\n", c.Date.Weekday(), c.Date.Format(time.DateOnly), c.SlotID))
		}
	}
	assert.Equal(t, 16, plan)
	ids := append(append([]int64{e.oats}, e.mains...), e.sides...)
	macros, err := food.NewNutritionResolver(sqlite.NewFoodRepo(e.db)).PerPortionDetailedMany(e.ctx, ids)
	require.NoError(t, err)
	for _, fid := range ids {
		row := candidateLine(t, sys, fid)
		kcal := macros[fid].Macros.Kcal
		require.Positive(t, kcal)
		assert.Equal(t, fmt.Sprintf("%.0f", kcal), strings.Split(row, "|")[4], "candidate %d", fid)
	}
	assert.Contains(t, sys, "- diet type: omnivore\n")
	assert.Contains(t, sys, "No coriander, ever.")
	assert.Contains(t, sys, "- dietary restrictions: nuts\n")
	assert.Contains(t, sys, "in English.")
}

// candidateLine returns the prompt's candidate row of food id.
func candidateLine(t *testing.T, prompt string, id int64) string {
	t.Helper()
	prefix := fmt.Sprintf("%d|", id)
	for _, l := range strings.Split(prompt, "\n") {
		if strings.HasPrefix(l, prefix) {
			return l
		}
	}
	t.Fatalf("no candidate row for %d", id)
	return ""
}

// B7-AC2: an H2 violation goes back to the model, which fixes it.
func TestLLMChooser_RepairsAnH2Violation(t *testing.T) {
	e := newLLMEnv(t)

	id := e.gen(submit(t, "tu_1", e.h2Plan()), submit(t, "tu_2", e.validPlan()))

	r := e.run(id)
	assert.Equal(t, "ok", r.Status, "violations %s", r.Violations)
	assert.JSONEq(t, `[]`, r.Violations)
	assert.Equal(t, 1, r.RepairAttempts)
	assert.Equal(t, 2, r.LLMSteps)
	assert.Equal(t, 2000, r.InputTokens)
	assert.Equal(t, 400, r.OutputTokens)
	assert.Equal(t, "One batch a day.", r.Notes)

	reqs := e.client.Requests()
	require.Len(t, reqs, 2)
	repair := reqs[1]
	require.Len(t, repair.Messages, 3)
	assert.Equal(t, llm.RoleAssistant, repair.Messages[1].Role)
	assert.Equal(t, "tu_1", repair.Messages[2].Content[0].ToolResultID)
	assert.Equal(t, reqs[0].System, repair.System)
	assert.Equal(t, reqs[0].ToolChoice, repair.ToolChoice)
	feedback := toolResult(t, repair)
	assert.Contains(t, feedback, "- H2 too_many_cook_occasions on "+sun+" count=2 max=1\n")
	assert.NotContains(t, feedback, "H8")

	// The 16 proposed plates and 7 batches plus the 5 routine breakfasts.
	plates, sessions := e.rows(id)
	assert.Equal(t, 21, plates)
	assert.Equal(t, 12, sessions)
}

// B7-AC3: three invalid proposals: the last one is persisted with its
// violations, the run is partial and the board shows the issue.
func TestLLMChooser_UnresolvedViolationsArePersisted(t *testing.T) {
	e := newLLMEnv(t)

	id := e.gen(submit(t, "tu_1", e.h2Plan()))

	r := e.run(id)
	assert.Equal(t, "partial", r.Status)
	assert.Equal(t, 2, r.RepairAttempts)
	assert.Equal(t, 3, r.LLMSteps)
	var vs []planning.Issue
	require.NoError(t, json.Unmarshal([]byte(r.Violations), &vs))
	require.Len(t, vs, 1)
	assert.Equal(t, "H2", vs[0].Rule)
	assert.Equal(t, sun, vs[0].Date)
	plates, _ := e.rows(id)
	assert.Equal(t, 21, plates)

	board, err := testsvc.PlanView(e.db).Plan(e.ctx, &e.start, day(svcToday))
	require.NoError(t, err)
	assert.True(t, slices.ContainsFunc(board.Issues, func(is planning.Issue) bool {
		return is.Code == planning.CodeTooManyCookOccasions && is.Date == sun
	}), "board issues %+v", board.Issues)
	require.NotNil(t, board.Draft)
	require.NotNil(t, board.Draft.Notes)
	assert.Equal(t, "Two occasions on Sunday.", *board.Draft.Notes)
}

// B7-AC4: an unknown food id is an H7 violation that is repaired.
func TestLLMChooser_RepairsAnUnknownFood(t *testing.T) {
	e := newLLMEnv(t)

	id := e.gen(submit(t, "tu_1", e.unknownFoodPlan()), submit(t, "tu_2", e.validPlan()))

	r := e.run(id)
	assert.Equal(t, "ok", r.Status, "violations %s", r.Violations)
	assert.Equal(t, 1, r.RepairAttempts)
	reqs := e.client.Requests()
	require.Len(t, reqs, 2)
	assert.Contains(t, toolResult(t, reqs[1]), "H7 unknown_food on "+sat)
	assert.Contains(t, toolResult(t, reqs[1]), "food_id=999999")
}

// B7-AC5: a provider error ends the run in error and writes nothing.
func TestLLMChooser_ProviderError(t *testing.T) {
	e := newLLMEnv(t)

	id := e.gen() // an empty script fails every call

	r := e.run(id)
	assert.Equal(t, "error", r.Status)
	assert.Equal(t, "error.ai.provider_error", r.ErrorKey)
	assert.Equal(t, 1, r.LLMSteps)
	plates, sessions := e.rows(id)
	assert.Zero(t, plates)
	assert.Zero(t, sessions)
	assert.Empty(t, e.window(), "routine breakfasts are not written either")
}

// Unreadable replies count as repair attempts.
func TestLLMChooser_UnreadableReplies(t *testing.T) {
	bad := submitRaw("tu_bad", json.RawMessage(`{"sessions": "soon", "plates": [], "notes": ""}`))

	t.Run("a parse error is repaired with its message", func(t *testing.T) {
		e := newLLMEnv(t)
		id := e.gen(bad, submit(t, "tu_2", e.validPlan()))
		r := e.run(id)
		assert.Equal(t, "ok", r.Status)
		assert.Equal(t, 1, r.RepairAttempts)
		feedback := toolResult(t, e.client.Requests()[1])
		assert.True(t, strings.HasPrefix(feedback, "error: submit_plan input: sessions must be an array, not string\n"), feedback)
	})
	t.Run("a bad date is a parse error", func(t *testing.T) {
		e := newLLMEnv(t)
		p := e.validPlan()
		p.Plates[0]["date"] = "next saturday"
		id := e.gen(submit(t, "tu_1", p), submit(t, "tu_2", e.validPlan()))
		assert.Equal(t, 1, e.run(id).RepairAttempts)
		assert.Contains(t, toolResult(t, e.client.Requests()[1]), `plates[0].date "next saturday" is not a YYYY-MM-DD date`)
	})
	t.Run("no readable reply ends the run in error", func(t *testing.T) {
		e := newLLMEnv(t)
		id := e.gen(bad)
		r := e.run(id)
		assert.Equal(t, "error", r.Status)
		assert.Equal(t, "error.ai.invalid_plan", r.ErrorKey)
		assert.Equal(t, 3, r.LLMSteps)
		plates, _ := e.rows(id)
		assert.Zero(t, plates)
	})
	t.Run("the last readable proposal is kept", func(t *testing.T) {
		e := newLLMEnv(t)
		id := e.gen(submit(t, "tu_1", e.h2Plan()), bad)
		r := e.run(id)
		assert.Equal(t, "partial", r.Status)
		assert.Equal(t, 2, r.RepairAttempts)
		assert.Equal(t, "Two occasions on Sunday.", r.Notes)
	})
}

// Repair turns report the repairing stage (D35), and Ready tells a missing
// provider from a broken one.
func TestLLMChooser_StageAndReady(t *testing.T) {
	s := baseState()
	empty := submitRaw("tu_1", json.RawMessage(`{"sessions": [], "plates": [], "notes": ""}`)) // H1 everywhere
	c := planning.NewLLMChooser(llm.StaticResolver(fake.NewFromScript(&fake.Script{Turns: []fake.Turn{empty}}), "m"))
	var stages []planning.Stage
	p, err := c.Choose(context.Background(), planning.ChooseInput{
		State: s, Target: windowTarget(s), Stage: func(st planning.Stage) { stages = append(stages, st) },
	})
	require.NoError(t, err)
	assert.Equal(t, 2, p.RepairAttempts)
	assert.Equal(t, []planning.Stage{planning.StageRepairing, planning.StageRepairing}, stages)
	assert.Equal(t, "p1", c.PromptVersion())

	require.NoError(t, c.Ready(context.Background()))
	assert.ErrorIs(t, planning.NewLLMChooser(llm.StaticResolver(nil, "")).Ready(context.Background()), domain.ErrAIProviderMissing)
	assert.ErrorIs(t, planning.NewLLMChooser(brokenResolver{}).Ready(context.Background()), domain.ErrAIProviderFailed)
}

// A repaired reply with two tool calls gets a tool result for each.
func TestLLMChooser_AnswersEveryToolCall(t *testing.T) {
	s := baseState()
	empty := json.RawMessage(`{"sessions": [], "plates": [], "notes": ""}`)
	twice := fake.Turn{StopReason: "tool_use", Message: fake.ScriptedMessage{Content: []fake.ScriptedBlock{
		{Type: "tool_use", ToolUseID: "tu_a", ToolName: "submit_plan", ToolInput: empty},
		{Type: "tool_use", ToolUseID: "tu_b", ToolName: "submit_plan", ToolInput: empty},
	}}}
	client := fake.NewFromScript(&fake.Script{Turns: []fake.Turn{twice}})
	_, err := planning.NewLLMChooser(llm.StaticResolver(client, "m")).Choose(context.Background(), planning.ChooseInput{State: s, Target: windowTarget(s)})
	require.NoError(t, err)

	reqs := client.Requests()
	require.Len(t, reqs, 3)
	last := reqs[1].Messages[len(reqs[1].Messages)-1]
	require.Len(t, last.Content, 2)
	for i, id := range []string{"tu_a", "tu_b"} {
		assert.Equal(t, llm.ContentTypeToolResult, last.Content[i].Type)
		assert.Equal(t, id, last.Content[i].ToolResultID)
		assert.True(t, last.Content[i].ToolResultIsError)
	}
}

// Names never break a pipe-separated row, and a side swap over the budget
// keeps the sides.
func TestRenderPrompt_NamesAndSideBudget(t *testing.T) {
	s := baseState()
	f := s.Foods[chili]
	f.Name = "Chili | extra\nhot"
	s.Foods[chili] = f
	s.History = []planning.HistoryPlate{{Date: day("2026-11-01"), SlotID: dinner, FoodIDs: []int64{chili}}}
	prompt := planning.RenderPrompt(planning.ChooseInput{State: s, Target: windowTarget(s)})
	assert.NotContains(t, prompt, "Chili |")
	assert.Contains(t, prompt, "2026-11-01 | slot 2 | Chili / extra hot\n")

	s, tgt, _ := benchState()
	eaten := day("2026-11-01")
	for i := range 3000 {
		id := int64(5000 + i)
		m := recipe(id, strings.Repeat("Long recipe name ", 4), food.RoleMain, "vegan", 500)
		s.Foods[id] = m
	}
	side := recipe(9999, "Late side", food.RoleSideVeg, "vegan", 100)
	side.LastEaten = &eaten
	s.Foods[side.ID] = side
	sw := &planning.SideSwap{FoodID: rice, CookDate: day(sat), CookSlotID: dinner, Portions: 2}
	prompt = planning.RenderPrompt(planning.ChooseInput{State: s, Target: tgt, Side: sw})
	assert.Contains(t, prompt, "more recipes left out for length.")
	candidateLine(t, prompt, side.ID)
	candidateLine(t, prompt, rice)
}

type brokenResolver struct{}

func (brokenResolver) Current(context.Context) (llm.Client, string, error) {
	return nil, "", errors.New("script not found")
}

// D11: without a provider the default chooser is the heuristic, and asking
// for the LLM fails.
func TestLLMChooser_DefaultFollowsTheProvider(t *testing.T) {
	e := newSvcEnv(t)
	e.deps.Choosers[airun.ChooserLLM] = planning.NewLLMChooser(llm.StaticResolver(nil, ""))
	e.svc = planning.NewService(e.deps)

	id := e.generate(e.req())
	assert.Equal(t, airun.ChooserHeuristic, e.runs.start(id).Chooser)
	assert.Equal(t, planning.HeuristicVersion, e.runs.start(id).PromptVersion)

	req := e.req()
	req.Chooser = airun.ChooserLLM
	_, err := e.svc.Generate(e.ctx, req)
	assert.ErrorIs(t, err, domain.ErrAIProviderMissing)
}

// B7-AC6: the prompt for 300 recipes stays within 40 k estimated tokens;
// beyond the budget, candidates are left out and the prompt says so.
func TestRenderPrompt_TokenBudget(t *testing.T) {
	s, tgt, _ := benchState()
	prompt := planning.RenderPrompt(planning.ChooseInput{State: s, Target: tgt})
	assert.LessOrEqual(t, planning.EstimateTokens(prompt), planning.PromptTokenBudget)
	assert.NotContains(t, prompt, "left out")
	for i := range 300 {
		candidateLine(t, prompt, int64(1000+i))
	}

	t.Run("over the budget", func(t *testing.T) {
		s, tgt, _ := benchState()
		eaten := day("2026-11-01")
		for i := range 3000 {
			id := int64(5000 + i)
			f := recipe(id, strings.Repeat("Long recipe name ", 4), food.RoleMain, "vegan", 500)
			f.LastEaten = &eaten
			f.Favorite = i == 2999
			s.Foods[id] = f
		}
		prompt := planning.RenderPrompt(planning.ChooseInput{State: s, Target: tgt})
		assert.LessOrEqual(t, planning.EstimateTokens(prompt), planning.PromptTokenBudget)
		assert.Contains(t, prompt, "more recipes left out for length.")
		candidateLine(t, prompt, 7999) // the favorite stays
		candidateLine(t, prompt, 1000) // never eaten stays
	})
}

func TestRenderPrompt_TaskOptions(t *testing.T) {
	s := baseState()
	prompt := planning.RenderPrompt(planning.ChooseInput{
		State: s, Target: windowTarget(s), Locale: "de", IncludeNewRecipe: true,
		Instruction: "Gäste am Sonntag", EatersByDate: map[string]float64{sun: 4}, Avoid: []int64{curry},
	})
	assert.Contains(t, prompt, "Write notes and every reason in German.")
	assert.Contains(t, prompt, "never eaten")
	assert.Contains(t, prompt, "The user's request for this plan: Gäste am Sonntag")
	assert.Contains(t, prompt, "- plates on 2026-11-08 serve 4 eaters\n")
	assert.NotContains(t, prompt, fmt.Sprintf("\n%d|Curry|", curry), "replaced recipes are no candidates")
	candidateLine(t, prompt, chili)
}

// generate-week.json, the e2e fixture of the tool contract: filled in, its
// first turn violates H2 only and its second is a valid week; the error
// variant fails the run with error.ai.provider_error.
func TestGenerateWeekFixture(t *testing.T) {
	dir := filepath.Join("..", "..", "..", "..", "frontend", "e2e", "fixtures", "chat-scripts")
	raw, err := os.ReadFile(filepath.Join(dir, "generate-week.json"))
	require.NoError(t, err)

	e := newLLMEnv(t)
	require.NoError(t, e.avail.SetDefaults(e.ctx, nil)) // all 21 cells plan
	extra := e.insert(`INSERT INTO foods (name, kind, role, reference_portions) VALUES ('Main 11', 'composed', 'main', 4)`)
	e.exec(`INSERT INTO food_components (parent_id, child_id, amount, unit, grams) SELECT ?, child_id, amount, unit, grams FROM food_components WHERE parent_id = ?`, extra, e.mains[3])
	recipes := append(slices.Clone(e.mains[3:10]), extra)
	pairs := []string{
		`"{{breakfast}}"`, fmt.Sprint(e.breakfast), `"{{lunch}}"`, fmt.Sprint(e.lunch), `"{{dinner}}"`, fmt.Sprint(e.dinner),
	}
	for i, id := range recipes {
		pairs = append(pairs, fmt.Sprintf(`"{{recipe_%d}}"`, i+1), fmt.Sprint(id))
	}
	for i := range 7 {
		pairs = append(pairs, fmt.Sprintf("{{d%d}}", i), e.start.AddDate(0, 0, i).Format(time.DateOnly))
	}
	filled := strings.NewReplacer(pairs...).Replace(string(raw))
	var script fake.Script
	require.NoError(t, json.Unmarshal([]byte(filled), &script))

	id := e.gen(script.Turns...)

	r := e.run(id)
	assert.Equal(t, "ok", r.Status, "violations %s", r.Violations)
	assert.Equal(t, 1, r.RepairAttempts)
	assert.NotEmpty(t, r.Notes)
	feedback := toolResult(t, e.client.Requests()[1])
	assert.Contains(t, feedback, "H2 too_many_cook_occasions on "+sun)
	assert.Equal(t, 1, strings.Count(feedback, "\n- "), feedback)
	plates, sessions := e.rows(id)
	assert.Equal(t, 21, plates)
	assert.Equal(t, 7, sessions)

	raw, err = os.ReadFile(filepath.Join(dir, "generate-week-error.json"))
	require.NoError(t, err)
	var failing fake.Script
	require.NoError(t, json.Unmarshal(raw, &failing))
	e2 := newLLMEnv(t)
	assert.Equal(t, "error.ai.provider_error", e2.run(e2.gen(failing.Turns...)).ErrorKey)
}

// A regenerate with no empty plan cell to fill makes no LLM call: the run
// ends ok, with only the routine plates when the target has routine cells.
func TestLLMChooser_NothingToPlanCallsNoLLM(t *testing.T) {
	for name, tc := range map[string]struct {
		noBreakfast bool
		plates      int
	}{
		"routine cells only": {plates: 1},
		"no cells":           {noBreakfast: true, plates: 0},
	} {
		t.Run(name, func(t *testing.T) {
			e := newLLMEnv(t)
			e.none(fri, e.lunch)
			e.none(fri, e.dinner)
			if tc.noBreakfast {
				e.none(fri, e.breakfast)
			}

			id := e.regenerate(planning.Scope{Kind: planning.ScopeEmptyCells, From: day(fri), To: day(fri)})

			r := e.run(id)
			assert.Equal(t, "ok", r.Status, r.ErrorKey)
			assert.Equal(t, "llm", r.Chooser)
			assert.Zero(t, r.LLMSteps)
			assert.Empty(t, e.client.Requests())
			plates, sessions := e.rows(id)
			assert.Equal(t, tc.plates, plates)
			assert.Equal(t, tc.plates, sessions)
		})
	}
}
