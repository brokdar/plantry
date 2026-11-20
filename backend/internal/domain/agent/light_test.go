package agent_test

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
)

// A light request given to apply_changes is part of the evaluation of that
// call: the touched day says it is light (and records it in the run log's
// dry-run step); a malformed one is refused before anything runs.
func TestApplyChanges_LightRequest(t *testing.T) {
	t.Parallel()
	e := newPlanToolEnv(t)
	ctx, _ := e.chat()
	p, _ := e.freshMain()

	input := fmt.Sprintf(`{"dry_run":true,"light":[{"date":%q,"scope":"day"}],"ops":[{"op":"set_eaters","plate_id":%d,"eaters":2}]}`, p.DateString(), p.ID)
	out, _, _, err := e.ts.Execute(ctx, "apply_changes", json.RawMessage(input))
	require.NoError(t, err)
	var res map[string]any
	require.NoError(t, json.Unmarshal(out, &res))
	var light string
	for _, d := range res["after"].(map[string]any)["days"].([]any) {
		if day := d.(map[string]any); day["date"] == p.DateString() {
			light, _ = day["light"].(string)
		}
	}
	assert.Equal(t, "day", light)

	bad := fmt.Sprintf(`{"dry_run":true,"light":[{"date":%q,"scope":"meal"}],"ops":[{"op":"set_eaters","plate_id":%d,"eaters":2}]}`, p.DateString(), p.ID)
	_, _, _, err = e.ts.Execute(ctx, "apply_changes", json.RawMessage(bad))
	assert.ErrorIs(t, err, domain.ErrInvalidInput, "a meal request needs a slot")
}
