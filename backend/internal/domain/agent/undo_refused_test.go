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

// PR #177 review: every refusal kind in both languages, the undoAlready kind, an
// unknown kind (stale), and the stop appended to a block that has a report.
func TestUndoStop_TextsOfEveryKind(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		kind, en, de string
	}{
		{"", "no change of this conversation is left to undo.", "in diesem Gespräch ist keine Änderung mehr rückgängig zu machen."},
		{undoStale, "the plan was changed since that change", "der Plan wurde seit dieser Änderung geändert"},
		{undoConfirmed, "a meal of that change is confirmed now.", "eine Mahlzeit dieser Änderung ist jetzt bestätigt."},
		{undoAlready, "that change was undone already on the board.", "diese Änderung wurde schon auf dem Plan rückgängig gemacht."},
		{"unheard_of", "the plan was changed since that change", "der Plan wurde seit dieser Änderung geändert"},
	} {
		obj := map[string]any{"undone": float64(0), "refused": map[string]any{"kind": c.kind}}
		if c.kind == "" {
			obj = map[string]any{"undone": float64(0)}
		}
		f, ok := undoStop(obj)
		require.True(t, ok, c.kind)
		assert.Contains(t, f.EN, "Nothing was undone: "+c.en, c.kind)
		assert.Contains(t, f.DE, "Nichts wurde rückgängig gemacht: "+c.de, c.kind)
		if c.kind == "" {
			continue // undone some and no refusal: nothing was stopped
		}
		obj["undone"] = float64(1)
		f, ok = undoStop(obj)
		require.True(t, ok, c.kind)
		assert.Contains(t, f.EN, "The next change was not undone: "+c.en, c.kind)
		assert.Contains(t, f.DE, "Die nächste Änderung wurde nicht rückgängig gemacht: "+c.de, c.kind)
	}
}

func TestBlockOf_UndoStopAfterAReport(t *testing.T) {
	t.Parallel()
	out := mustJSON(map[string]any{
		"undone": 1, "restored": []string{"x"},
		"refused": map[string]any{"kind": undoAlready},
		"report":  map[string]any{"facts": []fact{{Class: classChanged, Kind: kindMeal, Status: statusOK, EN: "Back.", DE: "Zurück."}}},
	})
	b, check := blockOf(out)
	require.NotNil(t, b)
	assert.False(t, check)
	require.Len(t, b.facts, 2)
	assert.Equal(t, "Back.", b.facts[0].EN)
	assert.Equal(t, classRefused, b.facts[1].Class)
	assert.Contains(t, b.facts[1].DE, "Die nächste Änderung wurde nicht rückgängig gemacht: diese Änderung wurde schon")
}
