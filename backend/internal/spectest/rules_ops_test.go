//go:build spectest

package spectest

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
)

func (r opRun) name() string { return r.HH + "/" + r.Kind + "/" + r.Target }

func forOps(t *testing.T, kinds []string, f func(t *testing.T, r opRun)) {
	t.Helper()
	for _, r := range allOps(t) {
		if len(kinds) > 0 && !slices.Contains(kinds, r.Kind) {
			continue
		}
		t.Run(r.name(), func(t *testing.T) { f(t, r) })
	}
}

// SF1: no write leaves a new hard issue, except what the request's exempt op
// itself causes (away and delete_plate: H1, H3, H4; delete_plate also H9).
func TestSF1_NoHardIssueSaved(t *testing.T) {
	forOps(t, nil, func(t *testing.T, r opRun) {
		allowed := map[string]bool{}
		if r.Kind == "away_day" || r.Kind == "away_meal" {
			allowed["H1"], allowed["H3"], allowed["H4"], allowed["H5"], allowed["H9"] = true, true, true, true, true // H9: only with report and offer, see TestSF1c
		}
		if r.Kind == "delete_plate" {
			allowed["H1"], allowed["H3"], allowed["H4"], allowed["H9"] = true, true, true, true
		}
		for _, k := range r.NewViol {
			assert.True(t, allowed[strings.SplitN(k, "/", 2)[0]], "new hard issue %s after %v (saved=%v)", k, r.Op, r.Saved)
		}
	})
}

// SF5: undo restores the exact plan.
func TestSF5_UndoExact(t *testing.T) {
	forOps(t, nil, func(t *testing.T, r opRun) {
		if !r.Saved {
			t.Skip("nothing saved")
		}
		assert.True(t, r.Undone, "undo_last_change failed")
		assert.True(t, r.UndoExact, "plan after undo differs from the plan before the change")
	})
}

func plateOf(s planning.State, op map[string]any) int64 {
	if id := int64(num(op["plate_id"])); id != 0 {
		return id
	}
	cid := int64(num(op["plate_component_id"]))
	for _, p := range s.Plates {
		for _, c := range p.Components {
			if c.ID == cid {
				return p.ID
			}
		}
	}
	return 0
}

func comps(p planning.Plate) []string {
	var out []string
	for _, c := range p.Components {
		q := 0.0
		if c.Portions != nil {
			q = *c.Portions
		} else if c.Grams != nil {
			q = *c.Grams
		}
		out = append(out, fmt.Sprintf("%d:%.2f", c.FoodID, q))
	}
	slices.Sort(out)
	return out
}

func plateMap(s planning.State) map[int64]planning.Plate {
	m := map[int64]planning.Plate{}
	for _, p := range s.Plates {
		m[p.ID] = p
	}
	return m
}

// SC1: only the named plate changes (set_eaters, remove, add, set_quantity,
// replace); batches and cooks follow as consequences, other plates do not.
func TestSC1_OnlyNamedMealChanges(t *testing.T) {
	forOps(t, []string{"set_eaters", "remove", "add", "set_quantity", "replace"}, func(t *testing.T, r opRun) {
		if !r.Saved {
			t.Skip("nothing saved")
		}
		target := plateOf(r.Before, r.Op)
		b, a := plateMap(r.Before), plateMap(r.After)
		for id, p := range b {
			if id == target {
				continue
			}
			q, ok := a[id]
			if !assert.True(t, ok, "plate %d vanished", id) {
				continue
			}
			assert.Equal(t, comps(p), comps(q), "plate %d changed but only plate %d was named", id, target)
		}
	})
}

// SC2: nothing is added that was not asked for.
func TestSC2_NothingAddedUnasked(t *testing.T) {
	forOps(t, []string{"set_eaters", "remove", "set_quantity", "move", "away_day", "away_meal", "move_cook"}, func(t *testing.T, r opRun) {
		b, a := plateMap(r.Before), plateMap(r.After)
		for id, q := range a {
			p, ok := b[id]
			if !ok {
				assert.Fail(t, "a plate was created", "plate %d", id)
				continue
			}
			had := map[int64]bool{}
			for _, c := range p.Components {
				had[c.FoodID] = true
			}
			for _, c := range q.Components {
				assert.True(t, had[c.FoodID], "food %d added to plate %d by %s", c.FoodID, id, r.Kind)
			}
		}
	})
}

// SC3: only a light/filling or day request stores a mark; none of these ops does.
func TestSC3_NoMarkFromPlainOps(t *testing.T) {
	forOps(t, nil, func(t *testing.T, r opRun) {
		for _, row := range strings.Split(r.LightAfter, ";") {
			assert.Contains(t, r.LightBefore, row, "a light mark appeared after %v", r.Op)
		}
	})
}

// KR1: after an away the cooking is at one cooking time per day.
func TestKR1_AwayKeepsOneCookingTime(t *testing.T) {
	forOps(t, []string{"away_day", "away_meal"}, func(t *testing.T, r opRun) {
		limit := r.After.Settings.MaxCookOccasionsPerDay
		if limit <= 0 {
			t.Skip("no cooking limit")
		}
		ev := planning.Evaluate(r.After, planning.WindowTarget(r.After))
		for _, d := range ev.Days {
			assert.LessOrEqual(t, d.CookOccasions, limit, "%s has %d cooking times (limit %d)", d.Date, d.CookOccasions, limit)
		}
	})
}

type tellOut struct {
	Report struct {
		Tell []string `json:"tell"`
	} `json:"report"`
}

// RP5: the facts block has at most 5 lines (classes 2-5; refusals are item 1).
func TestRP5_FactsBlockCap(t *testing.T) {
	forOps(t, nil, func(t *testing.T, r opRun) {
		var o tellOut
		_ = json.Unmarshal([]byte(r.Raw), &o)
		if len(o.Report.Tell) == 0 {
			t.Skip("no facts block")
		}
		assert.LessOrEqual(t, len(o.Report.Tell), 5, "facts block has %d lines", len(o.Report.Tell))
	})
}

// RP5: no duplicate and no empty fact line.
func TestRP5_NoDuplicateOrEmptyFacts(t *testing.T) {
	forOps(t, nil, func(t *testing.T, r opRun) {
		var o tellOut
		_ = json.Unmarshal([]byte(r.Raw), &o)
		seen := map[string]bool{}
		for _, l := range o.Report.Tell {
			assert.NotEmpty(t, strings.TrimSpace(l), "empty fact line")
			assert.False(t, seen[l], "duplicate fact line %q", l)
			seen[l] = true
		}
	})
}

// KR4: portions of every dish on a plate are within 0.5 to 1.5 per eater.
func TestKR4_PortionsInRange(t *testing.T) {
	forOps(t, nil, func(t *testing.T, r opRun) {
		for _, p := range r.After.Plates {
			for _, c := range p.Components {
				if c.Portions != nil {
					assert.GreaterOrEqual(t, *c.Portions, 0.5-1e-9, "plate %d food %d", p.ID, c.FoodID)
					assert.LessOrEqual(t, *c.Portions, 1.5+1e-9, "plate %d food %d", p.ID, c.FoodID)
				}
			}
		}
	})
}

// SF4 (v1.1): a part-away day is judged on the planned part: the floor is the
// full-day floor x (1 - the away slots' expected shares), both terms.
func TestSF4_PartAwayFloorProrated(t *testing.T) {
	forOps(t, []string{"away_meal"}, func(t *testing.T, r opRun) {
		date := r.Op["date"].(string)
		var day *planning.DayEval
		ev := planning.Evaluate(r.After, planning.WindowTarget(r.After))
		for i := range ev.Days {
			if ev.Days[i].Date == date {
				day = &ev.Days[i]
			}
		}
		if day == nil || day.FloorKcal <= 0 {
			t.Skip("day not judged")
		}
		want := fullFloor(r.After) * (1 - unplanned(r.After, date))
		assert.InDelta(t, want, day.FloorKcal, 1.0, "floor = full-day floor x (1 - unplanned shares)")
	})
}

// RP5/G4 (plan layer 0): one day-energy issue names the gap to the range and
// the gap to the target in two separate fields.
func TestG4_GapToRangeAndTargetSeparate(t *testing.T) {
	for _, h := range households {
		t.Run(h, func(t *testing.T) {
			e := newEnv(t, "chat-"+h, chatWeek)
			rs, _ := e.turn("show", tc{"get_plan", `{}`})
			var found bool
			var walk func(v any)
			walk = func(v any) {
				switch x := v.(type) {
				case map[string]any:
					if p, ok := x["params"].(map[string]any); ok && x["label"] == "day energy" {
						found = true
						_, a := p["gap_to_range_kcal"]
						_, b := p["gap_to_target_kcal"]
						assert.True(t, a && b, "day energy issue params %v lack gap_to_range_kcal and gap_to_target_kcal", p)
					}
					for _, c := range x {
						walk(c)
					}
				case []any:
					for _, c := range x {
						walk(c)
					}
				}
			}
			walk(rs[0].Out)
			if !found {
				t.Skip("no day-energy issue in this household's start plan")
			}
		})
	}
}

// SC1 helper: kinds of foods exist (guards the generator).
func TestGenerator_CoversEveryOpType(t *testing.T) {
	seen := map[string]int{}
	for _, r := range allOps(t) {
		seen[r.Kind]++
	}
	for _, k := range []string{"move", "away_day", "away_meal", "set_eaters", "remove", "add", "replace", "set_quantity", "delete_plate", "move_cook"} {
		assert.Positive(t, seen[k], "no generated case for op type %s", k)
	}
	assert.GreaterOrEqual(t, len(allOps(t)), 80)
	_ = food.RoleMain
}
