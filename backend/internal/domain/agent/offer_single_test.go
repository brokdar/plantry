package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/domain/window"
)

func dayFixObj(opts ...map[string]any) map[string]any {
	var list []any
	for _, o := range opts {
		list = append(list, o)
	}
	return map[string]any{"after": map[string]any{"issues": map[string]any{"day_fix": []any{
		map[string]any{"date": "2026-11-26", "options": list},
	}}}}
}

// OF2/RL6: a filler offer never adds a nut (or cheese); the next option is used.
func TestOffer_NoNutFiller(t *testing.T) {
	t.Parallel()
	nut := map[string]any{"action": "add", "name": "Peanuts (salted)", "slot": "lunch", "day_kcal": 1700.0, "op": map[string]any{"amount": 30.0}}
	bread := map[string]any{"action": "add", "name": "Wholegrain bread", "slot": "lunch", "day_kcal": 1690.0, "op": map[string]any{"amount": 60.0}}
	got := offerOf(dayFixObj(nut, bread))
	require.NotNil(t, got)
	assert.Contains(t, got.EN, "Wholegrain bread")
	assert.NotContains(t, got.EN, "Peanuts")
	assert.Nil(t, offerOf(dayFixObj(nut)))
}

// OF2/C1/G3: the goulash shape. A guard offer for the skipped part stays the one
// offer: no back-to-range add (here a nut) is merged into its sentence.
func TestBackToRange_NeverMergesASecondChangeIntoTheOffer(t *testing.T) {
	t.Parallel()
	res := applyRun{after: planning.Evaluation{Issues: []planning.Issue{{
		Code: planning.CodeKcalBelowBand, Date: "2026-11-26",
		Options: []planning.Option{{Action: "add", Name: "Peanuts (salted)", Date: "2026-11-26", DayKcal: 1700}},
	}}}}
	first := map[string]any{"what": `use "Chili sin carne" instead of "Beef goulash" on Mon 23 Nov dinner`, "recommended": true}
	got := withBackToRange(t.Context(), Services{}, window.Window{}, res, []map[string]any{first})
	require.Len(t, got, 1)
	assert.Equal(t, `use "Chili sin carne" instead of "Beef goulash" on Mon 23 Nov dinner`, got[0]["what"])
	assert.NotContains(t, got[0]["what"], "Peanuts")
	// With no other offer, a nut filler is no offer either.
	assert.Empty(t, withBackToRange(t.Context(), Services{}, window.Window{}, res, nil))
}
