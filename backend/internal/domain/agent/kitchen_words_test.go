package agent_test

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/agent"
	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
)

var (
	plannerWord = regexp.MustCompile(`(?i)\b(bands?|floors?|caps?|soft|hard|violations?|drafts?|sessions?)\b`)
	// E017b: the words of the last paid round, also inside identifiers
	// (band_low_pct, floor_kcal, cook_occasions: "_" separates words here).
	cookWord = regexp.MustCompile(`(?i)(^|[^a-z])(bands?|floors?|caps?|occasions?|add-ons?|hearty minimum|required[ -]rules?)([^a-z]|$)`)
	isoDate  = regexp.MustCompile(`\b\d{4}-\d{2}-\d{2}\b`)
)

// E017: the chat model reads kitchen words. The planner's words (band, floor,
// cap, soft, hard, violation, draft, session) appear in no chat tool result,
// error, tool description, tool schema or the system prompt; identifiers such
// as session_id are ids the model passes back and are not words. The report
// and every prose field hold no ISO date (days are "Thu 26 Nov").
func TestChatModelReadsKitchenWords(t *testing.T) {
	e := newPlanToolEnv(t)
	texts := map[string]string{}
	kcal, kg := 2000.0, 70.0
	prompt, _ := agent.ComposePrompt(&profile.Profile{KcalTarget: &kcal, WeightKg: &kg, HouseholdPortions: 1, Locale: "en"}, "", "", time.Time{})
	texts["prompt"] = prompt
	for _, tl := range e.ts.Describe() {
		texts["description "+tl.Name] = tl.Description
		texts["schema "+tl.Name] = string(tl.Schema)
	}
	v, p := e.laterMealSession()
	day := p.DateString()
	away := fmt.Sprintf(`{"op":"away","cells":[%s]}`, cellJSON(day, e.slotID("dinner")))
	calls := map[string][2]string{
		"get_plan":                {"get_plan", `{}`},
		"check an away":           {"apply_changes", `{"dry_run":true,"ops":[` + away + `]}`},
		"find_alternatives":       {"find_alternatives", fmt.Sprintf(`{"plate_id":%d,"what":"main"}`, p.ID)},
		"find_ways lighter":       {"find_ways", fmt.Sprintf(`{"goal":"lighter","plate_id":%d}`, p.ID)},
		"find_foods":              {"find_foods", `{"limit":3}`},
		"portions below the need": {"apply_changes", fmt.Sprintf(`{"ops":[{"op":"set_portions","session_id":%d,"portions":0.1}]}`, v.ID)},
	}
	for name, c := range calls {
		out, _, _, err := e.ts.Execute(e.ctx, c[0], json.RawMessage(c[1]))
		if err != nil {
			texts["error of "+name] = err.Error()
			continue
		}
		texts["result of "+name] = string(out)
		var m map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(out, &m))
		if rep, ok := m["report"]; ok {
			assert.NotRegexp(t, isoDate, string(rep), "the report of "+name)
		}
	}
	ctx, conv := e.chat()
	e.change(ctx, "apply_changes", `{"ops":[`+away+`]}`)
	res := e.undoLast(ctx, conv, `{}`)
	raw, err := json.Marshal(res)
	require.NoError(t, err)
	texts["result of undo_last_change"] = string(raw)
	require.Contains(t, res, "report", "an undo carries a report")
	for name, text := range texts {
		assert.Empty(t, plannerWord.FindAllString(text, -1), name)
		assert.Empty(t, cookWord.FindAllString(text, -1), name)
		if strings.HasPrefix(name, "description") || strings.HasPrefix(name, "schema") || name == "prompt" {
			assert.NotContains(t, strings.ToLower(text), "hearty", name+": the level reads as filling")
		}
	}
	assert.Contains(t, strings.ToLower(texts["result of check an away"]), `"report"`)
}
