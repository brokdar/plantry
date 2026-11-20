package agent_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper/testsvc"
)

// awayCells marks the three slots of date as away (a real write).
func (e *planToolEnv) awayCells(date string) []string {
	e.t.Helper()
	var cells []string
	for _, s := range []string{"breakfast", "lunch", "dinner"} {
		cells = append(cells, cellJSON(date, e.slotID(s)))
	}
	e.mustExec("apply_changes", `{"ops":[{"op":"away","cells":[`+strings.Join(cells, ",")+`]}]}`)
	return cells
}

// A cook cell that is marked No plate is refused by every op that names one,
// with the reason and at most three valid cells nearby.
func TestApplyChanges_CookCellUnavailable(t *testing.T) {
	t.Parallel()
	e := newPlanToolEnv(t)
	v, p := e.laterMealSession()
	require.NoError(t, testsvc.Settings(e.db).Set(context.Background(), "plan.max_cook_occasions_per_day", "3"))
	day := p.DateString()
	breakfast := e.slotID("breakfast")
	e.mustExec("apply_changes", `{"ops":[{"op":"away","cells":[`+cellJSON(day, breakfast)+`]}]}`)
	other := e.plateAtCell(p.Date.AddDate(0, 0, 2).Format(time.DateOnly), e.slotID("lunch"))
	require.NotNil(t, other)

	for _, tc := range []struct{ name, op string }{
		{"move_cook", fmt.Sprintf(`{"op":"move_cook","session_id":%d,"cook_at":%s}`, v.ID, cellJSON(day, breakfast))},
		{"add cook_at", fmt.Sprintf(`{"op":"add","plate_id":%d,"food_id":%d,"portions":1,"position":"main","cook_at":%s}`, other.ID, e.unusedFood("main"), cellJSON(day, breakfast))},
		{"replace cook_at", fmt.Sprintf(`{"op":"replace","plate_id":%d,"position":"main","food_id":%d,"cook_at":%s}`, other.ID, e.unusedFood("main"), cellJSON(day, breakfast))},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := e.planDump()
			_, err := e.exec("apply_changes", `{"dry_run":true,"ops":[`+tc.op+`]}`)
			require.ErrorIs(t, err, domain.ErrCellUnavailable)
			assert.Contains(t, err.Error(), "nobody is home")
			assert.NotRegexp(t, ruleToken, err.Error(), "the refusal names no rule code")
			_, hint, ok := strings.Cut(err.Error(), "instead: ")
			require.True(t, ok, err.Error())
			n := strings.Count(hint, "slot_id")
			assert.True(t, n >= 1 && n <= 3, "up to three valid cells: %s", hint)
			assert.NotContains(t, hint, day+" slot_id "+fmt.Sprint(breakfast), "the away cell is not suggested")
			assert.Equal(t, before, e.planDump())
		})
	}
}

// An away op earlier in the same call counts for a later move_cook.
func TestApplyChanges_CookCellMadeAwayInTheSameCall(t *testing.T) {
	t.Parallel()
	e := newPlanToolEnv(t)
	v, p := e.laterMealSession()
	day, breakfast := p.DateString(), e.slotID("breakfast")
	_, err := e.exec("apply_changes", fmt.Sprintf(`{"dry_run":true,"ops":[{"op":"away","cells":[%s]},{"op":"move_cook","session_id":%d,"cook_at":%s}]}`,
		cellJSON(day, breakfast), v.ID, cellJSON(day, breakfast)))
	require.ErrorIs(t, err, domain.ErrCellUnavailable)
}

// walkOptions collects every option object (and its op list) in a tool result.
func walkOptions(v any, out *[][]any) {
	switch x := v.(type) {
	case map[string]any:
		if opts, ok := x["options"].([]any); ok {
			for _, o := range opts {
				m := o.(map[string]any)
				if ops, ok := m["ops"].([]any); ok {
					*out = append(*out, ops)
				} else if op, ok := m["op"]; ok {
					*out = append(*out, []any{op})
				}
			}
		}
		for _, c := range x {
			walkOptions(c, out)
		}
	case []any:
		for _, c := range v.([]any) {
			walkOptions(c, out)
		}
	}
}

// Property: every option the tools emit, for the plan states below, passes its
// own dry run without a new hard issue and names no cook cell that is away.
func TestOptions_PassTheirOwnDryRun(t *testing.T) {
	t.Parallel()
	d5 := planStart.AddDate(0, 0, 5).Format(time.DateOnly)
	// Each state returns the day its probe marks away (a dry run) and the cells
	// it made away for real.
	states := map[string]func(e *planToolEnv) (string, []string){
		"generated plan": func(e *planToolEnv) (string, []string) { return d5, nil },
		"tight cook limit": func(e *planToolEnv) (string, []string) {
			require.NoError(t, testsvc.Settings(e.db).Set(context.Background(), "plan.max_cook_occasions_per_day", "1"))
			return d5, nil
		},
		// A batch cooks at a dinner and feeds the next lunch: away on the
		// dinner day hands its cook to that lunch, over the cook limit.
		"cook day goes away": func(e *planToolEnv) (string, []string) {
			_, p := e.laterMealSession()
			require.NoError(t, testsvc.Settings(e.db).Set(context.Background(), "plan.max_cook_occasions_per_day", "1"))
			return p.DateString(), nil
		},
		"cells already away": func(e *planToolEnv) (string, []string) {
			_, p := e.laterMealSession()
			require.NoError(t, testsvc.Settings(e.db).Set(context.Background(), "plan.max_cook_occasions_per_day", "1"))
			return p.DateString(), e.awayCells(p.Date.AddDate(0, 0, -1).Format(time.DateOnly))
		},
	}
	// The cook day goes away with its neighbours: the only cook cells left are
	// on the day itself (one batch moved to a cook cell of it, or all of them).
	states["neighbours away"] = func(e *planToolEnv) (string, []string) {
		_, p := e.laterMealSession()
		require.NoError(t, testsvc.Settings(e.db).Set(context.Background(), "plan.max_cook_occasions_per_day", "1"))
		var away []string
		for _, off := range []int{-2, -1, 2, 3} {
			away = append(away, e.awayCells(p.Date.AddDate(0, 0, off).Format(time.DateOnly))...)
		}
		return p.DateString(), away
	}
	for name, setup := range states {
		t.Run(name, func(t *testing.T) {
			e := newPlanToolEnv(t)
			day, away := setup(e)
			var cells []string
			for _, s := range []string{"breakfast", "lunch", "dinner"} {
				cells = append(cells, cellJSON(day, e.slotID(s)))
			}
			var results []map[string]any
			results = append(results, e.mustExec("get_plan", `{}`))
			awayOp := `{"op":"away","cells":[` + strings.Join(cells, ",") + `]}`
			results = append(results, e.mustExec("apply_changes", `{"dry_run":true,"ops":[`+awayOp+`]}`))
			awayDay := map[string]bool{}
			for _, c := range append(away, cells...) {
				var r struct {
					Date   string `json:"date"`
					SlotID int64  `json:"slot_id"`
				}
				require.NoError(t, json.Unmarshal([]byte(c), &r))
				awayDay[fmt.Sprint(r.Date, r.SlotID)] = true
			}

			checked := 0
			for i, res := range results {
				var options [][]any
				walkOptions(res, &options)
				for _, ops := range options {
					for _, o := range ops {
						m := o.(map[string]any)
						if c, ok := m["cook_at"].(map[string]any); ok && i == 1 {
							assert.False(t, awayDay[fmt.Sprint(c["date"], int64(c["slot_id"].(float64)))], "option cooks on an away cell: %v", m)
						}
					}
					raw, err := json.Marshal(ops)
					require.NoError(t, err)
					// The away probe's options fix the plan after the away.
					call := string(raw)
					if i == 1 {
						call = "[" + awayOp + "," + string(raw)[1:]
					}
					dry, err := e.exec("apply_changes", `{"dry_run":true,"ops":`+call+`}`)
					if err != nil {
						t.Errorf("option %s fails its dry run: %v", raw, err)
						continue
					}
					checked++
					issues, _ := dry["after"].(map[string]any)["issues"].(map[string]any)
					for _, is := range issues["new"].([]any) {
						m := is.(map[string]any)
						assert.NotEqual(t, "violation", m["severity"], "option %s adds a hard issue: %v", raw, m)
					}
				}
			}
			t.Logf("%d options checked", checked)
			// With a cap of two dishes per cooking time the day's own cook cells
			// may offer nothing: what is offered passes its own dry run.
		})
	}
}

// An away that takes a cook's cell away hands the cook to the first meal that
// eats it; when that day already cooks, the call moves that day's other cooks
// into the same cell instead of leaving a second cooking occasion (the real
// plan: away on Sunday, the curry cooked Sunday feeds Monday lunch, Monday
// dinner cooks the salmon).
func TestApplyChanges_AwayRehomesCooks(t *testing.T) {
	t.Parallel()
	e := newPlanToolEnv(t)
	_, p := e.laterMealSession()
	require.NoError(t, testsvc.Settings(e.db).Set(context.Background(), "plan.max_cook_occasions_per_day", "1"))
	next := p.Date.AddDate(0, 0, 1).Format(time.DateOnly)
	lunch, dinner := e.slotID("lunch"), e.slotID("dinner")
	// The next day cooks at its dinner only (its breakfast plate is gone).
	if q := e.plateAtCell(next, e.slotID("breakfast")); q != nil {
		_, err := e.plates.Delete(context.Background(), q.ID)
		require.NoError(t, err)
	}
	require.NotNil(t, e.plateAtCell(next, dinner))
	before := e.planDump()

	away := fmt.Sprintf(`{"op":"away","cells":[%s,%s]}`, cellJSON(p.DateString(), lunch), cellJSON(p.DateString(), dinner))
	res := e.mustExec("apply_changes", `{"dry_run":true,"ops":[`+away+`]}`)
	issues := res["after"].(map[string]any)["issues"].(map[string]any)
	var autos []map[string]any
	for _, a := range res["applied"].([]any) {
		if m := a.(map[string]any); m["auto_fix"] != nil {
			autos = append(autos, m)
		}
	}
	var moves []map[string]any
	for _, m := range autos {
		assert.NotRegexp(t, ruleToken, m["auto_fix"], "auto_fix names no rule code")
		if m["op"] != "move_cook" {
			// A dish that did not fit one cooking time is swapped in the same change.
			assert.Contains(t, m["auto_fix"], "at most two dishes", "the swap says why")
			continue
		}
		moves = append(moves, m)
		assert.NotNil(t, m["cook_date"], "the result says where the cook went")
	}
	if len(moves) > 0 {
		facts, _ := res["cooking_moved"].(map[string]any)
		require.NotNil(t, facts, "the result says where the cooks went")
		assert.Equal(t, true, facts["nothing_else_changed"])
	}
	for _, is := range issues["new"].([]any) {
		m := is.(map[string]any)
		if m["label"] == "cooking times per day" {
			assert.NotEmpty(t, m["options"], "an open cooking problem comes with an option: %v", m)
		}
	}
	assert.Equal(t, before, e.planDump(), "a dry run saves nothing")
	whole, err := json.Marshal(res)
	require.NoError(t, err)
	assert.NotRegexp(t, ruleToken, string(whole), "nothing the model reads names a rule code")
	assert.NotContains(t, string(whole), "too_many_cook_occasions")

	// For real: the cooks are where the result says, and none is piled up.
	e.mustExec("apply_changes", `{"ops":[`+away+`]}`)
	for _, m := range moves {
		v := e.session(int64(m["session_id"].(float64)))
		assert.Equal(t, m["cook_date"], v.CookDate.Format(time.DateOnly))
		assert.EqualValues(t, m["cook_slot_id"], v.CookSlotID)
	}
	// The household's cook-once day wins on the away path: the dishes move onto
	// one cooking time, the result says which and that nothing else changed.
	fresh := e.mustExec("get_plan", `{}`)
	for _, is := range fresh["issues"].([]any) {
		assert.NotEqual(t, "cooking times per day", is.(map[string]any)["label"], "no cooking problem is left open")
	}
}
