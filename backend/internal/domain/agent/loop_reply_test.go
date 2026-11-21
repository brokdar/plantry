package agent

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/fake"
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
