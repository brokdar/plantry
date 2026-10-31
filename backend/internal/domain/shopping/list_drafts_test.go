package shopping_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/shopping"
)

// B2-AC4: drafts stay off the list unless asked for; HasDrafts reports them
// either way.
func TestList_Drafts(t *testing.T) {
	confirmed := chiliSession(7, 4)
	confirmed.State = plate.StateConfirmed
	draft := plate.CookSession{ID: 8, FoodID: sauceID, CookDate: day("2026-11-02"), CookSlotID: dinner, Portions: 2, State: plate.StateDraft}
	draftLeaf := shopping.LeafUse{FoodID: appleID, Date: day("2026-11-02"), SlotID: 1, Eaters: 1, Amount: 1, Unit: "piece", Grams: 150, Draft: true}
	tests := []struct {
		name          string
		sessions      []plate.CookSession
		leaves        []shopping.LeafUse
		includeDrafts bool
		wantFoods     []int64
		wantHasDrafts bool
	}{
		{"without include_drafts", []plate.CookSession{confirmed, draft}, []shopping.LeafUse{draftLeaf}, false, []int64{beansID, onionID}, true},
		{"with include_drafts", []plate.CookSession{confirmed, draft}, []shopping.LeafUse{draftLeaf}, true, []int64{appleID, beansID, onionID, tomatoID}, true},
		{"only a draft leaf plate", []plate.CookSession{confirmed}, []shopping.LeafUse{draftLeaf}, false, []int64{beansID, onionID}, true},
		{"no drafts in the range", []plate.CookSession{confirmed}, nil, true, []int64{beansID, onionID}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := &fakeSource{sessions: tt.sessions, leaves: tt.leaves}
			l, err := newSvc(src, ac1Foods()).List(context.Background(), day("2026-10-31"), day("2026-11-06"), tt.includeDrafts)
			require.NoError(t, err)
			var got []int64
			for _, s := range l.Sections {
				for _, it := range s.Items {
					got = append(got, it.FoodID)
				}
			}
			assert.ElementsMatch(t, tt.wantFoods, got)
			assert.Equal(t, tt.wantHasDrafts, l.HasDrafts)
		})
	}
}
