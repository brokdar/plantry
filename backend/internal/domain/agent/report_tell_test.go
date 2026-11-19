package agent

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMealChange_SaysWhatHappenedInPlainWords(t *testing.T) {
	from, to := 1583, 1212
	for _, c := range []struct {
		name string
		m    reportMeal
		want string
	}{
		{
			"replaced",
			reportMeal{Day: "Tue 24 Nov", Meal: "dinner", Before: []string{"Salmon 1 portion", "Potatoes 1 portion"}, After: []string{"Salmon 1 portion", "Broccoli 1 portion"}, KcalFrom: &from, KcalTo: &to},
			"Tue 24 Nov dinner: Potatoes replaced by Broccoli (1583 → 1212 kcal).",
		},
		{
			"resized and added",
			reportMeal{Day: "Tue 24 Nov", Meal: "lunch", Before: []string{"Curry 1 portion"}, After: []string{"Curry 1.5 portions", "Bread 80 g"}},
			"Tue 24 Nov lunch: added Bread; Curry from 1 portion to 1.5 portions.",
		},
		{
			"moved",
			reportMeal{Day: "Mon 23 Nov", Meal: "dinner", MovedFrom: "Fri 27 Nov dinner", Before: []string{"Chili 1 portion"}, After: []string{"Chili 1 portion"}},
			"Mon 23 Nov dinner: Chili moved here from Fri 27 Nov dinner.",
		},
		{"away", reportMeal{Day: "Sun 22 Nov", Meal: "lunch", Before: []string{"Pasta 1 portion"}}, "Sun 22 Nov lunch: no meal any more."},
	} {
		assert.Equal(t, c.want, mealChange(c.m), c.name)
	}
}

// The structured report keeps changed, days and cooking; the sentences are in
// tell only.
func TestChangeReport_JSONHoldsTellNotTheSentenceLists(t *testing.T) {
	r := changeReport{Leftovers: []string{"a"}, Week: []string{"b"}, NewWarnings: []string{"c"}, Worse: []string{"d"}, AlsoChanged: []string{"e"}, Tell: []string{"x"}}
	raw, _ := json.Marshal(r)
	assert.JSONEq(t, `{"tell":["x"],"earlier_warnings_unaffected":0}`, string(raw))
}
