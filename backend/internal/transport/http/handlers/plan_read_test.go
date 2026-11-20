package handlers_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// readAPI is planAPI with AC-1's shape: shopping day Saturday, a breakfast
// slot (3 active slots) and a Chili of 400 g beans at 100 kcal / 100 g, i.e.
// 100 kcal per portion (reference portions 4).
type readAPI struct {
	*planAPI
	breakfast int64
}

const chiliPortionKcal = 100.0

func newReadAPI(t *testing.T) *readAPI {
	t.Helper()
	a := &readAPI{planAPI: newPlanAPI(t)}
	a.breakfast = a.insert(`INSERT INTO time_slots (name_key, icon, sort_order, active) VALUES ('slot.breakfast', 'coffee', 0, 1)`)
	beans := a.insert(`INSERT INTO foods (name, kind, source, kcal_100g, protein_100g, fat_100g, carbs_100g) VALUES ('Kidney beans', 'leaf', 'manual', 100, 8, 1, 15)`)
	a.insert(fmt.Sprintf(`INSERT INTO food_components (parent_id, child_id, amount, unit, grams) VALUES (%d, %d, 400, 'g', 400)`, a.chili, beans))
	a.insert(`INSERT INTO app_settings (key, value) VALUES ('plan.shopping_day', '5')`)
	return a
}

// ac1 plans AC-1: Chili Sun dinner for 2 eaters as a 3-meal batch with Mon
// lunch and Mon dinner. It returns the session id.
func (a *readAPI) ac1() int64 {
	a.t.Helper()
	_, sid := a.sunChili()
	code, body := a.do(http.MethodPost, fmt.Sprintf("/api/cook-sessions/%d/leftovers", sid),
		fmt.Sprintf(`{"cells":[{"date":"2026-11-02","slot_id":%d},{"date":"2026-11-02","slot_id":%d}]}`, a.lunch, a.dinner))
	require.Equal(a.t, http.StatusOK, code, body)
	return sid
}

// B4-AC2
func TestPlanHTTP_Start(t *testing.T) {
	a := newReadAPI(t)
	tests := []struct {
		name, query string
		status      int
		key, start  string
	}{
		{"not a shopping day", "start=2026-11-01&today=2026-11-04", http.StatusBadRequest, "error.plan.invalid_start", ""},
		{"malformed start", "start=01.11.2026", http.StatusBadRequest, "error.plan.invalid_start", ""},
		{"malformed today", "today=tomorrow", http.StatusBadRequest, "error.invalid_body", ""},
		{"a shopping day", "start=2026-10-31&today=2026-11-04", http.StatusOK, "", "2026-10-31"},
		{"anchored without start", "today=2026-11-05", http.StatusOK, "", "2026-11-07"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, body := a.do(http.MethodGet, "/api/plan?"+tt.query, "")
			require.Equal(t, tt.status, code, body)
			if tt.key != "" {
				assert.Equal(t, tt.key, body["message_key"])
				return
			}
			assert.Equal(t, tt.start, body["window"].(map[string]any)["start"])
		})
	}
}

// B4-AC3
func TestPlanHTTP_AC1(t *testing.T) {
	a := newReadAPI(t)
	sid := a.ac1()

	code, body := a.do(http.MethodGet, "/api/plan?start=2026-10-31&today=2026-11-04", "")
	require.Equal(t, http.StatusOK, code, body)

	assert.Equal(t, map[string]any{
		"start": "2026-10-31", "end": "2026-11-06", "today": "2026-11-04", "is_current": true,
		"prev_start": "2026-10-24", "next_start": "2026-11-07", "shopping_day": float64(5),
	}, body["window"])
	slots := body["slots"].([]any)
	require.Len(t, slots, 3)
	assert.Equal(t, map[string]any{"id": float64(a.breakfast), "name_key": "slot.breakfast", "icon": "coffee", "sort_order": float64(0), "kind": "meal"}, slots[0])

	cells := body["cells"].([]any)
	require.Len(t, cells, 21)
	sunDinner := cells[1*3+2].(map[string]any)
	assert.Equal(t, "2026-11-01", sunDinner["date"])
	assert.EqualValues(t, a.dinner, sunDinner["slot_id"])
	assert.Equal(t, "plan", sunDinner["mode"])
	assert.Equal(t, "fallback", sunDinner["source"])
	assert.Nil(t, sunDinner["preset_id"])
	require.NotNil(t, sunDinner["plate"])
	assert.EqualValues(t, 2, sunDinner["plate"].(map[string]any)["eaters"])
	assert.Nil(t, cells[0].(map[string]any)["plate"])

	sessions := body["sessions"].([]any)
	require.Len(t, sessions, 1)
	sess := sessions[0].(map[string]any)
	assert.EqualValues(t, sid, sess["id"])
	assert.Len(t, sess["links"], 3)
	assert.EqualValues(t, 3, sess["meals"])
	assert.EqualValues(t, 6, sess["portions"])

	days := body["days"].([]any)
	require.Len(t, days, 7)
	monday := days[2].(map[string]any)
	assert.Equal(t, "2026-11-02", monday["date"])
	assert.InDelta(t, 2*chiliPortionKcal, monday["macros"].(map[string]any)["kcal"], 1e-6, "per eater, whatever the eaters")
	assert.Equal(t, true, monday["complete"])
	assert.Equal(t, []any{}, monday["missing"])

	assert.Nil(t, body["targets"])
	assert.Equal(t, map[string]any{"plates": float64(3), "cook_days": float64(1), "warnings": float64(0)}, body["stats"])
}

func TestPlanHTTP_Targets(t *testing.T) {
	a := newReadAPI(t)
	_, err := a.db.Exec(`UPDATE user_profile SET kcal_target = 2000, protein_pct = 30, fat_pct = 30, carbs_pct = 40 WHERE id = 1`)
	require.NoError(t, err)
	code, body := a.do(http.MethodGet, "/api/plan?start=2026-10-31", "")
	require.Equal(t, http.StatusOK, code, body)
	targets := body["targets"].(map[string]any)
	assert.InDelta(t, 2000, targets["kcal"], 1e-9)
	assert.InDelta(t, 150, targets["protein_g"], 1e-9)
	assert.InDelta(t, 2000*0.3/9, targets["fat_g"], 1e-9)
	assert.InDelta(t, 200, targets["carbs_g"], 1e-9)
}

// B4-AC4
func TestPlanHTTP_LeafWithMissingNutrition(t *testing.T) {
	a := newReadAPI(t)
	sauce := a.insert(`INSERT INTO foods (name, kind, source, kcal_100g) VALUES ('Mystery sauce', 'leaf', 'manual', 200)`)
	code, body := a.do(http.MethodPost, "/api/plates",
		fmt.Sprintf(`{"date":"2026-11-03","slot_id":%d,"components":[{"food_id":%d,"amount":50,"unit":"g"}]}`, a.lunch, sauce))
	require.Equal(t, http.StatusCreated, code, body)

	code, body = a.do(http.MethodGet, "/api/plan?start=2026-10-31", "")
	require.Equal(t, http.StatusOK, code, body)
	tue := body["days"].([]any)[3].(map[string]any)
	assert.Equal(t, false, tue["complete"])
	assert.Equal(t, []any{"Mystery sauce"}, tue["missing"])
	assert.InDelta(t, 100, tue["macros"].(map[string]any)["kcal"], 1e-6)
}

// B4-AC5 over HTTP: the verdict and the JSON shape.
func TestTodayHTTP_AC1(t *testing.T) {
	a := newReadAPI(t)
	sid := a.ac1()

	code, body := a.do(http.MethodGet, "/api/today?date=2026-11-01&today=2026-11-01", "")
	require.Equal(t, http.StatusOK, code, body)
	assert.Equal(t, "2026-11-01", body["date"])
	assert.Equal(t, "2026-10-31", body["window_start"])
	assert.Equal(t, map[string]any{
		"kind": "cook", "session_id": float64(sid), "food_name": "Chili", "portions": float64(6), "slot_id": float64(a.dinner),
	}, body["verdict"])
	cook := body["cook"].([]any)
	require.Len(t, cook, 1)
	assert.EqualValues(t, sid, cook[0].(map[string]any)["session"].(map[string]any)["id"])
	assert.Nil(t, cook[0].(map[string]any)["minutes"])
	meals := body["meals"].([]any)
	require.Len(t, meals, 3)
	dinnerMeal := meals[2].(map[string]any)
	assert.Equal(t, "plan", dinnerMeal["mode"])
	require.NotNil(t, dinnerMeal["plate"])
	link := dinnerMeal["link"].(map[string]any)
	assert.Equal(t, "fresh", link["kind"])
	assert.EqualValues(t, 1, link["position"])
	assert.Nil(t, meals[0].(map[string]any)["link"])
	assert.Equal(t, []any{}, body["reminders"])
	assert.Nil(t, body["rate"])
	assert.Equal(t, map[string]any{"due": false, "next_start": "2026-11-07"}, body["plan_banner"])
	assert.Equal(t, map[string]any{"recipe_count": float64(2), "recipe_target": float64(10)}, body["setup"])

	code, body = a.do(http.MethodGet, "/api/today?date=2026-11-02", "")
	require.Equal(t, http.StatusOK, code, body)
	assert.Equal(t, map[string]any{"kind": "nothing_to_cook"}, body["verdict"])
	assert.Equal(t, "Chili", body["rate"].(map[string]any)["food_name"])

	code, body = a.do(http.MethodGet, "/api/today?date=2026-11-04", "")
	require.Equal(t, http.StatusOK, code, body)
	assert.Equal(t, "no_plates", body["verdict"].(map[string]any)["kind"])

	code, body = a.do(http.MethodGet, "/api/today?date=2026-11-10", "")
	require.Equal(t, http.StatusOK, code, body)
	assert.Equal(t, "no_plan", body["verdict"].(map[string]any)["kind"])
}

func TestTodayHTTP_BadDates(t *testing.T) {
	a := newReadAPI(t)
	for _, q := range []string{"date=01.11.2026", "today=x"} {
		code, body := a.do(http.MethodGet, "/api/today?"+q, "")
		assert.Equal(t, http.StatusBadRequest, code, q)
		assert.Equal(t, "error.invalid_body", body["message_key"], q)
	}
	code, _ := a.do(http.MethodGet, "/api/today", "")
	assert.Equal(t, http.StatusOK, code, "date and today default to the server date")
}

// planWeek plans three complete days (Nov 1-3, three slots, one Chili
// portion each: 300 kcal a day) under a 2000 kcal target.
func (a *readAPI) planWeek() {
	a.t.Helper()
	_, err := a.db.Exec(`UPDATE user_profile SET kcal_target = 2000, protein_pct = 30, fat_pct = 30, carbs_pct = 40 WHERE id = 1`)
	require.NoError(a.t, err)
	for _, d := range []string{"2026-11-01", "2026-11-02", "2026-11-03"} {
		for _, slot := range []int64{a.breakfast, a.lunch, a.dinner} {
			code, body := a.do(http.MethodPost, "/api/plates",
				fmt.Sprintf(`{"date":"%s","slot_id":%d,"components":[{"food_id":%d,"portions":1}]}`, d, slot, a.chili))
			require.Equal(a.t, http.StatusCreated, code, body)
		}
	}
}

// The default slot bands (spec v1.1 SB1): breakfast 20-30, lunch 30-40, dinner
// 25-40; the expected share is the midpoint over the sum of the midpoints.
const (
	dinnerShare    = 32.5 / 92.5
	breakfastShare = 25.0 / 92.5
)

func TestPlanHTTP_WeekLedger(t *testing.T) {
	t.Run("without a kcal target or complete days there is none", func(t *testing.T) {
		a := newReadAPI(t)
		a.ac1()
		code, body := a.do(http.MethodGet, "/api/plan?start=2026-10-31&today=2026-10-30", "")
		require.Equal(t, http.StatusOK, code, body)
		assert.Contains(t, body, "week")
		assert.Nil(t, body["week"])
		assert.Equal(t, []any{}, body["soft"])
		for _, d := range body["days"].([]any) {
			assert.Nil(t, d.(map[string]any)["target_kcal"])
			assert.NotContains(t, d, "light")
		}
	})

	t.Run("fewer than three complete days is no week", func(t *testing.T) {
		a := newReadAPI(t)
		a.planWeek()
		_, err := a.db.Exec(`DELETE FROM plates WHERE date = '2026-11-03'`)
		require.NoError(t, err)
		_, body := a.do(http.MethodGet, "/api/plan?start=2026-10-31&today=2026-10-30", "")
		assert.Nil(t, body["week"])
	})

	t.Run("a week with light marks and soft hints", func(t *testing.T) {
		a := newReadAPI(t)
		a.planWeek()
		a.insert(`INSERT INTO plan_light (date, scope, slot) VALUES ('2026-11-02', 'day', '')`)
		a.insert(`INSERT INTO plan_light (date, scope, slot) VALUES ('2026-11-03', 'meal', 'dinner')`)
		code, body := a.do(http.MethodGet, "/api/plan?start=2026-10-31&today=2026-10-30", "")
		require.Equal(t, http.StatusOK, code, body)

		week := body["week"].(map[string]any)
		assert.EqualValues(t, 3, week["days"])
		assert.InDelta(t, 300, week["avg_kcal"], 1e-6)
		assert.InDelta(t, (2000+1700+(2000-2000.0*dinnerShare/4))/3.0, week["target_kcal"], 1e-6, "the light day lowers the week target")
		assert.InDelta(t, 300/week["target_kcal"].(float64)*100, week["pct"], 1e-6)

		days := body["days"].([]any)
		day := func(i int) map[string]any { return days[i].(map[string]any) }
		assert.Equal(t, "2026-11-02", day(2)["date"])
		assert.Equal(t, "day", day(2)["light"])
		assert.InDelta(t, 1700, day(2)["target_kcal"], 1e-6)
		assert.NotContains(t, day(2), "light_slot")
		assert.Equal(t, "meal", day(3)["light"])
		assert.Equal(t, "dinner", day(3)["light_slot"])
		assert.InDelta(t, 2000-2000.0*dinnerShare/4, day(3)["target_kcal"], 1e-6, "a light meal moves the day target by 25 % of its share")
		assert.NotContains(t, day(1), "light")

		rules := map[string]bool{}
		for _, it := range body["soft"].([]any) {
			is := it.(map[string]any)
			assert.Equal(t, "soft", is["severity"])
			rules[is["rule"].(string)] = true
		}
		assert.True(t, rules["S2"], "days far under their band: %v", body["soft"])
		assert.True(t, rules["S4"], "the week is under its band: %v", body["soft"])
		for _, it := range body["issues"].([]any) {
			assert.NotEqual(t, "soft", it.(map[string]any)["severity"], "soft hints stay out of issues")
		}
	})

	// B1: a routine cell without a plate leaves the day's target like a
	// No-plate cell, so a normal week with a routine breakfast is in band.
	t.Run("a routine cell without a plate leaves the day target", func(t *testing.T) {
		a := newReadAPI(t)
		_, err := a.db.Exec(`UPDATE user_profile SET kcal_target = 1500 WHERE id = 1`)
		require.NoError(t, err)
		preset := a.insert(`INSERT INTO presets (name) VALUES ('Oatmeal')`)
		for wd := 0; wd < 7; wd++ {
			a.insert(fmt.Sprintf(`INSERT INTO availability_defaults (weekday, slot_id, mode, preset_id) VALUES (%d, %d, 'routine', %d)`, wd, a.breakfast, preset))
		}
		for _, d := range []string{"2026-11-01", "2026-11-02", "2026-11-03"} {
			for _, slot := range []int64{a.lunch, a.dinner} {
				code, body := a.do(http.MethodPost, "/api/plates",
					fmt.Sprintf(`{"date":"%s","slot_id":%d,"components":[{"food_id":%d,"portions":5.5}]}`, d, slot, a.chili))
				require.Equal(t, http.StatusCreated, code, body)
			}
		}
		code, body := a.do(http.MethodGet, "/api/plan?start=2026-10-31&today=2026-10-30", "")
		require.Equal(t, http.StatusOK, code, body)

		week := body["week"].(map[string]any)
		assert.InDelta(t, 1500*(1-breakfastShare), week["target_kcal"], 1e-6)
		assert.InDelta(t, 100, week["pct"], 1, "1100 kcal against the target of the planned part")
		for _, it := range body["soft"].([]any) {
			rule := it.(map[string]any)["rule"]
			assert.NotEqual(t, "S2", rule, "no below-band hint: %v", body["soft"])
			assert.NotEqual(t, "S4", rule, "no week hint: %v", body["soft"])
		}
	})
}
