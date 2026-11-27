package handlers_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain/finds"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
	"github.com/jaltszeimer/plantry/backend/internal/transport/http/handlers"
)

type noPlates struct{}

func (noPlates) Range(context.Context, time.Time, time.Time) ([]plate.Plate, error) { return nil, nil }

// onDraft is a Library whose recipes all sit on a draft plate.
type onDraft struct{ finds.Library }

func (onDraft) OnDraft(context.Context, int64, time.Time) (bool, error) { return true, nil }

// PRO-4, PRO-5: the assistant's list and the window's tips stay within five
// unasked lines; the rest is counted. The next window opens a planning
// session; with the Chef off the list is empty and nothing opens.
func TestChefPlanHandler(t *testing.T) {
	t.Parallel()
	db := testhelper.NewTestDB(t)
	ctx := context.Background()
	store := sqlite.NewFindRepo(db)
	start := time.Date(2026, 11, 7, 0, 0, 0, 0, time.UTC)
	for i := range 4 {
		res, err := db.Exec(`INSERT INTO foods (name, kind, role, reference_portions) VALUES (?, 'composed', 'main', 2)`, fmt.Sprintf("Find %d", i))
		require.NoError(t, err)
		foodID, err := res.LastInsertId()
		require.NoError(t, err)
		f, err := store.Create(ctx, fmt.Sprintf("dish %d", i), finds.TriggerPlanning)
		require.NoError(t, err)
		_, err = store.Finish(ctx, f.ID, finds.Outcome{State: finds.StateReady})
		require.NoError(t, err)
		_, err = store.Place(ctx, f.ID, foodID, start)
		require.NoError(t, err)
	}
	on, suggested := true, false
	chefOn := func(context.Context) (bool, error) { return on, nil }
	now := time.Date(2026, 11, 2, 18, 0, 0, 0, time.UTC)
	svc := finds.NewService(finds.Deps{Store: store, Planning: finds.Planning{
		Library: onDraft{},
		ChefOn:  chefOn, Now: func() time.Time { return now },
		NewDishes: func(context.Context) (int, error) { return 0, nil },
	}})
	h := handlers.NewChefPlanHandler(handlers.ChefPlanDeps{
		Finds: svc, ChefOn: chefOn, Plates: noPlates{},
		ShoppingDay: func(context.Context) (int, error) { return 5, nil }, // Saturday
		LiveTips:    func(context.Context, time.Time) (int, error) { return 3, nil },
		Suggested:   func(context.Context, time.Time) (bool, error) { return suggested, nil },
		Now:         func() time.Time { return now },
	})
	r := chi.NewRouter()
	r.Post("/api/plan/opened", h.Opened)
	r.Get("/api/plan/assistant", h.Assistant)
	do := func(method, path, body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(method, path, strings.NewReader(body)))
		return w
	}
	var list struct {
		Items []finds.Activity `json:"items"`
		More  int              `json:"more"`
	}

	w := do(http.MethodGet, "/api/plan/assistant?start=2026-11-07", "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &list))
	assert.Len(t, list.Items, 2, "3 tips + 2 lines = the budget of 5")
	assert.Equal(t, 2, list.More)
	suggested = true // THEME-4: a theme suggestion counts in the budget
	w = do(http.MethodGet, "/api/plan/assistant?start=2026-11-07", "")
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &list))
	assert.Len(t, list.Items, 1, "3 tips + a suggestion + 1 line")
	assert.Equal(t, 3, list.More)

	// Today Mon 11-02: the next window starts Sat 11-07.
	w = do(http.MethodPost, "/api/plan/opened", `{"start":"2026-10-31","today":"2026-11-02"}`)
	require.Equal(t, http.StatusNoContent, w.Code, w.Body.String())
	open, err := svc.InSession(ctx)
	require.NoError(t, err)
	assert.False(t, open, "the current window without drafts opens no session")
	w = do(http.MethodPost, "/api/plan/opened", `{"start":"2026-11-07","today":"2026-11-02"}`)
	require.Equal(t, http.StatusNoContent, w.Code, w.Body.String())
	open, err = svc.InSession(ctx)
	require.NoError(t, err)
	assert.True(t, open, "the next window opens a session")

	on = false
	w = do(http.MethodGet, "/api/plan/assistant?start=2026-11-07", "")
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &list))
	assert.Empty(t, list.Items)
}
