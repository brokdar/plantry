package planning_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/actor"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/window"
)

// B7-AC1: three user replacements of a recipe on weekday dinners in the
// last 4 weeks reach the prompt by name; an older one does not.
func TestBuilder_CorrectionsInPrompt(t *testing.T) {
	e := newSvcEnv(t)
	one := 1.0
	fish, other := e.mains[0], e.mains[1]
	swap := func(date string) {
		p, err := e.plates.Create(actor.With(e.ctx, actor.Agent, 0), plate.PlateInput{
			Date: day(date), SlotID: e.dinner, Components: []plate.PlateComponent{{FoodID: fish, Portions: &one}},
		})
		require.NoError(t, err)
		_, err = e.plates.SwapComponent(actor.With(e.ctx, actor.User, 0), p.Components[0].ID, other, nil)
		require.NoError(t, err)
	}
	for _, d := range []string{"2026-10-05", "2026-10-13", "2026-10-21", "2026-10-29"} { // Mon, Tue, Wed, Thu
		swap(d)
	}

	var st planning.State
	require.NoError(t, e.deps.Tx.InTx(e.ctx, func(ctx context.Context) error {
		var err error
		w := day(sat)
		st, err = e.deps.Builder.Build(ctx, window.Window{Start: w, End: w.AddDate(0, 0, 6)}, day(svcToday), w, w.AddDate(0, 0, 6))
		return err
	}))
	require.Len(t, st.Corrections, 3, "the 2026-10-05 swap is older than 4 weeks")

	prompt := planning.RenderPrompt(planning.ChooseInput{State: st, Target: planning.Target{}})
	name := st.Foods[fish].Name
	_, section, found := strings.Cut(prompt, "The user replaced")
	require.True(t, found)
	assert.Equal(t, 3, strings.Count(section, name+" -> "+st.Foods[other].Name))
	assert.NotContains(t, section, "2026-10-05")
}

func TestRenderPrompt_NoCorrectionsNoSection(t *testing.T) {
	prompt := planning.RenderPrompt(planning.ChooseInput{State: baseState(), Target: windowTarget(baseState())})
	assert.NotContains(t, prompt, "The user replaced")
}
