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

// namedDayKcal reads a day number only with one weekday, one kcal number and
// no meal word.
func TestNamedDayKcal(t *testing.T) {
	t.Parallel()
	sat := time.Date(2026, 11, 21, 0, 0, 0, 0, time.UTC)
	w := window.Window{Start: sat, End: sat.AddDate(0, 0, 6)}
	for text, want := range map[string]map[string]float64{
		"Saturday around 600 kcal":        {"2026-11-21": 600},
		"Dienstag etwa 650 kcal":          {"2026-11-24": 650},
		"Saturday dinner around 600 kcal": nil,
		"Saturday and Sunday 1800 kcal":   nil,
		"make Saturday lighter":           nil,
	} {
		got := agent.NamedDayKcal(text, w)
		if want == nil {
			assert.Empty(t, got, text)
			continue
		}
		assert.Equal(t, want, got, text)
	}
}
