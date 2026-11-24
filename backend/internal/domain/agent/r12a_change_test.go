package agent

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
	"github.com/jaltszeimer/plantry/backend/internal/domain/window"
)

func r12aWindow() (window.Window, map[int64]string) {
	return window.Window{Start: time.Date(2026, 11, 21, 0, 0, 0, 0, time.UTC), End: time.Date(2026, 11, 27, 0, 0, 0, 0, time.UTC)},
		map[int64]string{2: "slot.lunch", 3: "slot.dinner"}
}

func msg(role llm.Role, text string) llm.Message {
	return llm.Message{Role: role, Content: []llm.ContentBlock{{Type: llm.ContentTypeText, Text: text}}}
}

// R12a A1: "lieber" is a change word.
func TestUnmetChange_LieberIsAChangeWord(t *testing.T) {
	t.Parallel()
	w, slots := r12aWindow()
	_, st, _ := unmetFixture()
	for _, text := range []string{
		"Mach mir am Mittwoch zum Abendessen lieber was anderes",
		"I'd rather have something else for Wednesday dinner",
	} {
		_, b := unmetOfTurn([]llm.Message{msg(llm.RoleUser, text)}, text, w, slots, st)
		require.NotNil(t, b, text)
		assert.True(t, b.nothingSaved)
		assert.Contains(t, b.facts[0].EN, "Wed 25 Nov dinner", text)
	}
	// Read-only questions and chit-chat never get the line.
	for _, text := range []string{
		"Was gibt es am Mittwoch zum Abendessen?",
		"Ist Mittwoch Abendessen zu viel?",
		"Wie lieber Mittwoch Abendessen, warm oder kalt?",
		"Danke, das Mittwoch Abendessen passt.",
		"Mach mir lieber was anderes",
	} {
		_, b := unmetOfTurn([]llm.Message{msg(llm.RoleUser, text)}, text, w, slots, st)
		assert.Nil(t, b, text)
	}
}

// R12a A2: a short answer to the agent's question inherits the request.
func TestUnmetChange_AnswerTurnInheritsRequest(t *testing.T) {
	t.Parallel()
	w, slots := r12aWindow()
	_, st, _ := unmetFixture()
	ask := msg(llm.RoleUser, "Tausch mir bitte das Abendessen aus, ich will was anderes")
	q := msg(llm.RoleAssistant, "Für welchen Tag soll ich das Abendessen tauschen?")
	for _, answer := range []string{"Das am Sonntag, bitte.", "Sonntag."} {
		cells, b := unmetOfTurn([]llm.Message{ask, q, msg(llm.RoleUser, answer)}, answer, w, slots, st)
		require.NotNil(t, b, answer)
		assert.Equal(t, map[string]bool{"2026-11-22|3": true}, cells)
		assert.Contains(t, b.facts[0].EN, "Sun 22 Nov dinner")
	}

	answer := "Das am Sonntag, bitte."
	for name, hist := range map[string][]llm.Message{
		"agent did not ask":      {ask, msg(llm.RoleAssistant, "Ich habe nachgesehen."), msg(llm.RoleUser, answer)},
		"earlier was read-only":  {msg(llm.RoleUser, "Was gibt es zum Abendessen?"), q, msg(llm.RoleUser, answer)},
		"no earlier message":     {msg(llm.RoleUser, answer)},
		"answer is a question":   {ask, q, msg(llm.RoleUser, "Und was gibt es am Sonntag?")},
		"answer is a long topic": {ask, q, msg(llm.RoleUser, "Sonntag ist gut, aber erzähl mir erst, wie das Wetter am Wochenende wird, bitte")},
	} {
		text := hist[len(hist)-1].Content[0].Text
		_, b := unmetOfTurn(hist, text, w, slots, st)
		assert.Nil(t, b, name)
	}
}

// R12a A3: an empty find_alternatives settles the request: the model's
// question does not stand, the code line is told.
func TestNoSaveLine_EmptyAlternativesSettles(t *testing.T) {
	t.Parallel()
	w, slots := r12aWindow()
	_, st, _ := unmetFixture()
	user := "Tausch das Abendessen am Mittwoch gegen etwas Leichtes"
	_, b := unmetOfTurn([]llm.Message{msg(llm.RoleUser, user)}, user, w, slots, st)
	require.NotNil(t, b)
	ask := "Soll ich etwas anderes suchen?"
	empty := func(out string) replyTurn {
		turn := replyTurn{user: user, unmet: b, numbers: map[string]bool{}}
		turn.noteTool("find_alternatives", json.RawMessage(`{}`), json.RawMessage(out), nil)
		return turn
	}
	got := composeReply(ask, empty(`{"alternatives":[],"empty_reason":{"candidates":3}}`).final())
	assert.Contains(t, got, nothingChangedDE)
	// An omitted list is empty too.
	assert.Contains(t, composeReply(ask, empty(`{"empty_reason":{"candidates":0}}`).final()), nothingChangedDE)
	// A search with a result keeps the question.
	full := empty(`{"alternatives":[{"food_id":1,"name":"X","ops":[]}]}`)
	assert.Equal(t, ask, composeReply(ask, full.final()))
}
