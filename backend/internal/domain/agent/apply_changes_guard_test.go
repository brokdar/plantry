package agent_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

// The guards hold inside a batch: a past date or a confirmed row in a later
// op fails the call and rolls back the valid ops before it.
func TestApplyChanges_GuardsRollBackTheBatch(t *testing.T) {
	e := newPlanToolEnv(t)
	p, _ := e.freshMain()
	week := e.week()
	confirmed := week[len(week)-1]
	require.NotEqual(t, p.ID, confirmed.ID)
	_, err := e.plates.Confirm(context.Background(), plate.RowSet{PlateIDs: []int64{confirmed.ID}})
	require.NoError(t, err)
	before := e.dbCounts()

	past := planToday.AddDate(0, 0, -1).Format(time.DateOnly)
	_, err = e.exec("apply_changes", fmt.Sprintf(`{"ops":[
		{"op":"set_eaters","plate_id":%d,"eaters":3},
		{"op":"create_plate","date":%q,"slot_id":%d}]}`, p.ID, past, p.SlotID))
	require.ErrorIs(t, err, domain.ErrAIPastDate)
	assert.Contains(t, err.Error(), "op 2 of 2 (create_plate)")
	assert.Equal(t, before, e.dbCounts())

	_, err = e.exec("apply_changes", fmt.Sprintf(`{"ops":[
		{"op":"set_eaters","plate_id":%d,"eaters":3},
		{"op":"delete_plate","plate_id":%d}]}`, p.ID, confirmed.ID))
	require.ErrorIs(t, err, domain.ErrRowConfirmed)
	assert.Equal(t, before, e.dbCounts())
	got, err := e.plates.Get(context.Background(), p.ID)
	require.NoError(t, err)
	assert.InDelta(t, p.Eaters, got.Eaters, 1e-9)
}

// A multi-op call is one change; its undo reverts every op.
func TestApplyChanges_UndoRevertsTheWholeBatch(t *testing.T) {
	e := newPlanToolEnv(t)
	ctx, _ := e.chat()
	p, _ := e.freshMain()
	slotID := e.emptyOtherSlot(p)
	before := e.dbCounts()

	ch := e.change(ctx, "apply_changes", fmt.Sprintf(`{"ops":[
		{"op":"set_eaters","plate_id":%d,"eaters":3},
		{"op":"add","date":%q,"slot_id":%d,"food_id":%d,"portions":1}]}`, p.ID, p.DateString(), slotID, e.sides[0]))
	assert.Len(t, opsOf(ch), 2, "one line entry per op")
	assert.Equal(t, before["ai_changes"]+1, e.dbCounts()["ai_changes"], "one change for the call")
	assert.Equal(t, before["plates"]+1, e.dbCounts()["plates"])

	_, err := e.changes.Undo(context.Background(), ch.ID)
	require.NoError(t, err)
	got, err := e.plates.Get(context.Background(), p.ID)
	require.NoError(t, err)
	assert.InDelta(t, p.Eaters, got.Eaters, 1e-9, "set_eaters undone")
	assert.Equal(t, before["plates"], e.dbCounts()["plates"], "the created plate is gone")
	assert.Equal(t, before["cook_sessions"], e.dbCounts()["cook_sessions"])
}

// A date outside the chat window is evaluated in its own window: the days
// and issues of that window come back and issues_checked stays true.
func TestApplyChanges_EvaluatesWindowsOfTouchedDates(t *testing.T) {
	e := newPlanToolEnv(t)
	p, _ := e.freshMain()
	next := planStart.AddDate(0, 0, 9).Format(time.DateOnly)

	res, err := e.exec("apply_changes", fmt.Sprintf(
		`{"dry_run":true,"ops":[{"op":"add","date":%q,"slot_id":%d,"food_id":%d,"portions":1}]}`, next, p.SlotID, e.sides[0]))
	require.NoError(t, err)
	after := res["after"].(map[string]any)
	assert.Equal(t, true, after["issues_checked"])
	var dates []string
	for _, d := range after["days"].([]any) {
		dates = append(dates, d.(map[string]any)["date"].(string))
	}
	assert.Contains(t, dates, next, "the later window's day is reported")
}

// after.plates carries per-plate and per-component kcal and protein per
// eater, and a cook_at op reports the cook day too.
func TestApplyChanges_PlateFiguresAndCookDay(t *testing.T) {
	e := newPlanToolEnv(t)
	p, _ := e.freshMain()
	slotID := e.emptyOtherSlot(p)
	var cookDate string
	var cookSlot int64
	for _, q := range e.week() {
		if q.DateString() != p.DateString() {
			cookDate, cookSlot = q.DateString(), q.SlotID
			break
		}
	}
	require.NotEmpty(t, cookDate)

	res, err := e.exec("apply_changes", fmt.Sprintf(
		`{"dry_run":true,"ops":[{"op":"add","date":%q,"slot_id":%d,"food_id":%d,"portions":1,"cook_at":{"date":%q,"slot_id":%d}}]}`,
		p.DateString(), slotID, e.sides[0], cookDate, cookSlot))
	require.NoError(t, err)
	after := res["after"].(map[string]any)
	var days []string
	for _, d := range after["days"].([]any) {
		days = append(days, d.(map[string]any)["date"].(string))
	}
	assert.Contains(t, days, cookDate, "the cook day is in after.days")

	plates := after["plates"].([]any)
	require.NotEmpty(t, plates)
	pl := plates[0].(map[string]any)
	// The fixture foods have no macros: zero figures are left out of the result.
	assert.NotContains(t, pl, "kcal")
	assert.NotContains(t, pl, "protein_g")
	details := pl["component_details"].([]any)
	require.NotEmpty(t, details)
	assert.Equal(t, true, details[0].(map[string]any)["provisional"], "a dry run's new component has no real id")
	assert.NotContains(t, details[0], "kcal")
}

// A dry run rolls back, so the rows it creates have no real ids: they are
// marked provisional and say to re-send the op list; rows that exist keep
// their ids, and a real apply carries real ids and no note.
func TestApplyChanges_DryRunMarksNewRowsProvisional(t *testing.T) {
	e := newPlanToolEnv(t)
	p, _ := e.freshMain()
	slotID := e.emptyOtherSlot(p)
	existing := p.Components[0].ID
	input := func(dry bool) string {
		return fmt.Sprintf(
			`{"dry_run":%t,"ops":[{"op":"add","date":%q,"slot_id":%d,"food_id":%d,"portions":1},{"op":"set_quantity","plate_component_id":%d,"portions":1}]}`,
			dry, p.DateString(), slotID, e.sides[0], existing)
	}

	res, err := e.exec("apply_changes", input(true))
	require.NoError(t, err)
	assert.Contains(t, res["dry_run_note"], "send the full op list again")
	applied := res["applied"].([]any)
	added, set := applied[0].(map[string]any), applied[1].(map[string]any)
	assert.Equal(t, true, added["provisional"])
	assert.NotContains(t, added, "plate_component_id")
	assert.NotContains(t, added, "plate_id")
	assert.NotContains(t, set, "provisional", "an existing component keeps its id")
	assert.EqualValues(t, existing, set["plate_component_id"])
	var provisionalPlates int
	for _, pl := range res["after"].(map[string]any)["plates"].([]any) {
		m := pl.(map[string]any)
		if m["provisional"] == true {
			provisionalPlates++
			assert.NotContains(t, m, "plate_id")
			for _, d := range m["component_details"].([]any) {
				assert.Equal(t, true, d.(map[string]any)["provisional"])
			}
		}
	}
	assert.Equal(t, 1, provisionalPlates)

	res, err = e.exec("apply_changes", input(false))
	require.NoError(t, err)
	assert.NotContains(t, res, "dry_run_note")
	added = res["applied"].([]any)[0].(map[string]any)
	assert.NotContains(t, added, "provisional")
	assert.Contains(t, added, "plate_component_id")
}
