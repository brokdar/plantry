package planning_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

func optionsOf(t *testing.T, s planning.State, rule string) []planning.Option {
	t.Helper()
	tgt := windowTarget(s)
	ev := planning.Evaluate(s, tgt)
	for _, is := range planning.IssueOptions(s, tgt, ev, ev.Issues) {
		if is.Rule == rule {
			return is.Options
		}
	}
	t.Fatalf("no %s issue in %v", rule, ev.Issues)
	return nil
}

// An S6 carries verified options: a smaller portion, a lighter swap; each
// has a runnable op and, applied, leaves the plate under the cap.
func TestIssueOptions_S6(t *testing.T) {
	s := altState()
	s.Profile.KcalTarget = f64(2700) // cap 675: the 780 kcal lunch is 105 over
	s.Light = []planning.LightIntent{{Date: sat, Scope: "meal", Slot: "lunch"}}
	ev := planning.Evaluate(s, windowTarget(s))
	for _, p := range ev.Plates {
		t.Log(p.Date, p.SlotID, p.Kcal, p.Intent, p.MaxKcal)
	}
	opts := optionsOf(t, s, "S6")
	require.NotEmpty(t, opts)
	assert.Equal(t, "portions", opts[0].Action, "the least invasive option first")
	assert.Equal(t, 0.75, *opts[0].Portions)
	assert.Equal(t, "set_quantity", opts[0].Op.Op)
	for _, o := range opts {
		b, _ := json.Marshal(o)
		t.Log(string(b))
		assert.NotEmpty(t, o.Op.Op)
		assert.LessOrEqual(t, o.PlateKcal, 675.0, o.Action)
		assert.Equal(t, sat, o.Date)
	}
}

// An S6b carries heartier options only (no removal).
func TestIssueOptions_S6b(t *testing.T) {
	s := altState()
	s.Profile.KcalTarget = f64(2000) // hearty minimum 833
	s.Light = []planning.LightIntent{{Date: sun, Scope: "meal", Slot: "lunch", Level: "hearty"}}
	opts := optionsOf(t, s, "S6b")
	require.NotEmpty(t, opts)
	for _, o := range opts {
		b, _ := json.Marshal(o)
		t.Log(string(b))
		assert.NotEqual(t, "remove", o.Action)
		assert.GreaterOrEqual(t, o.PlateKcal, 833.0, o.Action)
	}
}

// H2: a day over the cook limit offers a verified way out: the batch cooked
// on it moves to an earlier cell.
func TestIssueOptions_H2(t *testing.T) {
	s := baseState()
	s.SlotNames = map[int64]string{breakfast: "slot.breakfast", lunch: "slot.lunch", dinner: "slot.dinner"}
	s.Settings.MaxCookOccasionsPerDay = 1
	s.Sessions = []planning.Session{
		persistedSession(1, curry, sun, lunch, 1, false),
		persistedSession(2, soup, sun, dinner, 1, false),
	}
	s.Plates = []planning.Plate{persisted(10, sun, lunch, false, curry, 1), persisted(11, sun, dinner, false, soup, 2)}
	for i := range s.Plates {
		s.Plates[i].Components[0].ID, s.Plates[i].Components[0].Position = int64(100+i), plate.PositionMain
	}
	opts := optionsOf(t, s, "H2")
	require.NotEmpty(t, opts)
	assert.Equal(t, "move_cook", opts[0].Action)
	for _, o := range opts {
		assert.LessOrEqual(t, o.CookOccasions, 1)
		assert.NotEmpty(t, o.Op.Op)
	}
	assert.Equal(t, "move_cook", opts[0].Op.Op)
	assert.NotNil(t, opts[0].Op.SessionID)
	assert.NotNil(t, opts[0].Op.CookAt)
}
