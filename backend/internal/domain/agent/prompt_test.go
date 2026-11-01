package agent

import (
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
)

func day(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

func TestPrompt_NoLegacyDayIndex(t *testing.T) {
	if strings.Contains(ComposePrompt(nil, "", time.Time{}), "0=Monday") {
		t.Error("system prompt must not contain legacy day-index notation '0=Monday'")
	}
}

func TestPrompt_CarriesSummaryBetweenProfileAndRules(t *testing.T) {
	kcal := 2000.0
	prompt := ComposePrompt(&profile.Profile{KcalTarget: &kcal}, "SUMMARY\n", day(2026, 11, 2))

	prof, sum, rules := strings.Index(prompt, "kcal_target: 2000"), strings.Index(prompt, "SUMMARY\n"), strings.Index(prompt, "Tool usage rules:")
	if prof < 0 || sum < prof || rules < sum {
		t.Errorf("want profile < summary < rules, got %d %d %d", prof, sum, rules)
	}
}

// D3: drafts, unlock, past days, regenerate vs direct tools, one summary; no
// week-copy paragraph.
func TestPrompt_ToolRules(t *testing.T) {
	prompt := ComposePrompt(nil, "", time.Time{})
	for _, want := range []string{"drafts", "unlock", "later message", "before today", "regenerate", "single short assistant message", "get_plan"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt must mention %q", want)
		}
	}
	if strings.Contains(prompt, "copy"+"_week") {
		t.Error("the week-copy paragraph is gone")
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
	summary := planLines(4, strings.Repeat("Rindergulasch mit Spätzle ", 4))
	today := day(2026, 10, 20)

	prompt := ComposePrompt(nil, summary, today)

	if len(prompt) > promptCharBudget {
		t.Errorf("prompt is %d bytes, budget is %d", len(prompt), promptCharBudget)
	}
	if !strings.HasSuffix(prompt, toolRules) {
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
	summary := planLines(8, strings.Repeat("Käse, Möhren und Brötchen. ", 20))

	prompt := ComposePrompt(nil, summary, day(2026, 10, 1))

	if len(prompt) > promptCharBudget {
		t.Errorf("prompt is %d bytes, budget is %d", len(prompt), promptCharBudget)
	}
	if !strings.HasSuffix(prompt, truncationNote+toolRules) {
		t.Error("a cut prompt must end with the truncation note followed by the tool rules")
	}
	if !utf8.ValidString(prompt) {
		t.Error("prompt must be cut on a UTF-8 rune boundary")
	}
}

func TestPrompt_OversizedProfileCutsOnRuneBoundary(t *testing.T) {
	long := strings.Repeat("Ich esse gern Käse, Möhren und Brötchen. ", 700)
	p := &profile.Profile{SystemPrompt: &long}

	prompt := ComposePrompt(p, "", time.Time{})

	if len(prompt) > promptCharBudget {
		t.Errorf("prompt is %d bytes, budget is %d", len(prompt), promptCharBudget)
	}
	if !strings.HasSuffix(prompt, truncationNote+toolRules) {
		t.Error("a cut prompt must end with the truncation note followed by the tool rules")
	}
	if !utf8.ValidString(prompt) {
		t.Error("prompt must be cut on a UTF-8 rune boundary")
	}
}
