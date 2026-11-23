package agent

import (
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
)

// compose is ComposePrompt without facts, for the tests that only look at the
// summary and the rules.
func compose(p *profile.Profile, summary string, today time.Time) string {
	prompt, _ := ComposePrompt(p, "", summary, today)
	return prompt
}

func day(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

func TestPrompt_NoLegacyDayIndex(t *testing.T) {
	t.Parallel()
	if strings.Contains(compose(nil, "", time.Time{}), "0=Monday") {
		t.Error("system prompt must not contain legacy day-index notation '0=Monday'")
	}
}

func TestPrompt_CarriesSummaryBetweenProfileAndRules(t *testing.T) {
	t.Parallel()
	kcal := 2000.0
	prompt := compose(&profile.Profile{KcalTarget: &kcal}, "SUMMARY\n", day(2026, 11, 2))

	prof, sum, rules := strings.Index(prompt, "kcal_target: 2000"), strings.Index(prompt, "SUMMARY\n"), strings.Index(prompt, "Tool usage rules:")
	if prof < 0 || sum < prof || rules < sum {
		t.Errorf("want profile < summary < rules, got %d %d %d", prof, sum, rules)
	}
}

// D3: drafts, unlock, past days, regenerate vs direct tools, one summary; no
// week-copy paragraph.
func TestPrompt_ToolRules(t *testing.T) {
	t.Parallel()
	prompt := compose(nil, "", time.Time{})
	for _, want := range []string{"wait until the user accepts", "unlock", "later message", "call unlock in that same turn", "before today", "regenerate", "Reply after a change", "get_plan"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt must mention %q", want)
		}
	}
	if strings.Contains(prompt, "copy"+"_week") {
		t.Error("the week-copy paragraph is gone")
	}
}

// v16: meal edits, moves as swaps, absence as one away op, replacements via
// find_alternatives with minutes, tool limits and one undo per request.
func TestPrompt_MealEditRules(t *testing.T) {
	t.Parallel()
	prompt := compose(nil, "", time.Time{})
	for _, want := range []string{
		"change only what the user asked", "the program tells it and offers the fix", "only to clear a problem",
		`occupant "swap"`,
		"one away op", "A day away is not counted",
		"find_alternatives with what main", "Apply the recommended alternative's ops at once",
		"only do what your tools do", "logging food eaten out", "say so plainly",
		"one change the user can undo", "all ops of a request in one call",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt must mention %q", want)
		}
	}
	if strings.Contains(prompt, `"something different" or "another X" requests prefer regenerate`) {
		t.Error("replacements go through find_alternatives, not regenerate")
	}
}

// v17 and v18: consequences only from tool results, one dry-run rule, the ops
// that were dry-ran, warnings named but not blocking, the meal intent's shift.
func TestPrompt_V17Rules(t *testing.T) {
	t.Parallel()
	prompt := compose(nil, "", time.Time{})
	for _, want := range []string{
		"never infer what happens to other meals", "without a check first", "without asking \"should I save?\"",
		"dry_run true only for a what-if question", "dish repeated this week", "carry out the request",
		"moves the day's target by that meal's share",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt must mention %q", want)
		}
	}
}

// The "not done while an issue is open" rule covers hard issues only; a soft
// issue is reported with the fix offered, never applied unasked (no
// contradiction with "offer the fix" and the protein instruction).
func TestPrompt_SoftIssuesAreOffered(t *testing.T) {
	t.Parallel()
	prompt := compose(nil, "", time.Time{})
	for _, want := range []string{
		"A request is not done while a problem it caused is open",
		"is not fixed on your own: the program tells it and offers the fix",
		"nothing is applied unasked",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt must contain %q", want)
		}
	}
	// SF2: no prompt sentence lets the model write a second time; RP1: no "when in doubt".
	for _, gone := range []string{"Close protein gaps actively", "Close protein gaps by offering", "Fix a new issue with another apply_changes call", "Fix a new problem with another apply_changes call", "When in doubt, ask once"} {
		if strings.Contains(prompt, gone) {
			t.Errorf("prompt must not contain %q", gone)
		}
	}
}

// The chat prompt holds no energy policy (CC9/M2): the program says the figures.
func TestPrompt_NoEnergyPolicy(t *testing.T) {
	t.Parallel()
	kcal := 2000.0
	prompt := compose(&profile.Profile{KcalTarget: &kcal}, "", time.Time{})
	// Code owns the figures and their ranking: the prompt carries no policy text.
	for _, gone := range []string{"6 carb and fat ranges", "food-group rules (the DGE weekly food rules", "energy policy"} {
		if strings.Contains(prompt, gone) {
			t.Errorf("prompt must not contain %q", gone)
		}
	}
}

// planLines builds a summary with n cell lines per day over 31 days.
func planLines(n int, note string) string {
	var b strings.Builder
	b.WriteString("Cells (date | slot_id | mode | content):\n")
	for d := range 31 {
		for s := range n {
			fmt.Fprintf(&b, "%s | %d dinner | plan | plate_id %d, draft, 2 eaters: %s\n",
				day(2026, 10, 1+d).Format("Monday 2006-01-02"), s+1, d*n+s+1, note)
		}
	}
	return b.String()
}

func TestPrompt_OverBudgetDropsPastCellsFirst(t *testing.T) {
	t.Parallel()
	summary := planLines(4, strings.Repeat("Rindergulasch mit Spätzle ", 4))
	today := day(2026, 10, 20)

	prompt := compose(nil, summary, today)

	if len(prompt) > promptCharBudget {
		t.Errorf("prompt is %d bytes, budget is %d", len(prompt), promptCharBudget)
	}
	if !strings.HasSuffix(prompt, plainWords(toolRules)) {
		t.Error("an over-budget prompt must keep the tool rules verbatim")
	}
	if strings.Contains(prompt, "Thursday 2026-10-01 |") {
		t.Error("cells before today go first")
	}
	if !strings.Contains(prompt, "cells before today left out for length") {
		t.Error("the prompt must say that past cells were left out")
	}
	if !strings.Contains(prompt, "2026-10-31 |") {
		t.Errorf("future cells must stay")
	}
}

func TestPrompt_StillOverBudgetIsCutOnRuneBoundary(t *testing.T) {
	t.Parallel()
	summary := planLines(8, strings.Repeat("Käse, Möhren und Brötchen. ", 20))

	prompt := compose(nil, summary, day(2026, 10, 1))

	if len(prompt) > promptCharBudget {
		t.Errorf("prompt is %d bytes, budget is %d", len(prompt), promptCharBudget)
	}
	if !strings.HasSuffix(prompt, truncationNote+plainWords(toolRules)) {
		t.Error("a cut prompt must end with the truncation note followed by the tool rules")
	}
	if !utf8.ValidString(prompt) {
		t.Error("prompt must be cut on a UTF-8 rune boundary")
	}
}

func TestPrompt_OversizedProfileCutsOnRuneBoundary(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("Ich esse gern Käse, Möhren und Brötchen. ", 700)
	p := &profile.Profile{SystemPrompt: &long}

	prompt := compose(p, "", time.Time{})

	if len(prompt) > promptCharBudget {
		t.Errorf("prompt is %d bytes, budget is %d", len(prompt), promptCharBudget)
	}
	if !strings.HasSuffix(prompt, truncationNote+plainWords(toolRules)) {
		t.Error("a cut prompt must end with the truncation note followed by the tool rules")
	}
	if !utf8.ValidString(prompt) {
		t.Error("prompt must be cut on a UTF-8 rune boundary")
	}
}

// v18: one dry-run rule, the contradictions of v17 gone, the unused read tools
// and code execution paragraph gone, three playbooks and the cook-occasion
// count.
func TestPrompt_V18(t *testing.T) {
	t.Parallel()
	prompt := compose(nil, "", time.Time{})
	for _, want := range []string{
		"Playbook, light, filling, more protein, lower or raise a day", "Playbook, replace a dish", "Playbook, away",
		"call find_ways (goal, plate_id or date; to_limit true", "what main", "check a fix from the issue's options",
		"Cooking times per day", "a routine cell add none",
		"A warning, new or worse",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt must mention %q", want)
		}
	}
	for _, gone := range []string{"list_slots", "get_profile", "code execution", "call apply_changes with dry_run true first", "only for a change of more than one step", "S5 a DGE"} {
		if strings.Contains(prompt, gone) {
			t.Errorf("prompt must not contain %q", gone)
		}
	}
	// E006: the relative-request playbook names no rule code and no dry run.
	a := strings.Index(prompt, "Playbook, light, filling, more protein, lower or raise a day")
	block := prompt[a : a+strings.Index(prompt[a:], "Playbook, replace a dish")]
	for _, bad := range []string{"S6", "H9", "dry run", "dry_run"} {
		if strings.Contains(block, bad) {
			t.Errorf("the light/hearty playbook must not say %q", bad)
		}
	}
	if strings.Count(prompt, "Priority when rules conflict") > 0 {
		t.Error("the priority list is the energy policy's, not the rules'")
	}
}

// Over budget the prompt never cuts silently: ComposePrompt reports the cut.
func TestPrompt_ReportsTheCut(t *testing.T) {
	t.Parallel()
	short, cut := ComposePrompt(nil, "", "SUMMARY\n", day(2026, 11, 2))
	if cut != 0 || strings.Contains(short, truncationNote) {
		t.Errorf("a prompt that fits has no cut, got %d", cut)
	}
	long, cut := ComposePrompt(nil, "", planLines(8, strings.Repeat("Käse ", 20)), day(2026, 10, 1))
	if cut <= 0 || !strings.Contains(long, truncationNote) {
		t.Errorf("a cut prompt reports it, got %d", cut)
	}
}

// The facts come before the plan summary, so a cut at the end never takes them.
func TestPrompt_FactsBeforeSummary(t *testing.T) {
	t.Parallel()
	prompt, _ := ComposePrompt(nil, "FACTS\n", "SUMMARY\n", day(2026, 11, 2))
	if f, s := strings.Index(prompt, "FACTS"), strings.Index(prompt, "SUMMARY"); f < 0 || s < f {
		t.Errorf("want facts < summary, got %d %d", f, s)
	}
}

// V72 T2 (tune-v2 g2-ask-11/-16/-17/-22): "change the dinner" → the agent
// asks → "Thursday's." → the agent asked again (lighter, heartier, a different
// main?). Spec G1/RP1: ask only when nothing is named; once the answer names
// the meal, act, and "change" is a different main dish.
func TestPrompt_AskOnceThenAct(t *testing.T) {
	t.Parallel()
	prompt := compose(nil, "", time.Time{})
	for _, want := range []string{
		"Ask only when no meal, day or food is named",
		"Once named (also in an answer), act",
		`"change" is a new main dish`,
		"names only a day acts on that day's dinner",
		"Playbook, replace a dish (quicker, vegetarian, different, more protein):",
		"(the lines after say them)",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt must contain %q", want)
		}
	}
	if strings.Contains(prompt, "Ask only when the request needs a choice") {
		t.Error("the old ask rule left the second question open")
	}
}

// V81 (tune-v2c g2-planner-14): "Woche neu planen" was declined as out of
// scope ("planning a whole week is the Planner"), though regenerate plans the
// week of the plan again. Only another week is the Planner's.
func TestPrompt_ReplanningTheWeekIsNoOutOfScope(t *testing.T) {
	t.Parallel()
	prompt := compose(nil, "", time.Time{})
	if strings.Contains(prompt, "planning a whole week is the Planner") {
		t.Error("replanning this week is regenerate, not the Planner")
	}
	if !strings.Contains(prompt, "a week not shown is the Planner") {
		t.Error("another week stays the Planner's")
	}
}
