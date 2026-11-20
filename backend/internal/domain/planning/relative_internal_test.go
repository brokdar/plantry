package planning

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

// KR2: a way, offer or fix never puts more than two dishes on one cooking time
// unless the plan already did.
func TestOverloadedCooking(t *testing.T) {
	d := time.Date(2026, 11, 24, 0, 0, 0, 0, time.UTC)
	state := func(n int) State {
		var s State
		for i := 1; i <= n; i++ {
			id := int64(i)
			s.Sessions = append(s.Sessions, Session{CookSession: plate.CookSession{ID: id, CookDate: d, CookSlotID: 3}})
			s.Plates = append(s.Plates, Plate{ID: id, Components: []Component{{ID: id, SessionID: id}}})
		}
		return s
	}
	assert.False(t, overloadedCooking(state(2), state(2)))
	assert.True(t, overloadedCooking(state(2), state(3)), "a third dish on the cooking time")
	assert.False(t, overloadedCooking(state(3), state(3)), "already so before the way")
	s := state(3)
	s.Plates = s.Plates[:2] // the third batch feeds no meal: it is not a dish of this cooking
	assert.False(t, overloadedCooking(state(2), s))
}
