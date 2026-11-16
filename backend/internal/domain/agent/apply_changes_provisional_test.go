package agent

import (
	"bytes"
	"encoding/json"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Ids of rows that no longer exist are stripped wherever they appear: in the
// applied list, the plates and sessions, the issue deltas and an option's op.
// An option whose op used one is marked provisional too; rows that exist keep
// their ids.
func TestMarkProvisional_IssuesAndOptions(t *testing.T) {
	gone := map[string]bool{"plate_id:90": true, "plate_component_id:91": true, "session_id:92": true}
	missing := func(kind string, id int64) bool { return gone[kind+":"+strconv.FormatInt(id, 10)] }

	var after map[string]any
	dec := json.NewDecoder(bytes.NewReader([]byte(`{
	  "plates":[{"plate_id":90,"component_details":[{"plate_component_id":91}]},{"plate_id":5,"component_details":[{"plate_component_id":6}]}],
	  "sessions":[{"session_id":92},{"session_id":7}],
	  "issues":{"new":[
	    {"rule":"S2","plate_id":90,"options":[
	      {"action":"raise","op":{"op":"set_quantity","plate_component_id":91,"portions":2}},
	      {"action":"add","op":{"op":"add","plate_id":90,"food_id":1}},
	      {"action":"add","op":{"op":"add","plate_id":5,"food_id":1}},
	      {"action":"raise","op":{"op":"set_quantity","plate_component_id":6,"portions":2}}]},
	    {"rule":"H3","session_id":92},
	    {"rule":"S3","plate_id":5,"session_id":7}],
	   "resolved":[]}}`)))
	dec.UseNumber() // as planning.Compact decodes
	require.NoError(t, dec.Decode(&after))
	applied := []map[string]any{{"op": "add", "plate_id": int64(5), "plate_component_id": int64(91)}}

	markProvisional(applied, after, missing)

	assert.Equal(t, map[string]any{"op": "add", "plate_id": int64(5), "provisional": true}, applied[0])
	plates := after["plates"].([]any)
	assert.Equal(t, map[string]any{"provisional": true, "component_details": []any{map[string]any{"provisional": true}}}, plates[0])
	assert.Equal(t, map[string]any{"plate_id": json.Number("5"), "component_details": []any{map[string]any{"plate_component_id": json.Number("6")}}}, plates[1])
	sessions := after["sessions"].([]any)
	assert.Equal(t, map[string]any{"provisional": true}, sessions[0])
	assert.Equal(t, map[string]any{"session_id": json.Number("7")}, sessions[1])

	issues := after["issues"].(map[string]any)["new"].([]any)
	s2 := issues[0].(map[string]any)
	assert.Equal(t, true, s2["provisional"])
	assert.NotContains(t, s2, "plate_id")
	opts := s2["options"].([]any)
	for i, wantProvisional := range []bool{true, true, false, false} {
		o := opts[i].(map[string]any)
		if wantProvisional {
			assert.Equal(t, true, o["provisional"], "option %d is marked", i)
			assert.Equal(t, true, o["op"].(map[string]any)["provisional"])
			assert.NotContains(t, o["op"], "plate_id")
			assert.NotContains(t, o["op"], "plate_component_id")
		} else {
			assert.NotContains(t, o, "provisional", "option %d is real", i)
			assert.NotContains(t, o["op"], "provisional")
		}
	}
	assert.Equal(t, map[string]any{"rule": "H3", "provisional": true}, issues[1])
	assert.Equal(t, map[string]any{"rule": "S3", "plate_id": json.Number("5"), "session_id": json.Number("7")}, issues[2])
}
