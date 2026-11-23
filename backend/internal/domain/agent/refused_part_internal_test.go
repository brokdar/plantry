package agent

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/window"
)

// V81 (tune-v2c g2-replan-01): a refused replace of Sunday's Shakshuka by the
// curry was told as "Left out: replacing "Easy chicken curry" at Sun 22 Nov
// dinner", which reads as if the curry were replaced. The line names the dish
// taken out and the one asked for.
func TestRefusedPart_ReplaceNamesBothDishes(t *testing.T) {
	t.Parallel()
	sun := time.Date(2026, 11, 22, 0, 0, 0, 0, time.UTC)
	st := planning.State{
		SlotNames: map[int64]string{3: "slot.dinner"},
		Foods: map[int64]planning.FoodFacts{
			166: {ID: 166, Name: "Shakshuka"}, 125: {ID: 125, Name: "Green salad"}, 120: {ID: 120, Name: "Easy chicken curry"},
		},
		Plates: []planning.Plate{{ID: 83, Date: sun, SlotID: 3, Components: []planning.Component{
			{ID: 1, FoodID: 166, Position: plate.PositionMain}, {ID: 2, FoodID: 125, Position: plate.PositionSide},
		}}},
	}
	svc := Services{State: func(context.Context, window.Window, time.Time) (planning.State, error) { return st, nil }}
	w := window.Window{Start: sun, End: sun}
	for name, op := range map[string]changeOp{
		"by plate and position": {Op: "replace", PlateID: 83, Position: "main", FoodID: 120},
		"by component":          {Op: "swap", PlateComponentID: 1, FoodID: 120},
	} {
		en, de := refusedPart(context.Background(), svc, w, op, applyRun{})
		assert.Equal(t, `replacing "Shakshuka" with "Easy chicken curry" at Sun 22 Nov dinner`, en, name)
		assert.Equal(t, `„Shakshuka“ bei Sonntag, 22. Nov. Abendessen durch „Easy chicken curry“ ersetzen`, de, name)
	}
}
