package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/fake"
	"github.com/jaltszeimer/plantry/backend/internal/domain"
)

// G3/G4: the stored final reply of a write turn is the model's framing, the
// facts block of the saved change and the one offer, rendered by code in the
// language of the request; the model never sees the block or the offer.
func TestRun_FinalReplyIsComposedByCode(t *testing.T) {
	t.Parallel()
	result := json.RawMessage(`{"dry_run":false,"applied":[],"report":{"tell":["Fri 27 Nov dinner: A replaced by B."],
		"facts":[{"class":2,"kind":"meal","status":"ok","en":"Fri 27 Nov dinner: A replaced by B.","de":"Freitag, 27. Nov. Abendessen: A ersetzt durch B."}],
		"offer":{"en":"I can also add 120 g of Bread: say so if you want it.","de":"Ich kann auch 120 g Brot hinzufügen: sag Bescheid, wenn du das möchtest."}}}`)
	for _, c := range []struct{ name, user, model, want string }{
		{"english", "swap the dinner", "Done. Anything else?", "Done.\n\n- Fri 27 Nov dinner: A replaced by B.\n\nI can also add 120 g of Bread: say so if you want it."},
		{"german, text with a digit is dropped", "tausche das Abendessen", "Erledigt, das sind 1800 kcal.", "- Freitag, 27. Nov. Abendessen: A ersetzt durch B.\n\nIch kann auch 120 g Brot hinzufügen: sag Bescheid, wenn du das möchtest."},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			ts := minimalToolSet(t, "apply_changes", result, ToolEffectNone, nil)
			client := fake.NewFromScript(&fake.Script{Turns: []fake.Turn{scriptToolUseTurn("t1", "apply_changes"), scriptEndTurn(c.model)}})
			req := makeReq(1)
			req.UserText = c.user
			req.Tools = ts.Describe()
			repo := &memRepo{}
			require.NoError(t, runLoopRepo(t, req, client, ts, repo))

			msgs, _ := repo.ListMessages(context.Background(), 1)
			var blocks []struct{ Type, Text string }
			require.NoError(t, json.Unmarshal(msgs[len(msgs)-1].Content, &blocks))
			require.NotEmpty(t, blocks)
			assert.Equal(t, c.want, blocks[len(blocks)-1].Text)

			// the model read the tell, not the block nor the offer
			reqs := client.Requests()
			last := reqs[len(reqs)-1].Messages
			seen := string(last[len(last)-1].Content[0].ToolResultContent)
			assert.Contains(t, seen, `"tell"`)
			assert.NotContains(t, seen, `"facts"`)
			assert.NotContains(t, seen, `"offer"`)
		})
	}
}

// H1, round 7 R2: the unmet block reaches the stored reply after the model's
// text on a turn that saved nothing, unless its text ends in a question.
func TestRun_UnmetBlockFollowsTheModelsText(t *testing.T) {
	t.Parallel()
	unmet := &replyBlock{nothingSaved: true, facts: []fact{{Class: classRefused, Kind: kindRefused, Status: statusOK, EN: "Left out: the change to Mon 23 Nov dinner (x).", DE: "Weggelassen: x."}}}
	for _, c := range []struct {
		name, tool, result, model, want string
		input                           string // the tool call's input, {} when empty
	}{
		{"no tool", "", "", "That does not fit.", "That does not fit.\n\n- " + nothingChangedEN + "\n- Left out: the change to Mon 23 Nov dinner (x).", ""},
		{"lookup found nothing", "find_foods", `{"items":[],"total":0}`, "No such dish.", "No such dish.\n\n- " + nothingChangedEN + "\n- Left out: the change to Mon 23 Nov dinner (x).", `{"search":"beef goulash"}`},
		{"an empty filter search: told as a look-up", "find_foods", `{"items":[],"total":0}`, "No such dish.", "No such dish.\n\n- " + nothingChangedEN + "\n- Left out: the change to Mon 23 Nov dinner (x).", `{"roles":["main"]}`},
		{"lookup found a dish", "find_foods", `{"items":[{"id":1}],"total":1}`, "Here it is.", "Here it is.\n\n- " + nothingChangedEN + "\n- Left out: the change to Mon 23 Nov dinner (x).", ""},
		{"a question stands", "", "", "Which one do you want?", "Which one do you want?", ""},
		{"a question stands after a read tool", "get_plan", `{"days":[]}`, "Which one do you want?", "Which one do you want?", ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			var turns []fake.Turn
			ts := minimalToolSet(t, "find_foods", json.RawMessage(`{}`), ToolEffectNone, nil)
			if c.tool != "" {
				ts = minimalToolSet(t, c.tool, json.RawMessage(c.result), ToolEffectNone, nil)
				turn := scriptToolUseTurn("t1", c.tool)
				if c.input != "" {
					turn.Message.Content[0].ToolInput = json.RawMessage(c.input)
				}
				turns = append(turns, turn)
			}
			turns = append(turns, scriptEndTurn(c.model))
			client := fake.NewFromScript(&fake.Script{Turns: turns})
			req := makeReq(1)
			req.UserText = "put the Beef goulash on Monday dinner"
			req.unmet = unmet
			req.Tools = ts.Describe()
			repo := &memRepo{}
			require.NoError(t, runLoopRepo(t, req, client, ts, repo))
			msgs, _ := repo.ListMessages(context.Background(), 1)
			var blocks []struct{ Type, Text string }
			require.NoError(t, json.Unmarshal(msgs[len(msgs)-1].Content, &blocks))
			require.NotEmpty(t, blocks)
			assert.Equal(t, c.want, blocks[len(blocks)-1].Text)
		})
	}
}

// Round 7 R2 (tune-v2c g2-cook_elsewhere-07 on 347b003): a write the program
// refused with an error saved nothing, and the reply said so only when the
// model did. The reply now tells it in code, in the user's language, also
// after the model's offer question; the model's sentence in the other language
// goes (R1).
func TestRun_RefusedWriteSaysNothingChanged(t *testing.T) {
	t.Parallel()
	unmet := &replyBlock{nothingSaved: true, facts: []fact{{
		Class: classRefused, Kind: kindRefused, Status: statusOK,
		EN: "Left out: the change to Wed 2 Dec lunch (" + unmetWhyEN + ").", DE: "Weggelassen: die Änderung an Mittwoch, 2. Dez. Mittagessen (" + unmetWhyDE + ").",
	}}}
	for _, c := range []struct {
		name, user, model, want string
		unmet                   *replyBlock
	}{
		{"EN, a named meal", "cook the gratin on Wednesday lunch instead", "That does not work.", "That does not work.\n\n- " + nothingChangedEN + "\n- Left out: the change to Wed 2 Dec lunch (the program refused it).", unmet},
		{"DE, after an offer question the question stands alone", "Koch den Gratin stattdessen am Mittwochmittag", "Das geht so nicht. Soll ich ihn verschieben?", "Das geht so nicht. Soll ich ihn verschieben?", unmet},
		{"DE, a named meal, no question", "Koch den Gratin stattdessen am Mittwochmittag", "Das geht so nicht.", "Das geht so nicht.\n\n- " + nothingChangedDE + "\n- Weggelassen: die Änderung an Mittwoch, 2. Dez. Mittagessen (das Programm hat sie abgelehnt).", unmet},
		{"no change was asked for: no Left-out line", "Koch den Gratin bitte früher", "Das geht so nicht mit den Kochzeiten.", "Das geht so nicht mit den Kochzeiten.", nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			ts := minimalToolSet(t, "apply_changes", nil, ToolEffectNone, fmt.Errorf("op 1 of 1 (move_cook): %w: plate 134 comes before the cook", domain.ErrInvalidInput))
			client := fake.NewFromScript(&fake.Script{Turns: []fake.Turn{scriptToolUseTurn("t1", "apply_changes"), scriptEndTurn(c.model)}})
			req := makeReq(1)
			req.UserText = c.user
			req.unmet = c.unmet
			req.Tools = ts.Describe()
			repo := &memRepo{}
			require.NoError(t, runLoopRepo(t, req, client, ts, repo))
			msgs, _ := repo.ListMessages(context.Background(), 1)
			var blocks []struct{ Type, Text string }
			require.NoError(t, json.Unmarshal(msgs[len(msgs)-1].Content, &blocks))
			require.NotEmpty(t, blocks)
			assert.Equal(t, c.want, blocks[len(blocks)-1].Text)
		})
	}
}
