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

// m3VgStartDB is the M3 vg household's frozen start database (1700 kcal, lose).
const m3VgStartDB = "testdata/m3_vg_start.db"

// KR6 and the rule order (the day band ranks above protein): on the stored
// tune2 g-protein-02 plan, Friday already sat over its range top (1868 of 1785
// kcal) and find_ways told the model to add 130 g of quark, taking the day to
// 1955 kcal. A more_protein way never takes a day further over its top.
func TestFindWays_MoreProteinNeverTakesTheDayFurtherOverItsTop(t *testing.T) {
	t.Parallel()
	e := replayEnvAt(t, m3VgStartDB, time.Date(2026, 12, 5, 0, 0, 0, 0, time.UTC))
	ctx, _ := e.chat()
	out, _, _, err := e.ts.Execute(ctx, "find_ways", json.RawMessage(`{"goal":"more_protein","date":"2026-12-11"}`))
	require.NoError(t, err)
	var res map[string]any
	require.NoError(t, json.Unmarshal(out, &res))
	before := res["before"].(map[string]any)["day_kcal"].(float64)
	top := res["range_high"].(float64)
	require.Greater(t, before, top, "the stored Friday is over its top: %s", out)
	for _, k := range []string{"ways", "offer"} {
		if res[k] == nil {
			continue
		}
		for _, w := range relMaps(res[k]) {
			after := w["after"].(map[string]any)["day_kcal"].(float64)
			assert.LessOrEqual(t, after, before, "%s %v raises a day over its top", k, w["what"])
		}
	}
}
