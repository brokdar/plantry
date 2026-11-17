package handlers_test

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper/testsvc"
	transport "github.com/jaltszeimer/plantry/backend/internal/transport/http"
	"github.com/jaltszeimer/plantry/backend/internal/transport/http/handlers"
)

// Meal operations over HTTP (slice 2b). Dates lie in 2030 so they are never
// past for the real clock.

func newMealAPI(t *testing.T) *planAPI {
	t.Helper()
	db := testhelper.NewTestDB(t)
	plates, avail := testsvc.Plan(db)
	a := &planAPI{t: t, db: db}
	a.router = transport.NewRouter(slog.New(slog.NewTextHandler(io.Discard, nil)), http.NotFoundHandler(), transport.Handlers{
		Plates:       handlers.NewPlateHandler(plates),
		CookSessions: handlers.NewCookSessionHandler(plates),
		PlanWrite:    handlers.NewPlanWriteHandler(plates),
		Availability: handlers.NewAvailabilityHandler(avail),
	})
	a.lunch = a.insert(`INSERT INTO time_slots (name_key, icon, sort_order, active) VALUES ('slot.lunch', 'sun', 1, 1)`)
	a.dinner = a.insert(`INSERT INTO time_slots (name_key, icon, sort_order, active) VALUES ('slot.dinner', 'moon', 2, 1)`)
	a.chili = a.insert(`INSERT INTO foods (name, kind, role, reference_portions) VALUES ('Chili', 'composed', 'main', 4)`)
	a.curry = a.insert(`INSERT INTO foods (name, kind, role, reference_portions) VALUES ('Curry', 'composed', 'main', 2)`)
	_, err := db.Exec(`UPDATE user_profile SET household_portions = 2 WHERE id = 1`)
	require.NoError(t, err)
	return a
}

func (a *planAPI) plateOn(date string, slot, food int64) (plateID, sessionID int64) {
	a.t.Helper()
	code, body := a.do(http.MethodPost, "/api/plates",
		fmt.Sprintf(`{"date":%q,"slot_id":%d,"components":[{"food_id":%d,"portions":1}]}`, date, slot, food))
	require.Equal(a.t, http.StatusCreated, code, body)
	comp := body["components"].([]any)[0].(map[string]any)
	return int64(body["id"].(float64)), int64(comp["cook_session_id"].(float64))
}

func TestMealHTTP_MoveThenRevert(t *testing.T) {
	a := newMealAPI(t)
	id, _ := a.plateOn("2030-01-07", a.dinner, a.chili)

	code, body := a.do(http.MethodPost, fmt.Sprintf("/api/plates/%d/move", id), fmt.Sprintf(`{"date":"2030-01-08","slot_id":%d}`, a.lunch))
	require.Equal(t, http.StatusOK, code, body)
	p := body["plates"].([]any)[0].(map[string]any)
	assert.Equal(t, "2030-01-08", p["date"])
	assert.Equal(t, []any{}, body["also_changed"])
	undo := body["undo"].(map[string]any)
	require.NotZero(t, undo["first_event_id"])

	code, body = a.do(http.MethodPost, "/api/plan/revert",
		fmt.Sprintf(`{"first_event_id":%v,"last_event_id":%v}`, undo["first_event_id"], undo["last_event_id"]))
	require.Equal(t, http.StatusOK, code, body)
	code, body = a.do(http.MethodGet, fmt.Sprintf("/api/plates/%d", id), "")
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, "2030-01-07", body["date"])

	code, body = a.do(http.MethodPost, "/api/plan/revert",
		fmt.Sprintf(`{"first_event_id":%v,"last_event_id":%v}`, undo["first_event_id"], undo["last_event_id"]))
	assert.Equal(t, http.StatusConflict, code, "the revert itself made the range stale")
	assert.Equal(t, "error.plan.change_stale", body["message_key"])
}

func TestMealHTTP_TakenCellIs409WithTheOccupant(t *testing.T) {
	a := newMealAPI(t)
	id, _ := a.plateOn("2030-01-07", a.dinner, a.chili)
	occ, _ := a.plateOn("2030-01-08", a.dinner, a.curry)

	code, body := a.do(http.MethodPost, fmt.Sprintf("/api/plates/%d/move", id), fmt.Sprintf(`{"date":"2030-01-08","slot_id":%d}`, a.dinner))
	require.Equal(t, http.StatusConflict, code, body)
	assert.Equal(t, "error.plate.cell_occupied", body["message_key"])
	assert.EqualValues(t, occ, body["occupant_plate_id"])

	code, body = a.do(http.MethodPut, fmt.Sprintf("/api/plates/%d", id), fmt.Sprintf(`{"date":"2030-01-08","slot_id":%d}`, a.dinner))
	require.Equal(t, http.StatusConflict, code, body)
	assert.EqualValues(t, occ, body["occupant_plate_id"])

	code, body = a.do(http.MethodPost, fmt.Sprintf("/api/plates/%d/move", id), fmt.Sprintf(`{"date":"2030-01-08","slot_id":%d,"occupant":"swap"}`, a.dinner))
	require.Equal(t, http.StatusOK, code, body)
	assert.Len(t, body["plates"], 2)
}

func TestMealHTTP_UpdateCarriesAlsoChanged(t *testing.T) {
	a := newMealAPI(t)
	id, _ := a.plateOn("2030-01-07", a.dinner, a.chili)

	code, body := a.do(http.MethodPut, fmt.Sprintf("/api/plates/%d", id), `{"date":"2030-01-09"}`)

	require.Equal(t, http.StatusOK, code, body)
	assert.Equal(t, "2030-01-09", body["date"])
	assert.Equal(t, []any{}, body["also_changed"])
	assert.NotNil(t, body["undo"])
}

func TestMealHTTP_ReplaceCopyAndMoveCook(t *testing.T) {
	a := newMealAPI(t)
	id, sid := a.plateOn("2030-01-08", a.dinner, a.chili)

	code, body := a.do(http.MethodPost, fmt.Sprintf("/api/cook-sessions/%d/move-cook", sid), fmt.Sprintf(`{"date":"2030-01-08","slot_id":%d}`, a.lunch))
	require.Equal(t, http.StatusOK, code, body)
	assert.Equal(t, []any{float64(sid)}, body["session_ids"])

	code, body = a.do(http.MethodPost, fmt.Sprintf("/api/plates/%d/copy", id), fmt.Sprintf(`{"cells":[{"date":"2030-01-09","slot_id":%d}],"as":"leftover"}`, a.lunch))
	require.Equal(t, http.StatusCreated, code, body)
	assert.Len(t, body["plates"], 1)

	code, body = a.do(http.MethodPost, fmt.Sprintf("/api/plates/%d/replace", id), fmt.Sprintf(`{"position":"main","food_id":%d}`, a.curry))
	require.Equal(t, http.StatusOK, code, body)
	comp := body["plates"].([]any)[0].(map[string]any)["components"].([]any)[0].(map[string]any)
	assert.Equal(t, "Curry", comp["food_name"])

	code, body = a.do(http.MethodPost, fmt.Sprintf("/api/plates/%d/replace", id), fmt.Sprintf(`{"position":"main","food_id":%d}`, a.curry))
	assert.Equal(t, http.StatusBadRequest, code, "same food: %v", body)
}

func TestMealHTTP_CopyIsAllOrNothing(t *testing.T) {
	a := newMealAPI(t)
	id, _ := a.plateOn("2030-01-08", a.dinner, a.chili)

	code, body := a.do(http.MethodPost, fmt.Sprintf("/api/plates/%d/copy", id),
		fmt.Sprintf(`{"cells":[{"date":"2030-01-09","slot_id":%d},{"date":"2020-01-09","slot_id":%d}],"as":"fresh"}`, a.lunch, a.lunch))

	require.Equal(t, http.StatusBadRequest, code, body)
	assert.Equal(t, "error.plan.past_cell", body["message_key"])
	var n int
	require.NoError(t, a.db.QueryRow(`SELECT COUNT(*) FROM plates`).Scan(&n))
	assert.Equal(t, 1, n, "the valid cell was not written either")
}

func TestMealHTTP_ComponentEditReturnsUndo(t *testing.T) {
	a := newMealAPI(t)
	id, _ := a.plateOn("2030-01-08", a.dinner, a.chili)

	code, body := a.do(http.MethodPost, fmt.Sprintf("/api/plates/%d/components", id), fmt.Sprintf(`{"food_id":%d}`, a.curry))

	require.Equal(t, http.StatusCreated, code, body)
	assert.Len(t, body["components"], 2)
	assert.NotZero(t, body["undo"].(map[string]any)["first_event_id"])
}

func TestMealHTTP_BadRequests(t *testing.T) {
	a := newMealAPI(t)
	id, sid := a.plateOn("2030-01-08", a.dinner, a.chili)
	cases := []struct {
		name, method, path, body string
		want                     int
	}{
		{"move malformed", http.MethodPost, fmt.Sprintf("/api/plates/%d/move", id), `{`, 400},
		{"move bad id", http.MethodPost, "/api/plates/abc/move", `{}`, 400},
		{"move bad date", http.MethodPost, fmt.Sprintf("/api/plates/%d/move", id), `{"date":"x","slot_id":1}`, 400},
		{"move bad occupant word", http.MethodPost, fmt.Sprintf("/api/plates/%d/move", id), fmt.Sprintf(`{"date":"2030-01-09","slot_id":%d,"occupant":"shove"}`, a.lunch), 400},
		{"move bad occupant shape", http.MethodPost, fmt.Sprintf("/api/plates/%d/move", id), fmt.Sprintf(`{"date":"2030-01-09","slot_id":%d,"occupant":[1]}`, a.lunch), 400},
		{"move bad occupant cell", http.MethodPost, fmt.Sprintf("/api/plates/%d/move", id), fmt.Sprintf(`{"date":"2030-01-09","slot_id":%d,"occupant":{"date":"2030-01-10"}}`, a.lunch), 400},
		{"move unknown plate", http.MethodPost, "/api/plates/999/move", fmt.Sprintf(`{"date":"2030-01-09","slot_id":%d}`, a.lunch), 404},
		{"move into the past", http.MethodPost, fmt.Sprintf("/api/plates/%d/move", id), fmt.Sprintf(`{"date":"2020-01-09","slot_id":%d}`, a.lunch), 400},
		{"replace malformed", http.MethodPost, fmt.Sprintf("/api/plates/%d/replace", id), `{`, 400},
		{"replace no selector", http.MethodPost, fmt.Sprintf("/api/plates/%d/replace", id), fmt.Sprintf(`{"food_id":%d}`, a.curry), 400},
		{"replace both selectors", http.MethodPost, fmt.Sprintf("/api/plates/%d/replace", id), fmt.Sprintf(`{"position":"main","plate_component_id":1,"food_id":%d}`, a.curry), 400},
		{"replace bad cook_at", http.MethodPost, fmt.Sprintf("/api/plates/%d/replace", id), fmt.Sprintf(`{"position":"main","food_id":%d,"cook_at":{"date":"x"}}`, a.curry), 400},
		{"copy malformed", http.MethodPost, fmt.Sprintf("/api/plates/%d/copy", id), `{`, 400},
		{"copy no cells", http.MethodPost, fmt.Sprintf("/api/plates/%d/copy", id), `{"cells":[],"as":"fresh"}`, 400},
		{"copy bad as", http.MethodPost, fmt.Sprintf("/api/plates/%d/copy", id), fmt.Sprintf(`{"cells":[{"date":"2030-01-09","slot_id":%d}],"as":"twice"}`, a.lunch), 400},
		{"move-cook malformed", http.MethodPost, fmt.Sprintf("/api/cook-sessions/%d/move-cook", sid), `{`, 400},
		{"move-cook bad id", http.MethodPost, "/api/cook-sessions/x/move-cook", `{}`, 400},
		{"move-cook bad cell", http.MethodPost, fmt.Sprintf("/api/cook-sessions/%d/move-cook", sid), `{"date":"2030-01-08"}`, 400},
		{"revert malformed", http.MethodPost, "/api/plan/revert", `{`, 400},
		{"revert empty token", http.MethodPost, "/api/plan/revert", `{"first_event_id":0,"last_event_id":0}`, 400},
		{"revert first id missing", http.MethodPost, "/api/plan/revert", `{"first_event_id":0,"last_event_id":5}`, 400},
		{"revert inverted range", http.MethodPost, "/api/plan/revert", `{"first_event_id":5,"last_event_id":2}`, 400},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, body := a.do(tc.method, tc.path, tc.body)
			assert.Equal(t, tc.want, code, body)
		})
	}
}

func TestMealHTTP_DryRunPreviewsWithoutWriting(t *testing.T) {
	a := newMealAPI(t)
	id, sid := a.plateOn("2030-01-07", a.dinner, a.chili)
	code, body := a.do(http.MethodPost, fmt.Sprintf("/api/cook-sessions/%d/leftovers", sid), fmt.Sprintf(`{"cells":[{"date":"2030-01-08","slot_id":%d}]}`, a.dinner))
	require.Less(t, code, 300, body)
	occ, _ := a.plateOn("2030-01-09", a.dinner, a.curry)

	// The cooking meal moved past its leftover moves the cook: the preview
	// lists that and writes nothing.
	code, body = a.do(http.MethodPost, fmt.Sprintf("/api/plates/%d/move?dry_run=1", id), fmt.Sprintf(`{"date":"2030-01-09","slot_id":%d,"occupant":"drop"}`, a.dinner))
	require.Equal(t, http.StatusOK, code, body)
	assert.NotEmpty(t, body["also_changed"])
	code, body = a.do(http.MethodGet, fmt.Sprintf("/api/plates/%d", id), "")
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, "2030-01-07", body["date"])
	code, _ = a.do(http.MethodGet, fmt.Sprintf("/api/plates/%d", occ), "")
	assert.Equal(t, http.StatusOK, code, "the dropped occupant is still there")

	code, body = a.do(http.MethodPost, fmt.Sprintf("/api/plates/%d/replace?dry_run=1", id), fmt.Sprintf(`{"position":"main","food_id":%d}`, a.curry))
	require.Equal(t, http.StatusOK, code, body)
	code, body = a.do(http.MethodGet, fmt.Sprintf("/api/plates/%d", id), "")
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, "Chili", body["components"].([]any)[0].(map[string]any)["food_name"])

	code, body = a.do(http.MethodPost, fmt.Sprintf("/api/plates/%d/move?dry_run=1", id), fmt.Sprintf(`{"date":"2030-01-09","slot_id":%d}`, a.dinner))
	assert.Equal(t, http.StatusConflict, code, "a dry run still reports the refusal: %v", body)
	assert.EqualValues(t, occ, body["occupant_plate_id"])

	code, body = a.do(http.MethodPost, "/api/availability/away?dry_run=1&today=2030-01-01", `{"date":"2030-01-07"}`)
	require.Equal(t, http.StatusOK, code, body)
	assert.NotNil(t, body["removed"])
	code, _ = a.do(http.MethodGet, fmt.Sprintf("/api/plates/%d", id), "")
	assert.Equal(t, http.StatusOK, code, "an away dry run keeps the plate")
	code, body = a.do(http.MethodGet, "/api/availability?from=2030-01-07&to=2030-01-07", "")
	require.Equal(t, http.StatusOK, code)
	for _, c := range body["items"].([]any) {
		assert.Equal(t, "plan", c.(map[string]any)["mode"], "an away dry run writes no override")
	}
}
