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

func TestModeHint_FillEmpty_StartsAndExplains(t *testing.T) {
	h := modeHint("fill_empty")
	// The Fill-empty action auto-sends, so the assistant must not stall on a
	// confirmation prompt, and it must report its picks back to the user.
	for _, want := range []string{"don't ask for confirmation", "rundown", "why you picked"} {
		if !strings.Contains(h, want) {
			t.Errorf("fill_empty mode hint must mention %q so the assistant starts and explains itself; got:\n%s", want, h)
		}
	}
}

func TestModeHint_FillEmpty_PlansForVariety(t *testing.T) {
	h := modeHint("fill_empty")
	// Guard against the old behaviour of repeating the same favourites every
	// week: it must read recent weeks and treat favourites as a tie-breaker.
	for _, want := range []string{"preceding weeks", "Do not repeat meals", "tie-breaker"} {
		if !strings.Contains(h, want) {
			t.Errorf("fill_empty mode hint must drive variety via %q; got:\n%s", want, h)
		}
	}
}

func TestModeHint_ReplaceAll_PlansForVariety(t *testing.T) {
	h := modeHint("replace_all")
	for _, want := range []string{"swap_food", "tie-breaker"} {
		if !strings.Contains(h, want) {
			t.Errorf("replace_all mode hint must drive variety via %q; got:\n%s", want, h)
		}
	}
}

func day(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

// dateSection returns the prompt from the today line up to the tool rules.
func dateSection(t *testing.T, prompt string) string {
	t.Helper()
	start := strings.Index(prompt, "Today is ")
	end := strings.Index(prompt, "\nTool usage rules:")
	if start < 0 || end < start {
		t.Fatalf("prompt has no date section; got:\n%s", prompt)
	}
	return strings.TrimRight(prompt[start:end], "\n") + "\n"
}

func TestPrompt_DateSection(t *testing.T) {
	tests := []struct {
		name   string
		dr     DateRange
		plates []PromptPlate
		prefs  *PlanningPrefs
		want   string
	}{
		{
			name: "window crosses a month, today outside",
			dr:   DateRange{Today: day(2026, 10, 26), From: day(2026, 10, 31), To: day(2026, 11, 6)},
			plates: []PromptPlate{
				{ID: 5, Date: day(2026, 10, 31), SlotID: 2, Slot: "slot.dinner", Foods: []string{"Pasta", "Green salad"}},
				{ID: 6, Date: day(2026, 11, 2), SlotID: 1, Slot: "slot.lunch", Skipped: true},
			},
			want: `Today is Monday, 2026-10-26.
Planning window: Saturday 2026-10-31 to Friday 2026-11-06 (7 days).
Dates in the window:
- Saturday 2026-10-31
- Sunday 2026-11-01
- Monday 2026-11-02
- Tuesday 2026-11-03
- Wednesday 2026-11-04
- Thursday 2026-11-05
- Friday 2026-11-06
Plates already planned:
- 2026-10-31 slot.dinner (plate_id=5, slot_id=2): Pasta, Green salad
- 2026-11-02 slot.lunch (plate_id=6, slot_id=1): skipped
`,
		},
		{
			name: "today inside, empty plate list",
			dr:   DateRange{Today: day(2026, 2, 27), From: day(2026, 2, 26), To: day(2026, 3, 1)},
			want: `Today is Friday, 2026-02-27.
Planning window: Thursday 2026-02-26 to Sunday 2026-03-01 (4 days).
Dates in the window:
- Thursday 2026-02-26
- Friday 2026-02-27 (today)
- Saturday 2026-02-28
- Sunday 2026-03-01
This window has no plates yet.
`,
		},
		{
			name:  "single day with planning prefs",
			dr:    DateRange{Today: day(2026, 10, 26), From: day(2026, 10, 26), To: day(2026, 10, 26)},
			prefs: &PlanningPrefs{ShoppingDay: "Saturday", AnchorMode: "today"},
			want: `Today is Monday, 2026-10-26.
Planning window: Monday 2026-10-26 to Monday 2026-10-26 (1 day).
Dates in the window:
- Monday 2026-10-26 (today)
Shopping day: Saturday
Plan anchor: today
This window has no plates yet.
`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := dateSection(t, ComposePrompt(nil, tt.plates, &tt.dr, tt.prefs))
			if got != tt.want {
				t.Errorf("date section mismatch\ngot:\n%s\nwant:\n%s", got, tt.want)
			}
		})
	}
}

func TestPrompt_DateSection_StaysWithinBudget(t *testing.T) {
	dr := &DateRange{Today: day(2026, 1, 1), From: day(2026, 1, 1), To: day(2026, 1, 31)}
	plates := make([]PromptPlate, 0, 400)
	for i := range 400 {
		plates = append(plates, PromptPlate{
			ID: int64(i), Date: day(2026, 1, 1+i%31), SlotID: 1, Slot: "slot.dinner",
			Foods: []string{strings.Repeat("Very long food name ", 3)},
		})
	}

	prompt := ComposePrompt(nil, plates, dr, nil)

	if len(prompt) > promptCharBudget {
		t.Errorf("prompt is %d chars, budget is %d", len(prompt), promptCharBudget)
	}
	if !strings.HasSuffix(prompt, truncationNote) {
		t.Errorf("an over-budget prompt must end with the truncation note")
	}
}
