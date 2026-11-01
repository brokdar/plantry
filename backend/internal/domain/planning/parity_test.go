package planning_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/actor"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper/testsvc"
)

// boardRules keeps the issues the board shows, as planview does: H2, H5,
// H8, and H1 (missing_plate only from today on, the window holding drafts).
func boardRules(issues []planning.Issue, today string) []planning.Issue {
	out := []planning.Issue{}
	for _, is := range issues {
		switch is.Rule {
		case "H1":
			if is.Code == planning.CodeMissingPlate && is.Date < today {
				continue
			}
		case "H2", "H5", "H8":
		default:
			continue
		}
		out = append(out, is)
	}
	return out
}

// The board (planview) and the generator's context builder build the same
// state from the same rows: Check finds the same issues over both. The
// fixture holds an H1, an H2, H5s and an H8.
func TestStateParity_BoardAndBuilder(t *testing.T) {
	for _, dietType := range []string{"omnivore", "vegetarian"} {
		t.Run(dietType, func(t *testing.T) {
			e := newSvcEnv(t)
			e.ok(e.generate(e.req()))
			e.exec(`UPDATE user_profile SET diet_type = ? WHERE id = 1`, dietType)
			w := e.window()
			// H1: an empty plan cell in a draft window.
			_, err := e.plates.Delete(e.ctx, w[key(tue, e.lunch)].ID)
			require.NoError(t, err)
			// H8 and H2: a draft cooks a main eaten last week at lunch on a
			// day whose dinner cooks.
			cookDay := ""
			for _, d := range []string{sun, mon, wed, "2026-11-12", fri} {
				p := w[key(d, e.dinner)]
				if v, err := e.plates.Session(e.ctx, *p.Components[0].CookSessionID); err == nil && v.CookCell().Equal(p.Cell()) {
					cookDay = d
					break
				}
			}
			require.NotEmpty(t, cookDay, "the heuristic cooks some dinner")
			_, err = e.plates.Delete(e.ctx, w[key(cookDay, e.lunch)].ID)
			require.NoError(t, err)
			one := 1.0
			_, err = e.plates.Create(actor.With(e.ctx, actor.Agent, 0), plate.PlateInput{
				Date: day(cookDay), SlotID: e.lunch, Components: []plate.PlateComponent{{FoodID: e.mains[0], Portions: &one}},
			})
			require.NoError(t, err)

			start, today := day(sat), day(svcToday)
			board, err := testsvc.PlanView(e.db).Plan(e.ctx, &start, today)
			require.NoError(t, err)

			var built []planning.Issue
			require.NoError(t, e.deps.Tx.InTx(e.ctx, func(ctx context.Context) error {
				st, err := e.deps.Builder.Build(ctx, board.Window, today, board.Window.Start, board.Window.End)
				if err != nil {
					return err
				}
				var tgt planning.Target
				for _, c := range st.Cells {
					tgt.Cells = append(tgt.Cells, c.Key())
				}
				built = boardRules(planning.Check(st, tgt), svcToday)
				return nil
			}))

			assert.ElementsMatch(t, built, board.Issues)
			rules := map[string]bool{}
			for _, is := range board.Issues {
				rules[is.Rule] = true
			}
			want := []string{"H1", "H2", "H8"}
			if dietType == "vegetarian" {
				want = append(want, "H5")
			}
			for _, r := range want {
				assert.True(t, rules[r], "the fixture has an %s issue: %v", r, board.Issues)
			}
		})
	}
}
