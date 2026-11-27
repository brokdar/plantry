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

// TASTE-8, TASTE-10: a likes_/dislikes_ preference is a taste statement that
// applies to the current window until the user taps [Always]; the result
// carries the chip's data. A health statement is stored nowhere without the
// tap. Any other key writes the profile.
func TestTool_RecordPreference_ProposesStatements(t *testing.T) {
	t.Parallel()
	db := testhelper.NewTestDB(t)
	var got statements
	profiles := profile.NewService(sqlite.NewProfileRepo(db))
	ts, err := agent.NewToolSet(agent.Services{
		Foods: food.NewService(sqlite.NewFoodRepo(db)), Plates: testsvc.Plates(db),
		Profile: profiles, Slots: slot.NewService(sqlite.NewSlotRepo(db)),
		Tx: sqlite.NewTxRunner(db), Taste: &got,
	})
	require.NoError(t, err)
	run := func(in string) map[string]any {
		out, _, _, err := ts.Execute(context.Background(), "record_preference", json.RawMessage(in))
		require.NoError(t, err, in)
		var m map[string]any
		require.NoError(t, json.Unmarshal(out, &m))
		return m
	}
	thai := run(`{"key":"likes_thai","value":true}`)
	run(`{"key":"hates_coriander","value":true}`)
	run(`{"key":"likes_spicy","value":false}`)
	for _, in := range []string{`{"key":"allergic_peanuts","value":true}`, `{"key":"celiac","value":true}`, `{"key":"intolerant_to_lactose","value":true}`} {
		m := run(in)
		assert.Equal(t, true, m["waiting_for_tap"], in)
		assert.Equal(t, "health", m["statement"].(map[string]any)["kind"], in)
	}
	run(`{"key":"prefers_quick_meals","value":true}`)

	assert.Len(t, got, 2, "taste statements only")
	for _, s := range got {
		assert.Equal(t, taste.ScopeWindow, s.scope, "no tap: this window only")
	}
	assert.Equal(t, taste.Ref{Level: taste.LevelCuisine, Key: "thai"}, got[0].ref)
	assert.Equal(t, taste.Ref{Level: taste.LevelHero, Key: "coriander"}, got[1].ref)
	assert.Equal(t, map[string]any{"kind": "taste", "level": "cuisine", "key": "thai", "polarity": "love", "window_start": "0001-01-01"}, thai["statement"])

	p, err := profiles.Get(context.Background())
	require.NoError(t, err)
	assert.Equal(t, true, p.Preferences["prefers_quick_meals"])
	assert.NotContains(t, p.Preferences, "allergic_peanuts", "a health statement is stored only with the tap")
	assert.NotContains(t, p.Preferences, "likes_thai")
}
