package agent_test

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// E048c: de-lighter-wed on the real start plan. The light dinner is a compound
// offer that makes the lunch bigger; the lunch was light-marked, and the mark
// stayed on a lunch of 858 kcal (a light lunch holds 550), so no day fix
// existed. f4bd01b clears the mark with the offer; fca42e7 also tries a
// vegetable side plus bread, so the day gets its one fix.
func e048cOffer(t *testing.T) (e *planToolEnv, res map[string]any, offer map[string]any) {
	t.Helper()
	e = replayEnv(t, chatStartDB)
	ctx, _ := e.chat()
	var plateID int64
	require.NoError(t, e.db.QueryRow(`SELECT id FROM plates WHERE date = '2026-11-25' AND slot_id = 3`).Scan(&plateID))
	out, _, _, err := e.ts.Execute(ctx, "find_ways", json.RawMessage(fmt.Sprintf(`{"goal":"lighter","plate_id":%d}`, plateID)))
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(out, &res))
	offers := relMaps(res["offer"])
	require.NotEmpty(t, offers)
	return e, res, offers[0]
}

// The two checks read the same offer: it is computed once (the search is the
// test's cost) and each check is a subtest of it.
func TestReplayE048c(t *testing.T) {
	t.Parallel()
	e, res, o := e048cOffer(t)
	t.Run("the offer clears the lunch mark it grows", func(t *testing.T) { e048cClearsTheMark(t, o) })
	t.Run("the day gets one bread fix inside its range", func(t *testing.T) { e048cFixInsideRange(t, e, res, o) })
}

func e048cClearsTheMark(t *testing.T, o map[string]any) {
	assert.Equal(t, true, o["recommended"])
	var cleared bool
	lights, _ := o["light"].([]any)
	for _, x := range lights {
		if l, _ := x.(map[string]any); l["slot"] == "lunch" && l["scope"] == "none" {
			cleared = true
		}
	}
	assert.True(t, cleared, "the lunch the offer grows is no longer light: %v", o["light"])
	issues, _ := o["new_issues"].([]any)
	for _, x := range issues {
		n, _ := x.(map[string]any)
		assert.NotEqual(t, "light meal", n["label"], "no warning for a light mark that is gone")
	}
}

func e048cFixInsideRange(t *testing.T, e *planToolEnv, res, o map[string]any) {
	fix, ok := o["fix"].(map[string]any)
	if !ok {
		// v1.2 (SB5): a routine breakfast far under its share moves the day's
		// expected kcal up for the other meals, so the offer can end the day
		// inside its range with nothing left to fix.
		assert.Equal(t, "inside its range", o["day_vs_range"])
		return
	}
	// v1.1: the slot bands move the day target, so the fix is the one the new
	// shares call for (a banana, 1902 kcal); what stays is that it is one normal
	// food and that its promise equals the dry run below.
	assert.Contains(t, []string{"Ciabatta", "Banana"}, fix["name"])
	want := fix["day_kcal_after"]

	// The promise is the dry run of the offer's ops plus the fix's.
	ctx, _ := e.chat()
	ops := append(o["ops"].([]any), fix["ops"].([]any)...)
	raw, err := json.Marshal(map[string]any{"ops": ops, "goal": res["goal"], "light": o["light"], "dry_run": true})
	require.NoError(t, err)
	out, _, _, err := e.ts.Execute(ctx, "apply_changes", raw)
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal(out, &got))
	assert.InDelta(t, want, dayKcalOf(got, "2026-11-25"), 1)
	for _, is := range relMaps(got["after"].(map[string]any)["issues"].(map[string]any)["new"]) {
		assert.NotEqual(t, "problem", is["severity"])
	}
}
