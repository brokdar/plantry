package agent_test

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/agent"
	"github.com/jaltszeimer/plantry/backend/internal/domain/availability"
	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper/testsvc"
)

// planDump is the plan as undo must restore it: every plate (cell, eaters,
// components with position and the cook cell of their batch), every session
// and every availability override, without row ids.
func (e *planToolEnv) planDump() string {
	e.t.Helper()
	var lines []string
	rows, err := e.db.Query(`SELECT p.date, p.slot_id, p.eaters, f.name, COALESCE(pc.portions, 0), pc.position,
		COALESCE(cs.cook_date, ''), COALESCE(cs.cook_slot_id, 0)
		FROM plates p JOIN plate_components pc ON pc.plate_id = p.id JOIN foods f ON f.id = pc.food_id
		LEFT JOIN cook_sessions cs ON cs.id = pc.cook_session_id`)
	require.NoError(e.t, err)
	for rows.Next() {
		var date, name, pos, cook string
		var slotID, cookSlot int64
		var eaters, portions float64
		require.NoError(e.t, rows.Scan(&date, &slotID, &eaters, &name, &portions, &pos, &cook, &cookSlot))
		lines = append(lines, fmt.Sprintf("plate %s/%d x%g: %s %g %s cook %s/%d", date, slotID, eaters, name, portions, pos, cook, cookSlot))
	}
	require.NoError(e.t, rows.Err())
	rows, err = e.db.Query(`SELECT f.name, cs.cook_date, cs.cook_slot_id, cs.portions, cs.frozen_portions FROM cook_sessions cs JOIN foods f ON f.id = cs.food_id`)
	require.NoError(e.t, err)
	for rows.Next() {
		var name, date string
		var slotID int64
		var portions, frozen float64
		require.NoError(e.t, rows.Scan(&name, &date, &slotID, &portions, &frozen))
		lines = append(lines, fmt.Sprintf("session %s %s/%d %g/%g", name, date, slotID, portions, frozen))
	}
	require.NoError(e.t, rows.Err())
	rows, err = e.db.Query(`SELECT date, slot_id, mode, COALESCE(note, '') FROM availability_overrides`)
	require.NoError(e.t, err)
	for rows.Next() {
		var date, mode, note string
		var slotID int64
		require.NoError(e.t, rows.Scan(&date, &slotID, &mode, &note))
		lines = append(lines, fmt.Sprintf("override %s/%d %s %q", date, slotID, mode, note))
	}
	require.NoError(e.t, rows.Err())
	slices.Sort(lines)
	return strings.Join(lines, "\n")
}

// mealOp runs ops as a dry run (nothing may change), then for real in a chat
// run, and returns the real result and its change. The caller checks the
// write; undoRestores checks the undo.
func (e *planToolEnv) mealOp(ops string) (dry, real map[string]any, ch *agent.Change) {
	e.t.Helper()
	before := e.dbCounts()
	dump := e.planDump()
	dry = e.mustExec("apply_changes", `{"dry_run":true,"ops":`+ops+`}`)
	assert.Equal(e.t, before, e.dbCounts(), "a dry run writes nothing")
	assert.Equal(e.t, dump, e.planDump())
	ctx, _ := e.chat()
	out, _, ch, err := e.ts.Execute(ctx, "apply_changes", json.RawMessage(`{"ops":`+ops+`}`))
	require.NoError(e.t, err)
	require.NotNil(e.t, ch)
	require.NoError(e.t, json.Unmarshal(out, &real))
	return dry, real, ch
}

// undoRestores undoes ch and checks the plan is back at dump.
func (e *planToolEnv) undoRestores(ch *agent.Change, dump string) {
	e.t.Helper()
	_, err := e.changes.Undo(context.Background(), ch.ID)
	require.NoError(e.t, err)
	assert.Equal(e.t, dump, e.planDump(), "undo restores plates, sessions and overrides")
}

func (e *planToolEnv) slotID(name string) int64 {
	e.t.Helper()
	var id int64
	require.NoError(e.t, e.db.QueryRow(`SELECT id FROM time_slots WHERE name_key = ?`, "slot."+name).Scan(&id))
	return id
}

// plateAtCell returns the plate on (date, slot), nil when empty.
func (e *planToolEnv) plateAtCell(date string, slotID int64) *plate.Plate {
	e.t.Helper()
	for _, p := range e.week() {
		if p.DateString() == date && p.SlotID == slotID {
			return &p
		}
	}
	return nil
}

func applied0(t *testing.T, res map[string]any) map[string]any {
	t.Helper()
	a := res["applied"].([]any)
	require.NotEmpty(t, a)
	return a[0].(map[string]any)
}

// twoDinners returns the dinner plates of the window's 3rd and 4th day.
func (e *planToolEnv) twoDinners() (plate.Plate, plate.Plate) {
	e.t.Helper()
	dinner := e.slotID("dinner")
	a := e.plateAtCell(planStart.AddDate(0, 0, 2).Format(time.DateOnly), dinner)
	b := e.plateAtCell(planStart.AddDate(0, 0, 3).Format(time.DateOnly), dinner)
	require.NotNil(e.t, a)
	require.NotNil(e.t, b)
	return *a, *b
}

func cellJSON(date string, slotID int64) string {
	return fmt.Sprintf(`{"date":%q,"slot_id":%d}`, date, slotID)
}

func TestApplyChanges_MoveToEmptyCell(t *testing.T) {
	e := newPlanToolEnv(t)
	p, _ := e.twoDinners()
	lunch := e.slotID("lunch")
	if q := e.plateAtCell(p.DateString(), lunch); q != nil {
		_, err := e.plates.Delete(context.Background(), q.ID)
		require.NoError(t, err)
	}
	dump := e.planDump()

	dry, real, ch := e.mealOp(fmt.Sprintf(`[{"op":"move","plate_id":%d,"to":%s}]`, p.ID, cellJSON(p.DateString(), lunch)))
	assert.Equal(t, p.DateString(), applied0(t, dry)["date"])
	a := applied0(t, real)
	assert.EqualValues(t, lunch, a["slot_id"])
	assert.EqualValues(t, p.SlotID, a["from_slot_id"])
	moved, err := e.plates.Get(context.Background(), p.ID)
	require.NoError(t, err)
	assert.Equal(t, lunch, moved.SlotID)
	assert.Nil(t, e.plateAtCell(p.DateString(), p.SlotID), "the source cell is empty")
	line := opsOf(ch)[0]
	assert.Equal(t, "move", line["op"])
	assert.Equal(t, "slot.lunch", line["to_slot_key"])
	assert.Contains(t, ch.Dates, p.DateString())

	e.undoRestores(ch, dump)
}

func TestApplyChanges_MoveOccupant(t *testing.T) {
	cases := []struct {
		name     string
		occupant func(e *planToolEnv, q plate.Plate) (string, string) // json, the cell q ends on ("" removed)
	}{
		{"swap", func(_ *planToolEnv, _ plate.Plate) (string, string) { return `"swap"`, "source" }},
		{"drop", func(_ *planToolEnv, _ plate.Plate) (string, string) { return `"drop"`, "" }},
		{"to", func(e *planToolEnv, q plate.Plate) (string, string) {
			lunch := e.slotID("lunch")
			if r := e.plateAtCell(q.DateString(), lunch); r != nil {
				_, err := e.plates.Delete(context.Background(), r.ID)
				require.NoError(e.t, err)
			}
			return cellJSON(q.DateString(), lunch), fmt.Sprintf("%s/%d", q.DateString(), lunch)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newPlanToolEnv(t)
			p, q := e.twoDinners()
			occ, ends := tc.occupant(e, q)
			dump := e.planDump()

			_, real, ch := e.mealOp(fmt.Sprintf(`[{"op":"move","plate_id":%d,"to":%s,"occupant":%s}]`, p.ID, cellJSON(q.DateString(), q.SlotID), occ))
			a := applied0(t, real)
			assert.EqualValues(t, q.ID, a["occupant_plate_id"])
			moved, err := e.plates.Get(context.Background(), p.ID)
			require.NoError(t, err)
			assert.Equal(t, q.DateString(), moved.DateString(), "the plate moved onto the taken cell")
			other, err := e.plates.Get(context.Background(), q.ID)
			switch ends {
			case "":
				require.ErrorIs(t, err, domain.ErrNotFound, "dropped")
			case "source":
				require.NoError(t, err)
				assert.Equal(t, p.DateString(), other.DateString(), "swapped onto the source cell")
				assert.Equal(t, p.SlotID, other.SlotID)
			default:
				require.NoError(t, err)
				assert.Equal(t, ends, fmt.Sprintf("%s/%d", other.DateString(), other.SlotID))
			}
			line := opsOf(ch)[0]
			assert.Equal(t, tc.name, line["occupant"])
			assert.Equal(t, plateNamesOf(q), line["other"], "the change line names both meals")
			assert.Equal(t, plateNamesOf(p), line["food"])

			e.undoRestores(ch, dump)
		})
	}
}

func plateNamesOf(p plate.Plate) string {
	names := make([]string, len(p.Components))
	for i, c := range p.Components {
		names[i] = c.FoodName
	}
	return strings.Join(names, ", ")
}

func TestApplyChanges_MoveOntoTakenNeedsOccupant(t *testing.T) {
	e := newPlanToolEnv(t)
	p, q := e.twoDinners()
	_, err := e.exec("apply_changes", fmt.Sprintf(`{"ops":[{"op":"move","plate_id":%d,"to":%s}]}`, p.ID, cellJSON(q.DateString(), q.SlotID)))
	require.ErrorIs(t, err, domain.ErrCellOccupied)
	assert.Contains(t, err.Error(), fmt.Sprintf("holds plate %d (%s)", q.ID, plateNamesOf(q)))
	assert.Contains(t, err.Error(), `occupant "swap"`)
}

// unusedFood returns a food of role that is on no plate of the week.
func (e *planToolEnv) unusedFood(role string) int64 {
	e.t.Helper()
	var id int64
	require.NoError(e.t, e.db.QueryRow(`SELECT id FROM foods WHERE role = ? AND id NOT IN (SELECT food_id FROM plate_components) ORDER BY id LIMIT 1`, role).Scan(&id))
	return id
}

func TestApplyChanges_ReplaceMain(t *testing.T) {
	e := newPlanToolEnv(t)
	p, _ := e.twoDinners()
	food := e.unusedFood("main")
	dump := e.planDump()

	dry, real, ch := e.mealOp(fmt.Sprintf(`[{"op":"replace","plate_id":%d,"position":"main","food_id":%d}]`, p.ID, food))
	assert.EqualValues(t, food, applied0(t, dry)["food_id"])
	a := applied0(t, real)
	assert.Equal(t, "main", a["position"])
	got, err := e.plates.Get(context.Background(), p.ID)
	require.NoError(t, err)
	assert.True(t, slices.ContainsFunc(got.Components, func(c plate.PlateComponent) bool {
		return c.FoodID == food && c.Position == plate.PositionMain
	}), "the new food is the main")
	line := opsOf(ch)[0]
	assert.Equal(t, "replace", line["op"])
	assert.NotEmpty(t, line["to"])

	e.undoRestores(ch, dump)
}

func TestApplyChanges_ReplaceSide(t *testing.T) {
	e := newPlanToolEnv(t)
	p, _ := e.twoDinners()
	// Give the plate exactly one side.
	var side int64
	for _, c := range p.Components {
		if c.Position == plate.PositionSide {
			side = c.FoodID
		}
	}
	if side == 0 {
		side = e.sides[0]
		e.mustExec("apply_changes", fmt.Sprintf(`{"ops":[{"op":"add","plate_id":%d,"food_id":%d,"portions":1,"position":"side"}]}`, p.ID, side))
	}
	to := e.sides[0]
	if to == side {
		to = e.sides[1]
	}
	dump := e.planDump()

	_, real, ch := e.mealOp(fmt.Sprintf(`[{"op":"replace","plate_id":%d,"position":"side","food_id":%d}]`, p.ID, to))
	assert.Equal(t, "side", applied0(t, real)["position"])
	got, err := e.plates.Get(context.Background(), p.ID)
	require.NoError(t, err)
	var sides []int64
	for _, c := range got.Components {
		if c.Position == plate.PositionSide {
			sides = append(sides, c.FoodID)
		}
	}
	assert.Equal(t, []int64{to}, sides)

	e.undoRestores(ch, dump)
}

// noCookOn reports whether a session cooks on (date, slot).
func (e *planToolEnv) noCookOn(date string, slotID int64) bool {
	e.t.Helper()
	var n int
	require.NoError(e.t, e.db.QueryRow(`SELECT COUNT(*) FROM cook_sessions WHERE cook_date = ? AND cook_slot_id = ?`, date, slotID).Scan(&n))
	return n == 0
}

func TestApplyChanges_Away(t *testing.T) {
	for _, mode := range []string{"cells", "date"} {
		t.Run(mode, func(t *testing.T) {
			e := newPlanToolEnv(t)
			day := planStart.AddDate(0, 0, 2).Format(time.DateOnly)
			slots := []int64{e.slotID("breakfast"), e.slotID("lunch"), e.slotID("dinner")}
			op := fmt.Sprintf(`{"op":"away","date":%q,"note":"out"}`, day)
			if mode == "cells" {
				op = fmt.Sprintf(`{"op":"away","cells":[%s,%s,%s],"note":"out"}`,
					cellJSON(day, slots[0]), cellJSON(day, slots[1]), cellJSON(day, slots[2]))
			}
			dump := e.planDump()

			dry, real, ch := e.mealOp("[" + op + "]")
			assert.Len(t, applied0(t, dry)["cells"], 3)
			for _, res := range []map[string]any{dry, real} {
				assert.NotContains(t, issueRules(t, res, "new"), "day energy floor", "a day away is not judged for calories")
			}
			for _, s := range slots {
				assert.Nil(t, e.plateAtCell(day, s), "no plate on an away cell")
				assert.True(t, e.noCookOn(day, s), "no cook on an away cell")
				o, err := e.avail.Override(context.Background(), planStart.AddDate(0, 0, 2), s)
				require.NoError(t, err)
				require.NotNil(t, o)
				assert.Equal(t, availability.ModeNone, o.Mode)
			}
			assert.Len(t, ch.Cells, 3, "the change stores every cell")
			assert.Len(t, opsOf(ch)[0]["cells"], 3)

			e.undoRestores(ch, dump)
		})
	}
}

// laterMealSession returns a draft batch that cooks at a dinner and feeds a
// lunch the next day (made here by a leftover copy), with the dinner plate.
func (e *planToolEnv) laterMealSession() (*plate.SessionView, plate.Plate) {
	e.t.Helper()
	p, _ := e.twoDinners()
	// A new main cooked fresh at the dinner.
	require.NoError(e.t, testsvc.Settings(e.db).Set(context.Background(), "plan.max_cook_occasions_per_day", "3"))
	e.mustExec("apply_changes", fmt.Sprintf(`{"ops":[{"op":"replace","plate_id":%d,"position":"main","food_id":%d}]}`, p.ID, e.unusedFood("main")))
	p = *e.plateAtCell(p.DateString(), p.SlotID)
	var sid int64
	for _, c := range p.Components {
		if c.Position == plate.PositionMain && c.CookSessionID != nil {
			sid = *c.CookSessionID
		}
	}
	require.NotZero(e.t, sid)
	v := e.session(sid)
	require.True(e.t, v.CookDate.Equal(p.Date) && v.CookSlotID == p.SlotID, "the dinner cooks its main")
	lunch := e.slotID("lunch")
	next := p.Date.AddDate(0, 0, 1).Format(time.DateOnly)
	if q := e.plateAtCell(next, lunch); q != nil {
		_, err := e.plates.Delete(context.Background(), q.ID)
		require.NoError(e.t, err)
	}
	e.mustExec("apply_changes", fmt.Sprintf(`{"ops":[{"op":"add","date":%q,"slot_id":%d,"food_id":%d,"portions":1,"session_id":%d}]}`, next, lunch, v.FoodID, sid))
	return e.session(sid), p
}

func TestApplyChanges_MoveCook(t *testing.T) {
	e := newPlanToolEnv(t)
	v, p := e.laterMealSession()
	day := p.DateString()

	// After its first meal: refused, nothing written.
	before := e.planDump()
	late := v.CookDate.AddDate(0, 0, 1).Format(time.DateOnly)
	_, err := e.exec("apply_changes", fmt.Sprintf(`{"ops":[{"op":"move_cook","session_id":%d,"cook_at":%s}]}`, v.ID, cellJSON(late, v.CookSlotID)))
	require.ErrorIs(t, err, domain.ErrInvalidInput)
	assert.Contains(t, err.Error(), "op 1 of 1 (move_cook)")
	assert.Equal(t, before, e.planDump())

	// Earlier the same day: allowed.
	breakfast := e.slotID("breakfast")
	dry, real, ch := e.mealOp(fmt.Sprintf(`[{"op":"move_cook","session_id":%d,"cook_at":%s}]`, v.ID, cellJSON(day, breakfast)))
	assert.EqualValues(t, breakfast, applied0(t, dry)["cook_slot_id"])
	assert.EqualValues(t, v.CookSlotID, applied0(t, real)["from_slot_id"])
	assert.Equal(t, breakfast, e.session(v.ID).CookSlotID)
	assert.Equal(t, "move_cook", opsOf(ch)[0]["op"])

	e.undoRestores(ch, before)
}

func TestApplyChanges_Copy(t *testing.T) {
	for _, as := range []string{"leftover", "fresh"} {
		t.Run(as, func(t *testing.T) {
			e := newPlanToolEnv(t)
			p, _ := e.twoDinners()
			lunch := e.slotID("lunch")
			next := p.Date.AddDate(0, 0, 1).Format(time.DateOnly)
			if q := e.plateAtCell(next, lunch); q != nil {
				_, err := e.plates.Delete(context.Background(), q.ID)
				require.NoError(t, err)
			}
			dump := e.planDump()

			_, real, ch := e.mealOp(fmt.Sprintf(`[{"op":"copy","plate_id":%d,"to":[%s],"as":%q}]`, p.ID, cellJSON(next, lunch), as))
			copies := applied0(t, real)["copies"].([]any)
			require.Len(t, copies, 1)
			cp := e.plateAtCell(next, lunch)
			require.NotNil(t, cp)
			src, err := e.plates.Get(context.Background(), p.ID)
			require.NoError(t, err)
			for i, c := range cp.Components {
				require.NotNil(t, c.CookSessionID)
				same := *c.CookSessionID == *src.Components[i].CookSessionID
				assert.Equal(t, as == "leftover", same, "leftover eats the same batch, fresh cooks again")
			}
			assert.Equal(t, as, opsOf(ch)[0]["as"])

			e.undoRestores(ch, dump)
		})
	}
}

// Several availability ops in one call are one change; undo restores every
// cell and is stale when any cell was edited since.
func TestApplyChanges_MultiCellAvailabilityUndo(t *testing.T) {
	setup := func(t *testing.T) (*planToolEnv, *agent.Change, string, []plate.Plate) {
		e := newPlanToolEnv(t)
		p, q := e.twoDinners()
		dump := e.planDump()
		ctx, _ := e.chat()
		ch := e.change(ctx, "apply_changes", fmt.Sprintf(`{"ops":[
			{"op":"set_availability","date":%q,"slot_id":%d,"mode":"none"},
			{"op":"set_availability","date":%q,"slot_id":%d,"mode":"none","note":"party"}]}`,
			p.DateString(), p.SlotID, q.DateString(), q.SlotID))
		require.Len(t, ch.Cells, 2)
		stored, err := sqliteChange(e, ch.ID)
		require.NoError(t, err)
		require.Len(t, stored.Cells, 2, "the stored change keeps both cells")
		return e, ch, dump, []plate.Plate{p, q}
	}
	t.Run("restores all", func(t *testing.T) {
		e, ch, dump, _ := setup(t)
		e.undoRestores(ch, dump)
	})
	t.Run("stale", func(t *testing.T) {
		e, ch, _, cells := setup(t)
		note := "changed my mind"
		_, _, err := e.avail.SetOverride(context.Background(), availability.OverrideInput{Date: cells[1].Date, SlotID: cells[1].SlotID, Mode: availability.ModeNone, Note: &note})
		require.NoError(t, err)
		_, err = e.changes.Undo(context.Background(), ch.ID)
		require.ErrorIs(t, err, domain.ErrAIChangeStale)
	})
}

func TestApplyChanges_UnknownFieldNamesTheField(t *testing.T) {
	e := newPlanToolEnv(t)
	p, _ := e.twoDinners()
	_, err := e.exec("apply_changes", fmt.Sprintf(`{"ops":[{"op":"set_quantity","plate_component_id":%d,"quantity":2}]}`, p.Components[0].ID))
	require.ErrorIs(t, err, domain.ErrInvalidInput)
	assert.Contains(t, err.Error(), `op 1 (set_quantity): unknown field "quantity"`)
	assert.Contains(t, err.Error(), "set_quantity takes plate_component_id, portions or amount + unit")

	// A field of another op is refused too.
	_, err = e.exec("apply_changes", fmt.Sprintf(`{"ops":[{"op":"remove","plate_component_id":%d,"to":{"date":"2026-11-08","slot_id":1}}]}`, p.Components[0].ID))
	require.ErrorIs(t, err, domain.ErrInvalidInput)
	assert.Contains(t, err.Error(), `op 1 (remove): unknown field "to"; remove takes plate_component_id`)
}

func TestFindAlternatives(t *testing.T) {
	e := newPlanToolEnv(t)
	p, _ := e.twoDinners()
	var main plate.PlateComponent
	for _, c := range p.Components {
		if c.Position == plate.PositionMain {
			main = c
		}
	}
	require.NotZero(t, main.ID)

	// A fresh main next to the batch still cooking there is a second
	// cooking occasion: allow two a day so fresh mains qualify.
	require.NoError(t, testsvc.Settings(e.db).Set(context.Background(), "plan.max_cook_occasions_per_day", "2"))
	res := e.mustExec("find_alternatives", fmt.Sprintf(`{"plate_id":%d,"what":"main","limit":10}`, p.ID))
	alts := res["alternatives"].([]any)
	require.NotEmpty(t, alts)
	for _, a := range alts {
		m := a.(map[string]any)
		assert.NotEqualValues(t, main.FoodID, m["food_id"], "the replaced food is excluded")
		assert.NotEmpty(t, m["ops"], "each alternative carries its ops")
	}

	// Constraints narrow the list.
	res = e.mustExec("find_alternatives", fmt.Sprintf(`{"plate_id":%d,"what":"main","constraints":{"search":"Chili"}}`, p.ID))
	for _, a := range res["alternatives"].([]any) {
		assert.Equal(t, "Chili", a.(map[string]any)["name"])
	}

	// Code may call it.
	code := llm.WithCaller(e.ctx, &llm.Caller{Type: llm.CallerCodeExecution, ToolID: "srv1"})
	_, _, _, err := e.ts.Execute(code, "find_alternatives", json.RawMessage(fmt.Sprintf(`{"plate_id":%d,"what":"add"}`, p.ID)))
	require.NoError(t, err)

	// An alternative's ops apply as they are.
	first := alts[0].(map[string]any)
	ops, err := json.Marshal(first["ops"])
	require.NoError(t, err)
	e.mustExec("apply_changes", `{"dry_run":true,"ops":`+string(ops)+`}`)
}

// With one cooking occasion a day, a plate whose cell does not cook gets
// cook_at alternatives (cooked with an existing cooking cell); their replace
// op applies through apply_changes and adds no H2.
func TestFindAlternatives_CookAtAppliesWithoutH2(t *testing.T) {
	e := newPlanToolEnv(t)
	require.NoError(t, testsvc.Settings(e.db).Set(context.Background(), "plan.max_cook_occasions_per_day", "1"))
	lunch := e.slotID("lunch")
	var pick map[string]any
	for d := 0; d < 7 && pick == nil; d++ {
		p := e.plateAtCell(planStart.AddDate(0, 0, d).Format(time.DateOnly), lunch)
		if p == nil {
			continue
		}
		res := e.mustExec("find_alternatives", fmt.Sprintf(`{"plate_id":%d,"what":"main","limit":10}`, p.ID))
		for _, a := range res["alternatives"].([]any) {
			if m := a.(map[string]any); m["source"] == "cook_at" {
				pick = m
				break
			}
		}
	}
	require.NotNil(t, pick, "a lunch plate with a cook_at alternative")
	require.NotNil(t, pick["cook_at"])
	ops := pick["ops"].([]any)
	require.Len(t, ops, 1)
	op := ops[0].(map[string]any)
	assert.Equal(t, "replace", op["op"])
	assert.Equal(t, pick["cook_at"], op["cook_at"])
	raw, err := json.Marshal(ops)
	require.NoError(t, err)
	res := e.mustExec("apply_changes", `{"dry_run":false,"ops":`+string(raw)+`}`)
	issues := res["after"].(map[string]any)["issues"].(map[string]any)
	news, _ := issues["new"].([]any)
	for _, is := range news {
		assert.NotEqual(t, "H2", is.(map[string]any)["rule"], "%v", is)
	}
}

// A batch that exactly covers its meals is offered as a session source and
// grows with the plate as the evaluation assumed: after the apply its
// portions equal the old ones plus the plate's, covering its meals, and no
// H3 appears.
func TestFindAlternatives_GrowingSessionAppliesAsEvaluated(t *testing.T) {
	e := newPlanToolEnv(t)
	v, _ := e.laterMealSession()
	require.InDelta(t, v.Consumed, v.Portions, plate.PortionEpsilon, "the batch exactly covers its meals")
	_, target := e.twoDinners()
	target = *e.plateAtCell(target.DateString(), target.SlotID)
	need := 0.0
	for _, c := range target.Components {
		if c.Position == plate.PositionMain {
			need = *c.Portions * target.Eaters
		}
	}
	require.Positive(t, need)

	res := e.mustExec("find_alternatives", fmt.Sprintf(`{"plate_id":%d,"what":"main","limit":10}`, target.ID))
	var pick map[string]any
	for _, a := range res["alternatives"].([]any) {
		if m := a.(map[string]any); m["session_id"] == float64(v.ID) {
			pick = m
		}
	}
	require.NotNil(t, pick, "the exactly covering batch is offered: %v", res)
	assert.Equal(t, "session", pick["source"])
	ops, err := json.Marshal(pick["ops"])
	require.NoError(t, err)
	out := e.mustExec("apply_changes", `{"ops":`+string(ops)+`}`)

	ops0 := pick["ops"].([]any)
	require.Len(t, ops0, 1, "a session alternative is one replace")
	assert.Equal(t, "replace", ops0[0].(map[string]any)["op"])
	after := e.session(v.ID)
	assert.InDelta(t, v.Portions+need, after.Portions, plate.PortionEpsilon, "grown by the plate's portions")
	assert.InDelta(t, after.Consumed, after.Portions, plate.PortionEpsilon, "still exactly covers its meals")
	issues := out["after"].(map[string]any)["issues"].(map[string]any)
	news, _ := issues["new"].([]any)
	for _, is := range news {
		assert.NotEqual(t, "H3", is.(map[string]any)["rule"], "%v", is)
	}
	got, err := e.plates.Get(context.Background(), target.ID)
	require.NoError(t, err)
	assert.True(t, slices.ContainsFunc(got.Components, func(c plate.PlateComponent) bool {
		return c.FoodID == int64(pick["food_id"].(float64)) && c.Position == plate.PositionMain
	}), "the batch's food took the main position")
}

// A dish the plan already cooks this week, cooked fresh a second time on the
// saved plan, is the warning variety_repeat_week, not a hard variety_repeat
// (the user chose the dish).
func TestApplyChanges_WindowRepeatIsAWarning(t *testing.T) {
	e := newPlanToolEnv(t)
	require.NoError(t, testsvc.Settings(e.db).Set(context.Background(), "plan.max_cook_occasions_per_day", "3"))
	a, b := e.twoDinners()
	var main plate.PlateComponent
	for _, c := range a.Components {
		if c.Position == plate.PositionMain {
			main = c
		}
	}
	require.NotZero(t, main.ID)

	res := e.mustExec("apply_changes", fmt.Sprintf(`{"dry_run":true,"ops":[{"op":"replace","plate_id":%d,"position":"main","food_id":%d}]}`, b.ID, main.FoodID))
	issues := res["after"].(map[string]any)["issues"].(map[string]any)
	var codes []string
	for _, is := range issues["new"].([]any) {
		m := is.(map[string]any)
		codes = append(codes, m["label"].(string))
		if m["label"] == "dish repeated this week" {
			assert.Equal(t, "warning", m["severity"])
		}
	}
	assert.Contains(t, codes, "dish repeated this week")
	assert.NotContains(t, codes, "dish variety")
}

func sqliteChange(e *planToolEnv, id int64) (*agent.Change, error) {
	return sqlite.NewAIChangeRepo(e.db).GetChange(context.Background(), id)
}
