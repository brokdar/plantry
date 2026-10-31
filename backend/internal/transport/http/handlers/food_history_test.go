package handlers_test

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper/testsvc"
	transport "github.com/jaltszeimer/plantry/backend/internal/transport/http"
	"github.com/jaltszeimer/plantry/backend/internal/transport/http/handlers"
)

// newHistoryAPI is planAPI with the food and feedback handlers mounted too.
func newHistoryAPI(t *testing.T) *planAPI {
	t.Helper()
	a := newPlanAPI(t)
	plates := testsvc.Plates(a.db)
	foods := sqlite.NewFoodRepo(a.db)
	a.router = transport.NewRouter(slog.New(slog.NewTextHandler(io.Discard, nil)), http.NotFoundHandler(), transport.Handlers{
		Foods:        handlers.NewFoodHandler(food.NewService(foods), food.NewNutritionResolver(foods), nil),
		Plates:       handlers.NewPlateHandler(plates),
		CookSessions: handlers.NewCookSessionHandler(plates),
		PlanRead:     handlers.NewPlanReadHandler(testsvc.PlanView(a.db)),
		Feedback:     handlers.NewFeedbackHandler(testsvc.Feedback(a.db)),
	})
	return a
}

// chiliBatch plans Chili cooked Sun 2026-10-25 dinner with leftovers on the
// Mon and Tue dinners, and returns the three plate ids by date.
func (a *planAPI) chiliBatch() map[string]int64 {
	a.t.Helper()
	code, body := a.do(http.MethodPost, "/api/plates",
		fmt.Sprintf(`{"date":"2026-10-25","slot_id":%d,"components":[{"food_id":%d}]}`, a.dinner, a.chili))
	require.Equal(a.t, http.StatusCreated, code, body)
	sid := int64(body["components"].([]any)[0].(map[string]any)["cook_session_id"].(float64))
	code, sess := a.do(http.MethodPost, fmt.Sprintf("/api/cook-sessions/%d/leftovers", sid),
		fmt.Sprintf(`{"cells":[{"date":"2026-10-26","slot_id":%d},{"date":"2026-10-27","slot_id":%d}]}`, a.dinner, a.dinner))
	require.Equal(a.t, http.StatusOK, code, sess)
	plates := map[string]int64{}
	for _, l := range sess["links"].([]any) {
		link := l.(map[string]any)
		plates[link["date"].(string)] = int64(link["plate_id"].(float64))
	}
	require.Len(a.t, plates, 3)
	return plates
}

// recipe returns the composed food id from GET /api/foods?kind=composed.
func (a *planAPI) recipe(query string, id int64) map[string]any {
	a.t.Helper()
	code, body := a.do(http.MethodGet, "/api/foods?kind=composed&"+query, "")
	require.Equal(a.t, http.StatusOK, code, body)
	for _, it := range body["items"].([]any) {
		f := it.(map[string]any)
		if int64(f["id"].(float64)) == id {
			return f
		}
	}
	a.t.Fatalf("food %d not listed for %q", id, query)
	return nil
}

func (a *planAPI) feedback(plateID int64, status string) (int, map[string]any) {
	a.t.Helper()
	return a.do(http.MethodPut, fmt.Sprintf("/api/plates/%d/feedback", plateID), fmt.Sprintf(`{"status":%q}`, status))
}

// B7-AC1
func TestFoodHistory_BatchCountsOnceAndLastEatenIsTheLatestLeftover(t *testing.T) {
	a := newHistoryAPI(t)
	a.chiliBatch()

	chili := a.recipe("today=2026-10-30", a.chili)
	assert.Equal(t, "2026-10-27", chili["last_eaten"])
	assert.EqualValues(t, 1, chili["times_cooked"], "one session, not three plates")

	code, got := a.do(http.MethodGet, fmt.Sprintf("/api/foods/%d?today=2026-10-30", a.chili), "")
	require.Equal(t, http.StatusOK, code, got)
	assert.Equal(t, "2026-10-27", got["last_eaten"])
	assert.EqualValues(t, 1, got["times_cooked"])
}

// B7-AC2
func TestFoodHistory_NotEatenPlatesLeaveTheHistory(t *testing.T) {
	a := newHistoryAPI(t)
	plates := a.chiliBatch()

	code, body := a.feedback(plates["2026-10-27"], "not_eaten")
	require.Equal(t, http.StatusOK, code, body)
	chili := a.recipe("today=2026-10-30", a.chili)
	assert.Equal(t, "2026-10-26", chili["last_eaten"])
	assert.EqualValues(t, 1, chili["times_cooked"])

	for _, d := range []string{"2026-10-25", "2026-10-26"} {
		code, body := a.feedback(plates[d], "not_eaten")
		require.Equal(t, http.StatusOK, code, body)
	}
	chili = a.recipe("today=2026-10-30", a.chili)
	assert.Contains(t, chili, "last_eaten")
	assert.Nil(t, chili["last_eaten"])
	assert.EqualValues(t, 0, chili["times_cooked"])

	code, _ = a.do(http.MethodDelete, fmt.Sprintf("/api/plates/%d/feedback", plates["2026-10-25"]), "")
	require.Equal(t, http.StatusNoContent, code)
	chili = a.recipe("today=2026-10-30", a.chili)
	assert.Equal(t, "2026-10-25", chili["last_eaten"], "clearing the feedback restores the plate")
	assert.EqualValues(t, 1, chili["times_cooked"])
}

// B7-AC3
func TestFoodHistory_FuturePlatesDoNotCount(t *testing.T) {
	a := newHistoryAPI(t)
	a.chiliBatch()

	chili := a.recipe("today=2026-10-26", a.chili)
	assert.Equal(t, "2026-10-26", chili["last_eaten"], "the 10-27 leftover is still ahead")
	assert.EqualValues(t, 1, chili["times_cooked"])

	chili = a.recipe("today=2026-10-24", a.chili)
	assert.Nil(t, chili["last_eaten"])
	assert.EqualValues(t, 0, chili["times_cooked"], "the session has not been cooked yet")
}

func TestFoodHistory_LeavesCarryUsedInCountNotHistory(t *testing.T) {
	a := newHistoryAPI(t)
	beans := a.insert(`INSERT INTO foods (name, kind, source) VALUES ('Beans', 'leaf', 'manual')`)
	a.insert(fmt.Sprintf(`INSERT INTO food_components (parent_id, child_id, amount, unit, grams) VALUES (%d, %d, 400, 'g', 400)`, a.chili, beans))
	a.insert(fmt.Sprintf(`INSERT INTO food_components (parent_id, child_id, amount, unit, grams) VALUES (%d, %d, 200, 'g', 200)`, a.curry, beans))
	a.insert(fmt.Sprintf(`INSERT INTO food_components (parent_id, child_id, amount, unit, grams) VALUES (%d, %d, 100, 'g', 100)`, a.curry, beans))

	code, got := a.do(http.MethodGet, fmt.Sprintf("/api/foods/%d", beans), "")
	require.Equal(t, http.StatusOK, code, got)
	assert.EqualValues(t, 2, got["used_in_count"], "two recipes, however many lines")
	assert.NotContains(t, got, "last_eaten")
	assert.NotContains(t, got, "times_cooked")
	assert.NotContains(t, got, "per_portion")

	code, got = a.do(http.MethodGet, fmt.Sprintf("/api/foods/%d", a.chili), "")
	require.Equal(t, http.StatusOK, code, got)
	assert.NotContains(t, got, "used_in_count")
	pp := got["per_portion"].(map[string]any)
	assert.Contains(t, pp, "kcal")
	assert.Contains(t, pp, "protein")
	assert.Equal(t, false, pp["complete"], "Beans have no nutrition")
}

// listNames returns the names of GET /api/foods?kind=composed&query, in order.
func (a *planAPI) listNames(query string) []string {
	a.t.Helper()
	code, body := a.do(http.MethodGet, "/api/foods?kind=composed&"+query, "")
	require.Equal(a.t, http.StatusOK, code, body)
	names := []string{}
	for _, it := range body["items"].([]any) {
		names = append(names, it.(map[string]any)["name"].(string))
	}
	assert.EqualValues(a.t, len(names), body["total"], "total matches the filter")
	return names
}

// B7-AC4
func TestFoodList_RecipeFiltersAndLastEatenSort(t *testing.T) {
	a := newHistoryAPI(t)
	a.chiliBatch() // Chili last eaten 2026-10-27
	soup := a.insert(`INSERT INTO foods (name, kind, role, reference_portions) VALUES ('Soup', 'composed', 'main', 2)`)
	stew := a.insert(`INSERT INTO foods (name, kind, role, reference_portions, favorite) VALUES ('Stew', 'composed', 'main', 2, 1)`)
	for date, id := range map[string]int64{"2026-10-01": soup, "2026-10-02": stew} {
		code, body := a.do(http.MethodPost, "/api/plates",
			fmt.Sprintf(`{"date":%q,"slot_id":%d,"components":[{"food_id":%d}]}`, date, a.lunch, id))
		require.Equal(t, http.StatusCreated, code, body)
	}
	// Curry is never eaten.

	t.Run("sort=last_eaten: never eaten first, then oldest; favorites do not float", func(t *testing.T) {
		assert.Equal(t, []string{"Curry", "Soup", "Stew", "Chili"}, a.listNames("sort=last_eaten&today=2026-10-30"))
	})

	t.Run("not_eaten_weeks=4 excludes recipes eaten within 28 days", func(t *testing.T) {
		// The cutoff is 2026-10-02: Stew, eaten exactly 28 days ago, stays.
		assert.Equal(t, []string{"Stew", "Curry", "Soup"}, a.listNames("not_eaten_weeks=4&today=2026-10-30"))
		assert.Equal(t, []string{"Stew", "Curry", "Soup"}, a.listNames("not_eaten_weeks=4&today=2026-10-30&sort=name"))
		// A day earlier the cutoff is 2026-10-01 and Stew is within it.
		assert.Equal(t, []string{"Curry", "Soup"}, a.listNames("not_eaten_weeks=4&today=2026-10-29"))
	})

	t.Run("max_minutes=30 filters on prep + cook", func(t *testing.T) {
		a.insert(fmt.Sprintf(`UPDATE foods SET prep_minutes = 20, cook_minutes = 15 WHERE id = %d`, a.chili))
		a.insert(fmt.Sprintf(`UPDATE foods SET prep_minutes = 10, cook_minutes = 20 WHERE id = %d`, a.curry))
		a.insert(fmt.Sprintf(`UPDATE foods SET cook_minutes = 25 WHERE id = %d`, soup))
		// Stew has no times at all: unknown is not quick.
		assert.Equal(t, []string{"Curry", "Soup"}, a.listNames("max_minutes=30&today=2026-10-30"))
	})

	t.Run("created_within_days=30 filters on created_at", func(t *testing.T) {
		a.insert(`UPDATE foods SET created_at = '2026-09-29 23:59:59' WHERE kind = 'composed'`)
		a.insert(fmt.Sprintf(`UPDATE foods SET created_at = '2026-09-30 00:00:00' WHERE id = %d`, a.curry))
		a.insert(fmt.Sprintf(`UPDATE foods SET created_at = '2026-10-29 12:00:00' WHERE id = %d`, soup))
		assert.Equal(t, []string{"Curry", "Soup"}, a.listNames("created_within_days=30&today=2026-10-30"))
	})

	t.Run("sort=created orders on created_at", func(t *testing.T) {
		names := a.listNames("sort=created&order=desc&today=2026-10-30")
		assert.Equal(t, []string{"Stew", "Soup", "Curry"}, names[:3], "favorites first, then newest")
	})
}

func TestFoodList_InvalidQueryParams(t *testing.T) {
	a := newHistoryAPI(t)
	for _, q := range []string{
		"max_minutes=abc", "max_minutes=0", "max_minutes=-5",
		"not_eaten_weeks=x", "not_eaten_weeks=0",
		"created_within_days=1.5", "created_within_days=-1",
	} {
		t.Run(q, func(t *testing.T) {
			code, body := a.do(http.MethodGet, "/api/foods?"+q, "")
			assert.Equal(t, http.StatusBadRequest, code)
			assert.Equal(t, "error.invalid_query", body["message_key"])
		})
	}
	code, _ := a.do(http.MethodGet, "/api/foods?today=2026-13-01", "")
	assert.Equal(t, http.StatusBadRequest, code)
	code, _ = a.do(http.MethodGet, fmt.Sprintf("/api/foods/%d?today=yesterday", a.chili), "")
	assert.Equal(t, http.StatusBadRequest, code)
}

// B7-AC5
func TestFeedback_StatusesAndSideEffects(t *testing.T) {
	a := newHistoryAPI(t)
	plates := a.chiliBatch()
	a.insert(fmt.Sprintf(`INSERT INTO food_tags (food_id, tag) VALUES (%d, 'spicy')`, a.chili))
	plate := plates["2026-10-25"]

	foodRow := func() []any {
		rows, err := a.db.Query(`SELECT * FROM foods WHERE id = ?`, a.chili)
		require.NoError(t, err)
		defer func() { _ = rows.Close() }()
		cols, err := rows.Columns()
		require.NoError(t, err)
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		require.True(t, rows.Next())
		require.NoError(t, rows.Scan(ptrs...))
		return vals
	}
	before := foodRow()

	for _, status := range []string{"cooked", "skipped", "", "LOVED"} {
		t.Run("rejects "+status, func(t *testing.T) {
			code, body := a.feedback(plate, status)
			assert.Equal(t, http.StatusBadRequest, code)
			assert.Equal(t, "error.feedback.invalid_status", body["message_key"])
		})
	}

	code, body := a.feedback(plate, "loved")
	require.Equal(t, http.StatusOK, code, body)
	assert.Equal(t, "loved", body["status"])
	prof, err := profile.NewService(sqlite.NewProfileRepo(a.db)).Get(context.Background())
	require.NoError(t, err)
	assert.Contains(t, prof.Preferences[profile.PreferenceKeyLikes], "spicy", "loved appends the tags")

	for _, status := range []string{"disliked", "not_eaten"} {
		code, body := a.feedback(plate, status)
		require.Equal(t, http.StatusOK, code, body)
		assert.Equal(t, status, body["status"])
	}
	assert.Equal(t, before, foodRow(), "no feedback status changes the food row")

	code, got := a.do(http.MethodGet, "/api/plan?start=2026-10-19&today=2026-10-30", "")
	require.Equal(t, http.StatusOK, code, got)
	var fb map[string]any
	for _, c := range got["cells"].([]any) {
		if p, ok := c.(map[string]any)["plate"].(map[string]any); ok && int64(p["id"].(float64)) == plate {
			fb = p["feedback"].(map[string]any)
		}
	}
	require.NotNil(t, fb, "the plan's plate JSON carries its feedback")
	assert.Equal(t, "not_eaten", fb["status"])
}

// B7-AC6: GET /api/foods/insights is gone. The path now falls through to the
// numeric food route, which rejects it.
func TestFoodRotationEndpoint_IsGone(t *testing.T) {
	a := newHistoryAPI(t)
	code, body := a.do(http.MethodGet, "/api/foods/insights", "")
	assert.NotEqual(t, http.StatusOK, code)
	assert.NotContains(t, body, "forgotten")
	assert.NotContains(t, body, "most_cooked")
}
