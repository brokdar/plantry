package agent_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// G4/SB3 (V68): a deleted plate is no meal out. The day already has a meal
// out (Thursday lunch none, so it holds assumed energy), and delete_plate on
// the planned dinner keeps "no meal any more" for the dinner: never "dinner
// out (about N kcal assumed)", which the spectest case newuser
// delete_plate/plate_87 told although the cell stays planned.
func TestDeletePlate_IsNoMealOut(t *testing.T) {
	t.Parallel()
	e := replayEnvAt(t, m3DevStartDB, time.Date(2026, 11, 21, 0, 0, 0, 0, time.UTC))
	_, err := e.db.Exec(`INSERT INTO availability_overrides (date, slot_id, mode) VALUES ('2026-11-26', 2, 'none')
		ON CONFLICT (date, slot_id) DO UPDATE SET mode = 'none', preset_id = NULL`)
	require.NoError(t, err)
	_, err = e.db.Exec(`DELETE FROM plates WHERE date = '2026-11-26' AND slot_id = 2`)
	require.NoError(t, err)
	var dinner int64
	require.NoError(t, e.db.QueryRow(`SELECT id FROM plates WHERE date = '2026-11-26' AND slot_id = 3`).Scan(&dinner))

	ctx, _ := e.chat()
	out, _, _, err := e.ts.Execute(ctx, "apply_changes", json.RawMessage(`{"ops":[{"op":"delete_plate","plate_id":`+jsonInt(dinner)+`}]}`))
	require.NoError(t, err)
	var res struct {
		Report struct {
			Facts []struct{ EN, DE string } `json:"facts"`
		} `json:"report"`
	}
	require.NoError(t, json.Unmarshal(out, &res))
	var en, de []string
	for _, f := range res.Report.Facts {
		en, de = append(en, f.EN), append(de, f.DE)
	}
	all, allDE := strings.Join(en, "\n"), strings.Join(de, "\n")
	assert.Contains(t, all, "Thu 26 Nov dinner: no meal any more.")
	assert.NotContains(t, all, " out ")
	assert.NotContains(t, all, "assumed")
	assert.NotContains(t, allDE, "außer Haus")
	assert.NotContains(t, allDE, "angenommen")
	assert.Contains(t, allDE, "kein Essen mehr")
}

func jsonInt(n int64) string { b, _ := json.Marshal(n); return string(b) }
