//go:build spectest

package spectest

import (
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

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
		Tell  []string `json:"tell"`
		Facts []struct {
			Class int    `json:"class"`
			Kind  string `json:"kind"`
			EN    string `json:"en"`
			Must  bool   `json:"must"`
		} `json:"facts"`
		Changed []json.RawMessage `json:"changed"`
	} `json:"report"`
}

// whatMoved are the G4.3 kinds of class 3 (cooking now elsewhere, leftover
// portions moved or resized, the guest's portions, an away slot's energy).
var whatMoved = map[string]bool{"cook": true, "leftover": true, "guest": true, "away": true}

// dayLabel is the facts block's day name ("Mon 23 Nov").
func dayLabel(date string) string {
	d, _ := time.Parse("2006-01-02", date)
	return d.Format("Mon 2 Jan")
}

// cookDays are the dates whose cooking after the write holds a cooking (dish,
// cooking time, portions) it did not hold before: "cooking now elsewhere" and
// batch size changes, the lines G4.3 tells per cooking time. A day that only
// lost its cooking is not required (G4.3 names where the cooking is now).
func cookDays(a, b planning.State) []string {
	sig := func(s planning.State) map[string][]string {
		out := map[string][]string{}
		for _, ss := range s.Sessions {
			if !s.Window.Contains(ss.CookDate) {
				continue
			}
			d := ss.CookDate.Format("2006-01-02")
			out[d] = append(out[d], fmt.Sprintf("%d/%d/%.3f", ss.FoodID, ss.CookSlotID, ss.Portions))
		}
		return out
	}
	sa, sb := sig(a), sig(b)
	var days []string
	for d, after := range sb {
		for _, x := range after {
			if !slices.Contains(sa[d], x) {
				days = append(days, d)
				break
			}
		}
	}
	slices.Sort(days)
	return days
}

// RP5 / G4 as changed by spec v1.5 (b, V63): what the household must know is
// never cut. The SF1(c) day under its minimum and every G4.3 what-moved line are
// told outside the cap, like class 1; the cap of 5 holds for the rest of classes
// 2-5 (changed meals, worse figures, warnings, side figures), and at least one
// changed-meal line stays when a meal changed.
func TestRP5_FactsBlockCap(t *testing.T) {
	forOps(t, nil, func(t *testing.T, r opRun) {
		var o tellOut
		_ = json.Unmarshal([]byte(r.Raw), &o)
		if len(o.Report.Tell) == 0 {
			t.Skip("no facts block")
		}
		capped, meal := 0, false
		for _, f := range o.Report.Facts {
			if f.Must {
				assert.Regexp(t, `(?i)minimum|mindest`, f.EN, "a must line is the day under its minimum")
				continue
			}
			if f.Class < 2 || (f.Class == 3 && whatMoved[f.Kind]) {
				continue
			}
			capped++
			if f.Class == 2 && (f.Kind == "meal" || f.Kind == "away") {
				meal = true
			}
		}
		assert.LessOrEqual(t, capped, 5, "the capped lines (classes 2-5 without the must and what-moved lines) are %d: %q", capped, o.Report.Tell)
		if len(o.Report.Changed) > 0 {
			assert.True(t, meal, "a meal changed, so at least one changed-meal line stays: %q", o.Report.Tell)
		}
		if !r.Saved {
			return
		}
		moved := ""
		for _, f := range o.Report.Facts {
			if f.Class == 3 && whatMoved[f.Kind] {
				moved += f.EN + "\n"
			}
		}
		for _, d := range cookDays(r.Before, r.After) {
			assert.Contains(t, moved, dayLabel(d), "the cooking of %s changed: a what-moved line names it (G4.3, never cut): %q", d, o.Report.Tell)
		}
		tell := strings.Join(o.Report.Tell, "\n")
		for _, k := range r.NewViol {
			if p := strings.Split(k, "/"); p[0] == "H9" && len(p) > 2 {
				assert.Regexp(t, `(?i)`+regexp.QuoteMeta(dayLabel(p[2]))+`[^\n]*(minimum|mindest)|(minimum|mindest)[^\n]*`+regexp.QuoteMeta(dayLabel(p[2])), tell, "%s ends under its minimum: the SF1(c) line is told (never cut)", p[2])
			}
		}
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
