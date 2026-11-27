package discovery_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain/finds"
)

// FIND-2: with the Chef on the daily idea is a ready find and never searches;
// without one it does not appear that day.
func TestDaily_ChefOnShowsReadyFindsOnly(t *testing.T) {
	site := recipeSite(t, "r1")
	client := &batchClient{site: site, batches: [][]string{{"r1"}}}
	h := newHarness(t, client, site)
	*h.chef = true
	ctx := context.Background()

	none := h.daily(t, dayD)
	assert.Nil(t, none.Suggestion, "no ready find: no idea that day")
	assert.Zero(t, none.RunID, "the daily idea never searches")

	store := sqlite.NewFindRepo(h.db)
	f, err := store.Create(ctx, "curry", finds.TriggerChat)
	require.NoError(t, err)
	_, err = store.Finish(ctx, f.ID, finds.Outcome{State: finds.StateReady, SourceURL: "https://example.org/curry"})
	require.NoError(t, err)

	got := h.daily(t, dayD1)
	require.NotNil(t, got.Suggestion)
	assert.Equal(t, f.ID, got.Suggestion.ID, "the ready find is the idea")
	assert.Equal(t, got.Suggestion.ID, h.daily(t, dayD1).Suggestion.ID, "the same find all day")
	assert.Nil(t, h.daily(t, dayD1.AddDate(0, 0, 1)).Suggestion, "shown once; nothing new: no idea")
	assert.Zero(t, client.Calls(), "no model call")
}
