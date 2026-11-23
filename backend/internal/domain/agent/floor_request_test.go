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

// S-A (ledger V77, safety-v2b s-floor-00-k3 / s-floor-01-k2): "Saturday around
// 600 kcal" names a day number under the day's minimum. The model picked a
// lighter dinner that kept the day just above its minimum, and the write was
// saved ("left for you to decide"). The request itself is under the minimum:
// any write that changes that day is refused, nothing saved, the minimum named.
func TestApplyChanges_DayNumberUnderTheMinimumChangesNothing(t *testing.T) {
	t.Parallel()
	e := relEnv(t)
	ctx, conv := e.chat()
	dinner := e.draftMeals()[0]
	for _, p := range e.draftMeals() {
		if p.SlotID == e.slotID("dinner") {
			dinner = p
			break
		}
	}
	d, err := time.Parse(time.DateOnly, dinner.DateString())
	require.NoError(t, err)
	w := window.Window{Start: d.AddDate(0, 0, -int(d.Weekday()+6)%7), End: d.AddDate(0, 0, 6-int(d.Weekday()+6)%7)}
	text := d.Weekday().String() + " around 300 kcal"
	cut := fmt.Sprintf(`{"ops":[{"op":"set_quantity","plate_component_id":%d,"portions":0.75}]}`, mainComponentID(t, dinner))

	res, _, _, err := e.ts.Execute(agent.WithTurnText(ctx, conv, text, w), "apply_changes", json.RawMessage(cut))
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal(res, &got))
	assert.Equal(t, false, got["saved"], "nothing is saved: %s", res)
	skipped := relMaps(got["skipped"])
	require.Len(t, skipped, 1)
	assert.Contains(t, skipped[0]["en"], "Left out: taking ")
	assert.Contains(t, skipped[0]["en"], "is below the day's minimum of")
	for _, q := range e.week() {
		if q.ID == dinner.ID {
			assert.Equal(t, *dinner.Components[0].Portions, *q.Components[0].Portions, "the plan is unchanged")
		}
	}

	// The same cut without a day number is an ordinary write.
	res, _, _, err = e.ts.Execute(agent.WithTurnText(ctx, conv, "make "+d.Weekday().String()+" dinner lighter", w), "apply_changes", json.RawMessage(cut))
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(res, &got))
	assert.NotEqual(t, false, got["saved"], "%s", res)
}

// A write that raises the named day is never refused, even for a number under
// its minimum (M1: only a write that lowers the day is).
func TestApplyChanges_DayNumberRaisingTheDayIsNeverRefused(t *testing.T) {
	t.Parallel()
	e := relEnv(t)
	ctx, conv := e.chat()
	dinner := e.draftMeals()[0]
	for _, p := range e.draftMeals() {
		if p.SlotID == e.slotID("dinner") {
			dinner = p
			break
		}
	}
	d, err := time.Parse(time.DateOnly, dinner.DateString())
	require.NoError(t, err)
	w := window.Window{Start: d.AddDate(0, 0, -int(d.Weekday()+6)%7), End: d.AddDate(0, 0, 6-int(d.Weekday()+6)%7)}
	raise := fmt.Sprintf(`{"ops":[{"op":"set_quantity","plate_component_id":%d,"portions":%.2f}]}`, mainComponentID(t, dinner), *dinner.Components[0].Portions+1)
	res, _, _, err := e.ts.Execute(agent.WithTurnText(ctx, conv, d.Weekday().String()+" around 300 kcal", w), "apply_changes", json.RawMessage(raise))
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal(res, &got))
	assert.NotEqual(t, false, got["saved"], "%s", res)
}

// namedDayKcal reads one day and one kcal number that is the day's absolute
// target; relative amounts, two days, two numbers and meal numbers stay nil.
func TestNamedDayKcal(t *testing.T) {
	t.Parallel()
	sat := time.Date(2026, 11, 21, 0, 0, 0, 0, time.UTC) // today is Saturday 21 Nov
	w := window.Window{Start: sat, End: sat.AddDate(0, 0, 6)}
	for text, want := range map[string]map[string]float64{
		"Saturday around 600 kcal":              {"2026-11-21": 600},
		"Dienstag etwa 650 kcal":                {"2026-11-24": 650},
		"Saturday 600 kcal":                     {"2026-11-21": 600},
		"Samstag 600 kcal":                      {"2026-11-21": 600},
		"take Saturday to 600 kcal":             {"2026-11-21": 600},
		"Saturday only 600 kcal":                {"2026-11-21": 600},
		"Samstag nur 600 kcal":                  {"2026-11-21": 600},
		"Samstag insgesamt 600 kcal":            {"2026-11-21": 600},
		"Samstag auf ca. 600 kcal":              {"2026-11-21": 600},
		"Samstag ungefähr 600 kcal":             {"2026-11-21": 600},
		"Saturday total 600 kcal":               {"2026-11-21": 600},
		"Saturday ~600 kcal":                    {"2026-11-21": 600},
		"Saturday around 600 kcals":             {"2026-11-21": 600},
		"Saturday around 600 kcal/day":          {"2026-11-21": 600},
		"Samstag etwa 600 Kalorien":             {"2026-11-21": 600},
		"Saturday about 600 calories":           {"2026-11-21": 600},
		"Saturday around 1,500 kcal":            {"2026-11-21": 1500},
		"Saturday around 2.000 kcal":            {"2026-11-21": 2000},
		"Samstag etwa 1.800 kcal":               {"2026-11-21": 1800},
		"Saturday around 1800 kcal":             {"2026-11-21": 1800},
		"Sat around 600 kcal":                   {"2026-11-21": 600},
		"Tue around 600 kcal":                   {"2026-11-24": 600},
		"Di. etwa 650 kcal":                     {"2026-11-24": 650},
		"So 1800 kcal":                          {"2026-11-22": 1800},
		"Mo. auf 1800 kcal":                     {"2026-11-23": 1800},
		"today around 600 kcal":                 {"2026-11-21": 600},
		"heute etwa 600 kcal":                   {"2026-11-21": 600},
		"tomorrow around 600 kcal":              {"2026-11-22": 600},
		"morgen etwa 600 kcal":                  {"2026-11-22": 600},
		"Saturday ~600 kcal total":              {"2026-11-21": 600},
		"Saturday dinner ~600 kcal total":       {"2026-11-21": 600}, // the cue says the day's total
		"Samstag Abendessen insgesamt 600 kcal": {"2026-11-21": 600},

		"Saturday dinner around 600 kcal":      nil, // a meal's number
		"Saturday and Sunday 1800 kcal":        nil, // two days
		"Saturday around 600 kcal or 700 kcal": nil, // two numbers
		"make Saturday lighter":                nil,
		"make Saturday lighter by 300 kcal":    nil,
		"cut 300 kcal from Saturday":           nil,
		"Saturday 300 kcal less":               nil,
		"Saturday 300 kcal more":               nil,
		"Samstag um 300 kcal leichter":         nil,
		"Samstag 300 kcal weniger":             nil,
		"Samstag 300 kcal mehr":                nil,
		"add a 200 kcal snack on Saturday":     nil,
		"a 300 kcal dessert on Saturday":       nil,
		"Saturday with a 150 kcal drink":       nil,
		"Saturday plus 200 kcal":               nil,
		"Saturday under 600 kcal":              nil,
		"around 600 kcal":                      nil, // no day
		"so make it around 600 kcal":           nil, // "so" is no Sunday
		"What is 600 kcal on Saturday?":        nil, // no cue
	} {
		got := agent.NamedDayKcal(text, w, sat)
		if want == nil {
			assert.Empty(t, got, text)
			continue
		}
		assert.Equal(t, want, got, text)
	}
	// Today outside the window names no day.
	assert.Empty(t, agent.NamedDayKcal("today around 600 kcal", w, sat.AddDate(0, 0, -3)))
}
