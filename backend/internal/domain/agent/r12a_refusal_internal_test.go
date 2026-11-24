package agent

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/domain/window"
)

// R12a C3: every hard issue has a German label; none falls back to "eine
// Regel wäre verletzt".
func TestHardLabelDE_EveryHardCodeHasGerman(t *testing.T) {
	t.Parallel()
	hard := []string{
		planning.CodeMissingPlate, planning.CodeExtraPlate, planning.CodePlateInNoneCell, planning.CodeCookInNoneCell,
		planning.CodeTooManyCookOccasions, planning.CodeDietIncompatible, planning.CodeMeatDaysExceeded,
		planning.CodeConfirmedInTarget, planning.CodeSessionOutsideTarget,
		planning.CodeDuplicateSessionKey, planning.CodeInvalidPortions, planning.CodeNotComposed, planning.CodePlateOnConfirmedCell,
		planning.CodePlateOutsideTarget, planning.CodePlateWithoutComponent, planning.CodePortionsOverCap,
		planning.CodeSessionFoodMismatch, planning.CodeUnknownFood, planning.CodeUnknownSession,
		planning.CodeKcalBelowFloor, planning.CodeVarietyRepeat,
		"leftover_before_cook", "past_keep_days", "deficit", "frozen_deficit",
	}
	for _, code := range hard {
		assert.NotEmpty(t, hardLabelDE[code], code)
		_, de := hardWhyL(planning.Issue{Code: code})
		assert.NotContains(t, de, "eine Regel wäre verletzt", code)
	}
	// An unknown code keeps the generic line.
	_, de := hardWhyL(planning.Issue{Code: "no_such_code"})
	assert.Contains(t, de, "eine Regel wäre verletzt")
}

// R12a C4: a turn that saved nothing tells the reason the tools found.
func TestNoSaveLine_CarriesTheReasonTheToolsFound(t *testing.T) {
	t.Parallel()
	w, slots := r12aWindow()
	_, st, _ := unmetFixture()
	user := "Put the Thai green curry on Wednesday dinner instead"
	_, b := unmetOfTurn([]llm.Message{msg(llm.RoleUser, user)}, user, w, slots, st)
	require.NotNil(t, b)

	// The dish is not in the library.
	miss := replyTurn{user: user, unmet: b, numbers: map[string]bool{}}
	miss.noteTool("find_foods", json.RawMessage(`{"search":"Thai green curry"}`), json.RawMessage(`{"items":[],"total":0}`), nil)
	f, ok := miss.noSaveLine("")
	require.True(t, ok)
	assert.Equal(t, `Left out: the change to Wed 25 Nov dinner ("Thai green curry" is not in your library).`, f.EN)
	assert.Contains(t, f.DE, "ist nicht in deiner Bibliothek")

	// The guard refused the checked change: its reason, with the rule.
	check := replyTurn{user: user, unmet: b, numbers: map[string]bool{}, write: true}
	check.noteTool("apply_changes", json.RawMessage(`{}`), json.RawMessage(`{"dry_run":true,"saved":false,"skipped":[{"op":1,"why":"it would create a problem on Wed 25 Nov: cooking times per day (2 cooking times, at most 1)","why_de":"es würde am Mittwoch ein Problem entstehen: mehr getrennte Kochzeiten an einem Tag als erlaubt"}]}`), nil)
	f, ok = check.noSaveLine("")
	require.True(t, ok)
	assert.Contains(t, f.EN, "the check refused it: it would create a problem on Wed 25 Nov: cooking times per day")
	assert.Contains(t, f.DE, "die Prüfung hat sie abgelehnt: es würde am Mittwoch")

	// A checked change the guard did not refuse keeps the generic reason.
	plain := replyTurn{user: user, unmet: b, numbers: map[string]bool{}, write: true}
	plain.noteTool("apply_changes", json.RawMessage(`{}`), json.RawMessage(`{"dry_run":true,"saved":true}`), nil)
	f, _ = plain.noSaveLine("")
	assert.Contains(t, f.EN, checkedWhyEN)

	// A lookup that found the dish gives no "not in your library".
	hit := replyTurn{user: user, unmet: b, numbers: map[string]bool{}}
	hit.noteTool("find_foods", json.RawMessage(`{"search":"Thai green curry"}`), json.RawMessage(`{"items":[{"id":1,"name":"Thai green curry"}],"total":1}`), nil)
	f, _ = hit.noSaveLine("")
	assert.NotContains(t, f.EN, "not in your library")
}

// R12a C1 negative: a reason that is no cooking count, or a day nothing cooks
// on, stays as hardWhyL says it.
func TestHardWhyIn_OnlyCookingTimesGetTheCookList(t *testing.T) {
	t.Parallel()
	svc := Services{State: func(context.Context, window.Window, time.Time) (planning.State, error) {
		return planning.State{SlotNames: map[int64]string{1: "slot.lunch"}}, nil
	}}
	for _, is := range []planning.Issue{
		{Code: planning.CodeTooManyCookOccasions, Date: "2026-11-25", Params: map[string]any{"count": 2.0, "max": 1.0}}, // nothing cooks that day in the state
		{Code: planning.CodeMeatDaysExceeded, Date: "2026-11-25", Params: map[string]any{"days": 4.0, "max": 3.0}},
	} {
		wantEN, wantDE := hardWhyL(is)
		en, de := hardWhyIn(context.Background(), svc, window.Window{}, is)
		assert.Equal(t, wantEN, en, is.Code)
		assert.Equal(t, wantDE, de, is.Code)
	}
}
