package agent_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
)

// newPlanToolEnvClock is newPlanToolEnv with the plate service's wall clock
// at clock; the chat turn's today stays planToday.
func newPlanToolEnvClock(t *testing.T, clock time.Time) *planToolEnv {
	t.Helper()
	e := newPlanToolEnv(t)
	e.clock = clock
	e.wire()
	return e
}

// SF1/G2: a chat turn has one today, the request's. A day between it and a
// later wall clock is not past: the move is applied.
func TestApplyChanges_TurnTodayRulesOverALaterClock(t *testing.T) {
	t.Parallel()
	e := newPlanToolEnvClock(t, planStart.AddDate(0, 0, 3))
	p, _ := e.twoDinners() // the window's 3rd day, before the clock
	lunch := e.slotID("lunch")
	if q := e.plateAtCell(p.DateString(), lunch); q != nil {
		_, err := e.plates.Delete(context.Background(), q.ID)
		require.NoError(t, err)
	}

	_, real, _ := e.mealOp(fmt.Sprintf(`[{"op":"move","plate_id":%d,"to":%s}]`, p.ID, cellJSON(p.DateString(), lunch)))

	assert.EqualValues(t, lunch, applied0(t, real)["slot_id"])
	moved, err := e.plates.Get(context.Background(), p.ID)
	require.NoError(t, err)
	assert.Equal(t, lunch, moved.SlotID)
}

// A day before the turn's today stays past, even when the wall clock lags
// behind it.
func TestApplyChanges_DayBeforeTurnTodayIsRefused(t *testing.T) {
	t.Parallel()
	e := newPlanToolEnvClock(t, planToday.AddDate(0, 0, -3))
	p, _ := e.twoDinners()
	past := planToday.AddDate(0, 0, -1).Format(time.DateOnly)

	_, err := e.exec("apply_changes", fmt.Sprintf(`{"ops":[{"op":"move","plate_id":%d,"to":%s}]}`, p.ID, cellJSON(past, p.SlotID)))

	require.ErrorIs(t, err, domain.ErrAIPastDate)
	got, gerr := e.plates.Get(context.Background(), p.ID)
	require.NoError(t, gerr)
	assert.Equal(t, p.DateString(), got.DateString(), "nothing moved")
}
