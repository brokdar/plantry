package handlers_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// POST /api/plan/alternatives lists verified replacements for a plate and
// writes nothing.
func TestPlanAlternativesHTTP(t *testing.T) {
	a := newGenAPI(t)
	var lunch, food int64
	require.NoError(t, a.db.QueryRow(`SELECT id FROM time_slots WHERE name_key = 'slot.lunch'`).Scan(&lunch))
	require.NoError(t, a.db.QueryRow(`SELECT id FROM foods WHERE name = 'Main 1'`).Scan(&food))
	code, body := a.do(http.MethodPost, "/api/plates", fmt.Sprintf(`{"date":"2026-11-12","slot_id":%d,"components":[{"food_id":%d,"portions":1}]}`, lunch, food))
	require.Equal(t, http.StatusCreated, code, body)
	pid := int64(body["id"].(float64))
	_, err := a.db.Exec(`UPDATE plates SET state = 'draft' WHERE id = ?`, pid)
	require.NoError(t, err, "a hand-made plate is confirmed; alternatives are for drafts")
	_, err = a.db.Exec(`UPDATE cook_sessions SET state = 'draft'`)
	require.NoError(t, err)

	code, body = a.do(http.MethodPost, "/api/plates", fmt.Sprintf(`{"date":"2026-11-13","slot_id":%d,"components":[{"food_id":%d,"portions":1}]}`, lunch, food))
	require.Equal(t, http.StatusCreated, code, body)
	confirmed := int64(body["id"].(float64))

	code, body = a.do(http.MethodPost, "/api/plates", fmt.Sprintf(`{"date":"2026-11-20","slot_id":%d,"components":[{"food_id":%d,"portions":1}]}`, lunch, food))
	require.Equal(t, http.StatusCreated, code, body)
	outside := int64(body["id"].(float64))

	url := "/api/plan/alternatives?start=2026-11-07&today=2026-11-11"
	var before int
	require.NoError(t, a.db.QueryRow(`SELECT COUNT(*) FROM plate_components`).Scan(&before))
	code, body = a.do(http.MethodPost, url, fmt.Sprintf(`{"plate_id":%d,"what":"main","limit":3}`, pid))
	require.Equal(t, http.StatusOK, code, body)
	list := body["alternatives"].([]any)
	require.Len(t, list, 3)
	for _, e := range list {
		alt := e.(map[string]any)
		assert.NotEqual(t, float64(food), alt["food_id"], "the replaced main is never offered")
		assert.Equal(t, "fresh", alt["source"])
		ops := alt["ops"].([]any)
		require.NotEmpty(t, ops)
		assert.Equal(t, "swap", ops[0].(map[string]any)["op"])
		assert.Contains(t, alt, "new_issues")
	}
	var after int
	require.NoError(t, a.db.QueryRow(`SELECT COUNT(*) FROM plate_components`).Scan(&after))
	assert.Equal(t, before, after, "nothing written")

	code, body = a.do(http.MethodPost, url, fmt.Sprintf(`{"plate_id":%d,"what":"main","constraints":{"search":"zzz-nothing"}}`, pid))
	require.Equal(t, http.StatusOK, code, body)
	assert.Equal(t, []any{}, body["alternatives"])
	assert.Equal(t, map[string]any{"candidates": float64(0)}, body["empty_reason"], "nothing fits the constraints")

	// A dinner eating the lunch batch does not cook: its alternatives cook at
	// the lunch cell (cook_at), and POST /api/plates/{id}/replace takes the op.
	// Future dates: the plate service's clock is the real one.
	var dinner, sid int64
	require.NoError(t, a.db.QueryRow(`SELECT id FROM time_slots WHERE name_key = 'slot.dinner'`).Scan(&dinner))
	code, body = a.do(http.MethodPost, "/api/plates", fmt.Sprintf(`{"date":"2030-01-09","slot_id":%d,"components":[{"food_id":%d,"portions":1}]}`, lunch, food))
	require.Equal(t, http.StatusCreated, code, body)
	cooking := int64(body["id"].(float64))
	require.NoError(t, a.db.QueryRow(`SELECT cook_session_id FROM plate_components WHERE plate_id = ?`, cooking).Scan(&sid))
	code, body = a.do(http.MethodPost, "/api/plates", fmt.Sprintf(`{"date":"2030-01-09","slot_id":%d,"components":[{"food_id":%d,"portions":1,"cook_session_id":%d}]}`, dinner, food, sid))
	require.Equal(t, http.StatusCreated, code, body)
	leftover := int64(body["id"].(float64))
	_, err = a.db.Exec(`UPDATE plates SET state = 'draft' WHERE date >= '2030-01-01'`)
	require.NoError(t, err)
	_, err = a.db.Exec(`UPDATE cook_sessions SET state = 'draft'`)
	require.NoError(t, err)
	code, body = a.do(http.MethodPost, "/api/plan/alternatives?start=2030-01-05&today=2030-01-05", fmt.Sprintf(`{"plate_id":%d,"what":"main","limit":1}`, leftover))
	require.Equal(t, http.StatusOK, code, body)
	alt := body["alternatives"].([]any)[0].(map[string]any)
	require.Equal(t, "cook_at", alt["source"], "%v", alt)
	op := alt["ops"].([]any)[0].(map[string]any)
	require.Equal(t, "replace", op["op"])
	cookAt := op["cook_at"].(map[string]any)
	assert.Equal(t, map[string]any{"date": "2030-01-09", "slot_id": float64(lunch)}, cookAt)
	code, body = a.do(http.MethodPost, fmt.Sprintf("/api/plates/%d/replace", leftover), fmt.Sprintf(`{"plate_component_id":%v,"food_id":%v,"cook_at":{"date":%q,"slot_id":%v},"portions":%v}`,
		op["plate_component_id"], op["food_id"], cookAt["date"], cookAt["slot_id"], op["portions"]))
	require.Equal(t, http.StatusOK, code, body)
	var cookDate string
	var cookSlot int64
	require.NoError(t, a.db.QueryRow(`SELECT cs.cook_date, cs.cook_slot_id FROM plate_components pc JOIN cook_sessions cs ON cs.id = pc.cook_session_id WHERE pc.plate_id = ?`, leftover).Scan(&cookDate, &cookSlot))
	assert.Equal(t, "2030-01-09", cookDate[:10])
	assert.Equal(t, lunch, cookSlot, "the new dish cooks with the lunch")

	for _, c := range []struct {
		name, url, body string
		status          int
		key             string
	}{
		{"malformed body", url, `{`, http.StatusBadRequest, "error.invalid_body"},
		{"missing plate", url, `{"what":"main"}`, http.StatusBadRequest, "error.invalid_body"},
		{"bad start", "/api/plan/alternatives?start=nope", fmt.Sprintf(`{"plate_id":%d,"what":"main"}`, pid), http.StatusBadRequest, "error.plan.invalid_start"},
		{"start not a shopping day", "/api/plan/alternatives?start=2026-11-08&today=2026-11-11", fmt.Sprintf(`{"plate_id":%d,"what":"main"}`, pid), http.StatusBadRequest, "error.plan.invalid_start"},
		{"unknown plate", url, `{"plate_id":99999,"what":"main"}`, http.StatusNotFound, ""},
		{"confirmed plate", url, fmt.Sprintf(`{"plate_id":%d,"what":"main"}`, confirmed), http.StatusBadRequest, "error.invalid_body"},
		{"negative limit", url, fmt.Sprintf(`{"plate_id":%d,"what":"main","limit":-1}`, pid), http.StatusBadRequest, "error.invalid_body"},
		{"limit over 10", url, fmt.Sprintf(`{"plate_id":%d,"what":"main","limit":11}`, pid), http.StatusBadRequest, "error.invalid_body"},
		{"plate outside the window", url, fmt.Sprintf(`{"plate_id":%d,"what":"main"}`, outside), http.StatusNotFound, ""},
		{"unknown diet", url, fmt.Sprintf(`{"plate_id":%d,"what":"main","constraints":{"diet":"carnivore"}}`, pid), http.StatusBadRequest, "error.invalid_body"},
		{"unknown role", url, fmt.Sprintf(`{"plate_id":%d,"what":"main","constraints":{"roles":["dessertt"]}}`, pid), http.StatusBadRequest, "error.invalid_body"},
		{"bad what", url, fmt.Sprintf(`{"plate_id":%d,"what":"dessert"}`, pid), http.StatusBadRequest, "error.invalid_body"},
	} {
		t.Run(c.name, func(t *testing.T) {
			code, body := a.do(http.MethodPost, c.url, c.body)
			assert.Equal(t, c.status, code, body)
			if c.key != "" {
				assert.Equal(t, c.key, body["message_key"])
			}
		})
	}
}
