package planning_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/airun"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
)

// KR7 (v1.2): a day replan that ends with the same main at both meals saves
// nothing only where H8 is triggered. With a library of one main H8 is relaxed
// (variety_relaxed), so the same dish twice is saved.
func TestKR7_SameMainTwiceIsSavedWhereVarietyIsRelaxed(t *testing.T) {
	e := newSvcEnv(t)
	e.exec(`DELETE FROM foods WHERE name IN ('Vollkornbrot', 'Gouda Käse', 'Gurke')`)
	for _, id := range e.mains[1:] {
		e.exec(`UPDATE foods SET role = 'side_veg' WHERE id = ?`, id)
	}
	date := "2026-11-10"
	id := e.regenerate(planning.Scope{Kind: planning.ScopeDay, Date: day(date)})

	end := e.runs.end(id)

	assert.NotEqual(t, airun.StatusError, end.Status, "notes %q key %q", end.Notes, end.ErrorKey)
}

// sameMainStub is the heuristic with the replanned day made to show one main at
// both meals, from one cooking.
type sameMainStub struct {
	date string
	dish int64
}

func (r sameMainStub) Choose(ctx context.Context, in planning.ChooseInput) (planning.Proposal, error) {
	p, err := planning.Heuristic{}.Choose(ctx, in)
	if err != nil {
		return p, err
	}
	var key string
	first := true
	for i := range p.Plates {
		pl := &p.Plates[i]
		if pl.Date.Format("2006-01-02") != r.date {
			continue
		}
		for j := range pl.Components {
			c := &pl.Components[j]
			if f := in.State.Foods[c.FoodID]; f.Kind != food.KindComposed || f.Role != food.RoleMain {
				continue
			}
			if first {
				first, key = false, c.SessionKey
				c.FoodID = r.dish
				for k := range p.Sessions {
					if p.Sessions[k].Key == key {
						p.Sessions[k].FoodID = r.dish
						p.Sessions[k].Portions *= 2
					}
				}
			} else {
				c.FoodID, c.SessionKey = r.dish, key
			}
		}
	}
	return p, nil
}

// KR7 (v1.2), the refusal end: a day replan whose same-main pair triggers H8
// (an open variety_repeat for that main, not relaxed) saves nothing and says
// so. Nine of the ten mains were eaten inside the variety window and the tenth
// is out of reach of the repair, so no other main fits and the pool is not
// small enough for variety_relaxed. (Coverage: passes with or without fba818f.)
func TestKR7_SameMainThatTriggersH8SavesNothing(t *testing.T) {
	e := newSvcEnv(t)
	e.exec(`DELETE FROM foods WHERE name IN ('Vollkornbrot', 'Gouda Käse', 'Gurke')`)
	for i, d := range []string{"2026-10-22", "2026-10-23", "2026-10-24", "2026-10-26", "2026-10-27", "2026-10-29"} {
		e.userPlate(d, e.lunch, e.mains[3+i])
	}
	// The tenth main is cooked (confirmed) on both neighbouring days: the repair
	// may not cook it on the 10th too (KR7), yet it keeps the pool above the
	// day's cooks.
	e.userPlate("2026-11-09", e.dinner, e.mains[9])
	e.userPlate("2026-11-11", e.dinner, e.mains[9])
	e.deps.Choosers = map[airun.Chooser]planning.Chooser{airun.ChooserHeuristic: sameMainStub{date: "2026-11-10", dish: e.mains[0]}}
	e.svc = planning.NewService(e.deps)
	before := len(e.window())

	req := e.req()
	req.Scope = planning.Scope{Kind: planning.ScopeDay, Date: day("2026-11-10")}
	req.Chooser = airun.ChooserHeuristic
	id, err := e.svc.Regenerate(e.ctx, req)
	require.NoError(t, err)
	end := e.runs.end(id)

	assert.Equal(t, airun.StatusError, end.Status, "notes %q", end.Notes)
	assert.Equal(t, "error.plan.day_same_dish", end.ErrorKey, "notes %q", end.Notes)
	assert.Len(t, e.window(), before, "nothing was saved")
}
