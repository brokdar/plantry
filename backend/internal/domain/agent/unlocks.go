package agent

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

// unlockTTL bounds how long an unlock preview waits for the user's yes (D6).
const unlockTTL = 30 * time.Minute

// pendingUnlocks holds the unlock previews waiting for the user's yes (D6):
// (conversation, closure) → the chat turn that previewed it. It lives in
// memory, so a restart loses it and the model previews again. Safe for
// concurrent use.
type pendingUnlocks struct {
	mu      sync.Mutex
	now     func() time.Time
	entries map[unlockKey]pendingUnlock
}

type unlockKey struct {
	conversationID int64
	rows           string
}

type pendingUnlock struct {
	turn uint64
	at   time.Time
}

func newPendingUnlocks(now func() time.Time) *pendingUnlocks {
	return &pendingUnlocks{now: now, entries: map[unlockKey]pendingUnlock{}}
}

func keyOf(conversationID int64, rows plate.RowSet) unlockKey {
	plates, sessions := slices.Clone(rows.PlateIDs), slices.Clone(rows.SessionIDs)
	slices.Sort(plates)
	slices.Sort(sessions)
	return unlockKey{conversationID: conversationID, rows: fmt.Sprint(plates, sessions)}
}

// issue records that turn t previewed rows in its conversation.
func (p *pendingUnlocks) issue(t chatTurn, rows plate.RowSet) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.prune()
	p.entries[keyOf(t.conversationID, rows)] = pendingUnlock{turn: t.seq, at: p.now()}
}

// confirmed reports whether an earlier turn of t's conversation previewed
// exactly rows, so the user has written since (D6).
func (p *pendingUnlocks) confirmed(t chatTurn, rows plate.RowSet) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.prune()
	e, ok := p.entries[keyOf(t.conversationID, rows)]
	return ok && e.turn < t.seq
}

// drop forgets the preview of rows once they are unlocked.
func (p *pendingUnlocks) drop(t chatTurn, rows plate.RowSet) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.entries, keyOf(t.conversationID, rows))
}

// prune drops expired previews; the caller holds mu.
func (p *pendingUnlocks) prune() {
	cutoff := p.now().Add(-unlockTTL)
	for k, e := range p.entries {
		if e.at.Before(cutoff) {
			delete(p.entries, k)
		}
	}
}

// chatTurn is one Chat call as the tools see it: its conversation, its place
// in the service's turn order and the service's pending unlocks.
type chatTurn struct {
	conversationID int64
	seq            uint64
	unlocks        *pendingUnlocks
}

type turnKey struct{}

// withTurn carries the chat turn to the tools.
func withTurn(ctx context.Context, t chatTurn) context.Context {
	return context.WithValue(ctx, turnKey{}, t)
}

// turnFrom returns the turn set by withTurn.
func turnFrom(ctx context.Context) (chatTurn, bool) {
	t, ok := ctx.Value(turnKey{}).(chatTurn)
	return t, ok
}
