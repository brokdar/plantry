package planning_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

// h2-planner-00 (audit 2026-11-24): under a time wish the variety repair must
// keep the wish. The repeated chili cooks at the limited Saturday dinner; the
// closest main in energy (curry) is slow, so the swap takes the quick soup.
// Without the wish the same repair takes the curry (the twin).
func TestFixVariety_KeepsAQuickWish(t *testing.T) {
	run := func(limited bool) int64 {
		s := baseState()
		last := day(sat).AddDate(0, 0, -7)
		f := s.Foods[chili]
		f.LastEaten = &last
		s.Foods[chili] = f
		f = s.Foods[curry]
		f.Minutes = 90
		s.Foods[curry] = f
		f = s.Foods[soup]
		f.Minutes = 20
		s.Foods[soup] = f
		delete(s.Foods, stew)
		in := planning.ChooseInput{State: s, Target: windowTarget(s)}
		if limited {
			in.State.MaxMinutes = planning.MaxMinutesAt(plate.CellKey{Date: day(sat), SlotID: dinner}, 30)
		}
		prop := planning.Proposal{
			Sessions: []planning.ProposedSession{session("s1", chili, sat, dinner, 1)},
			Plates:   []planning.ProposedPlate{pplate(sat, dinner, onKey(chili, "s1"))},
		}
		got, notes := planning.FixVariety(in, prop)
		require.Len(t, notes, 1)
		return got.Sessions[0].FoodID
	}
	assert.Equal(t, int64(curry), run(false), "without a wish the closest main in energy wins")
	assert.Equal(t, int64(soup), run(true), "under a 30 minute wish the swap is a quick main")
}
