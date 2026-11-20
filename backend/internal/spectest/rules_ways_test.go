//go:build spectest

package spectest

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
)

// appliedWay is one way or offer of a find_ways result, applied for real on a
// fresh copy of the household (as apply_changes with the way's ops, goal and
// light, exactly as the tool text tells the model to).
type appliedWay struct {
	wayCase
	Offer       bool
	Idx         int
	Way         planning.Way
	Err         bool
	Saved       bool
	Raw         string
	After       planning.State
	Eval        planning.Evaluation
	LightBefore string
	LightAfter  string
}

func (a appliedWay) name() string {
	k := "way"
	if a.Offer {
		k = "offer"
	}
	return fmt.Sprintf("%s/%s%d", a.wayCase.name(), k, a.Idx)
}

var (
	appliedOnce sync.Once
	applied     []appliedWay
)

func allApplied(t *testing.T) []appliedWay {
	appliedOnce.Do(func() {
		var list []appliedWay
		for _, wc := range allWays(t) {
			for i, w := range wc.Out.Ways {
				if i < 3 {
					list = append(list, appliedWay{wayCase: wc, Idx: i, Way: w})
				}
			}
			for i, w := range wc.Out.Offers {
				if i < 2 {
					list = append(list, appliedWay{wayCase: wc, Offer: true, Idx: i, Way: w})
				}
			}
		}
		var wg sync.WaitGroup
		sem := make(chan struct{}, 12)
		for i := range list {
			wg.Add(1)
			go func() {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				a := &list[i]
				e := newEnv(t, "chat-"+a.HH, chatWeek)
				a.LightBefore = e.lightRows()
				in := map[string]any{"ops": a.Way.Ops, "goal": map[string]any{"kind": a.Goal, "plate_id": a.PlateID}}
				if len(a.Way.Light) > 0 {
					in["light"] = a.Way.Light
				}
				rs, chs := e.turn("apply the way", tc{"apply_changes", in})
				if len(rs) == 1 {
					a.Raw, a.Err = rs[0].Raw, rs[0].Err
				}
				a.Saved = len(chs) > 0
				a.After = e.st()
				a.Eval = planning.Evaluate(a.After, planning.WindowTarget(a.After))
				a.LightAfter = e.lightRows()
			}()
		}
		wg.Wait()
		applied = list
	})
	return applied
}

func forWays(t *testing.T, pred func(a appliedWay) bool, f func(t *testing.T, a appliedWay)) {
	t.Helper()
	n := 0
	for _, a := range allApplied(t) {
		if pred != nil && !pred(a) {
			continue
		}
		n++
		t.Run(a.name(), func(t *testing.T) { f(t, a) })
	}
	if n == 0 {
		t.Log("no way matched; nothing to check")
	}
}

func comparative(a appliedWay) bool { return !a.ToLimit && !a.Offer }

func mealKcal(ev planning.Evaluation, plateID int64) (float64, bool) {
	for _, p := range ev.Plates {
		if p.PlateID == plateID {
			return p.Kcal, true
		}
	}
	return 0, false
}

func dayKcal(ev planning.Evaluation, date string) (float64, bool) {
	for _, d := range ev.Days {
		if d.Date == date {
			return d.Kcal, true
		}
	}
	return 0, false
}

func plateDate(s planning.State, id int64) string {
	for _, p := range s.Plates {
		if p.ID == id {
			return p.Date.Format("2006-01-02")
		}
	}
	return ""
}

// touchedPlates are the plates an op list changes (by plate or component).
func touchedPlates(s planning.State, ops []planning.OptionOp) map[int64]bool {
	out := map[int64]bool{}
	for _, o := range ops {
		if o.PlateID != nil {
			out[*o.PlateID] = true
		}
		if o.PlateComponentID != nil {
			for _, p := range s.Plates {
				for _, c := range p.Components {
					if c.ID == *o.PlateComponentID {
						out[p.ID] = true
					}
				}
			}
		}
		if o.Date != "" && o.SlotID != 0 && o.Op == "add" && o.PlateID == nil {
			for _, p := range s.Plates {
				if p.Date.Format("2006-01-02") == o.Date && p.SlotID == o.SlotID {
					out[p.ID] = true
				}
			}
		}
	}
	return out
}

// RL1: comparative lighter. Only the named meal changes, by >= 10 % of itself,
// the day stays at or above its minimum, and the figures promised equal the
// plan after the write (the way passes its own check).
func TestRL1_ComparativeLighter(t *testing.T) {
	forWays(t, func(a appliedWay) bool { return comparative(a) && a.Goal == "lighter" }, func(t *testing.T, a appliedWay) {
		if !assert.False(t, a.Err, "apply failed: %s", a.Raw) {
			return
		}
		before, _ := mealKcal(planning.Evaluate(a.Base, planning.WindowTarget(a.Base)), a.PlateID)
		after, ok := mealKcal(a.Eval, a.PlateID)
		assert.True(t, ok)
		assert.LessOrEqual(t, after, before*0.9+0.5, "the meal must drop by at least 10 %% (%.0f -> %.0f)", before, after)
		assert.InDelta(t, a.Way.After.MealKcal, after, 2, "promised meal kcal equals the plan after the write")
		d, _ := dayKcal(a.Eval, plateDate(a.After, a.PlateID))
		assert.GreaterOrEqual(t, d, a.Out.MinimumKcal-0.5, "day above its minimum")
		assert.InDelta(t, a.Way.After.DayKcal, d, 2, "promised day kcal equals the plan after the write")
		assert.Subset(t, []int64{a.PlateID}, keys(touchedPlates(a.Base, a.Way.Ops)), "ops touch only the named plate")
	})
}

func keys(m map[int64]bool) []int64 {
	var out []int64
	for k := range m {
		out = append(out, k)
	}
	return out
}

// RL1/G3: a lighter way never adds food to the meal it makes lighter.
func TestRL1_LighterNeverAddsFoodToTheMeal(t *testing.T) {
	forWays(t, func(a appliedWay) bool { return a.Goal == "lighter" }, func(t *testing.T, a appliedWay) {
		for _, o := range a.Way.Ops {
			if o.Op == "add" && ((o.PlateID != nil && *o.PlateID == a.PlateID) || (o.PlateID == nil && o.Date == plateDate(a.Base, a.PlateID))) {
				assert.Fail(t, "an add op on the meal made lighter", "%+v", o)
			}
		}
		if len(a.Way.Ops) > 0 && a.Saved {
			assert.LessOrEqual(t, mustMeal(a), a.Way.Before.MealKcal+0.5, "the meal moved against the asked direction")
		}
	})
}

func mustMeal(a appliedWay) float64 { k, _ := mealKcal(a.Eval, a.PlateID); return k }

// RL2: comparative heartier: the named meal grows by >= 10 %, the day stays
// at or below its band top, only that meal changes.
func TestRL2_ComparativeHeartier(t *testing.T) {
	forWays(t, func(a appliedWay) bool { return comparative(a) && a.Goal == "heartier" }, func(t *testing.T, a appliedWay) {
		if !assert.False(t, a.Err, "apply failed: %s", a.Raw) {
			return
		}
		before, _ := mealKcal(planning.Evaluate(a.Base, planning.WindowTarget(a.Base)), a.PlateID)
		after, _ := mealKcal(a.Eval, a.PlateID)
		assert.GreaterOrEqual(t, after, before*1.1-0.5, "the meal must grow by at least 10 %% (%.0f -> %.0f)", before, after)
		d, _ := dayKcal(a.Eval, plateDate(a.After, a.PlateID))
		assert.LessOrEqual(t, d, a.Out.RangeHigh+0.5, "day at or under its band top")
		assert.Subset(t, []int64{a.PlateID}, keys(touchedPlates(a.Base, a.Way.Ops)), "ops touch only the named plate")
	})
}

// KR5: heartier grows the main, a starch or a protein food, never fruit,
// dessert, drink or sauce.
func TestKR5_HeartierRoleWhitelist(t *testing.T) {
	forWays(t, func(a appliedWay) bool { return a.Goal == "heartier" }, func(t *testing.T, a appliedWay) {
		grown := func(foodID int64) {
			f := a.Base.Foods[foodID]
			assert.False(t, f.Sweet, "%s is sweet (fruit/dessert-like)", f.Name)
			assert.NotContains(t, []food.Role{food.RoleDessert, food.RoleDrink, food.RoleSauce}, f.Role, "%s has role %s", f.Name, f.Role)
			if f.Kind == food.KindLeaf {
				proteinFood := f.Macros.Kcal > 0 && f.Macros.Protein*4/f.Macros.Kcal >= 0.20
				starch := f.LeafRole == food.RoleSideStarch
				assert.True(t, proteinFood || starch, "leaf %s is neither a starch nor a protein food (role %q, %.0f%% protein energy)", f.Name, f.LeafRole, 100*f.Macros.Protein*4/max(f.Macros.Kcal, 1))
			}
		}
		for _, o := range a.Way.Ops {
			switch o.Op {
			case "add", "replace", "swap":
				if o.FoodID != 0 {
					grown(o.FoodID)
				}
			case "set_quantity":
				for _, p := range a.Base.Plates {
					for _, c := range p.Components {
						if o.PlateComponentID != nil && c.ID == *o.PlateComponentID {
							grown(c.FoodID)
						}
					}
				}
			}
		}
	})
}

// RL4 and C-8: light/filling to the limit. Everything in ways[] (what the
// model applies in the same turn) reaches the limit and the limit is met; when
// no way reaches it, nothing is applied (ways[] is empty) and exactly one offer
// is made. An offer itself may miss the limit (it is the nearest comparative
// change or the RL6 fix); it is never applied unasked.
func TestRL4_LightToLimitAppliesOnlyWhatReachesElseOneOffer(t *testing.T) {
	for _, wc := range allWays(t) {
		if !wc.ToLimit {
			continue
		}
		t.Run(wc.name(), func(t *testing.T) {
			if len(wc.Reach.Ways) != len(wc.Out.Ways) || len(wc.Reach.Offers) != len(wc.Out.Offers) {
				assert.Fail(t, "tool output not aligned", "raw: %.200s", wc.Raw)
				return
			}
			reaching := 0
			for i, w := range wc.Out.Ways {
				assert.True(t, wc.Reach.Ways[i].ReachesLimit, "ways[%d] is applied but does not reach the limit: %s (%s)", i, w.What, w.Why)
				if wc.Reach.Ways[i].ReachesLimit {
					reaching++
					if wc.Out.MaxKcal > 0 {
						assert.LessOrEqual(t, w.After.MealKcal, wc.Out.MaxKcal+0.5, "ways[%d] ends over the limit", i)
					}
				}
			}
			if reaching == 0 {
				assert.Empty(t, wc.Out.Ways, "no way reaches the limit: nothing is applied")
				assert.Len(t, wc.Out.Offers, 1, "case (b) needs exactly one offer")
			}
		})
	}
}

// RL4: the limit itself is 75 % of the meal's share (light).
func TestRL4_LightLimitIsThreeQuartersOfShare(t *testing.T) {
	for _, wc := range allWays(t) {
		if wc.Goal != "lighter" || wc.Out.MaxKcal == 0 {
			continue
		}
		t.Run(wc.name(), func(t *testing.T) {
			if wc.Base.Profile.KcalTarget == nil {
				t.Skip("no kcal target")
			}
			var slotID int64
			var date string
			for _, p := range wc.Base.Plates {
				if p.ID == wc.PlateID {
					slotID, date = p.SlotID, p.Date.Format("2006-01-02")
				}
			}
			// v1.2: the limit is 75 % of the slot's expected kcal (SB5: share x day target, shifted by a routine meal)
			exp, _ := slotExpected(wc.Base, planning.Evaluate(wc.Base, planning.WindowTarget(wc.Base)), date)
			assert.InDelta(t, 0.75*exp[slotID], wc.Out.MaxKcal, 1.0, "max_kcal must be 75 %% of the slot's expected kcal")
		})
	}
}

// G3: at most one offer per reply; exactly one recommended among ways and
// offers; every id in an offer exists.
func TestG3_AtMostOneOfferAndExistingOptions(t *testing.T) {
	for _, wc := range allWays(t) {
		t.Run(wc.name(), func(t *testing.T) {
			assert.LessOrEqual(t, len(wc.Out.Offers), 1, "more than one offer")
			rec := 0
			ids := map[int64]bool{}
			for _, p := range wc.Base.Plates {
				ids[p.ID] = true
			}
			comp := map[int64]bool{}
			for _, p := range wc.Base.Plates {
				for _, c := range p.Components {
					comp[c.ID] = true
				}
			}
			all := append(append([]planning.Way{}, wc.Out.Ways...), wc.Out.Offers...)
			for _, w := range all {
				if w.Recommended {
					rec++
				}
				for _, o := range w.Ops {
					if o.PlateID != nil {
						assert.True(t, ids[*o.PlateID], "op names plate %d which does not exist", *o.PlateID)
					}
					if o.PlateComponentID != nil {
						assert.True(t, comp[*o.PlateComponentID], "op names component %d which does not exist", *o.PlateComponentID)
					}
					if o.FoodID != 0 {
						_, ok := wc.Base.Foods[o.FoodID]
						assert.True(t, ok, "op names food %d which does not exist", o.FoodID)
					}
				}
			}
			if len(all) > 0 {
				assert.Equal(t, 1, rec, "exactly one way or offer is recommended")
			}
		})
	}
}

// SC3: a comparative lighter/heartier way carries no mark and stores none.
func TestSC3_ComparativeStoresNoMark(t *testing.T) {
	forWays(t, comparative, func(t *testing.T, a appliedWay) {
		assert.Empty(t, a.Way.Light, "the way hands the model a light mark to store")
		for _, row := range strings.Split(a.LightAfter, ";") {
			assert.Contains(t, a.LightBefore, row, "a mark was stored by a comparative request")
		}
	})
}

// RL6/day-fix: a fix the way offers, applied after the way's ops, lands the
// day inside its range, and the day figure promised equals the result.
func TestRL6_DayFixLandsInRange(t *testing.T) {
	forWays(t, func(a appliedWay) bool { return a.Way.Fix != nil }, func(t *testing.T, a appliedWay) {
		e := newEnv(t, "chat-"+a.HH, chatWeek)
		ops := append(append([]planning.OptionOp{}, a.Way.Ops...), a.Way.Fix.Ops...)
		in := map[string]any{"ops": ops, "goal": map[string]any{"kind": a.Goal, "plate_id": a.PlateID}}
		if len(a.Way.Light) > 0 {
			in["light"] = a.Way.Light
		}
		rs, _ := e.turn("apply way and fix", tc{"apply_changes", in})
		if !assert.Len(t, rs, 1) || !assert.False(t, rs[0].Err, "%s", rs[0].Raw) {
			return
		}
		s := e.st()
		ev := planning.Evaluate(s, planning.WindowTarget(s))
		d, _ := dayKcal(ev, plateDate(s, a.PlateID))
		assert.InDelta(t, a.Way.Fix.DayKcal, d, 2, "the fix's promised day kcal")
		assert.GreaterOrEqual(t, d, a.Way.RangeLow-0.5, "day inside its range after the fix")
		assert.LessOrEqual(t, d, a.Way.RangeHigh+0.5)
	})
}

// KR4: portions of every dish are 0.5 to 1.5, and half a portion only on a
// dish cooked for one meal (a session eaten by one plate).
func TestKR4_WaysKeepPortionsAndHalvesOnlySingleUse(t *testing.T) {
	forWays(t, nil, func(t *testing.T, a appliedWay) {
		uses := map[int64]int{}
		for _, p := range a.After.Plates {
			for _, c := range p.Components {
				if c.SessionID != 0 {
					uses[c.SessionID]++
				}
			}
		}
		was := map[string]bool{}
		for _, p := range a.Base.Plates {
			for _, c := range p.Components {
				if c.Portions != nil {
					was[fmt.Sprintf("%d/%d/%.2f", p.ID, c.FoodID, *c.Portions)] = true
				}
			}
		}
		for _, p := range a.After.Plates {
			for _, c := range p.Components {
				if c.Portions == nil {
					continue
				}
				assert.GreaterOrEqual(t, *c.Portions, 0.5-1e-9)
				assert.LessOrEqual(t, *c.Portions, 1.5+1e-9)
				if *c.Portions <= 0.5+1e-9 && !was[fmt.Sprintf("%d/%d/%.2f", p.ID, c.FoodID, *c.Portions)] {
					assert.LessOrEqual(t, uses[c.SessionID], 1, "plate %d eats %.2f portions of a dish that feeds %d plates", p.ID, *c.Portions, uses[c.SessionID])
				}
			}
		}
	})
}

// KR2: at most two dishes per cooking time after any way or offer.
func TestKR2_AtMostTwoDishesPerCookingTime(t *testing.T) {
	forWays(t, nil, func(t *testing.T, a appliedWay) {
		count := func(s planning.State) map[string]int {
			per := map[string]int{}
			for _, x := range s.Sessions {
				per[fmt.Sprintf("%s/%d", x.CookDate.Format("2006-01-02"), x.CookSlotID)]++
			}
			return per
		}
		was := count(a.Base)
		for cell, n := range count(a.After) {
			if n > was[cell] { // a way never makes a cooking time heavier than two dishes
				assert.LessOrEqual(t, n, 2, "%d dishes cook at %s (was %d)", n, cell, was[cell])
			}
		}
	})
}

var oneEightyRe = regexp.MustCompile(`(?i)80 kcal`)

// RL1 (tool text): the tools say "10 % of the meal", never "or 80 kcal, the smaller".
func TestRL1_ToolTextsSayTenPercentOfTheMeal(t *testing.T) {
	e := newEnv(t, "chat-dev", chatWeek)
	for _, d := range e.ts.Describe() {
		if d.Name == "find_ways" || d.Name == "apply_changes" {
			t.Run(d.Name, func(t *testing.T) {
				assert.False(t, oneEightyRe.MatchString(d.Description), "%s still mentions an 80 kcal alternative", d.Name)
			})
		}
	}
}

// RL4 (tool text): portions in light-meal options are 0.5-1.5 as the code has
// them, not 0.75-1.5; NU6: no protein-share clause when g/kg applies.
func TestRL4_ToolTextPortionRangeMatchesCode(t *testing.T) {
	e := newEnv(t, "chat-dev", chatWeek)
	for _, d := range e.ts.Describe() {
		if d.Name == "apply_changes" || d.Name == "get_plan" {
			t.Run(d.Name, func(t *testing.T) {
				assert.NotContains(t, d.Description, "0.75-1.5", "the text says 0.75-1.5, code allows 0.5-1.5")
			})
		}
	}
}

func TestNU6_ToolTextHasNoProteinShareClause(t *testing.T) {
	e := newEnv(t, "chat-dev", chatWeek)
	for _, d := range e.ts.Describe() {
		if d.Name == "apply_changes" {
			assert.NotContains(t, d.Description, "or the share under target")
		}
	}
}
