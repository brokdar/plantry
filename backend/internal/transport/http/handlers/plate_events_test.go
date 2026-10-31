package handlers_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain/actor"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

func (a *planAPI) events() []plate.Event {
	a.t.Helper()
	evs, err := sqlite.NewPlateEventRepo(a.db).List(context.Background())
	require.NoError(a.t, err)
	return evs
}

// B3-AC1: POST /api/plates writes create/user with the components in
// after_json.
func TestPlateEventsHTTP_CreateIsUser(t *testing.T) {
	a := newPlanAPI(t)
	plateID, sid := a.sunChili()

	evs := a.events()
	require.Len(t, evs, 2, "the plate and its fresh session")
	for _, ev := range evs {
		assert.Equal(t, plate.ActionCreate, ev.Action)
		assert.Equal(t, actor.User, ev.Actor)
	}
	assert.Nil(t, evs[1].PlateID)
	assert.Equal(t, sid, *evs[1].CookSessionID)
	ev := evs[0]
	assert.Equal(t, actor.User, ev.Actor)
	assert.Nil(t, ev.AIRunID)
	assert.Equal(t, plateID, *ev.PlateID)
	assert.Nil(t, ev.Before)
	var after plate.SnapshotPlate
	require.NoError(t, json.Unmarshal(ev.After, &after))
	require.Len(t, after.Components, 1)
	assert.Equal(t, a.chili, after.Components[0].FoodID)
	assert.Equal(t, sid, after.Components[0].Session.ID)
}

// B3-AC4 over HTTP: POST /api/plan/restore logs restore, not create.
func TestPlateEventsHTTP_RestoreIsRestore(t *testing.T) {
	a := newPlanAPI(t)
	plateID, _ := a.sunChili()
	code, body := a.do(http.MethodDelete, fmt.Sprintf("/api/plates/%d", plateID), "")
	require.Equal(t, http.StatusOK, code, body)
	snapshot, err := json.Marshal(body["removed"])
	require.NoError(t, err)

	code, body = a.do(http.MethodPost, "/api/plan/restore", string(snapshot))
	require.Equal(t, http.StatusOK, code, body)

	// create plate + session, delete plate + session, restore plate +
	// recreated session.
	evs := a.events()
	require.Len(t, evs, 6)
	assert.Equal(t, plate.ActionDelete, evs[2].Action)
	assert.Equal(t, plateID, *evs[2].PlateID)
	restored := evs[4]
	assert.Equal(t, plate.ActionRestore, restored.Action)
	require.NotNil(t, restored.PlateID)
	assert.Equal(t, actor.User, restored.Actor)
	assert.Nil(t, restored.Before)
	assert.Equal(t, plate.ActionCreate, evs[5].Action)
	assert.Nil(t, evs[5].PlateID)
}
