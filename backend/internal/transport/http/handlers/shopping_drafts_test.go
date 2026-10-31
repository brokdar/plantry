package handlers_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

// B2-AC4: a confirmed Chili batch plus a draft Chili session and a draft
// apple plate in the same range.
func TestShoppingList_Drafts(t *testing.T) {
	e := newShopEnv(t)
	ctx := context.Background()
	confirmed := e.chiliBatch(t)

	one, piece := 1.0, "piece"
	apple, err := e.plates.Create(ctx, plate.PlateInput{
		Date: day(t, "2026-11-04"), SlotID: e.slotID, Eaters: &one,
		Components: []plate.PlateComponent{{FoodID: e.apple.ID, Amount: &one, Unit: &piece}},
	})
	require.NoError(t, err)
	chili, err := e.plates.Create(ctx, plate.PlateInput{
		Date: day(t, "2026-11-05"), SlotID: e.slotID, Eaters: &one,
		Components: []plate.PlateComponent{{FoodID: e.chili.ID, Portions: &one}},
	})
	require.NoError(t, err)
	_, err = e.plates.Unlock(ctx, plate.RowSet{PlateIDs: []int64{apple.ID, chili.ID}})
	require.NoError(t, err)
	draft := *chili.Components[0].CookSessionID

	sessionsOf := func(b shopBody, foodID int64) []int64 {
		out := []int64{}
		for _, s := range b.Sections {
			for _, it := range s.Items {
				if it.FoodID != foodID {
					continue
				}
				for _, u := range it.UsedIn {
					if u.SessionID != nil {
						out = append(out, *u.SessionID)
					}
				}
			}
		}
		return out
	}

	tests := []struct {
		name         string
		query        string
		wantSessions []int64
		wantApple    bool
	}{
		{"confirmed rows only by default", "?from=2026-11-01&to=2026-11-07", []int64{confirmed}, false},
		{"include_drafts=false", "?from=2026-11-01&to=2026-11-07&include_drafts=false", []int64{confirmed}, false},
		{"include_drafts=true", "?from=2026-11-01&to=2026-11-07&include_drafts=true", []int64{confirmed, draft}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, b, raw := e.get(t, tt.query)
			require.Equal(t, http.StatusOK, code, raw)
			assert.True(t, b.HasDrafts)
			assert.Equal(t, tt.wantSessions, sessionsOf(b, e.beans.ID))
			hasApple := false
			for _, s := range b.Sections {
				for _, it := range s.Items {
					hasApple = hasApple || it.FoodID == e.apple.ID
				}
			}
			assert.Equal(t, tt.wantApple, hasApple, raw)
		})
	}

	t.Run("a range without drafts", func(t *testing.T) {
		code, b, raw := e.get(t, "?from=2026-11-01&to=2026-11-03&include_drafts=true")
		require.Equal(t, http.StatusOK, code, raw)
		assert.False(t, b.HasDrafts)
	})
}
