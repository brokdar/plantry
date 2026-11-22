package agent_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// m3DevStartDB is the M3 dev household's frozen start database (tune runs),
// slimmed like chat_start.db.
const m3DevStartDB = "testdata/m3_dev_start.db"

// RL2/KR5: a heartier way grows the main, a starch or a protein food. On the
// stored tune2 g-heartier-00 plan, find_ways recommended swapping Monday
// lunch's main for Spinach lasagne and, since the couscous no longer fit the
// pasta, "swapped Couscous for Carrot salad": a vegetable side came in. Such a
// way is dropped now.
func TestFindWays_HeartierNeverBringsInAVegetableSide(t *testing.T) {
	t.Parallel()
	e := replayEnvAt(t, m3DevStartDB, time.Date(2026, 12, 5, 0, 0, 0, 0, time.UTC))
	ctx, _ := e.chat()
	out, _, _, err := e.ts.Execute(ctx, "find_ways", json.RawMessage(`{"goal":"heartier","plate_id":111}`))
	require.NoError(t, err)
	var res map[string]any
	require.NoError(t, json.Unmarshal(out, &res))
	require.NotEmpty(t, res["ways"], "the meal can get heartier: %s", out)
	for _, w := range relMaps(res["ways"]) {
		for _, op := range relMaps(w["ops"]) {
			id, ok := op["food_id"].(float64)
			if !ok {
				continue
			}
			var role string
			require.NoError(t, e.db.QueryRow(`SELECT COALESCE(role,'') FROM foods WHERE id = ?`, int64(id)).Scan(&role))
			assert.NotEqual(t, "side_veg", role, "a heartier way brings in a vegetable side: %v", w["what"])
		}
	}
}
