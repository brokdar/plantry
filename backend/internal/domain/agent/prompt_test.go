package agent

import (
	"strings"
	"testing"
	"time"
)

func TestPrompt_NoLegacyDayIndex(t *testing.T) {
	prompt := ComposePrompt(nil, nil, nil, nil)
	if strings.Contains(prompt, "0=Monday") {
		t.Error("system prompt must not contain legacy day-index notation '0=Monday'")
	}
}

func TestPrompt_ContainsDateRange(t *testing.T) {
	from := time.Date(2025, 6, 9, 0, 0, 0, 0, time.UTC)
	to := time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC)
	dr := &DateRange{From: from, To: to}

	prompt := ComposePrompt(nil, nil, dr, nil)

	if !strings.Contains(prompt, "2025-06-09") {
		t.Errorf("system prompt must contain from date '2025-06-09'; got:\n%s", prompt)
	}
	if !strings.Contains(prompt, "2025-06-15") {
		t.Errorf("system prompt must contain to date '2025-06-15'; got:\n%s", prompt)
	}
}

func TestPrompt_ContainsPlanningPrefs(t *testing.T) {
	from := time.Date(2025, 6, 9, 0, 0, 0, 0, time.UTC)
	to := time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC)
	dr := &DateRange{From: from, To: to}
	prefs := &PlanningPrefs{ShoppingDay: "Saturday", AnchorMode: "today"}

	prompt := ComposePrompt(nil, nil, dr, prefs)

	if !strings.Contains(prompt, "Saturday") {
		t.Errorf("prompt must contain shopping day 'Saturday'; got:\n%s", prompt)
	}
	if !strings.Contains(prompt, "today") {
		t.Errorf("prompt must contain anchor mode 'today'; got:\n%s", prompt)
	}
}

func TestPrompt_NoGetWeekMention(t *testing.T) {
	prompt := ComposePrompt(nil, nil, nil, nil)
	// The role prefix should guide the model to use get_plates_range, not get_week.
	// Verify "get_week" does not appear as a recommended tool in the role instructions.
	if strings.Contains(prompt, "get_week\n") || strings.Contains(prompt, "get_week,") || strings.Contains(prompt, "get_week)") {
		t.Errorf("system prompt must not recommend 'get_week' as an available tool; got:\n%s", prompt)
	}
}
