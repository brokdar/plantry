package agent_test

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/agent"
	"github.com/jaltszeimer/plantry/backend/internal/domain/window"
)

// R12a B1: a day number of 1-4 digits with its unit is read, a day of no food
// without a number is 0; away, a meal, a unit-less number and sweets are not.
func TestNamedDayKcal_ZeroAndSmallNumbers(t *testing.T) {
	t.Parallel()
	sat := time.Date(2026, 11, 21, 0, 0, 0, 0, time.UTC)
	w := window.Window{Start: sat, End: sat.AddDate(0, 0, 6)}
	for text, want := range map[string]map[string]float64{
		"Samstag 0 kcal":                       {"2026-11-21": 0},
		"Saturday around 0 kcal":               {"2026-11-21": 0},
		"Saturday around 50 kcal":              {"2026-11-21": 50},
		"Samstag etwa 80 Kalorien":             {"2026-11-21": 80},
		"Am Samstag will ich gar nichts essen": {"2026-11-21": 0},
		"I want to eat nothing on Saturday":    {"2026-11-21": 0},

		"Samstag 2 Portionen":                          nil,
		"Saturday I want 2 portions":                   nil,
		"Samstag Abendessen nichts essen":              nil, // a meal, not the day
		"Samstag nichts Süßes essen":                   nil,
		"Ich bin am Samstag unterwegs und esse nichts": nil, // away
		"Saturday I am away, 0 kcal":                   nil,
		"Saturday and Sunday eat nothing":              nil, // two days
		"eat nothing":                                  nil, // no day
	} {
		got := agent.NamedDayKcal(text, w, sat)
		if want == nil {
			assert.Empty(t, got, text)
			continue
		}
		assert.Equal(t, want, got, text)
	}
}

func r12aDinner(t *testing.T, e *planToolEnv) (date time.Time, w window.Window) {
	t.Helper()
	for _, p := range e.draftMeals() {
		if p.SlotID == e.slotID("dinner") {
			d, err := time.Parse(time.DateOnly, p.DateString())
			require.NoError(t, err)
			off := int(d.Weekday()+6) % 7
			return d, window.Window{Start: d.AddDate(0, 0, -off), End: d.AddDate(0, 0, 6-off)}
		}
	}
	t.Fatal("no dinner")
	return
}

// R12a B2: marking the day away carries out "0 kcal" (the day leaves the plan
// and counts as 0): refused. A day the user said they are away stays allowed.
func TestApplyChanges_AwayDoesNotCarryOutZeroKcal(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, text string
		saved      bool
	}{
		{"zero kcal", "%s 0 kcal", false},
		{"eat nothing", "I want to eat nothing on %s", false},
		{"away stated", "I am away on %s", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			e := relEnv(t)
			ctx, conv := e.chat()
			d, w := r12aDinner(t, e)
			text := fmt.Sprintf(tc.text, d.Weekday().String())
			ops := fmt.Sprintf(`{"ops":[{"op":"away","date":%q}]}`, d.Format(time.DateOnly))
			res, _, _, err := e.ts.Execute(agent.WithTurnText(ctx, conv, text, w), "apply_changes", json.RawMessage(ops))
			require.NoError(t, err)
			var got map[string]any
			require.NoError(t, json.Unmarshal(res, &got))
			if tc.saved {
				assert.NotEqual(t, false, got["saved"], "%s", res)
				return
			}
			assert.Equal(t, false, got["saved"], "%s", res)
			skipped := relMaps(got["skipped"])
			require.Len(t, skipped, 1, "%s", res)
			assert.Contains(t, skipped[0]["en"], "is below the day's minimum of")
			assert.NotContains(t, fmt.Sprint(got["next"]), "away", "the refusal never offers an away mark: %s", res)
		})
	}
}

// R12a B3: find_ways takes a target of 0 and answers with the floor refusal.
func TestFindWays_ZeroTargetIsAFloorRefusal(t *testing.T) {
	t.Parallel()
	e := relEnv(t)
	d, _ := r12aDinner(t, e)
	res := e.mustExec("find_ways", fmt.Sprintf(`{"goal":"lower_day","date":%q,"target_kcal":0}`, d.Format(time.DateOnly)))
	assert.Contains(t, fmt.Sprint(res["empty_reason"]), "0 kcal is below the day's minimum of", "%v", res)
	assert.Empty(t, relMaps(res["ways"]))
}
