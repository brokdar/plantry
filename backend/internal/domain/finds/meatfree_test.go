package finds_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/finds"
)

// A planning find asked for a meat-free main never becomes ready with a meat
// or fish line (F3b: a "Taco Casserole" with 454 g beef mince came back);
// the adapted recipe may swap the meat for a plant protein.
func TestPipeline_MeatFreeFindHasNoMeat(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	run := func(t *testing.T, r *rig, dish string) *finds.Find {
		f, err := r.svc.Request(ctx, dish, finds.TriggerPlanning)
		require.NoError(t, err)
		_ = r.svc.Run(ctx, f.ID)
		got, err := r.svc.Get(ctx, f.ID)
		require.NoError(t, err)
		return got
	}

	t.Run("meat left in the recipe fails the find", func(t *testing.T) {
		t.Parallel()
		r := newRig(t, true, lineMatcher{}, kitchen, searchTurn(curryURLs...),
			adaptTurn(false, fitsKitchen), adaptTurn(false, fitsKitchen), adaptTurn(false, fitsKitchen))
		f := run(t, r, "indian meat-free curry")
		require.Equal(t, finds.StateFailed, f.State, f.Steps)
		require.NotNil(t, f.Gates)
		require.Equal(t, finds.CodeMeatInMeatFree, f.Gates.Blocks[0].Code)
	})

	t.Run("meat swapped for tofu is ready", func(t *testing.T) {
		t.Parallel()
		r := newRig(t, true, lineMatcher{}, kitchen, searchTurn(curryURLs...), adaptTurn(true, fitsKitchen))
		f := run(t, r, "indian meat-free curry")
		require.Equal(t, finds.StateReady, f.State, f.Steps)
	})

	t.Run("a dish not asked meat-free keeps its meat", func(t *testing.T) {
		t.Parallel()
		r := newRig(t, true, lineMatcher{}, kitchen, searchTurn(curryURLs...), adaptTurn(false, fitsKitchen))
		f := run(t, r, "indian curry")
		require.Equal(t, finds.StateReady, f.State, f.Steps)
	})
}
