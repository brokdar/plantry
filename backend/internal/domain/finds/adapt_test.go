package finds_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/finds"
)

func adaptJSON(swaps string) string {
	return `{"name":"Mild curry","portions":1,"steps":["Fry the onion until soft, about 5 minutes.","Simmer 20 minutes."],` +
		`"swaps":` + swaps + `,"requirements":` + fitsKitchen + `}`
}

// C2b 1 and 2 (canary 2026-11-27): a model that rescales the recipe to one
// portion and reports amount changes as swaps changes nothing: the find
// keeps the source's yield and amounts (the planner scales), has no change
// and is a light adaptation.
func TestAdapt_RescalingIsNotAnAdaptation(t *testing.T) {
	t.Parallel()
	r := newRig(t, true, lineMatcher{}, kitchen, searchTurn(curryURLs...), toolTurn(finds.AdaptTool, adaptJSON(
		`[{"line":0,"to":"150 g chicken thigh","reason":"scaled to 1 portion"},{"line":3,"to":"100 ml coconut milk ($0.50)","reason":"scaled"},{"line":2,"to":"0.5 tbsp curry paste","reason":"scaled"}]`)))
	f := r.run(t, finds.TriggerPlanning)
	require.Equal(t, finds.StateReady, f.State, f.Steps)
	assert.Empty(t, f.Changes)
	assert.Equal(t, finds.OriginAdaptedLight, *f.Origin)
	assert.InDelta(t, 4, f.Draft.ReferencePortions, 0)
	assert.InDelta(t, 600, f.Draft.Ingredients[0].Amount, 0)
	assert.InDelta(t, 400, f.Draft.Ingredients[3].Amount, 0)
}

// C2b 2 and 3: two real swaps stay "Adapted from" light, and a price the
// model repeats never reaches a change or a line.
func TestAdapt_TwoSwapsAreLightAndPricesAreStripped(t *testing.T) {
	t.Parallel()
	r := newRig(t, true, lineMatcher{}, kitchen, searchTurn(curryURLs...), toolTurn(finds.AdaptTool, adaptJSON(
		`[{"line":1,"to":"1 shallot ($0.32)","reason":"milder"},{"line":2,"to":"2 tbsp mild curry paste","reason":"heat"},{"line":3,"to":"100 ml coconut milk","reason":"scaled"}]`)))
	f := r.run(t, finds.TriggerPlanning)
	require.Equal(t, finds.StateReady, f.State, f.Steps)
	require.Len(t, f.Changes, 2)
	assert.Equal(t, "1 shallot", f.Changes[0].To)
	assert.Equal(t, finds.OriginAdaptedLight, *f.Origin)
	assert.Equal(t, "shallot", f.Draft.Ingredients[1].Name)
	assert.InDelta(t, 4, f.Draft.ReferencePortions, 0)
}

// C2b 4: a seasoning measured in no unit becomes a clean to-taste line the
// matcher can find.
func TestAdapt_LooseSeasoningIsToTaste(t *testing.T) {
	t.Parallel()
	r := newRig(t, true, lineMatcher{}, kitchen, searchTurn(curryURLs...), toolTurn(finds.AdaptTool, adaptJSON(
		`[{"line":1,"to":"a few turns of black pepper","reason":"seasoning"}]`)))
	f := r.run(t, finds.TriggerPlanning)
	require.Equal(t, finds.StateReady, f.State, f.Steps)
	line := f.Draft.Ingredients[1]
	assert.Equal(t, "black pepper", line.Name)
	assert.True(t, line.ToTaste)
}

// C2b 5: the draft keeps the steps under "instructions", where the review
// card reads them (the canary script looked for "steps"). C2b 6: the search
// request carries no cache marker request.
func TestAdapt_DraftJSONHasInstructionsAndSearchIsNotCached(t *testing.T) {
	t.Parallel()
	r := newRig(t, true, lineMatcher{}, kitchen, searchTurn(curryURLs...), adaptTurn(false, fitsKitchen))
	f := r.run(t, finds.TriggerPlanning)
	require.Equal(t, finds.StateReady, f.State, f.Steps)
	raw, err := json.Marshal(f)
	require.NoError(t, err)
	var got struct {
		Draft map[string]json.RawMessage `json:"draft"`
	}
	require.NoError(t, json.Unmarshal(raw, &got))
	assert.Contains(t, string(got.Draft["instructions"]), "Fry the onion")

	reqs := r.client.Requests()
	assert.True(t, reqs[0].NoCache, "search")
	assert.NotContains(t, reqs[1].Messages[0].Content[0].Text, "Household portions")
}

const reasoningSwap = `[{"line":0,"to":"15oz. can diced tomatoes is replaced by 1 15oz. can diced tomatoes omitted; add 1 cup of additional vegetable broth? no.","reason":"x"}]`

// CP3 round 3: a swap line holding the model's reasoning makes the adapt
// step ask once more (search + two adapt calls; the fake replays its first
// turn for the repeated request); a swap that still holds reasoning is
// dropped and the source line stays.
func TestAdapt_ReasoningSwapIsRetriedOnceThenDropped(t *testing.T) {
	t.Parallel()
	r := newRig(t, true, lineMatcher{}, kitchen, searchTurn(curryURLs...),
		toolTurn(finds.AdaptTool, adaptJSON(reasoningSwap)))
	f := r.run(t, finds.TriggerPlanning)
	require.Equal(t, finds.StateReady, f.State, f.Steps)
	assert.Len(t, r.client.Requests(), 3)
	assert.Empty(t, f.Changes)
	assert.Contains(t, f.Draft.Ingredients[0].RawText, "chicken")
	assert.NotContains(t, f.Draft.Ingredients[0].Name, "replaced")
}

// CP3 fix: the adapt request carries the household's heat tolerance, lowered
// to mild when taste leans away from heat (FIND-12), and a hot spice left
// in a mild household is flagged.
func TestAdapt_HeatFollowsTasteAndHotSpiceIsFlagged(t *testing.T) {
	t.Parallel()
	hotKitchen := kitchen
	hotKitchen.Heat = "hot"
	for _, tc := range []struct {
		name      string
		rather    []string
		wantHeat  string
		wantFlags int
	}{
		{"taste dislikes heat", []string{"heat"}, "Heat tolerance: mild.", 1},
		{"kitchen heat stands", nil, "Heat tolerance: hot.", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newRig(t, true, lineMatcher{}, hotKitchen, searchTurn(curryURLs...),
				toolTurn(finds.AdaptTool, adaptJSON(`[{"line":0,"to":"2 tsp hot curry powder","reason":"x"}]`)))
			r.house.RatherNot = tc.rather
			f := r.run(t, finds.TriggerPlanning)
			reqs := r.client.Requests()
			assert.Contains(t, reqs[1].Messages[0].Content[0].Text, tc.wantHeat)
			assert.Contains(t, reqs[1].System, "hot spice")
			n := 0
			for _, fl := range f.Flags {
				if fl.Code == "heat_hot_spice" {
					n++
					assert.Contains(t, fl.Reason, "hot curry powder")
				}
			}
			assert.Equal(t, tc.wantFlags, n, f.Flags)
		})
	}
}

func TestHotSpiceRE_OnlySpices(t *testing.T) {
	hot := []string{"1 tbsp hot curry powder", "2 tsp hot paprika", "1 tsp cayenne pepper", "2 red chilies, sliced", "1 tbsp sriracha", "1 TL scharfes Currypulver"}
	notHot := []string{"1 cup hot water", "1 hot dog bun", "2 tbsp hot cocoa mix", "1 tsp mild curry powder", "1 tbsp sweet paprika"}
	for _, l := range hot {
		if !finds.HotSpiceRE.MatchString(l) {
			t.Errorf("%q: want hot", l)
		}
	}
	for _, l := range notHot {
		if finds.HotSpiceRE.MatchString(l) {
			t.Errorf("%q: want not hot", l)
		}
	}
}
