package agent

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/fake"
)

// The stored find_ways results of the M3 tune runs (testdata/m3tune): a turn
// that found nothing to apply and saved nothing ends on a code-rendered
// "nothing changed", the reason with its numbers and the search's one offer
// (G4.1, SF2; a day number under the minimum names it and the asked number,
// C-11). Before, the reply was the model's text alone and missed the minimum
// (tune2 g-daynumber-14) or the "nothing changed" (g-lighter-06 turn 2).
func TestRun_FindWaysWithNothingToApplyTellsWhyInCode(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name, file, user, model string
		want                    []string
		absent                  []string
	}{
		{
			name: "day number under the minimum (tune2 g-daynumber-14)", file: "daynumber14_find_ways.json",
			user:  "Könntest du bitte den Freitag auf etwa 900 kcal insgesamt planen? Danke.",
			model: "Freitag bleibt unverändert, weil 900 kcal unter dem Tagesminimum liegt. Die kleinste machbare Variante steht unten als Vorschlag.",
			want: []string{
				"- " + nothingChangedDE,
				"- Freitag, 27. Nov. bleibt, wie er ist: 900 kcal liegen unter dem Mindestwert des Tages von 1275 kcal.",
				"Ich kann auch Freitag, 27. Nov. auf 1299 kcal senken, so tief es innerhalb seines Bereichs geht: „Linsen-Bolognese aus dem Slow Cooker“: 0,5 Portionen (vorher 1)",
				"sag Bescheid, wenn du das möchtest.",
			},
		},
		{
			name: "lighter still, no room above the minimum (tune2 g-lighter-06 turn 2)", file: "lighter06_turn2_find_ways.json",
			user:  "gut, aber noch ein bisschen leichter bitte",
			model: "Der Abendessen-Teller lässt sich allein nicht weiter senken: Er hat 458 kcal. Soll ich das so machen?",
			want: []string{
				"- " + nothingChangedDE,
				"- Diese Mahlzeit kann allein nicht leichter werden: sie hat 458 kcal und der Tag 1290 kcal, nur 15 kcal über seinem Mindestwert von 1275 kcal.",
				"Ich kann auch bei Montag, 30. Nov. Abendessen: „Red lentil soup“: 1,25 Portionen (vorher 1,5) (der Tag hätte dann 1486 kcal): sag Bescheid, wenn du das möchtest.",
			},
			absent: []string{"Soll ich", "458 kcal. "},
		},
		{
			name: "every way failed the real check (tune2 h-lighter2-00 turn 1)", file: "lighter2_00_turn1_find_ways.json",
			user:  "Could you please make Saturday's dinner a bit lighter? It is too heavy.",
			model: "Saturday's dinner stays as it is.",
			want: []string{
				"Saturday's dinner stays as it is.",
				"- " + nothingChangedEN,
				"- No change of this meal on its own passed the check: each one would add a problem to the plan.",
			},
		},
		{
			name: "heartier over the day's top (tune2 g-heartier-10)", file: "heartier10_find_ways.json",
			user:  "Friday dinner leaves me hungry again, put a bit more on that plate",
			model: "Friday dinner can't get bigger on its own. Shall I make that change?",
			want: []string{
				"- " + nothingChangedEN,
				"- This meal cannot get heartier on its own: the day holds 1699 kcal and a good amount for it ends at 1785 kcal.",
				"I can also ",
			},
			absent: []string{"Shall I"},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			result, err := os.ReadFile("testdata/m3tune/" + c.file)
			require.NoError(t, err)
			ts := minimalToolSet(t, findWaysName, result, ToolEffectNone, nil)
			client := fake.NewFromScript(&fake.Script{Turns: []fake.Turn{scriptToolUseTurn("t1", findWaysName), scriptEndTurn(c.model)}})
			req := makeReq(1)
			req.UserText = c.user
			req.Tools = ts.Describe()
			repo := &memRepo{}
			require.NoError(t, runLoopRepo(t, req, client, ts, repo))

			msgs, _ := repo.ListMessages(context.Background(), 1)
			var blocks []struct{ Type, Text string }
			require.NoError(t, json.Unmarshal(msgs[len(msgs)-1].Content, &blocks))
			require.NotEmpty(t, blocks)
			reply := blocks[len(blocks)-1].Text
			for _, w := range c.want {
				assert.Contains(t, reply, w)
			}
			for _, a := range c.absent {
				assert.NotContains(t, reply, a)
			}
		})
	}
}

// A find_ways that found nothing is not told when a write of the same turn
// saved a change: the write's block is the reply.
func TestReplyTurnFinal_WriteBlockWinsOverEmptyWays(t *testing.T) {
	t.Parallel()
	result, err := os.ReadFile("testdata/m3tune/heartier10_find_ways.json")
	require.NoError(t, err)
	write := &replyBlock{facts: []fact{{Class: classChanged, Kind: kindMeal, Status: statusOK, EN: "saved", DE: "gespeichert"}}}
	got := replyTurn{write: true, block: write, ways: waysBlock(result)}.final()
	assert.Same(t, write, got.block)

	got = replyTurn{ways: waysBlock(result)}.final()
	require.NotNil(t, got.block)
	assert.True(t, got.write)
	assert.True(t, got.block.nothingSaved)
}

func TestWaysBlock_NilWhenAWayExists(t *testing.T) {
	t.Parallel()
	assert.Nil(t, waysBlock(json.RawMessage(`{"ways":[{"what":"x"}],"empty_reason":"y"}`)))
	assert.Nil(t, waysBlock(json.RawMessage(`{"ways":[]}`)))
	assert.Nil(t, waysBlock(json.RawMessage(`not json`)))
}

// KR7/G4.1: a regenerate run that ended in error saved nothing; the reply says
// so with the reason (tune2 g-replan-06 told "Der Plan wurde neu gemacht." and
// the lint dropped the model's correct "nichts geändert").
func TestRun_RegenerateErrorIsToldAsNothingSaved(t *testing.T) {
	t.Parallel()
	result, err := os.ReadFile("testdata/m3tune/replan06_regenerate.json")
	require.NoError(t, err)
	ts := minimalToolSet(t, "regenerate", result, ToolEffectNone, nil)
	model := "Ich konnte den Montag nicht neu planen: Die Planung wurde abgebrochen, und deshalb ist nichts geändert worden."
	client := fake.NewFromScript(&fake.Script{Turns: []fake.Turn{scriptToolUseTurn("t1", "regenerate"), scriptEndTurn(model)}})
	req := makeReq(1)
	req.UserText = "Könntest du bitte den Montag noch einmal mit anderen Gerichten planen? Danke."
	req.Tools = ts.Describe()
	repo := &memRepo{}
	require.NoError(t, runLoopRepo(t, req, client, ts, repo))

	msgs, _ := repo.ListMessages(context.Background(), 1)
	var blocks []struct{ Type, Text string }
	require.NoError(t, json.Unmarshal(msgs[len(msgs)-1].Content, &blocks))
	reply := blocks[len(blocks)-1].Text
	assert.NotContains(t, reply, "Der Plan wurde neu gemacht.")
	assert.Contains(t, reply, "- "+nothingChangedDE)
	assert.Contains(t, reply, "- Nichts wurde neu geplant: der neue Plan hätte noch eine Regel des Plans verletzt, deshalb wurde nichts gespeichert.")

	b := savedBlock("regenerate", result)
	require.NotNil(t, b)
	assert.True(t, b.nothingSaved)
	assert.Equal(t, "Nothing was planned again: the new plan would still break a rule of the plan (dish variety), so nothing was saved.", b.facts[0].EN)
}
