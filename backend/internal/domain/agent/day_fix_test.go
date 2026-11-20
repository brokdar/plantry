package agent_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
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
				require.NotNil(t, fix, "%s %v leaves the day under its range", in, w["what"])
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
		assert.Positive(t, fixed, in)
	}
	res := e.mustExec("find_ways", fmt.Sprintf(`{"goal":"lighter","plate_id":%d}`, id))
	fix := relMaps(res["ways"])[0]["fix"].(map[string]any)
	assert.Contains(t, res["next"], fix["what"], "next names the one recommended fix")
}

// A day the way keeps in its range has no fix.
func TestFindWays_InRangeDayHasNoFix(t *testing.T) {
	e := relEnv(t)
	res := e.mustExec("find_ways", fmt.Sprintf(`{"goal":"lighter","plate_id":%d}`, e.dinner3()))
	for _, w := range relMaps(res["ways"]) {
		assert.Nil(t, w["fix"])
	}
	assert.NotContains(t, res["next"], "ONE small fix")
}

// A "lighter" write on a day that already was under its range (the German
// "make Wednesday lighter" case: no new issue, so no option was offered) ends
// the reply with the one fix that brings the day inside the range, and
// applying that fix gives what the reply said.
func TestApplyChanges_LighterLeavingDayUnderRangeOffersOneFix(t *testing.T) {
	e := fixEnv(t, 2700)
	res := e.mustExec("find_ways", fmt.Sprintf(`{"goal":"lighter","plate_id":%d}`, e.dinner3()))
	way := relMaps(res["ways"])[0]
	got, err := e.applyWay(res, way, false)
	require.NoError(t, err)
	kcal, low, _ := dayOf(t, got)
	require.Less(t, kcal, low, "the day ends under its range")
	fixes := relMaps(got["after"].(map[string]any)["issues"].(map[string]any)["day_fix"])
	require.Len(t, fixes, 1)
	opt := relMaps(fixes[0]["options"])[0]
	assert.Contains(t, []string{"Ciabatta", "Apple"}, opt["name"])
	tell := got["report"].(map[string]any)["tell"].([]any)
	last := fmt.Sprint(tell[len(tell)-1])
	assert.Contains(t, last, "I can also add")
	assert.Contains(t, last, opt["name"])
	assert.Contains(t, last, "Sat 7 Nov lunch")
	assert.Contains(t, last, "120 g of Ciabatta")
	assert.Contains(t, last, fmt.Sprintf("%.0f kcal", opt["day_kcal"]))

	raw, err := json.Marshal(map[string]any{"ops": []any{opt["op"]}, "dry_run": true})
	require.NoError(t, err)
	after, err := e.exec("apply_changes", string(raw))
	require.NoError(t, err)
	k, lo, hi := dayOf(t, after)
	assert.InDelta(t, opt["day_kcal"], k, 1)
	assert.GreaterOrEqual(t, k, lo-0.5)
	assert.LessOrEqual(t, k, hi+0.5)
}

func TestIsCheeseOrNut(t *testing.T) {
	for name, want := range map[string]bool{"Parmesan": true, "Bergkäse": true, "Walnüsse": true, "Peanut butter": true, "Coconut milk": false, "Ciabatta": false, "Apple": false} {
		assert.Equal(t, want, planning.IsCheeseOrNut(name), name)
	}
}

// Any write that leaves a touched day under its range, not only a light
// request, ends with the one fix and says the day was already under before.
func TestApplyChanges_AnyWriteUnderRangeOffersOneFix(t *testing.T) {
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
	tell := got["report"].(map[string]any)["tell"].([]any)
	last := fmt.Sprint(tell[len(tell)-1])
	assert.Contains(t, last, "was already under its range before this change")
	assert.Contains(t, last, "I can also add")
	assert.Contains(t, last, "kcal, inside its range")
}
