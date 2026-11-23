package agent

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/fake"
)

// replyOver runs one turn whose single tool call returns output and returns the stored reply.
func replyOver(t *testing.T, tool string, output json.RawMessage, user, model string) string {
	t.Helper()
	ts := minimalToolSet(t, tool, output, ToolEffectNone, nil)
	client := fake.NewFromScript(&fake.Script{Turns: []fake.Turn{scriptToolUseTurn("t1", tool), scriptEndTurn(model)}})
	req := makeReq(1)
	req.UserText = user
	req.Tools = ts.Describe()
	repo := &memRepo{}
	require.NoError(t, runLoopRepo(t, req, client, ts, repo))
	msgs, _ := repo.ListMessages(context.Background(), 1)
	var blocks []struct{ Type, Text string }
	require.NoError(t, json.Unmarshal(msgs[len(msgs)-1].Content, &blocks))
	require.NotEmpty(t, blocks)
	return blocks[len(blocks)-1].Text
}

// G4.1/SF2 (audit V58): an undo that undid nothing is never told as
// "Undone". The result is the stored test1 g-undo-08 turn 3 (the change
// before it was refused and saved nothing).
func TestReply_UndoThatUndidNothingSaysSo(t *testing.T) {
	t.Parallel()
	stored := json.RawMessage(`{"message":"Nothing to undo: every change made in this conversation is undone already or wrote nothing.","restored":[],"undone":0}`)
	got := replyOver(t, undoLastChangeName, stored, "undo that", "There's nothing to undo.")
	assert.NotContains(t, got, "Undone")
	assert.Contains(t, got, "Nothing was undone: no change of this conversation is left to undo.")

	de := replyOver(t, undoLastChangeName, stored, "mach das rückgängig", "Da gibt es nichts.")
	assert.NotContains(t, de, "Rückgängig gemacht;")
	assert.Contains(t, de, "Nichts wurde rückgängig gemacht")
}

// A refused undo (stale or confirmed) says nothing was undone and why.
func TestReply_RefusedUndoSaysWhy(t *testing.T) {
	t.Parallel()
	for kind, want := range map[string]string{
		undoStale:     "Nothing was undone: the plan was changed since that change",
		undoConfirmed: "Nothing was undone: a meal of that change is confirmed now.",
	} {
		out := mustJSON(undoOut{Restored: []string{}, Refused: &undoRefusal{Kind: kind, Reason: "x", WhatTo: "y"}, Message: "Not undone: x"})
		got := replyOver(t, undoLastChangeName, out, "undo that", "I could not undo it.")
		assert.NotContains(t, got, "Undone;", kind)
		assert.Contains(t, got, want, kind)
	}
	// Undone one, then stopped: the stop is told after what was undone.
	out := mustJSON(undoOut{Undone: 1, Restored: []string{"Sat 28 Nov dinner: Pasta"}, Refused: &undoRefusal{Kind: undoStale, Reason: "x", WhatTo: "y"}, Message: "Undone 1, then stopped: x"})
	got := replyOver(t, undoLastChangeName, out, "undo both", "Done.")
	assert.Contains(t, got, "The next change was not undone: the plan was changed since that change")
}
