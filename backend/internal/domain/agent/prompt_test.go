package agent_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaltszeimer/plantry/backend/internal/domain/agent"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planner"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
)

func TestComposePrompt_Empty(t *testing.T) {
	p := agent.ComposePrompt(nil, nil)
	assert.Contains(t, p, "You are Plantry")
	assert.Contains(t, p, "Tool usage rules")
}

func TestComposePrompt_IncludesProfile(t *testing.T) {
	kcal := 2200.0
	prot := 30.0
	sp := "I prefer quick weekday meals."
	prof := &profile.Profile{
		KcalTarget:          &kcal,
		ProteinPct:          &prot,
		DietaryRestrictions: []string{"vegetarian", "no-nuts"},
		Preferences:         map[string]any{"likes_spicy": true, "dislikes": []string{"mushrooms"}},
		SystemPrompt:        &sp,
		Locale:              "en",
	}
	out := agent.ComposePrompt(prof, nil)
	assert.Contains(t, out, "kcal_target: 2200")
	assert.Contains(t, out, "protein=30")
	assert.Contains(t, out, "vegetarian, no-nuts")
	assert.Contains(t, out, "likes_spicy: true")
	assert.Contains(t, out, "I prefer quick weekday meals.")
	assert.Contains(t, out, "locale: en")
}

func TestComposePrompt_IncludesWeekSummary(t *testing.T) {
	w := &planner.Week{
		ID: 42, Year: 2026, WeekNumber: 17,
		Plates: []plate.Plate{
			{ID: 1, Day: 0, SlotID: 2, Components: []plate.PlateComponent{{}, {}}},
			{ID: 2, Day: 1, SlotID: 3},
		},
	}
	out := agent.ComposePrompt(nil, w)
	assert.Contains(t, out, "year=2026 week=17")
	assert.Contains(t, out, "plate_id=1 day=0 slot_id=2 components=2")
	assert.Contains(t, out, "plate_id=2 day=1 slot_id=3 components=0")
}

func TestComposePrompt_EmptyWeekMentionsEmpty(t *testing.T) {
	w := &planner.Week{ID: 7, Year: 2026, WeekNumber: 18}
	out := agent.ComposePrompt(nil, w)
	assert.Contains(t, out, "no plates yet")
}

func TestComposePrompt_TruncatesLargePreferences(t *testing.T) {
	// Create a huge preferences blob that would exceed the budget.
	prefs := map[string]any{}
	for i := 0; i < 500; i++ {
		prefs[strings.Repeat("k", 20)+fmt.Sprintf("_%d", i)] = strings.Repeat("v", 200)
	}
	prof := &profile.Profile{Preferences: prefs}
	out := agent.ComposePrompt(prof, nil)
	assert.LessOrEqual(t, len(out), 16_000, "budget enforced")
	assert.Contains(t, out, "[prompt truncated]")
}

func TestComposePrompt_StaysUnderBudget_ForRealisticInput(t *testing.T) {
	kcal := 2200.0
	prof := &profile.Profile{
		KcalTarget:          &kcal,
		DietaryRestrictions: []string{"vegetarian"},
		Preferences:         map[string]any{"likes_spicy": true},
		Locale:              "en",
	}
	w := &planner.Week{
		ID: 1, Year: 2026, WeekNumber: 17,
		Plates: make([]plate.Plate, 21), // max realistic: 7 days × 3 slots
	}
	for i := range w.Plates {
		w.Plates[i] = plate.Plate{ID: int64(i + 1), Day: i / 3, SlotID: int64(i%3 + 1)}
	}
	out := agent.ComposePrompt(prof, w)
	assert.LessOrEqual(t, len(out), 16_000)
	assert.NotContains(t, out, "[prompt truncated]")
}
