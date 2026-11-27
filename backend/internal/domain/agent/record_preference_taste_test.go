package agent_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain/agent"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/foodevent"
	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
	"github.com/jaltszeimer/plantry/backend/internal/domain/slot"
	"github.com/jaltszeimer/plantry/backend/internal/domain/taste"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper/testsvc"
)

type statement struct {
	ref      taste.Ref
	polarity taste.Polarity
	scope    taste.Scope
}

type statements []statement

func (s *statements) State(_ context.Context, ref taste.Ref, p taste.Polarity, scope taste.Scope, _ time.Time) (taste.Item, foodevent.Undo, error) {
	*s = append(*s, statement{ref, p, scope})
	return taste.Item{}, foodevent.Undo{}, nil
}

// TASTE-10: record_preference writes a likes_/dislikes_ preference as a
// taste statement (source statement, scope always) besides the profile; any
// other key writes the profile only.
func TestTool_RecordPreference_WritesTasteItems(t *testing.T) {
	t.Parallel()
	db := testhelper.NewTestDB(t)
	var got statements
	ts, err := agent.NewToolSet(agent.Services{
		Foods: food.NewService(sqlite.NewFoodRepo(db)), Plates: testsvc.Plates(db),
		Profile: profile.NewService(sqlite.NewProfileRepo(db)), Slots: slot.NewService(sqlite.NewSlotRepo(db)),
		Tx: sqlite.NewTxRunner(db), Taste: &got,
	})
	require.NoError(t, err)
	for _, in := range []string{
		`{"key":"likes_thai","value":true}`,
		`{"key":"hates_coriander","value":true}`,
		`{"key":"likes_spicy","value":false}`,
		`{"key":"prefers_quick_meals","value":true}`,
	} {
		_, _, _, err := ts.Execute(context.Background(), "record_preference", json.RawMessage(in))
		require.NoError(t, err, in)
	}
	assert.Equal(t, statements{
		{taste.Ref{Level: taste.LevelCuisine, Key: "thai"}, taste.Love, taste.ScopeAlways},
		{taste.Ref{Level: taste.LevelHero, Key: "coriander"}, taste.RatherNot, taste.ScopeAlways},
	}, got)
}
