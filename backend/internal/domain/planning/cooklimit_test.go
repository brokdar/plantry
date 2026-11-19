package planning_test

import (
	"strings"
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
	t.Run("an earlier meal that cooks its own fresh main becomes a cold plate", func(t *testing.T) {
		s := batchCooker()
		coldFoods(&s)
		tgt := targetCells(cell(tue, lunch), cell(tue, dinner))
		p := planning.Proposal{
			Sessions: []planning.ProposedSession{session("s2", 105, tue, dinner, 1)},
			Plates:   []planning.ProposedPlate{pplate(tue, lunch, fresh(104)), pplate(tue, dinner, onKey(105, "s2"))},
		}
		require.Equal(t, 2, occasionsOn(p, s, tgt, tue))
		got, notes := planning.FixCookLimit(planning.ChooseInput{State: s, Target: tgt}, p)
		assert.Equal(t, 1, occasionsOn(got, s, tgt, tue))
		require.Len(t, notes, 1)
		assert.True(t, strings.Contains(notes[0], "cold plate instead of Main 4"), notes[0])
		for _, c := range got.Plates[0].Components {
			assert.Positive(t, c.Grams)
		}
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
