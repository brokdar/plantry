package handlers_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
)

// B3-AC5 (spec §6.6, D19): generic tags never reach the preferences, and a
// tag moved to the opposite list leaves the original one.
func TestFeedback_PreferenceTags(t *testing.T) {
	a := newHistoryAPI(t)
	plates := a.chiliBatch()
	for _, tag := range []string{"main", "explore", "spicy", "side_veg"} {
		a.insert(fmt.Sprintf(`INSERT INTO food_tags (food_id, tag) VALUES (%d, '%s')`, a.chili, tag))
	}
	prefs := func() (likes, dislikes any) {
		prof, err := profile.NewService(sqlite.NewProfileRepo(a.db)).Get(context.Background())
		require.NoError(t, err)
		return prof.Preferences[profile.PreferenceKeyLikes], prof.Preferences[profile.PreferenceKeyDislikes]
	}

	code, body := a.feedback(plates["2026-10-25"], "loved")
	require.Equal(t, http.StatusOK, code, body)
	likes, dislikes := prefs()
	assert.ElementsMatch(t, []any{"spicy"}, likes, "only the specific tag is liked")
	assert.Nil(t, dislikes)

	code, body = a.feedback(plates["2026-10-26"], "disliked")
	require.Equal(t, http.StatusOK, code, body)
	likes, dislikes = prefs()
	assert.Empty(t, likes, "the tag moved out of likes")
	assert.ElementsMatch(t, []any{"spicy"}, dislikes)
}
