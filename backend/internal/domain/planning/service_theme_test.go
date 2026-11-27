package planning_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain/flavor"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/domain/theme"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper/testsvc"
)

// AC-14: a theme set mid-week on the next window shapes 40-60 % of that
// window's cook sessions where the library and finds supply them, and ends
// with the window. Three of the seven plannable library mains are Japanese,
// the ones the planner would pick last, and a scripted ready find fills the
// new dish.
func TestGenerate_ThemeShapesNextWindow(t *testing.T) {
	e := newFindsEnv(t, true)
	attrs := sqlite.NewFoodAttributeRepo(e.db)
	japanese := map[int64]bool{}
	for _, id := range []int64{e.mains[7], e.mains[8], e.mains[9]} {
		japanese[id] = true
		require.NoError(t, attrs.Put(e.ctx, flavor.Record{
			FoodID: id, Version: 1, Values: flavor.Attributes{Cuisine: "japanese"},
			Sources: map[flavor.Field]flavor.Source{flavor.FieldCuisine: flavor.SourceUser},
		}))
	}
	e.readyFind("Dal")

	themes := theme.NewService(theme.Deps{
		Store:   sqlite.NewThemeRepo(e.db),
		ChefOn:  func(context.Context) (bool, error) { return true, nil },
		Library: func(context.Context, theme.Theme) (int, error) { return 3, nil },
		Now:     func() time.Time { return day(svcToday) }, // mid-week, before the window
	})
	_, avail := testsvc.Plan(e.db)
	bd := testsvc.BuilderDeps(e.db, avail)
	bd.Themes = themes.Themes
	e.deps.Builder = planning.NewBuilder(bd)
	e.deps.ThemeDishes = themes.NewDishes
	e.svc = planning.NewService(e.deps)

	share := func() (int, int) {
		rows, err := e.db.Query(`SELECT s.food_id FROM cook_sessions s JOIN foods f ON f.id = s.food_id
			WHERE f.role = 'main' AND s.cook_date BETWEEN ? AND ?`,
			e.start.Format(time.DateOnly), e.start.AddDate(0, 0, 6).Format(time.DateOnly))
		require.NoError(t, err)
		defer rows.Close()
		themed, all := 0, 0
		for rows.Next() {
			var id int64
			require.NoError(t, rows.Scan(&id))
			all++
			if japanese[id] {
				themed++
			}
		}
		require.NoError(t, rows.Err())
		return themed, all
	}

	e.generate(e.req())
	without, all := share()
	require.GreaterOrEqual(t, all, 4)
	require.Less(t, float64(without)/float64(all), 0.4, "without the theme")
	_, err := e.plates.Discard(e.ctx, e.start, e.start.AddDate(0, 0, 6))
	require.NoError(t, err)

	_, err = themes.Set(e.ctx, e.start, theme.Theme{Kind: theme.KindCuisine, Value: "Japanese", Source: theme.SourceUser})
	require.NoError(t, err)
	e.generate(e.req())
	themed, all := share()
	pct := float64(themed) / float64(all)
	assert.GreaterOrEqualf(t, pct, 0.4, "themed %d of %d cook sessions (without a theme %d)", themed, all, without)
	assert.LessOrEqualf(t, pct, 0.6, "themed %d of %d cook sessions", themed, all)

	next, err := themes.Themes(e.ctx, e.start.AddDate(0, 0, 7))
	require.NoError(t, err)
	assert.Empty(t, next, "the theme ends with its window")
}
