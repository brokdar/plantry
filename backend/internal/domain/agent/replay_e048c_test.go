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

func TestReplayE048c_TheOfferClearsTheLunchMarkItGrows(t *testing.T) {
	t.Parallel()
	_, _, o := e048cOffer(t)
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

func TestReplayE048c_TheDayGetsOneBreadFixInsideItsRange(t *testing.T) {
	t.Parallel()
	e, res, o := e048cOffer(t)
	fix, ok := o["fix"].(map[string]any)
	require.True(t, ok, "the offer ends the day under its range: it carries the fix (%v)", o["day_vs_range"])
	assert.Equal(t, "Ciabatta", fix["name"])
	assert.EqualValues(t, 1934, fix["day_kcal_after"])

	// The promise is the dry run of the offer's ops plus the fix's.
	ctx, _ := e.chat()
	ops := append(o["ops"].([]any), fix["ops"].([]any)...)
	raw, err := json.Marshal(map[string]any{"ops": ops, "goal": res["goal"], "light": o["light"], "dry_run": true})
	require.NoError(t, err)
	out, _, _, err := e.ts.Execute(ctx, "apply_changes", raw)
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal(out, &got))
	assert.InDelta(t, 1934, dayKcalOf(got, "2026-11-25"), 1)
	for _, is := range relMaps(got["after"].(map[string]any)["issues"].(map[string]any)["new"]) {
		assert.NotEqual(t, "problem", is["severity"])
	}
}
