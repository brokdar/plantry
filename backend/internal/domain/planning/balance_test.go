package planning_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
)

// balanceState is a batch cooker with a 2100 kcal target; mains hold 600 kcal,
// the library has sides (201-204, 150 kcal) and no-cook foods.
func balanceState() planning.State {
	s := batchCooker()
	s.Profile.KcalTarget = f64(2100)
	return s
}

// freshPlan plans every lunch and dinner with a fresh main at one portion.
func freshPlan(s planning.State) planning.Proposal {
	var p planning.Proposal
	n := 0
	for _, c := range s.Cells {
		if c.SlotID == breakfast {
			continue
		}
		p.Plates = append(p.Plates, planning.ProposedPlate{
			Date: c.Date, SlotID: c.SlotID, Eaters: 1,
			Components: []planning.ProposedComponent{{FoodID: int64(101 + n%7), Portions: 1}},
		})
		n++
	}
	return p
}

func balanceWeek(s planning.State, p planning.Proposal) *planning.WeekEval {
	t := windowTarget(s)
	return planning.Evaluate(planning.Merge(s, t, p), t).Week
}

func TestBalancePlan(t *testing.T) {
	t.Run("a plan short on energy is lifted into the week band, no hard issue, every change noted", func(t *testing.T) {
		s := balanceState()
		prop := freshPlan(s)
		before := balanceWeek(s, prop)
		require.NotNil(t, before)
		require.Less(t, before.Pct, 95.0, "the plan is short")
		got, notes := planning.BalancePlan(planning.ChooseInput{State: s, Target: windowTarget(s)}, prop)
		after := balanceWeek(s, got)
		assert.GreaterOrEqual(t, after.Pct, 95.0)
		assert.NotEmpty(t, notes)
		for _, n := range notes {
			assert.True(t, strings.HasPrefix(n, "Balanced: ") || strings.HasPrefix(n, "Balancing "), n)
		}
		hardOf := func(p planning.Proposal) int {
			return len(violations(planning.Evaluate(planning.Merge(s, windowTarget(s), p), windowTarget(s)).Issues))
		}
		assert.LessOrEqual(t, hardOf(got), hardOf(prop), "no new hard issue")
		for _, pl := range got.Plates {
			for _, c := range pl.Components {
				assert.LessOrEqual(t, c.Portions, planning.MaxDishPortions+1e-9)
			}
		}
	})
	t.Run("add-on excess is trimmed", func(t *testing.T) {
		s := balanceState()
		addOnFood(&s, 301, "Skyr", 60)
		prop := freshPlan(s)
		for i := range prop.Plates[:3] { // 600 g of skyr on three plates: over the cap
			prop.Plates[i].Components = append(prop.Plates[i].Components, planning.ProposedComponent{FoodID: 301, Grams: 600})
		}
		target := windowTarget(s)
		count := func(p planning.Proposal) int {
			n := 0
			for _, is := range planning.Evaluate(planning.Merge(s, target, p), target).Issues {
				if is.Code == planning.CodeAddOnExcess {
					n++
				}
			}
			return n
		}
		require.Positive(t, count(prop))
		got, notes := planning.BalancePlan(planning.ChooseInput{State: s, Target: target}, prop)
		assert.Less(t, count(got), count(prop))
		assert.True(t, slices.ContainsFunc(notes, func(n string) bool { return strings.Contains(n, "add-on") }), "%v", notes)
	})
	t.Run("a light dinner gets no add-on and keeps its portions", func(t *testing.T) {
		s := balanceState()
		s.SlotNames = map[int64]string{breakfast: "slot.breakfast", lunch: "slot.lunch", dinner: "slot.dinner"}
		light := []planning.LightIntent{{Date: tue, Scope: "meal", Slot: "dinner"}}
		require.NoError(t, planning.ResolveLight(s, light))
		s.Light = light
		prop := freshPlan(s)
		got, _ := planning.BalancePlan(planning.ChooseInput{State: s, Target: windowTarget(s)}, prop)
		for i, pl := range got.Plates {
			if pl.Date.Equal(day(tue)) && pl.SlotID == dinner {
				assert.Equal(t, prop.Plates[i].Components, pl.Components, "the light dinner is unchanged")
			}
		}
	})
	t.Run("confirmed plates do not change", func(t *testing.T) {
		s := balanceState()
		s.Plates = append(s.Plates, planning.Plate{ID: 900, Date: day(tue), SlotID: dinner, Eaters: 1, Confirmed: true, Components: []planning.Component{{FoodID: 101, Portions: f64(1)}}})
		target := windowTarget(s)
		prop := freshPlan(s)
		prop.Plates = prop.Plates[:0:0]
		for _, c := range s.Cells {
			if c.SlotID != breakfast && (!c.Date.Equal(day(tue)) || c.SlotID != dinner) {
				prop.Plates = append(prop.Plates, planning.ProposedPlate{Date: c.Date, SlotID: c.SlotID, Eaters: 1, Components: []planning.ProposedComponent{{FoodID: 102, Portions: 1}}})
			}
		}
		got, _ := planning.BalancePlan(planning.ChooseInput{State: s, Target: target}, prop)
		m := planning.Merge(s, target, got)
		for _, p := range m.Plates {
			if p.ID == 900 {
				require.Len(t, p.Components, 1)
				assert.InDelta(t, 1.0, *p.Components[0].Portions, 1e-9)
			}
		}
		for _, pl := range got.Plates {
			assert.False(t, pl.Date.Equal(day(tue)) && pl.SlotID == dinner, "no plate is made on the confirmed cell")
		}
	})
	t.Run("no kcal target changes nothing", func(t *testing.T) {
		s := batchCooker()
		prop := freshPlan(s)
		got, notes := planning.BalancePlan(planning.ChooseInput{State: s, Target: windowTarget(s)}, prop)
		assert.Empty(t, notes)
		assert.Equal(t, prop, got)
	})
}

// A day whose add-on count is used up with small adds still gets its energy:
// the adds it holds grow up to their caps (growShort).
func TestBalancePlan_GrowsTheAddsADayHolds(t *testing.T) {
	s := balanceState()
	s.Profile.KcalTarget = f64(2600)
	for i := range 8 {
		addOnFood(&s, int64(900+i), "Olives "+string(rune('A'+i)), 400)
	}
	prop := freshPlan(s)
	n := 0
	for i := range prop.Plates {
		adds := 2
		if i%2 == 0 {
			adds = 1
		}
		for range adds {
			prop.Plates[i].Components = append(prop.Plates[i].Components, planning.ProposedComponent{FoodID: int64(900 + n%8), Grams: 30})
			n++
		}
	}
	grams := func(p planning.Proposal) (sum float64) {
		for _, pl := range p.Plates {
			for _, c := range pl.Components {
				sum += c.Grams
			}
		}
		return sum
	}
	before := balanceWeek(s, prop)
	got, _ := planning.BalancePlan(planning.ChooseInput{State: s, Target: windowTarget(s)}, prop)
	after := balanceWeek(s, got)
	assert.Greater(t, grams(got), grams(prop), "the day's adds were raised")
	assert.Greater(t, after.Pct, before.Pct)
	assert.Empty(t, s9Of(planning.Merge(s, windowTarget(s), got)), "no add passed its cap or the day's count")
	for _, pl := range got.Plates {
		for _, c := range pl.Components {
			assert.LessOrEqual(t, c.Grams, 200.0, "within the add-on cap")
		}
	}
}
