package handlers_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain/foodevent"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
	transport "github.com/jaltszeimer/plantry/backend/internal/transport/http"
	"github.com/jaltszeimer/plantry/backend/internal/transport/http/handlers"
)

type nopRestorer struct{ restored []int64 }

func (n *nopRestorer) Restore(_ context.Context, id int64, _ json.RawMessage) error {
	n.restored = append(n.restored, id)
	return nil
}

func TestFoodEventHTTP_RevertAndStale(t *testing.T) {
	db := testhelper.NewTestDB(t)
	rest := &nopRestorer{}
	svc := foodevent.NewService(sqlite.NewTxRunner(db), sqlite.NewFoodEventRepo(db),
		map[foodevent.Kind]foodevent.Restorer{foodevent.KindAttribute: rest}, nil)
	router := transport.NewRouter(slog.New(slog.NewTextHandler(io.Discard, nil)), http.NotFoundHandler(),
		transport.Handlers{FoodEvents: handlers.NewFoodEventHandler(svc)})
	u, err := svc.Recorded(context.Background(), func(ctx context.Context) error {
		return svc.Record(ctx, foodevent.Event{Kind: foodevent.KindAttribute, SubjectID: 4, Action: foodevent.ActionUpdate})
	})
	require.NoError(t, err)

	post := func(body string) (int, map[string]any) {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/food-events/revert", strings.NewReader(body)))
		var out map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		return rec.Code, out
	}
	tok := fmt.Sprintf(`{"first_event_id":%d,"last_event_id":%d}`, u.FirstEventID, u.LastEventID)

	code, body := post(tok)
	require.Equal(t, http.StatusOK, code, body)
	assert.Equal(t, []any{map[string]any{"kind": "attribute", "subject_id": float64(4)}}, body["subjects"])
	assert.Equal(t, []int64{4}, rest.restored)

	code, body = post(tok)
	assert.Equal(t, http.StatusConflict, code)
	assert.Equal(t, "error.plan.change_stale", body["message_key"])

	code, _ = post(`{"first_event_id":0,"last_event_id":0}`)
	assert.Equal(t, http.StatusBadRequest, code)
}
