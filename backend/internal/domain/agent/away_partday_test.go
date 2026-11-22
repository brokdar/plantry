package agent_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// SB3/SF4/SF1(c): a day with a meal left (here a routine breakfast of 306
// kcal) is judged on its planned part, not "not counted": the facts say the
// day is under its prorated minimum, with the number. Stored tune2 g-away-08:
// Thursday lunch and dinner away on the M3 dev plan; the reply said "the day
// is not counted".
func TestAway_PartAwayDayIsJudgedOnItsPlannedPart(t *testing.T) {
	t.Parallel()
	e := replayEnvAt(t, m3DevStartDB, time.Date(2026, 11, 21, 0, 0, 0, 0, time.UTC))
	ctx, _ := e.chat()
	out, _, _, err := e.ts.Execute(ctx, "apply_changes", json.RawMessage(`{"ops": [{"op":"away","cells":[{"date":"2026-11-26","slot_id":2},{"date":"2026-11-26","slot_id":3}],"note":"Not home for lunch and dinner"}]}`))
	require.NoError(t, err)
	var res struct {
		Report struct {
			Facts []struct{ EN string } `json:"facts"`
		} `json:"report"`
	}
	require.NoError(t, json.Unmarshal(out, &res))
	var lines []string
	for _, f := range res.Report.Facts {
		lines = append(lines, f.EN)
	}
	all := strings.Join(lines, "\n")
	assert.NotContains(t, all, "not counted", "a day with a breakfast left is judged")
	assert.Contains(t, all, "Thu 26 Nov holds 306 kcal")
	assert.Contains(t, all, "minimum of 446 kcal")

	// The stored light mark of the away lunch goes with it (an away meal
	// carries no mark, so no RL9 offer leans on it).
	var marks int
	require.NoError(t, e.db.QueryRow(`SELECT COUNT(*) FROM plan_light WHERE date = '2026-11-26' AND slot = 'lunch'`).Scan(&marks))
	assert.Zero(t, marks)
}

// A day with every meal away, the routine breakfast too, stays unjudged (SB3).
func TestAway_FullyAwayDayIsNotCounted(t *testing.T) {
	t.Parallel()
	e := replayEnvAt(t, m3DevStartDB, time.Date(2026, 11, 21, 0, 0, 0, 0, time.UTC))
	ctx, _ := e.chat()
	out, _, _, err := e.ts.Execute(ctx, "apply_changes", json.RawMessage(`{"ops": [{"op":"away","cells":[{"date":"2026-11-26","slot_id":1},{"date":"2026-11-26","slot_id":2},{"date":"2026-11-26","slot_id":3}]}]}`))
	require.NoError(t, err)
	assert.NotContains(t, string(out), "day energy minimum", "a fully away day has no minimum")
	assert.NotContains(t, string(out), "under the day's minimum")
}
