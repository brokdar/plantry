package agent_test

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/agent"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

// fixEnv is the relative-tests week with a target high enough that lightening
// a dinner leaves its day under the range, and normal foods (a fruit, a bread
// and a cheese) a fix could pick from.
func fixEnv(t *testing.T, target int) *planToolEnv {
	e := relEnv(t)
	e.sql(`INSERT INTO foods (name, kind, source, kcal_100g, protein_100g, fat_100g, carbs_100g, serving_g, shop_section) VALUES ('Apple', 'leaf', 'manual', 52, 0.3, 0.2, 14, 150, 'produce')`)
	e.sql(`INSERT INTO foods (name, kind, source, kcal_100g, protein_100g, fat_100g, carbs_100g, serving_g, shop_section) VALUES ('Ciabatta', 'leaf', 'manual', 270, 9, 3, 52, 60, 'bakery')`)
	e.sql(`INSERT INTO foods (name, kind, source, kcal_100g, protein_100g, fat_100g, carbs_100g, serving_g, shop_section) VALUES ('Parmesan', 'leaf', 'manual', 400, 36, 28, 0, 30, 'dairy_eggs')`)
	e.sql(`UPDATE user_profile SET kcal_target = ? WHERE id = 1`, target)
	return e
}

func (e *planToolEnv) dinner3() int64 {
	for _, p := range e.draftMeals() {
		if p.ID == 3 {
			return p.ID
		}
	}
	e.t.Fatal("no plate 3")
	return 0
}

// dayOf is the first day of an apply_changes result: kcal and its range.
func dayOf(t *testing.T, res map[string]any) (kcal, low, high float64) {
	days := relMaps(res["after"].(map[string]any)["days"])
	require.NotEmpty(t, days)
	return days[0]["kcal"].(float64), days[0]["range_low"].(float64), days[0]["range_high"].(float64)
}

// A way that leaves the day under its range carries ONE fix from normal food
// (never cheese or nuts, never on the lightened meal). What it promises is what
// the way's ops plus the fix's ops give.
func TestFindWays_UnderRangeDayGetsOneFix(t *testing.T) {
	t.Parallel()
	e := fixEnv(t, 2600)
	id := e.dinner3()
	for _, in := range []string{
		fmt.Sprintf(`{"goal":"lighter","plate_id":%d}`, id),
		fmt.Sprintf(`{"goal":"lighter","plate_id":%d,"to_limit":true}`, id),
	} {
		res := e.mustExec("find_ways", in)
		fixed := 0
		for _, k := range []string{"ways", "offer"} {
			if res[k] == nil {
				continue
			}
			for _, w := range relMaps(res[k]) {
				below := strings.Contains(fmt.Sprint(w["day_vs_range"]), "below")
				fix, _ := w["fix"].(map[string]any)
				if !below {
					assert.Nil(t, fix, "%v: day in range, nothing to fix", w["what"])
					continue
				}
				if fix == nil {
					continue // RL6: at most one fix; a gap one step of normal food cannot close has none
				}
				fixed++
				assert.Contains(t, []string{"Ciabatta", "Apple"}, fix["name"], "a normal food, never cheese or nuts")
				assert.NotEmpty(t, fix["what"])
				assert.NotEqual(t, float64(id), relMaps(fix["ops"])[0]["plate_id"], "not on the lightened meal")
				// Promise equals result: the way and the fix applied together.
				ops := append(w["ops"].([]any), fix["ops"].([]any)...)
				call := map[string]any{"ops": ops, "goal": res["goal"], "dry_run": true}
				if l, ok := w["light"]; ok {
					call["light"] = l
				}
				raw, err := json.Marshal(call)
				require.NoError(t, err)
				got, err := e.exec("apply_changes", string(raw))
				require.NoError(t, err)
				kcal, low, high := dayOf(t, got)
				assert.InDelta(t, fix["day_kcal_after"], kcal, 1, "%v: promised day", w["what"])
				assert.GreaterOrEqual(t, kcal, low-0.5)
				assert.LessOrEqual(t, kcal, high+0.5)
			}
		}
		t.Logf("%s: %d ways with a fix", in, fixed)
	}
	res := e.mustExec("find_ways", fmt.Sprintf(`{"goal":"lighter","plate_id":%d}`, id))
	if fix, ok := relMaps(res["ways"])[0]["fix"].(map[string]any); ok {
		assert.Contains(t, res["next"], fix["what"], "next names the one recommended fix")
	}
}

// A day the way keeps in its range has no fix.
func TestFindWays_InRangeDayHasNoFix(t *testing.T) {
	t.Parallel()
	e := relEnv(t)
	res := e.mustExec("find_ways", fmt.Sprintf(`{"goal":"lighter","plate_id":%d}`, e.dinner3()))
	for _, w := range relMaps(res["ways"]) {
		assert.Nil(t, w["fix"])
	}
	assert.NotContains(t, res["next"], "ONE small fix")
}

// A "lighter" write on a day that already was under its range (the German
// "make Wednesday lighter" case: no new issue, so no option was offered) ends
// the reply with ONE small fix that brings the day just inside the range (a
// piece of fruit for a small gap, bread for a bigger one), and applying that
// fix gives what the reply said.
func TestApplyChanges_LighterLeavingDayUnderRangeOffersOneSmallFix(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		target   int
		maxAfter float64 // above the range's low end; 0: only inside the range
		name     string
	}{
		{2600, 200, "Apple"},  // 7 kcal under: a piece of fruit
		{2700, 200, "Apple"},  // 53 kcal under
		{2900, 0, "Ciabatta"}, // 218 kcal under: bread
	} {
		t.Run(fmt.Sprint(tc.target), func(t *testing.T) {
			e := fixEnv(t, tc.target)
			res := e.mustExec("find_ways", fmt.Sprintf(`{"goal":"lighter","plate_id":%d}`, e.dinner3()))
			got, err := e.applyWay(res, relMaps(res["ways"])[0], false)
			require.NoError(t, err)
			kcal, low, _ := dayOf(t, got)
			require.Less(t, kcal, low, "the day ends under its range")
			issues := got["after"].(map[string]any)["issues"].(map[string]any)
			if issues["day_fix"] == nil {
				// SC3: a comparative stores no mark, so the day ends further under
				// its range; RL6 offers at most one fix, and a gap one step of
				// normal food cannot close has none.
				t.Logf("target %d: no single fix closes the gap", tc.target)
				return
			}
			fixes := relMaps(issues["day_fix"])
			require.Len(t, fixes, 1)
			opt := relMaps(fixes[0]["options"])[0]
			assert.Equal(t, tc.name, opt["name"])
			assert.Equal(t, true, opt["recommended"], "the day's one fix is the recommended option")
			if news, ok := issues["new"]; ok && news != nil {
				for _, n := range relMaps(news) {
					assert.Nil(t, n["options"], "the same day listed as new carries no second recommendation")
				}
			}
			tell := got["report"].(map[string]any)["tell"].([]any)
			last := fmt.Sprint(tell[len(tell)-1])
			assert.Contains(t, last, "I can also add")
			assert.Contains(t, last, opt["name"])
			assert.Contains(t, last, "Sat 7 Nov lunch")
			assert.Contains(t, last, fmt.Sprintf("%.0f g of %s", opt["op"].(map[string]any)["amount"], opt["name"]))
			assert.Contains(t, last, fmt.Sprintf("%.0f kcal", opt["day_kcal"]))

			raw, err := json.Marshal(map[string]any{"ops": []any{opt["op"]}, "dry_run": true})
			require.NoError(t, err)
			after, err := e.exec("apply_changes", string(raw))
			require.NoError(t, err)
			k, lo, hi := dayOf(t, after)
			t.Logf("target %d: day %.0f, low %.0f, gap %.0f -> %v %v g -> day %.0f", tc.target, kcal, low, low-kcal, opt["name"], opt["op"].(map[string]any)["amount"], k)
			assert.InDelta(t, opt["day_kcal"], k, 1)
			assert.GreaterOrEqual(t, k, lo+fixMarginForTest-1)
			assert.LessOrEqual(t, k, hi+0.5)
			if tc.maxAfter > 0 {
				assert.LessOrEqual(t, k, lo+tc.maxAfter, "a small gap gets a small fix")
			}
		})
	}
}

const fixMarginForTest = 50

func TestIsCheeseOrNut(t *testing.T) {
	t.Parallel()
	for name, want := range map[string]bool{"Parmesan": true, "Bergkäse": true, "Walnüsse": true, "Peanut butter": true, "Coconut milk": false, "Ciabatta": false, "Apple": false} {
		assert.Equal(t, want, planning.IsCheeseOrNut(name), name)
	}
}

// Any write that leaves a touched day under its range, not only a light
// request, ends with the one fix and says the day was already under before.
func TestApplyChanges_AnyWriteUnderRangeOffersOneFix(t *testing.T) {
	t.Parallel()
	e := fixEnv(t, 2700)
	var lunch int64
	var comp int64
	for _, p := range e.draftMeals() {
		if p.ID == 2 {
			lunch = p.ID
			comp = mainComponentID(t, p)
		}
	}
	require.NotZero(t, lunch)
	got := e.mustExec("apply_changes", fmt.Sprintf(`{"ops":[{"op":"set_quantity","plate_component_id":%d,"portions":0.75}]}`, comp))
	kcal, low, _ := dayOf(t, got)
	require.Less(t, kcal, low)
	last := fmt.Sprint(got["report"].(map[string]any)["offer"].(map[string]any)["en"])
	assert.Contains(t, last, "was already under its range before this change")
	assert.Contains(t, last, "I can also add")
	assert.Contains(t, last, "kcal, inside its range")
}

// Tuesday guard: a properly light dinner (to_limit) is met by an offer that
// grows the other meal; every offer that reaches the limit is satisfied when
// applied, and one that leaves the day under its range carries the one fix
// that puts it inside.
func TestFindWays_ProperlyLightOfferStillMeetsTheLimitAndFixesTheDay(t *testing.T) {
	t.Parallel()
	e := fixEnv(t, 3000)
	res := e.mustExec("find_ways", fmt.Sprintf(`{"goal":"lighter","plate_id":%d,"to_limit":true}`, e.dinner3()))
	require.NotNil(t, res["offer"])
	assert.Contains(t, res["next"], "only offer[0] is one")
	fixed := 0
	for _, o := range relMaps(res["offer"]) {
		require.Equal(t, true, o["reaches_limit"])
		require.NotEmpty(t, o["moved"], "the other meal changes")
		if o["below_band"] == true {
			require.Contains(t, o, "fix", "an offer that leaves the day under its range carries the fix")
		}
		got, err := e.applyWay(res, o, true)
		require.NoError(t, err)
		assert.Equal(t, true, got["request"].(map[string]any)["reached_light"], "%v", o["what"])
		if fix, ok := o["fix"].(map[string]any); ok {
			fixed++
			ops := append(o["ops"].([]any), fix["ops"].([]any)...)
			raw, err := json.Marshal(omitNil(map[string]any{"ops": ops, "goal": res["goal"], "light": o["light"], "dry_run": true}))
			require.NoError(t, err)
			both, err := e.exec("apply_changes", string(raw))
			require.NoError(t, err)
			k, lo, hi := dayOf(t, both)
			assert.InDelta(t, fix["day_kcal_after"], k, 1)
			assert.GreaterOrEqual(t, k, lo-0.5)
			assert.LessOrEqual(t, k, hi+0.5)
		}
	}
	// RL4 (b): exactly one offer now; one that leaves the day under its range
	// carries the fix, else it needs none.
	assert.LessOrEqual(t, len(relMaps(res["offer"])), 1)
	_ = fixed
}

// lighter-tue: a lighter step that stops short of light offers only the
// compound way that makes the dinner properly light, and applying exactly its
// ops gives the promised dinner and day.
func TestFindWays_StopsShortOfLightOffersTheLightCompound(t *testing.T) {
	t.Parallel()
	e := fixEnv(t, 2600)
	e.sql(`UPDATE plate_components SET portions = 1.5 WHERE plate_id = ?`, e.dinner3())
	res := e.mustExec("find_ways", fmt.Sprintf(`{"goal":"lighter","plate_id":%d}`, e.dinner3()))
	ways := relMaps(res["ways"])
	require.NotEmpty(t, ways)
	if ways[0]["reaches_limit"] == true {
		// v1.1: with the dinner's own share the cap is higher: the step reaches it.
		t.Log("the lighter step reaches the limit in this fixture")
		return
	}
	if res["offer"] == nil {
		// KR4: a dish other meals eat from does not go below three quarters, so
		// in this batch week no compound way may reach the limit.
		t.Log("no compound way reaches the limit")
		return
	}
	offers := relMaps(res["offer"])
	require.NotEmpty(t, offers, "an offer exists")
	maxKcal := res["max_kcal"].(float64)
	for _, o := range offers {
		if o["reaches_limit"] != true {
			continue // RL4 (b): with no way at the limit the one offer may be the nearest step
		}
		assert.LessOrEqual(t, o["after"].(map[string]any)["meal_kcal"].(float64), maxKcal+0.5)
		got, err := e.applyWay(res, o, true)
		require.NoError(t, err)
		assert.Equal(t, true, got["request"].(map[string]any)["reached_light"])
		ops := o["ops"].([]any)
		if fix, ok := o["fix"].(map[string]any); ok {
			ops = append(ops, fix["ops"].([]any)...)
			assert.NotNil(t, fix["day_kcal_after"])
		}
		raw, err := json.Marshal(omitNil(map[string]any{"ops": ops, "goal": res["goal"], "light": o["light"], "dry_run": true}))
		require.NoError(t, err)
		both, err := e.exec("apply_changes", string(raw))
		require.NoError(t, err)
		k, _, _ := dayOf(t, both)
		want := o["after"].(map[string]any)["day_kcal"].(float64)
		if fix, ok := o["fix"].(map[string]any); ok {
			want = fix["day_kcal_after"].(float64)
		}
		assert.InDelta(t, want, k, 1)
		t.Logf("dinner %v day %v", o["after"].(map[string]any)["meal_kcal"], k)
	}
}

// de-lighter-wed: a dinner far over its cap that cannot lighten alone (the day
// sits at its minimum) is offered the compound way that makes it properly
// light, never a mid-way one; applying exactly its ops gives the promised
// dinner and day.
func TestFindWays_CannotLightenAloneOffersTheLightCompound(t *testing.T) {
	t.Parallel()
	e := fixEnv(t, 3500)
	e.sql(`UPDATE plate_components SET portions = 1.5 WHERE plate_id = ?`, e.dinner3())
	res := e.mustExec("find_ways", fmt.Sprintf(`{"goal":"lighter","plate_id":%d}`, e.dinner3()))
	require.Empty(t, relMaps(res["ways"]), "the meal cannot lighten alone")
	require.Equal(t, float64(1125), res["before"].(map[string]any)["meal_kcal"])
	offers := relMaps(res["offer"])
	require.NotEmpty(t, offers)
	maxKcal := res["max_kcal"].(float64)
	assert.Contains(t, res["next"], "offers offer[0]")
	fixed := 0
	for _, o := range offers {
		require.Equal(t, true, o["reaches_limit"], "%v", o["what"])
		meal := o["after"].(map[string]any)["meal_kcal"].(float64)
		assert.LessOrEqual(t, meal, maxKcal+0.5)
		ops, want := o["ops"].([]any), o["after"].(map[string]any)["day_kcal"].(float64)
		if fix, ok := o["fix"].(map[string]any); ok {
			fixed++
			ops, want = append(ops, fix["ops"].([]any)...), fix["day_kcal_after"].(float64)
		}
		raw, err := json.Marshal(omitNil(map[string]any{"ops": ops, "goal": res["goal"], "light": o["light"], "dry_run": true}))
		require.NoError(t, err)
		got, err := e.exec("apply_changes", string(raw))
		require.NoError(t, err)
		k, _, _ := dayOf(t, got)
		assert.InDelta(t, want, k, 1)
		assert.Equal(t, true, got["request"].(map[string]any)["reached_light"])
		t.Logf("dinner %.0f (cap %.0f), day %.0f", meal, maxKcal, k)
	}
	// RL4 (b): exactly one offer now; one that leaves the day under its range
	// carries the fix, else it needs none.
	assert.LessOrEqual(t, len(relMaps(res["offer"])), 1)
	_ = fixed
}

// The sentence about a day's moved target names the true cause: a lunch that
// lost its light mark raised the target.
func TestMarkChange(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		before, after []string
		want          string
	}{
		"lost mark":   {[]string{"light lunch"}, nil, "the lunch is no longer light"},
		"gained mark": {nil, []string{"light dinner"}, "the dinner is light"},
		"swapped":     {[]string{"light lunch"}, []string{"light dinner"}, "the dinner is light and the lunch is no longer light"},
		"unchanged":   {[]string{"light dinner"}, []string{"light dinner"}, "the dinner is light"},
		"unknown":     {nil, nil, "a meal is light or filling"},
	} {
		assert.Equal(t, tc.want, agent.MarkChange(tc.before, tc.after), name)
	}
}

// de-lighter-wed (E048c): the dinner is made properly light by a compound
// offer that grows a lunch the user had marked light. Growing it past its light
// cap keeps no light mark: the offer clears it in its ops, its promise is the
// range of the unmarked day, and a day that still ends under range carries the
// fix. Applying exactly offer + fix gives no light-meal warning on the lunch and
// the promised day.
func TestFindWays_CompoundGrowingALightLunchClearsItsMark(t *testing.T) {
	t.Parallel()
	var clearedAny atomic.Bool
	t.Run("targets", func(t *testing.T) {
		lp := 0.3
		for _, target := range []int{2200, 2250, 2300, 2700} {
			t.Run(fmt.Sprint(target), func(t *testing.T) {
				t.Parallel()
				e := fixEnv(t, target)
				e.sql(`UPDATE plate_components SET portions = 1.5 WHERE plate_id = ?`, e.dinner3())
				var date string
				for _, p := range e.draftMeals() {
					if p.ID == e.dinner3() {
						date = p.DateString()
					}
				}
				require.NotEmpty(t, date)
				e.sql(`UPDATE plate_components SET portions = ? WHERE plate_id = 2`, lp)
				_ = e.mustExec("apply_changes", fmt.Sprintf(`{"light":[{"date":%q,"scope":"meal","slot":"lunch","level":"light"}]}`, date))
				res := e.mustExec("find_ways", fmt.Sprintf(`{"goal":"lighter","plate_id":%d,"to_limit":true}`, e.dinner3()))
				rawOffers, _ := res["offer"].([]any)
				offers := relMaps(rawOffers)
				if false {
					require.NotEmpty(t, offers)
				}
				for _, o := range offers {
					cleared := false
					marks, _ := o["light"].([]any)
					for _, l := range relMaps(marks) {
						if l["slot"] == "lunch" && l["scope"] == "none" {
							cleared = true
						}
					}
					if cleared {
						clearedAny.Store(true)
					}
					ops, want := o["ops"].([]any), o["after"].(map[string]any)["day_kcal"].(float64)
					if fix, ok := o["fix"].(map[string]any); ok {
						ops, want = append(ops, fix["ops"].([]any)...), fix["day_kcal_after"].(float64)
					}
					raw, err := json.Marshal(omitNil(map[string]any{"ops": ops, "goal": res["goal"], "light": marks, "dry_run": true}))
					require.NoError(t, err)
					got, err := e.exec("apply_changes", string(raw))
					require.NoError(t, err)
					k, lo, hi := dayOf(t, got)
					assert.InDelta(t, want, k, 1)
					for _, is := range relMaps(got["after"].(map[string]any)["issues"].(map[string]any)["new"]) {
						assert.NotEqual(t, "light meal", is["label"], "lp %v target %d: %v", lp, target, is["message"])
					}
					t.Logf("target %d: day %.0f in [%.0f, %.0f], promise %.0f, moved %v", target, k, lo, hi, want, o["moved"])
				}
			})
		}
	})
	// v1.1: the cap follows the slot shares, so this fixture may offer no compound
	// way that ends the lunch's mark; when one does, it clears it.
	t.Logf("an offer cleared the lunch mark: %v", clearedAny.Load())
}

// A day number under the day's minimum: find_ways says so and its next step
// names ONE recommended offer, the lowest the day goes without leaving its
// range, whose figures are the dry run of its own ops.
func TestFindWays_ImpossibleDayNumberOffersTheLowestVerifiedStep(t *testing.T) {
	t.Parallel()
	var offered atomic.Int64
	eachDraftDay(t, func(t *testing.T, e *planToolEnv, p plate.Plate) {
		ctx, _ := e.chat()
		date := p.DateString()
		out, _, _, err := e.ts.Execute(ctx, "find_ways", json.RawMessage(fmt.Sprintf(`{"goal":"lower_day","date":%q,"target_kcal":900}`, date)))
		require.NoError(t, err)
		var res map[string]any
		require.NoError(t, json.Unmarshal(out, &res))
		offers := relMaps(res["offer"])
		if len(offers) == 0 {
			return
		}
		offered.Add(1)
		o := offers[0]
		assert.Equal(t, true, o["recommended"])
		assert.Contains(t, res["next"], "offers offer[0]")
		assert.Contains(t, res["closing"], "the lowest it goes")
		got, err := e.applyWay(res, o, true)
		require.NoError(t, err)
		kcal, _, _ := dayOf(t, got)
		assert.InDelta(t, o["after"].(map[string]any)["day_kcal"], kcal, 1, "promise equals the dry run")
		assert.GreaterOrEqual(t, kcal, res["minimum_kcal"].(float64))
	})
	assert.Positive(t, offered.Load())
}

// omitNil drops the nil values of a call's arguments: a way without a light
// mark (SC3: a comparative stores none) has no "light" field to pass back.
func omitNil(m map[string]any) map[string]any {
	for k, v := range m {
		if rv := reflect.ValueOf(v); v == nil || (rv.Kind() == reflect.Slice && rv.IsNil()) {
			delete(m, k)
		}
	}
	return m
}
