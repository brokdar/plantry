package agent_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// oldToolOps maps the removed single-write tools to apply_changes ops, so the
// tests keep their per-write cases.
var oldToolOps = map[string]string{
	"create_plate":           "create_plate",
	"add_food_to_plate":      "add",
	"swap_food":              "swap",
	"update_plate_component": "set_quantity",
	"remove_plate_component": "remove",
	"delete_plate":           "delete_plate",
	"set_session_portions":   "set_portions",
	"set_plate_eaters":       "set_eaters",
	"set_availability":       "set_availability",
}

// applyJSON builds an apply_changes input with one op from the op's fields.
func applyJSON(t testing.TB, op, fields string) string {
	t.Helper()
	m := map[string]any{}
	require.NoError(t, json.Unmarshal([]byte(fields), &m))
	m["op"] = op
	if v, ok := m["new_food_id"]; ok {
		m["food_id"] = v
		delete(m, "new_food_id")
	}
	out, err := json.Marshal(map[string]any{"ops": []any{m}})
	require.NoError(t, err)
	return string(out)
}

// firstApplied returns the first applied op of an apply_changes result,
// with the result's "after" under the key "after".
func firstApplied(t testing.TB, res map[string]any) map[string]any {
	t.Helper()
	applied, ok := res["applied"].([]any)
	require.True(t, ok && len(applied) > 0, "apply_changes result has applied ops: %v", res)
	m := applied[0].(map[string]any)
	m["after"] = res["after"]
	return m
}
