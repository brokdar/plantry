package planning_test

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
)

// E038: the preview settles what the model would otherwise spend turns on.

func TestFinishProposal_ReplacesARecentRecipe(t *testing.T) {
	s := batchCooker() // mains 101-103 were eaten in the last two weeks, variety is 3 weeks
	tgt := targetCells(cell(tue, lunch), cell(tue, dinner))
	p := planning.Proposal{
		Sessions: []planning.ProposedSession{session("s1", 101, tue, lunch, 2)},
		Plates:   []planning.ProposedPlate{pplate(tue, lunch, onKey(101, "s1")), pplate(tue, dinner, onKey(101, "s1"))},
	}
	in := planning.ChooseInput{State: s, Target: tgt}
	require.Contains(t, codes(planning.Check(planning.Merge(s, tgt, p), tgt), "H8"), "variety_repeat")
	got, notes := planning.FinishProposal(in, p)
	assert.NotEqual(t, int64(101), got.Sessions[0].FoodID, "the recently eaten dish is replaced")
	assert.NotContains(t, codes(planning.Check(planning.Merge(s, tgt, got), tgt), "H8"), "variety_repeat")
	assert.True(t, strings.Contains(strings.Join(notes, "\n"), "Variety: Main 1 replaced by"), "%v", notes)
}

func TestFinishProposal_KeepsALightMealLight(t *testing.T) {
	s := batchCooker()
	s.Profile.KcalTarget = f64(1800)
	tgt := targetCells(cell(tue, lunch), cell(tue, dinner))
	// Main 5 holds 600 kcal: a light dinner of 1.5 portions is far over its limit.
	p := planning.Proposal{
		Sessions: []planning.ProposedSession{session("s1", 105, tue, dinner, 1.5)},
		Plates:   []planning.ProposedPlate{pplate(tue, dinner, planning.ProposedComponent{FoodID: 105, Portions: 1.5, SessionKey: "s1"})},
		Light:    []planning.LightIntent{{Date: tue, Scope: planning.LightScopeMeal, Slot: "dinner"}},
	}
	s.SlotNames = map[int64]string{lunch: "slot.lunch", dinner: "slot.dinner"}
	got, _ := planning.FinishProposal(planning.ChooseInput{State: s, Target: tgt}, p)
	merged := planning.Merge(s, tgt, got)
	for _, pe := range planning.Evaluate(merged, tgt).Plates {
		if pe.Intent == planning.LevelLight {
			assert.LessOrEqual(t, pe.Kcal, pe.MaxKcal+0.5, "a light meal ends within its limit")
		}
	}
	// The empty lunch takes the day's dish again (KR7 fallback), so the batch
	// follows every plate that eats it.
	eaten := 0.0
	for _, pl := range got.Plates {
		for _, c := range pl.Components {
			if c.SessionKey == got.Sessions[0].Key {
				eaten += c.Portions
			}
		}
	}
	assert.InDelta(t, eaten, got.Sessions[0].Portions, 1e-9, "the batch follows the plates")
}

func TestParseProposal_ReadsAWrappedPlan(t *testing.T) {
	s := batchCooker()
	tgt := targetCells(cell(tue, dinner))
	in := planning.ChooseInput{State: s, Target: tgt}
	plan := `{"sessions":[{"key":"s1","food_id":104,"cook_date":"2026-11-10","cook_slot_id":3,"portions":1}],"plates":[{"date":"2026-11-10","slot_id":3,"components":[{"food_id":104,"portions":1,"session_key":"s1"}]}]}`
	for _, raw := range []string{plan, `{"params":` + plan + `}`} {
		prop, err := planning.ParseForProof([]byte(raw), in)
		require.NoError(t, err)
		assert.Len(t, prop.Plates, 1, raw[:20])
	}
}

func TestAutofix_ARecipeInGramsBecomesAPortion(t *testing.T) {
	s := batchCooker()
	tgt := targetCells(cell(tue, dinner))
	p := planning.Proposal{
		Sessions: []planning.ProposedSession{session("s1", 104, tue, dinner, 1)},
		Plates:   []planning.ProposedPlate{pplate(tue, dinner, onKey(104, "s1"), planning.ProposedComponent{FoodID: 201, Grams: 60})},
	}
	got, notes := planning.Autofix(planning.ChooseInput{State: s, Target: tgt}, p)
	require.Len(t, notes, 1)
	assert.Contains(t, notes[0], "a recipe: one portion instead of 60 g")
	assert.InDelta(t, 1, got.Plates[0].Components[1].Portions, 1e-9)
	assert.Zero(t, got.Plates[0].Components[1].Grams)
}

func TestRenderPrompt_NamesTheRecentMains(t *testing.T) {
	s := batchCooker()
	s.Window.Start = day(sat)
	prompt := planning.RenderPrompt(planning.ChooseInput{State: s, Target: windowTarget(s)})
	assert.Contains(t, prompt, "Eaten within the last 3 weeks (H8: do not cook these as mains unless the candidates run out): ")
	assert.Contains(t, prompt, "Main 1")
}

var plannerBanned = regexp.MustCompile(`(?i)\b(bands?|floors?|caps?|hearty|occasions?|add-ons?)\b`)

// E038: the prose the planner model reads uses the chat boundary's kitchen words.
func TestPlannerProseUsesKitchenWords(t *testing.T) {
	s := batchCooker()
	s.Profile.KcalTarget = f64(2000)
	in := planning.ChooseInput{State: s, Target: windowTarget(s)}
	prompt := planning.RenderPrompt(in)
	assert.NotRegexp(t, plannerBanned, prompt, "the system prompt")
	for _, tl := range planning.PlannerTools() {
		assert.NotRegexp(t, plannerBanned, tl.Description, tl.Name)
	}
	view, _, _ := planning.PlannerViewOf(in, freshPlan(s))
	raw, _ := json.Marshal(view)
	raw = planning.KitchenJSON(raw)
	// Field names hold underscores (band_low, cook_occasions) and stay; the prose does not.
	for _, line := range strings.Split(string(raw), "\"") {
		if strings.Contains(line, " ") {
			assert.NotRegexp(t, plannerBanned, line, "the evaluate_plan result")
		}
	}
	assert.True(t, strings.Contains(prompt, "range"))
}
