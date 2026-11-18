package agent_test

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/agent"
	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
)

var ruleToken = regexp.MustCompile(`\b[HS]\d+b?\b`)

// E003: the model reads each issue by a plain label and sentence, never by a
// rule code or an issue code (it repeats what it reads, the user cannot read
// "S2" or "H9").
func TestPlainRules_IssueHasLabelAndNoCodes(t *testing.T) {
	raw := json.RawMessage(`{"after":{"issues":{"new":[{"rule":"S2","code":"kcal_below_band","severity":"soft","date":"2026-11-10","params":{"kcal":1500}}],
		"worsened":[{"rule":"S7","code":"week_meat_above_dge","severity":"soft","before":{"meat_g":321},"after":{"meat_g":471}}]}},
		"empty_reason":{"candidates":3,"dropped":{"H9.kcal_below_floor":2,"S6.meal_not_light":1,"nutrition_missing":1}}}`)
	out := string(agent.PlainRules(raw))
	assert.NotRegexp(t, ruleToken, out)
	for _, gone := range []string{`"rule"`, `"code"`, "kcal_below_band", "week_meat_above_dge", "kcal_below_floor", "meal_not_light", "nutrition_missing"} {
		assert.NotContains(t, out, gone)
	}
	var res struct {
		After struct {
			Issues struct {
				New    []map[string]any `json:"new"`
				Worsen []map[string]any `json:"worsened"`
			} `json:"issues"`
		} `json:"after"`
		EmptyReason struct {
			Dropped map[string]json.Number `json:"dropped"`
		} `json:"empty_reason"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &res))
	require.Len(t, res.After.Issues.New, 1)
	is := res.After.Issues.New[0]
	assert.Equal(t, "day energy", is["label"])
	assert.Equal(t, "The day's energy is below its target range.", is["message"])
	assert.Equal(t, "warning", is["severity"])
	assert.Equal(t, "2026-11-10", is["date"])
	assert.Equal(t, map[string]any{"kcal": float64(1500)}, is["params"])
	require.Len(t, res.After.Issues.Worsen, 1)
	assert.Equal(t, "weekly meat", res.After.Issues.Worsen[0]["label"])
	assert.Equal(t, map[string]json.Number{"day energy minimum": "2", "light meal": "1", "missing nutrition data": "1"}, res.EmptyReason.Dropped)
}

func TestPlainRules_ResultWithoutIssuesIsUntouched(t *testing.T) {
	raw := json.RawMessage(`{"b":1,"a":[{"code":"x"}]}`)
	assert.JSONEq(t, string(raw), string(agent.PlainRules(raw)))
}

// What the chat model reads up front holds no rule code and no "dry run": the
// composed system prompt (with a profile, so the energy policy is in) and
// every chat tool description. The prompt names the phrase once, to forbid
// it, and the tools have a parameter named dry_run (an underscore, so no
// match); only that one prohibition sentence is excluded.
func TestChatModelReadsNoRuleCodesOrDryRun(t *testing.T) {
	kcal, kg := 2000.0, 70.0
	prof := &profile.Profile{KcalTarget: &kcal, WeightKg: &kg, HouseholdPortions: 1, Locale: "en"}
	prompt, _ := agent.ComposePrompt(prof, "", "", time.Time{})
	texts := map[string]string{"system prompt": prompt}
	e := newPlanToolEnv(t)
	for _, tl := range e.ts.Describe() {
		texts["tool "+tl.Name] = tl.Description
	}
	require.Contains(t, texts, "tool find_alternatives")
	require.Contains(t, texts, "tool apply_changes")
	require.Contains(t, texts, "tool get_plan")
	texts["system prompt"] = strings.Replace(texts["system prompt"], `never write "dry run", tool names`, "never write [the phrase], tool names", 1)
	for name, text := range texts {
		assert.NotRegexp(t, ruleToken, text, name)
		lower := strings.ToLower(text)
		assert.NotContains(t, lower, "dry run", name)
		assert.NotContains(t, lower, "dry-run", name)
	}
	assert.NotContains(t, prompt, "checked the change first")
}

// An issue with no verified fix (E002b) and a cook in a no-plate slot (E001)
// read by label too; no_fix_found and no_fix_reason stay.
func TestPlainRules_NoFixAndCookInNoneCell(t *testing.T) {
	raw := json.RawMessage(`{"issues":[{"rule":"H2","code":"too_many_cook_occasions","severity":"violation","no_fix_found":true,"no_fix_reason":"ask the user"},
		{"rule":"H1","code":"cook_in_none_cell","severity":"violation","session_id":4}]}`)
	var res struct {
		Issues []map[string]any `json:"issues"`
	}
	out := agent.PlainRules(raw)
	require.NoError(t, json.Unmarshal(out, &res))
	assert.NotRegexp(t, ruleToken, string(out))
	assert.Equal(t, "cooking occasions per day", res.Issues[0]["label"])
	assert.Equal(t, true, res.Issues[0]["no_fix_found"])
	assert.Equal(t, "ask the user", res.Issues[0]["no_fix_reason"])
	assert.Equal(t, "cooking in a no-plate slot", res.Issues[1]["label"])
}
