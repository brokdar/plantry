package planning_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
)

func occasionsOn(p planning.Proposal, s planning.State, tgt planning.Target, date string) int {
	ev := planning.Evaluate(planning.Merge(s, tgt, p), tgt)
	for _, d := range ev.Days {
		if d.Date == date {
			return d.CookOccasions
		}
	}
	return -1
}

func TestFixCookLimit(t *testing.T) {
	t.Run("a side cooked fresh on its own plate is dropped from a day over the limit", func(t *testing.T) {
		s := batchCooker()
		tgt := targetCells(cell(tue, lunch), cell(tue, dinner))
		p := planning.Proposal{
			Sessions: []planning.ProposedSession{session("s1", 104, tue, lunch, 2)},
			Plates:   []planning.ProposedPlate{pplate(tue, lunch, onKey(104, "s1")), pplate(tue, dinner, onKey(104, "s1"), fresh(201))},
		}
		require.Equal(t, 2, occasionsOn(p, s, tgt, tue))
		got, notes := planning.FixCookLimit(planning.ChooseInput{State: s, Target: tgt}, p)
		assert.Equal(t, 1, occasionsOn(got, s, tgt, tue))
		require.Len(t, notes, 1)
		assert.Contains(t, notes[0], "Side 1 dropped")
		assert.Len(t, got.Plates[1].Components, 1)
	})
	t.Run("an earlier meal that cooks its own fresh main stays: leftovers are the model's choice, the view hints at them", func(t *testing.T) {
		s := batchCooker()
		coldFoods(&s)
		tgt := targetCells(cell(tue, lunch), cell(tue, dinner))
		p := planning.Proposal{
			Sessions: []planning.ProposedSession{session("s0", 103, mon, dinner, 2), session("s2", 105, tue, dinner, 1)},
			Plates:   []planning.ProposedPlate{pplate(tue, lunch, fresh(104)), pplate(tue, dinner, onKey(105, "s2"))},
		}
		in := planning.ChooseInput{State: s, Target: tgt}
		got, notes := planning.FixCookLimit(in, p)
		assert.Equal(t, p, got)
		assert.Empty(t, notes)
		view, _, _ := planning.PlannerViewOf(in, p)
		hints, _ := view["cooking_hints"].([]string)
		require.Len(t, hints, 1)
		assert.Contains(t, hints[0], "Tuesday cooks 2 times")
		assert.Contains(t, hints[0], "Eat leftovers of Main 3 (cooked Mon slot 2)")
	})
	t.Run("a batch stays: a choice of dishes", func(t *testing.T) {
		s := batchCooker()
		tgt := targetCells(cell(tue, lunch), cell(tue, dinner))
		p := planning.Proposal{
			Sessions: []planning.ProposedSession{session("s1", 104, tue, lunch, 1), session("s2", 105, tue, dinner, 1)},
			Plates:   []planning.ProposedPlate{pplate(tue, lunch, onKey(104, "s1")), pplate(tue, dinner, onKey(105, "s2"))},
		}
		got, notes := planning.FixCookLimit(planning.ChooseInput{State: s, Target: tgt}, p)
		assert.Equal(t, p, got)
		assert.Empty(t, notes)
	})
}
