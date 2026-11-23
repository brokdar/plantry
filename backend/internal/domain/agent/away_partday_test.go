package agent_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// spec v1.5 SB3 (V63 c, replacing #172's "judged on its planned part"): a day
// whose only meal left is a routine (here the 306 kcal routine breakfast) is
// judged like a fully away day: no minimum, no number, no offer. Stored tune2
// g-away-08: Thursday lunch and dinner away on the M3 dev plan.
func TestAway_RoutineOnlyDayIsNotJudged(t *testing.T) {
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
	assert.Contains(t, all, "Thu 26 Nov lunch: no meal any more.")
	assert.Contains(t, all, "Thu 26 Nov dinner: no meal any more.")
	assert.NotContains(t, all, "minimum", "a routine-only day has no minimum")
	assert.NotContains(t, all, "306 kcal", "and is not judged by its kcal")

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
	var res struct {
		Saved  bool `json:"saved"`
		Report struct {
			Facts []struct{ EN string } `json:"facts"`
		} `json:"report"`
	}
	require.NoError(t, json.Unmarshal(out, &res))
	assert.True(t, res.Saved, "the away op is saved")
	var lines []string
	for _, f := range res.Report.Facts {
		lines = append(lines, f.EN)
	}
	facts := strings.Join(lines, "\n")
	for _, want := range []string{"Thu 26 Nov breakfast: no meal any more.", "Thu 26 Nov lunch: no meal any more.", "Thu 26 Nov dinner: no meal any more.", "Thu 26 Nov: nothing is cooked."} {
		assert.Contains(t, facts, want)
	}
	assert.NotContains(t, facts, "minimum", "a fully away day has no minimum")
	assert.NotContains(t, facts, "not counted")
}

// SF1(c)/P-1 (tune3 g-away-08, with Thursday's breakfast a confirmed normal
// planned meal instead of a routine, V63 c): the away leaves Thursday under its
// prorated minimum and no checked change lifts it, so the block tells the day
// under its minimum (a must line, never cut) and, in code, that nothing lifts
// it, with no offer.
func TestAway_DayUnderItsMinimumWithNoLeverIsToldInCode(t *testing.T) {
	t.Parallel()
	e := replayEnvAt(t, m3DevStartDB, time.Date(2026, 11, 21, 0, 0, 0, 0, time.UTC))
	// A normal planned breakfast the user confirmed: the day is judged, and no
	// change of a planned meal (the agent never edits a confirmed one) lifts it.
	_, err := e.db.Exec(`INSERT INTO availability_overrides (date, slot_id, mode) VALUES ('2026-11-26', 1, 'plan')`)
	require.NoError(t, err)
	_, err = e.db.Exec(`UPDATE plates SET state = 'confirmed' WHERE date = '2026-11-26' AND slot_id = 1`)
	require.NoError(t, err)
	ctx, _ := e.chat()
	out, _, _, err := e.ts.Execute(ctx, "apply_changes", json.RawMessage(`{"ops": [{"op":"away","cells":[{"date":"2026-11-26","slot_id":2},{"date":"2026-11-26","slot_id":3}]}]}`))
	require.NoError(t, err)
	var res struct {
		Offers []any `json:"offers"`
		Report struct {
			Offer *struct{ EN string } `json:"offer"`
			Facts []struct {
				EN   string
				Must bool
			} `json:"facts"`
		} `json:"report"`
	}
	require.NoError(t, json.Unmarshal(out, &res))
	assert.Empty(t, res.Offers, "no lever: no offer")
	assert.Nil(t, res.Report.Offer, "no lever: the reply carries no offer at all")
	var day, noLever bool
	for _, f := range res.Report.Facts {
		if strings.Contains(f.EN, "under the day's minimum of 446 kcal") {
			day = f.Must
		}
		if strings.Contains(f.EN, "under the day's minimum of 446 kcal") && strings.HasSuffix(f.EN, " No checked change of a planned meal brings Thu 26 Nov up to its minimum.") {
			noLever = f.Must
		}
	}
	assert.True(t, day, "the day under its minimum is a must line: %+v", res.Report.Facts)
	assert.True(t, noLever, "P-1's plain no-lever sentence is merged into the day's line: %+v", res.Report.Facts)
	for _, f := range res.Report.Facts {
		assert.False(t, strings.HasPrefix(f.EN, "No checked change"), "one line per day: %+v", res.Report.Facts)
	}
}
