package foodevent_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/actor"
	"github.com/jaltszeimer/plantry/backend/internal/domain/foodevent"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
)

// store is a restorer's subject rows: id -> snapshot.
type store map[int64]json.RawMessage

func (s store) Restore(_ context.Context, id int64, before json.RawMessage) error {
	if before == nil {
		delete(s, id)
		return nil
	}
	s[id] = before
	return nil
}

var allKinds = []foodevent.Kind{foodevent.KindRecipe, foodevent.KindAttribute, foodevent.KindTasteItem, foodevent.KindSubstitution}

type rig struct {
	svc    *foodevent.Service
	stores map[foodevent.Kind]store
}

func newRig(t *testing.T) *rig {
	t.Helper()
	db := testhelper.NewTestDB(t)
	r := &rig{stores: map[foodevent.Kind]store{}}
	restorers := map[foodevent.Kind]foodevent.Restorer{}
	for _, k := range allKinds {
		r.stores[k] = store{}
		restorers[k] = r.stores[k]
	}
	now := time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC)
	r.svc = foodevent.NewService(sqlite.NewTxRunner(db), sqlite.NewFoodEventRepo(db), restorers, func() time.Time { return now })
	return r
}

// write is a producer: it changes the store and records the event.
func (r *rig) write(ctx context.Context, k foodevent.Kind, id int64, action foodevent.Action, after string) error {
	before := r.stores[k][id]
	var a json.RawMessage
	if after != "" {
		a = json.RawMessage(after)
		r.stores[k][id] = a
	} else {
		delete(r.stores[k], id)
	}
	return r.svc.Record(ctx, foodevent.Event{Kind: k, SubjectID: id, Action: action, Before: before, After: a})
}

func TestEventAndUndo_EachKind(t *testing.T) {
	for _, k := range allKinds {
		t.Run(string(k), func(t *testing.T) {
			ctx := actor.With(context.Background(), actor.Agent, 0)
			r := newRig(t)
			require.NoError(t, r.stores[k].Restore(ctx, 5, json.RawMessage(`{"v":1}`)))

			// update then create then delete in one change: one token.
			u, err := r.svc.Recorded(ctx, func(ctx context.Context) error {
				if err := r.write(ctx, k, 5, foodevent.ActionUpdate, `{"v":2}`); err != nil {
					return err
				}
				if err := r.write(ctx, k, 6, foodevent.ActionCreate, `{"v":9}`); err != nil {
					return err
				}
				return r.write(ctx, k, 5, foodevent.ActionDelete, "")
			})
			require.NoError(t, err)
			require.Equal(t, foodevent.Undo{FirstEventID: 1, LastEventID: 3}, u)
			assert.NotContains(t, r.stores[k], int64(5))

			subjects, err := r.svc.Revert(ctx, u)
			require.NoError(t, err)
			assert.Equal(t, []foodevent.Subject{{Kind: k, SubjectID: 5}, {Kind: k, SubjectID: 6}}, subjects)
			assert.JSONEq(t, `{"v":1}`, string(r.stores[k][5]), "the row is back as before the change")
			assert.NotContains(t, r.stores[k], int64(6), "the created row is removed")
		})
	}
}

func TestEvent_RecordsActorAndTime(t *testing.T) {
	r := newRig(t)
	db := testhelper.NewTestDB(t)
	log := sqlite.NewFoodEventRepo(db)
	svc := foodevent.NewService(sqlite.NewTxRunner(db), log, nil, func() time.Time { return time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC) })
	ctx := context.Background()
	require.NoError(t, svc.Record(actor.With(ctx, actor.Agent, 3), foodevent.Event{Kind: foodevent.KindRecipe, SubjectID: 1, Action: foodevent.ActionCreate, After: json.RawMessage(`{"a":1}`)}))
	got, err := log.Range(ctx, 1, 1)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, actor.Agent, got[0].Actor)
	assert.Equal(t, 2026, got[0].At.Year())
	assert.Nil(t, got[0].Before)
	assert.JSONEq(t, `{"a":1}`, string(got[0].After))
	assert.ErrorIs(t, r.svc.Record(ctx, foodevent.Event{Kind: "bogus"}), domain.ErrInvalidInput)
}

func TestRevert_RefusedWhenALaterChangeTouchedTheRow(t *testing.T) {
	ctx := context.Background()
	for _, k := range allKinds {
		t.Run(string(k), func(t *testing.T) {
			r := newRig(t)
			require.NoError(t, r.stores[k].Restore(ctx, 5, json.RawMessage(`{"v":1}`)))
			first, err := r.svc.Recorded(ctx, func(ctx context.Context) error { return r.write(ctx, k, 5, foodevent.ActionUpdate, `{"v":2}`) })
			require.NoError(t, err)
			// A later change on another row does not block; on the same row it does.
			_, err = r.svc.Recorded(ctx, func(ctx context.Context) error { return r.write(ctx, k, 7, foodevent.ActionCreate, `{"v":0}`) })
			require.NoError(t, err)
			_, err = r.svc.Recorded(ctx, func(ctx context.Context) error { return r.write(ctx, k, 5, foodevent.ActionUpdate, `{"v":3}`) })
			require.NoError(t, err)

			_, err = r.svc.Revert(ctx, first)
			require.ErrorIs(t, err, domain.ErrPlanChangeStale)
			assert.JSONEq(t, `{"v":3}`, string(r.stores[k][5]), "nothing was written")
		})
	}
}

func TestRevert_SecondRevertOfTheSameTokenIsRefused(t *testing.T) {
	ctx := context.Background()
	r := newRig(t)
	u, err := r.svc.Recorded(ctx, func(ctx context.Context) error {
		return r.write(ctx, foodevent.KindRecipe, 1, foodevent.ActionCreate, `{"v":1}`)
	})
	require.NoError(t, err)
	_, err = r.svc.Revert(ctx, u)
	require.NoError(t, err)
	_, err = r.svc.Revert(ctx, u)
	assert.ErrorIs(t, err, domain.ErrPlanChangeStale)
}

func TestRevert_RefusalRollsBackEarlierRestores(t *testing.T) {
	ctx := context.Background()
	r := newRig(t)
	// Rows 1 and 2 in one change; row 1 is then changed again, so the whole
	// token is refused and row 2 stays.
	u, err := r.svc.Recorded(ctx, func(ctx context.Context) error {
		if err := r.write(ctx, foodevent.KindRecipe, 1, foodevent.ActionCreate, `{"v":1}`); err != nil {
			return err
		}
		return r.write(ctx, foodevent.KindRecipe, 2, foodevent.ActionCreate, `{"v":1}`)
	})
	require.NoError(t, err)
	_, err = r.svc.Recorded(ctx, func(ctx context.Context) error {
		return r.write(ctx, foodevent.KindRecipe, 1, foodevent.ActionUpdate, `{"v":2}`)
	})
	require.NoError(t, err)
	_, err = r.svc.Revert(ctx, u)
	require.ErrorIs(t, err, domain.ErrPlanChangeStale)
	assert.Contains(t, r.stores[foodevent.KindRecipe], int64(2))
}

func TestRevert_InvalidRanges(t *testing.T) {
	ctx := context.Background()
	r := newRig(t)
	for _, u := range []foodevent.Undo{{}, {FirstEventID: 3, LastEventID: 2}, {FirstEventID: 4, LastEventID: 9}} {
		_, err := r.svc.Revert(ctx, u)
		assert.True(t, errors.Is(err, domain.ErrInvalidInput), "%+v: %v", u, err)
	}
	// A kind without a Restorer is refused, nothing written.
	db := testhelper.NewTestDB(t)
	svc := foodevent.NewService(sqlite.NewTxRunner(db), sqlite.NewFoodEventRepo(db), nil, nil)
	u, err := svc.Recorded(ctx, func(ctx context.Context) error {
		return svc.Record(ctx, foodevent.Event{Kind: foodevent.KindTasteItem, SubjectID: 1, Action: foodevent.ActionCreate})
	})
	require.NoError(t, err)
	_, err = svc.Revert(ctx, u)
	assert.ErrorIs(t, err, domain.ErrInvalidInput)
}

func TestRecorded_NoEventsIsAnEmptyToken(t *testing.T) {
	r := newRig(t)
	u, err := r.svc.Recorded(context.Background(), func(context.Context) error { return nil })
	require.NoError(t, err)
	assert.Equal(t, foodevent.Undo{}, u)
}
