package finds_test

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/finds"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
)

// CP3b item 2: an autonomous find never brings a dish the library already
// has (every recorded curry is "Chicken Curry"); a requested one may.
func TestPipeline_PlanningFindSkipsLibraryDishes(t *testing.T) {
	t.Parallel()
	r := newRig(t, true, lineMatcher{}, kitchen, searchTurn(curryURLs...))
	r.house.Recipes = []string{"chicken  curry"}
	f := r.run(t, finds.TriggerPlanning)
	require.Equal(t, finds.StateFailed, f.State)
	assert.Equal(t, "nothing_found", f.Steps[len(f.Steps)-1].Key, f.Steps)

	asked := newRig(t, true, lineMatcher{}, kitchen, searchTurn(curryURLs...), adaptTurn(false, fitsKitchen))
	asked.house.Recipes = []string{"Chicken Curry"}
	assert.Equal(t, finds.StateReady, asked.run(t, finds.TriggerChat).State)
}

// racingMatcher stores a food from url while the find matches, as a find
// running alongside that kept the same page first.
type racingMatcher struct {
	lineMatcher
	db  *sql.DB
	url string
}

func (m *racingMatcher) Match(ctx context.Context, req food.MatchRequest) (*food.MatchResult, error) {
	if _, err := m.db.ExecContext(ctx, `INSERT INTO foods (name, kind, role, reference_portions, source_url) VALUES ('Curry', 'composed', 'main', 2, ?)`, m.url); err != nil {
		return nil, err
	}
	return m.lineMatcher.Match(ctx, req)
}

// searchingDishes lists the dishes of the searching finds.
func searchingDishes(t *testing.T, r *rig) []string {
	t.Helper()
	state := finds.StateSearching
	list, err := r.store.List(context.Background(), &state, 50)
	require.NoError(t, err)
	var out []string
	for _, f := range list {
		out = append(out, f.Dish)
	}
	return out
}

// CP3b items 3 and 4: a find whose page another find kept first fails as a
// duplicate (normalised URL), and an autonomous failure tries another dish
// (GATE-4), at most twice per session length.
func TestRun_DuplicatePageFailsAndTriesAnotherDish(t *testing.T) {
	t.Parallel()
	m := &racingMatcher{url: curryURLs[0] + "?utm_source=x"}
	r := newRig(t, true, m, kitchen, searchTurn(curryURLs...), adaptTurn(false, fitsKitchen))
	m.db = r.db
	f := r.run(t, finds.TriggerPlanning)
	require.Equal(t, finds.StateFailed, f.State)
	assert.Equal(t, "duplicate", f.Steps[len(f.Steps)-1].Key)
	assert.Equal(t, []string{"chicken curry one-pot main"}, searchingDishes(t, r), "the next dish is tried")

	ctx := context.Background()
	capped := newRig(t, true, lineMatcher{}, kitchen, searchTurn(curryURLs...))
	capped.house.Recipes = []string{"Chicken Curry"}
	for range 2 {
		g, err := capped.store.Create(ctx, "thai main dish", finds.TriggerPlanning)
		require.NoError(t, err)
		_, err = capped.store.Finish(ctx, g.ID, finds.Outcome{State: finds.StateFailed, Steps: []finds.Step{{Key: "searched"}}})
		require.NoError(t, err)
	}
	require.Equal(t, finds.StateFailed, capped.run(t, finds.TriggerPlanning).State)
	assert.Empty(t, searchingDishes(t, capped), "two failures in this session already: no further dish")
}

// CP4: a find that nearly duplicates a library dish (filler words ignored)
// is dropped before adapting; a requested find may repeat a dish.
func TestPipeline_TooSimilarToTheLibraryIsRejectedBeforeAdapting(t *testing.T) {
	t.Parallel()
	for _, trigger := range []finds.Trigger{finds.TriggerPlanning, finds.TriggerFill} {
		r := newRig(t, true, lineMatcher{}, kitchen, searchTurn(curryURLs...), adaptTurn(false, fitsKitchen))
		r.house.Recipes = []string{"Easy chicken curry with rice"}
		f := r.run(t, trigger)
		require.Equal(t, finds.StateFailed, f.State, trigger)
		assert.Equal(t, "too_similar", f.Steps[len(f.Steps)-1].Key, f.Steps)
		assert.Len(t, r.client.Requests(), 1, "search only, no adapt call")
	}
	asked := newRig(t, true, lineMatcher{}, kitchen, searchTurn(curryURLs...), adaptTurn(false, fitsKitchen))
	asked.house.Recipes = []string{"Easy chicken curry with rice"}
	assert.Equal(t, finds.StateReady, asked.run(t, finds.TriggerChat).State)
}

// CP4: an adaptation that replaced the chicken but left "brown the chicken"
// in a step never reaches the library (step_mentions_removed blocks).
func TestPipeline_StepMentioningReplacedIngredientBlocks(t *testing.T) {
	t.Parallel()
	r := newRig(t, true, lineMatcher{}, kitchen, searchTurn(curryURLs...), toolTurn(finds.AdaptTool,
		`{"name":"Mild curry","steps":["Brown the chicken.","Simmer 20 minutes."],"swaps":[{"line":0,"to":"600 g firm tofu","reason":"vegetarian","protein":true}],"requirements":`+fitsKitchen+`}`))
	f := r.run(t, finds.TriggerPlanning)
	require.Equal(t, finds.StateFailed, f.State)
	var failed bool
	for _, s := range f.Steps {
		failed = failed || (s.Key == "checks_failed" && strings.Contains(s.Params["codes"], "step_mentions_removed"))
	}
	assert.True(t, failed, f.Steps)
}
