package agent

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

// B4-AC3 (D6): a preview counts only for a later turn of its conversation,
// for exactly its rows, for 30 minutes and once.
func TestPendingUnlocks(t *testing.T) {
	now := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)
	p := newPendingUnlocks(func() time.Time { return now })
	rows := plate.RowSet{PlateIDs: []int64{1, 2}, SessionIDs: []int64{3}}
	first := chatTurn{conversationID: 7, seq: 1}
	next := chatTurn{conversationID: 7, seq: 2}

	assert.False(t, p.confirmed(next, rows), "no preview")
	p.issue(first, rows)
	assert.False(t, p.confirmed(first, rows), "same turn")
	assert.True(t, p.confirmed(next, rows), "later turn")
	assert.False(t, p.confirmed(chatTurn{conversationID: 8, seq: 3}, rows), "other conversation")
	assert.False(t, p.confirmed(next, plate.RowSet{PlateIDs: []int64{1}}), "other rows")

	p.drop(next, rows)
	assert.False(t, p.confirmed(next, rows), "used once")

	p.issue(first, rows)
	now = now.Add(unlockTTL + time.Second)
	assert.False(t, p.confirmed(next, rows), "expired")
	assert.Empty(t, p.entries, "expired previews are pruned")
}
