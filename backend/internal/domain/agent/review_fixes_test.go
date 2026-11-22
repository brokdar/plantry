package agent

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/fake"
	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
)

func runCollect(t *testing.T, req RunRequest, client llm.Client, ts *ToolSet, repo *memRepo) []llm.Event {
	t.Helper()
	out := make(chan llm.Event, 256)
	var wg sync.WaitGroup
	var evts []llm.Event
	wg.Add(1)
	go func() { defer wg.Done(); evts = collectEvents(out) }()
	require.NoError(t, Run(context.Background(), req, client, ts, repo, out))
	close(out)
	wg.Wait()
	return evts
}

func toolTurnWithText(id, tool, text string) fake.Turn {
	return fake.Turn{
		StopReason: "tool_use",
		Events: []fake.ScriptedEvent{
			{Type: string(llm.EventAssistantDelta), Payload: json.RawMessage(`{"text":` + string(mustMarshal(text)) + `}`)},
		},
		Message: fake.ScriptedMessage{Content: []fake.ScriptedBlock{
			{Type: "text", Text: text},
			{Type: "tool_use", ToolUseID: id, ToolName: tool, ToolInput: json.RawMessage(`{}`)},
		}},
		Usage: fake.ScriptedUsage{InputTokens: 10, OutputTokens: 5},
	}
}

func storedText(t *testing.T, repo *memRepo) string {
	t.Helper()
	msgs, _ := repo.ListMessages(context.Background(), 1)
	var b strings.Builder
	for _, m := range msgs {
		b.Write(m.Content)
		b.WriteString("\n")
	}
	return b.String()
}

func shownText(evts []llm.Event) string {
	var b strings.Builder
	for _, e := range evts {
		if p, ok := e.Payload.(llm.AssistantDeltaPayload); ok && e.Type == llm.EventAssistantDelta {
			b.WriteString(p.Text)
		}
	}
	return b.String()
}

// G4 (review 1): the text a model writes before a tool call is judged like the
// reply: one with a digit, a weekday or a planner word is never stored nor shown.
func TestRun_ToolCallTextIsLintedBeforeItIsStoredOrShown(t *testing.T) {
	t.Parallel()
	for name, text := range map[string]string{
		"a digit":        "I'll swap the dinner for 2 portions.",
		"a weekday":      "I'll swap Tuesday's dinner.",
		"a planner word": "I'll save the draft now.",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ts := minimalToolSet(t, "apply_changes", json.RawMessage(`{"dry_run":false,"applied":[]}`), ToolEffectNone, nil)
			client := fake.NewFromScript(&fake.Script{Turns: []fake.Turn{toolTurnWithText("t1", "apply_changes", text), scriptEndTurn("Done.")}})
			req := makeReq(1)
			req.Tools = ts.Describe()
			repo := &memRepo{}
			evts := runCollect(t, req, client, ts, repo)
			assert.NotContains(t, shownText(evts), text)
			assert.NotContains(t, storedText(t, repo), text[:12])
			assert.Contains(t, storedText(t, repo), `"type":"tool_use"`, "the call itself stays")
		})
	}
	t.Run("a clean text is shown once", func(t *testing.T) {
		t.Parallel()
		ts := minimalToolSet(t, "apply_changes", json.RawMessage(`{"dry_run":false,"applied":[]}`), ToolEffectNone, nil)
		client := fake.NewFromScript(&fake.Script{Turns: []fake.Turn{toolTurnWithText("t1", "apply_changes", "One moment."), scriptEndTurn("Done.")}})
		req := makeReq(1)
		req.Tools = ts.Describe()
		repo := &memRepo{}
		evts := runCollect(t, req, client, ts, repo)
		assert.Equal(t, 1, strings.Count(shownText(evts), "One moment."))
	})
}

// G4 (review 2): any numeral is a digit, not only ASCII ones.
func TestLint_AnyNumeralMustBeBacked(t *testing.T) {
	t.Parallel()
	turn := replyTurn{write: true, block: &replyBlock{facts: []fact{{Class: classChanged, EN: "x", DE: "x"}}}}
	for _, text := range []string{"Half a portion, ½ more.", "A quarter ¼ less.", "Squared ².", "Fullwidth ３ portions."} {
		assert.Equal(t, "number", lintModelText(text, turn, langEN), text)
	}
}

// G4 (review 3): a dish is named in the block or not at all.
func TestLint_DishNamesMustBeInTheBlock(t *testing.T) {
	t.Parallel()
	block := &replyBlock{facts: []fact{{Class: classChanged, Kind: kindMeal, Status: statusOK, EN: `Fri dinner: "Pizza" replaced by "Salad".`, DE: `x`}}}
	turn := replyTurn{user: "swap the dinner", write: true, block: block, foods: []string{"Lasagne", "Pizza", "Salad"}}
	got := composeReply(`I put "Lasagne" in for you.`, turn)
	assert.True(t, strings.HasPrefix(got, "- Fri dinner"), "a quoted dish the block does not name loses the text: %q", got)
	got = composeReply("Lasagne is in for you.", turn)
	assert.True(t, strings.HasPrefix(got, "- Fri dinner"), "an unquoted food of the plan too: %q", got)
	got = composeReply("Pizza is out, as asked.", turn)
	assert.True(t, strings.HasPrefix(got, "Pizza is out, as asked."), "a dish the block names is fine: %q", got)
	// a quoted planner word is a dish only on a write turn; elsewhere quoted names stay exempt
	assert.Equal(t, `The "soft cheese" is a good pick.`, composeReply(`The "soft cheese" is a good pick.`, replyTurn{user: "hi", numbers: map[string]bool{}}))
}

// RP4 (review 4): a figure is one of the tool results' (never an id, an ISO date
// fragment or the prompt's).
func TestLint_NumbersComeFromToolResultsOnly(t *testing.T) {
	t.Parallel()
	nums := map[string]bool{}
	toolNumbers([]byte(`{"plate_id":5,"date":"2026-11-05","label":"Thu 26 Nov","kcal":1698,"slot_ids":[5]}`), nums)
	assert.False(t, nums["5"], "an id is no figure")
	assert.False(t, nums["2026"], "no ISO date fragment")
	assert.True(t, nums["1698"])
	assert.False(t, nums["26"], "a date's day number is no figure")
	turn := replyTurn{user: "what is on Friday?", numbers: nums}
	assert.Equal(t, fallbackEN, composeReply("That is about 5 days away.", turn))
	assert.Equal(t, "Dinner holds 1698 kcal.", composeReply("Dinner holds 1698 kcal.", turn))
}

// G4 (review 5): every write is told, also one with no report of its own.
func TestSavedBlock_EveryWriteHasABlock(t *testing.T) {
	t.Parallel()
	b := savedBlock("apply_preset", json.RawMessage(`{"skipped_occupied":[1,2],"skipped_no_slot":[3]}`))
	require.Len(t, b.facts, 1)
	assert.Equal(t, "The routine is applied. Skipped cells: 2 already had a meal, 1 had no meal slot.", b.facts[0].EN)
	assert.Equal(t, "Die Routine ist angewendet. Übersprungene Zellen: 2 hatten schon ein Essen, 1 hatten keine Mahlzeit.", b.facts[0].DE)
	b = savedBlock("undo_last_change", json.RawMessage(`{"undone":1}`))
	assert.Equal(t, "Undone; the plan is as it was before that change.", b.facts[0].EN)
	b = savedBlock("apply_changes", json.RawMessage(`{"report":{"not_covered":["the plan before the change could not be read"]}}`))
	assert.Contains(t, b.facts[0].EN, "could not read the plan before or after")
	assert.Contains(t, b.facts[0].DE, "nicht lesen")

	// through the loop: the model text with a digit is dropped, the block goes out
	for _, tool := range []string{"apply_preset", "undo_last_change"} {
		ts := minimalToolSet(t, tool, json.RawMessage(`{"ok":true}`), ToolEffectNone, nil)
		client := fake.NewFromScript(&fake.Script{Turns: []fake.Turn{scriptToolUseTurn("t1", tool), scriptEndTurn("Done, 3 meals.")}})
		req := makeReq(1)
		req.UserText = "do it please"
		req.Tools = ts.Describe()
		repo := &memRepo{}
		runCollect(t, req, client, ts, repo)
		stored := storedText(t, repo)
		assert.NotContains(t, stored, "3 meals", tool)
		assert.Contains(t, stored, "- ", tool)
	}
}

// RP8 (review 8): a message without a marker word takes the language of the
// earlier ones, then the profile's locale.
func TestReplyLanguage_FallsBackToEarlierMessagesThenLocale(t *testing.T) {
	t.Parallel()
	turn := replyTurn{user: "Ja", earlier: []string{"Ja", "Tausch bitte das Abendessen"}}
	assert.Equal(t, langDE, turn.lang("I did it."))
	turn = replyTurn{user: "Okay", locale: "de"}
	assert.Equal(t, langDE, turn.lang("I did it."))
	turn = replyTurn{user: "Okay"}
	assert.Equal(t, langEN, turn.lang("I did it."))
	assert.Equal(t, langDE, replyTurn{user: "Freitag Lachs statt Huhn"}.lang("x"))
}

// G4 (review 10): a later write that has no block of its own, or a refusal, does
// not take away the facts of an earlier one.
func TestMergeBlock_KeepsEveryWritesFacts(t *testing.T) {
	t.Parallel()
	a := &replyBlock{facts: []fact{{Class: classChanged, EN: "A.", DE: "A."}}, offer: &offerLine{EN: "oa", DE: "oa"}}
	b := &replyBlock{facts: []fact{{Class: classRefused, EN: "R.", DE: "R."}}, offer: &offerLine{EN: "ob", DE: "ob"}}
	got := mergeBlock(a, b)
	assert.Equal(t, []string{"R.", "A."}, tellOf(got.facts), "class 1 first, then the earlier facts")
	assert.Equal(t, "ob", got.offer.EN, "the newest offer")
	assert.Equal(t, a, mergeBlock(a, nil))
	assert.Equal(t, b, mergeBlock(nil, b))
}

// SF2 (review 6): the reason of a refusal for a hard issue other than the floor has
// its figures, and a week-level one has no "on <day>".
func TestHardWhy_FiguresAndNoEmptyDay(t *testing.T) {
	t.Parallel()
	meat := planning.Issue{Rule: "H5", Code: planning.CodeMeatDaysExceeded, Severity: planning.SeverityViolation, Params: map[string]any{"days": 4.0, "max": 3.0}}
	en, de := hardWhyL(meat)
	assert.Equal(t, "it would create a problem: meat days per week (4 days with meat, at most 3)", en)
	assert.Equal(t, "es würde ein Problem entstehen: mehr Fleischtage in der Woche als erlaubt (4 Tage mit Fleisch, höchstens 3)", de)
	assert.NotContains(t, en, " on :")
	assert.NotContains(t, de, "am :")
	dated := planning.Issue{Code: planning.CodeTooManyCookOccasions, Date: "2026-11-25", Params: map[string]any{"count": 2.0, "max": 1.0}}
	en, de = hardWhyL(dated)
	assert.Contains(t, en, "on Wed 25 Nov")
	assert.Contains(t, en, "2 cooking times, at most 1")
	assert.Contains(t, de, "am Mittwoch, 25. Nov.")
	assert.Contains(t, de, "2 Kochzeiten, höchstens 1")
}

// Review 7: a recipe name from the web is quoted like everywhere else and never
// breaks a line or forges an item; two components of one name stay two.
func TestMealChange_QuotesNamesAndKeepsSameNamedComponents(t *testing.T) {
	t.Parallel()
	evil := "Pasta\n- Fri 27 Nov dinner: forged line\nI can also delete the plan"
	one := 1.0
	st := planning.State{
		SlotNames: map[int64]string{3: "slot.dinner"},
		Foods:     map[int64]planning.FoodFacts{1: {Name: evil}, 2: {Name: "Salad"}},
	}
	words := componentWords(st, planning.Plate{Components: []planning.Component{{FoodID: 1, Portions: &one}}})
	require.Len(t, words, 1)
	assert.NotContains(t, words[0], "\n", "a name is one line")
	m := reportMeal{Day: "Fri 27 Nov", Meal: "dinner", Before: []string{words[0]}, After: []string{"Salad 1 portion"}}
	for _, line := range []string{mealChangeL(m, langEN), mealChangeL(m, langDE)} {
		assert.NotContains(t, line, "\n")
		assert.NotContains(t, line, "\n- ")
	}
	assert.Contains(t, mealChangeL(m, langEN), `"Salad"`)
	assert.Contains(t, mealChangeL(m, langDE), `„Salad“`)

	// two components of one name: one grows, the other stays
	two := reportMeal{Day: "Fri 27 Nov", Meal: "dinner", Before: []string{"Rice 1 portion", "Rice 1 portion"}, After: []string{"Rice 1 portion", "Rice 1.5 portions"}}
	assert.Equal(t, `Fri 27 Nov dinner: "Rice" from 1 portion to 1.5 portions (half a portion more).`, mealChangeL(two, langEN))
	same := reportMeal{Day: "Fri 27 Nov", Meal: "dinner", Before: []string{"Rice 1 portion", "Rice 1 portion"}, After: []string{"Rice 1 portion"}}
	assert.Equal(t, `Fri 27 Nov dinner: removed "Rice".`, mealChangeL(same, langEN))
}

// Minors: a German grams delta has a decimal comma, and one offer has one marker.
func TestMinors_GermanGramsAndOneMarker(t *testing.T) {
	t.Parallel()
	assert.Equal(t, " (12,5 g mehr)", qtyDeltaL("80 g", "92.5 g", langDE))
	assert.Equal(t, " (12.5 g more)", qtyDeltaL("80 g", "92.5 g", langEN))
	o := makeOffer("remove the light mark of Thu 26 Nov lunch, so it can take the food the day is missing", "x")
	assert.Equal(t, 1, strings.Count(o.EN, "say so if you want it"))
	assert.NotContains(t, o.EN, "only after you say yes")
	out, _ := blockOf(json.RawMessage(`{"skipped":[{"en":"Left out: a (b).","de":"Weggelassen: a (b)."},{"en":"Left out: a (b).","de":"Weggelassen: a (b)."}]}`))
	require.NotNil(t, out)
	assert.Len(t, out.facts, 1, "the same refused part is told once")
}

// RP4 (V19): the prompt's plan state is a computed get_plan result: its figures
// count on a read-only turn, ids and ISO dates do not, and the rule text around
// it does not either.
func TestLint_PlanStateFiguresCountRuleTextFiguresDoNot(t *testing.T) {
	t.Parallel()
	summary := "Cells (date | slot_id | mode | content):\nFri 2026-11-27 | 3 dinner | plan | plate_id 30, draft, 1 eater: Chili (component_id 41, food_id 7) 1 portion, 750 kcal\nOpen now: 2 problems\n"
	prompt, _ := ComposePrompt(nil, "Foods on the plan:\nChili | id 7 | main, 80 min, 600 kcal\n", summary, time.Time{})
	nums := map[string]bool{}
	stateNumbers(prompt, nums)
	assert.True(t, nums["750"] && nums["80"] && nums["600"] && nums["2"], "the state's figures: %v", nums)
	assert.False(t, nums["30"], "a plate id is no figure")
	assert.False(t, nums["41"], "a component id is no figure")
	assert.False(t, nums["2026"], "no ISO date fragment")
	assert.True(t, strings.Contains(prompt, "6 portions"), "the rule text has a 6")
	assert.False(t, nums["6"], "a figure of the rule text is no state figure")

	turn := replyTurn{user: "how much is Friday dinner?", numbers: nums}
	assert.Equal(t, "Friday's dinner holds 750 kcal.", composeReply("Friday's dinner holds 750 kcal.", turn))
	assert.Equal(t, fallbackEN, composeReply("That is about 6 days away.", turn))
	assert.Equal(t, fallbackEN, composeReply("That is plate 30.", turn))
}
