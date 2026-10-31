package handlers_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Phase 3 §3.3: draft state in the read models and the food history.

// draftChili plans Chili Sun 2026-11-01 dinner with a Mon lunch leftover as
// a draft batch of AI run 7 (chooser heuristic). It returns the session id.
func (a *readAPI) draftChili() int64 {
	a.t.Helper()
	sid := a.ac1()
	a.insert(`INSERT INTO ai_runs (id, kind, started_at, chooser) VALUES (7, 'generate', '2026-10-30 10:00:00', 'heuristic')`)
	code, body := a.do(http.MethodPost, fmt.Sprintf("/api/cook-sessions/%d/unlock", sid), "")
	require.Equal(a.t, http.StatusOK, code, body)
	_, err := a.db.Exec(`UPDATE plates SET ai_run_id = 7 WHERE state = 'draft'`)
	require.NoError(a.t, err)
	_, err = a.db.Exec(`UPDATE cook_sessions SET ai_run_id = 7 WHERE state = 'draft'`)
	require.NoError(a.t, err)
	return sid
}

// B2-AC1
func TestPlanHTTP_Draft(t *testing.T) {
	t.Run("a window with one draft batch", func(t *testing.T) {
		a := newReadAPI(t)
		sid := a.draftChili()

		code, body := a.do(http.MethodGet, "/api/plan?start=2026-10-31&today=2026-10-30", "")
		require.Equal(t, http.StatusOK, code, body)
		assert.Equal(t, map[string]any{
			"plates": float64(3), "sessions": float64(1), "run_id": float64(7), "notes": nil, "chooser": "heuristic",
		}, body["draft"])
		assert.Contains(t, body, "running_run_id")
		assert.Nil(t, body["running_run_id"])

		sess := body["sessions"].([]any)[0].(map[string]any)
		assert.EqualValues(t, sid, sess["id"])
		assert.Equal(t, "draft", sess["state"])
		var drafts int
		for _, c := range body["cells"].([]any) {
			if p, ok := c.(map[string]any)["plate"].(map[string]any); ok {
				assert.Equal(t, "draft", p["state"])
				assert.EqualValues(t, 7, p["ai_run_id"])
				assert.Contains(t, p, "reason")
				drafts++
			}
		}
		assert.Equal(t, 3, drafts)
	})

	t.Run("a window without drafts", func(t *testing.T) {
		a := newReadAPI(t)
		a.ac1()
		code, body := a.do(http.MethodGet, "/api/plan?start=2026-10-31&today=2026-10-30", "")
		require.Equal(t, http.StatusOK, code, body)
		assert.Contains(t, body, "draft")
		assert.Nil(t, body["draft"])
	})
}

// B2-AC2 (HTTP half; H1 none cells and H5 are covered in planview)
func TestPlanHTTP_Issues(t *testing.T) {
	a := newReadAPI(t)
	a.sunChili()
	code, body := a.do(http.MethodPost, "/api/plates",
		fmt.Sprintf(`{"date":"2026-11-01","slot_id":%d,"components":[{"food_id":%d,"portions":1}]}`, a.lunch, a.curry))
	require.Equal(t, http.StatusCreated, code, body)

	code, body = a.do(http.MethodGet, "/api/plan?start=2026-10-31&today=2026-10-30", "")
	require.Equal(t, http.StatusOK, code, body)
	var h2 []map[string]any
	missing := 0
	for _, it := range body["issues"].([]any) {
		is := it.(map[string]any)
		switch is["code"] {
		case "too_many_cook_occasions":
			h2 = append(h2, is)
		case "missing_plate":
			missing++
		default:
			t.Errorf("unexpected issue %v", is)
		}
	}
	require.Len(t, h2, 1)
	assert.Equal(t, map[string]any{
		"rule": "H2", "code": "too_many_cook_occasions", "severity": "violation", "date": "2026-11-01",
		"params": map[string]any{"count": float64(2), "max": float64(1)},
	}, h2[0])
	assert.Zero(t, missing, "empty cells of a window without drafts need no attention")

	// As a draft, the window's empty cells from today on miss their plates.
	_, err := a.db.Exec(`UPDATE plates SET state = 'draft'`)
	require.NoError(t, err)
	_, err = a.db.Exec(`UPDATE cook_sessions SET state = 'draft'`)
	require.NoError(t, err)
	code, body = a.do(http.MethodGet, "/api/plan?start=2026-10-31&today=2026-11-05", "")
	require.Equal(t, http.StatusOK, code, body)
	var dates []string
	for _, it := range body["issues"].([]any) {
		if is := it.(map[string]any); is["code"] == "missing_plate" {
			dates = append(dates, is["date"].(string))
		}
	}
	assert.Equal(t, []string{"2026-11-05", "2026-11-05", "2026-11-05", "2026-11-06", "2026-11-06", "2026-11-06"}, dates)
}

// B2-AC3
func TestTodayHTTP_Drafts(t *testing.T) {
	a := newReadAPI(t)
	a.draftChili()
	code, body := a.do(http.MethodGet, "/api/today?date=2026-11-01&today=2026-11-01", "")
	require.Equal(t, http.StatusOK, code, body)
	assert.Equal(t, map[string]any{"kind": "draft_only"}, body["verdict"])
	assert.Empty(t, body["cook"])

	code, body = a.do(http.MethodPost, "/api/plan/accept", `{"from":"2026-10-31","to":"2026-11-06"}`)
	require.Equal(t, http.StatusOK, code, body)
	code, body = a.do(http.MethodGet, "/api/today?date=2026-11-01&today=2026-11-01", "")
	require.Equal(t, http.StatusOK, code, body)
	assert.Equal(t, "cook", body["verdict"].(map[string]any)["kind"])
}

// B2-AC5
func TestFoodHistory_PastDraftsDoNotCount(t *testing.T) {
	a := newHistoryAPI(t)
	a.chiliBatch()
	code, body := a.do(http.MethodPost, "/api/plates",
		fmt.Sprintf(`{"date":"2026-10-28","slot_id":%d,"components":[{"food_id":%d}]}`, a.dinner, a.chili))
	require.Equal(t, http.StatusCreated, code, body)
	code, body = a.do(http.MethodPost, fmt.Sprintf("/api/plates/%d/unlock", int64(body["id"].(float64))), "")
	require.Equal(t, http.StatusOK, code, body)

	chili := a.recipe("today=2026-10-30", a.chili)
	assert.Equal(t, "2026-10-27", chili["last_eaten"])
	assert.EqualValues(t, 1, chili["times_cooked"])

	// The list filter and sort read the same history.
	a.recipe("today=2026-11-10&not_eaten_weeks=2", a.chili)
}
