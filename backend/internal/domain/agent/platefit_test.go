package agent_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/testhelper/testsvc"
)

// PLATE-5: plate fit never enters the shared checker, so apply_changes saves
// a plate that clashes (a Japanese side beside a German main: two cuisines)
// without a refusal, and find_alternatives reads each candidate's plate in
// a phrase (the side stays beside every new German main).
func TestApplyChanges_NeverRefusesForPlateFit(t *testing.T) {
	t.Parallel()
	e := newPlanToolEnv(t)
	require.NoError(t, testsvc.Settings(e.db).Set(context.Background(), "plan.plate_fit", "true"))
	require.NoError(t, testsvc.Settings(e.db).Set(context.Background(), "plan.max_cook_occasions_per_day", "3"))
	attrs := func(id int64, cuisine string) {
		e.sql(`INSERT INTO food_attributes (food_id, version, values_json, sources_json) VALUES (?, 1, ?, '{"cuisine":"user"}')`,
			id, fmt.Sprintf(`{"cuisine":%q}`, cuisine))
	}
	var mains []int64
	rows, err := e.db.Query(`SELECT id FROM foods WHERE role = 'main'`)
	require.NoError(t, err)
	for rows.Next() {
		var id int64
		require.NoError(t, rows.Scan(&id))
		mains = append(mains, id)
	}
	require.NoError(t, rows.Close())
	for _, id := range mains {
		attrs(id, "german")
	}
	side := e.sides[1]
	attrs(side, "japanese")
	p, _ := e.twoDinners()

	res := e.mustExec("apply_changes", fmt.Sprintf(`{"ops":[{"op":"add","plate_id":%d,"food_id":%d,"portions":1}]}`, p.ID, side))

	assert.Len(t, res["applied"], 1)
	got, err := e.plates.Get(context.Background(), p.ID)
	require.NoError(t, err)
	assert.Len(t, got.Components, len(p.Components)+1, "the clashing side is saved")

	alts := e.mustExec("find_alternatives", fmt.Sprintf(`{"plate_id":%d,"what":"main","limit":3}`, p.ID))["alternatives"].([]any)
	require.NotEmpty(t, alts)
	for _, a := range alts {
		assert.Contains(t, a.(map[string]any)["fit"], "clash: mixed cuisines")
	}
}
