package plate_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain/actor"
	"github.com/jaltszeimer/plantry/backend/internal/domain/airun"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

// Phase 4 B6: run accounting for chat runs, edits and corrections.

// B6-AC1: a chat run that only edited a generated draft session follows the
// session's accept or discard; the generator run behaves as before.
func TestOutcome_ChatEditOfGeneratedDraft(t *testing.T) {
	setup := func(t *testing.T) *env {
		e := newEnv(t)
		gen := e.run(actor.Generator, 7)
		p, err := e.svc.Create(gen, plate.PlateInput{Date: day(mon), SlotID: e.dinner, Components: []plate.PlateComponent{composed(e.chili, 1)}})
		require.NoError(t, err)
		agent := e.run(actor.Agent, 8)
		_, err = e.svc.SetSessionPortions(agent, sessionOf(p, 0), plate.SessionPortions{Portions: 6})
		require.NoError(t, err)
		return e
	}
	t.Run("accept", func(t *testing.T) {
		e := setup(t)
		rows, err := e.svc.Accept(e.ctx, day(mon), day(nextSun))
		require.NoError(t, err)
		assert.Equal(t, "accepted", e.outcome(8))
		assert.Equal(t, "accepted", e.outcome(7))
		_, err = e.svc.Unaccept(e.ctx, rows)
		require.NoError(t, err)
		assert.Equal(t, "open", e.outcome(8))
		assert.Equal(t, "open", e.outcome(7))
	})
	t.Run("discard", func(t *testing.T) {
		e := setup(t)
		_, err := e.svc.Discard(e.ctx, day(mon), day(nextSun))
		require.NoError(t, err)
		assert.Equal(t, "discarded", e.outcome(8))
		assert.Equal(t, "discarded", e.outcome(7))
	})
	t.Run("an agent run that edited nothing stays untouched", func(t *testing.T) {
		e := setup(t)
		e.run(actor.Agent, 9)
		_, err := e.svc.Accept(e.ctx, day(mon), day(nextSun))
		require.NoError(t, err)
		assert.Empty(t, e.outcome(9))
	})
}

// B6-AC2: edits count the plates and sessions an agent run changed.
func TestRunEdits(t *testing.T) {
	e := newEnv(t)
	gen := e.run(actor.Generator, 7)
	p, err := e.svc.Create(gen, plate.PlateInput{Date: day(mon), SlotID: e.dinner, Components: []plate.PlateComponent{composed(e.chili, 1)}})
	require.NoError(t, err)

	// Run 8 edits one session: its linked plate's event does not count again.
	_, err = e.svc.SetSessionPortions(e.run(actor.Agent, 8), sessionOf(p, 0), plate.SessionPortions{Portions: 6})
	require.NoError(t, err)
	// Run 9 creates a plate with a new session: a plate and a session.
	_, err = e.svc.Create(e.run(actor.Agent, 9), plate.PlateInput{Date: day(tue), SlotID: e.dinner, Components: []plate.PlateComponent{composed(e.curry, 1)}})
	require.NoError(t, err)

	got, err := sqlite.NewAIRunRepo(e.db).Edits(e.ctx, 7, 9)
	require.NoError(t, err)
	assert.Equal(t, map[int64]int{8: 1, 9: 2}, got, "the generator run (7) has none")
}

// B6-AC3: corrections of agent-created plates and of generated plates edited
// by the agent.
func TestRunCorrections_Agent(t *testing.T) {
	e := newEnv(t)
	gen := e.run(actor.Generator, 7)
	generated, err := e.svc.Create(gen, plate.PlateInput{Date: day(mon), SlotID: e.dinner, Components: []plate.PlateComponent{composed(e.chili, 1)}})
	require.NoError(t, err)
	agent8 := e.run(actor.Agent, 8)
	// The agent's own edit of its run's creation is no correction.
	created, err := e.svc.Create(agent8, plate.PlateInput{Date: day(tue), SlotID: e.dinner, Components: []plate.PlateComponent{composed(e.curry, 1)}})
	require.NoError(t, err)
	_, err = e.svc.Update(agent8, created.ID, plate.PlatePatch{Eaters: f64(3)})
	require.NoError(t, err)
	// The user edits the agent's draft; another agent run edits the generated one.
	_, err = e.svc.Update(e.ctx, created.ID, plate.PlatePatch{Eaters: f64(4)})
	require.NoError(t, err)
	_, err = e.svc.Update(e.run(actor.Agent, 9), generated.ID, plate.PlatePatch{Eaters: f64(3)})
	require.NoError(t, err)

	got, err := sqlite.NewAIRunRepo(e.db).Corrections(e.ctx, 7, 9)
	require.NoError(t, err)
	assert.Equal(t, map[int64]airun.Correction{
		7: {Generated: 1, Corrected: 1},
		8: {Generated: 1, Corrected: 1},
	}, got)
}
