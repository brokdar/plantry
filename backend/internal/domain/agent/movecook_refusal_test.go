package agent_test

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// V81 (tune-v2c g2-cook_elsewhere-07, h2-cookref-00): a move_cook the plate
// service refuses (a meal of the batch comes before the new cooking time) came
// back as a tool error, so the reply had no code line and the model's prose
// missed "nothing changed" or the reason. It is now a refused result with the
// "Left out: ..." line naming the meals that block it.
func TestApplyChanges_MoveCookAfterItsMealsIsARefusal(t *testing.T) {
	t.Parallel()
	e := relEnv(t)
	ctx, _ := e.chat()
	gp := e.mustExec("get_plan", `{}`)
	var sid float64
	var last string
	for _, s := range gp["batches"].([]any) {
		m := s.(map[string]any)
		if links, _ := m["links"].([]any); len(links) > 0 && sid == 0 && m["cooked"] != true {
			sid = m["id"].(float64)
		}
	}
	for _, c := range gp["cells"].([]any) {
		if d := c.(map[string]any)["date"].(string); d > last {
			last = d
		}
	}
	require.NotZero(t, sid)
	in := fmt.Sprintf(`{"ops":[{"op":"move_cook","session_id":%.0f,"cook_at":{"date":%q,"slot_id":%d}}]}`, sid, last, e.slotID("dinner"))
	res, _, _, err := e.ts.Execute(ctx, "apply_changes", json.RawMessage(in))
	require.NoError(t, err, "a refused move is a result, not a tool error")
	var got map[string]any
	require.NoError(t, json.Unmarshal(res, &got))
	assert.Equal(t, false, got["saved"])
	skipped := relMaps(got["skipped"])
	require.Len(t, skipped, 1)
	assert.Regexp(t, `^Left out: cooking ".+" on \w{3} \d+ \w{3} dinner \(.+ eat it before then\)\.$`, skipped[0]["en"])
	assert.Contains(t, skipped[0]["de"], "essen schon vorher davon")
}
