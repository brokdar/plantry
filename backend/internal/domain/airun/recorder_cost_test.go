package airun_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/fake"
	"github.com/jaltszeimer/plantry/backend/internal/domain/airun"
	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
)

// A finished run carries its cost at the prices of that moment: the
// built-in table, an override when one is set, nil for an unpriced model.
func TestRecorder_FinishStoresCost(t *testing.T) {
	overrides := map[string]airun.Price{"house-model": {In: 1, Out: 1}}
	cases := []struct {
		name   string
		model  string
		prices func(context.Context) (map[string]airun.Price, error)
		want   *float64
	}{
		{"built-in", "claude-sonnet-5-5", nil, ptr((3.0*2 + 4*10) / 1e6)},
		{"override", "house-model", func(context.Context) (map[string]airun.Price, error) { return overrides, nil }, ptr(7.0 / 1e6)},
		{
			"unreadable overrides fall back to the table", "claude-sonnet-5-5",
			func(context.Context) (map[string]airun.Price, error) { return nil, errors.New("bad") }, ptr((3.0*2 + 4*10) / 1e6),
		},
		{"unpriced", "gpt-6.2", nil, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			store := newMemStore()
			rec, _ := newRecorder(t, store)
			if c.prices != nil {
				rec.WithPrices(c.prices)
			}
			inner := fake.NewFromScript(&fake.Script{Turns: []fake.Turn{textTurn("hi", fake.ScriptedUsage{InputTokens: 3, OutputTokens: 4})}})
			ctx, run := rec.Start(context.Background(), airun.Start{Kind: airun.KindChefFind})
			_, err := rec.Meter(inner, "anthropic").Complete(ctx, llm.Request{Model: c.model})
			require.NoError(t, err)
			run.Finish(ctx, airun.StatusOK, "")

			_, finishes, _, _ := store.snapshot()
			require.Len(t, finishes, 1)
			if c.want == nil {
				assert.Nil(t, finishes[0].CostUSD)
				return
			}
			require.NotNil(t, finishes[0].CostUSD)
			assert.InDelta(t, *c.want, *finishes[0].CostUSD, 1e-12)
		})
	}
}
