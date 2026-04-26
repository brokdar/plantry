package agent

import (
	"strings"
	"testing"
	"time"
)

func TestPrompt_NoLegacyDayIndex(t *testing.T) {
	prompt := ComposePrompt(nil, nil, nil)
	if strings.Contains(prompt, "0=Monday") {
		t.Error("system prompt must not contain legacy day-index notation '0=Monday'")
	}
}

func TestPrompt_ContainsDateRange(t *testing.T) {
	from := time.Date(2025, 6, 9, 0, 0, 0, 0, time.UTC)
	to := time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC)
	dr := &DateRange{From: from, To: to}

	prompt := ComposePrompt(nil, nil, dr)

	if !strings.Contains(prompt, "2025-06-09") {
		t.Errorf("system prompt must contain from date '2025-06-09'; got:\n%s", prompt)
	}
	if !strings.Contains(prompt, "2025-06-15") {
		t.Errorf("system prompt must contain to date '2025-06-15'; got:\n%s", prompt)
	}
}
