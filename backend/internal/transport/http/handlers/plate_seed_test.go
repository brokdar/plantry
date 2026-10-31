package handlers_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

// seedPlate creates p through the plate service (so composed components get
// their cook sessions) and writes the stored plate back into p.
func seedPlate(t *testing.T, svc *plate.Service, p *plate.Plate) {
	t.Helper()
	comps := make([]plate.PlateComponent, len(p.Components))
	for i, c := range p.Components {
		comps[i] = plate.PlateComponent{FoodID: c.FoodID, Portions: c.Portions, Amount: c.Amount, Unit: c.Unit, Grams: c.Grams}
	}
	var eaters *float64
	if p.Eaters > 0 {
		eaters = &p.Eaters
	}
	out, err := svc.Create(context.Background(), plate.PlateInput{
		Date: p.Date, SlotID: p.SlotID, Eaters: eaters, Note: p.Note, Components: comps,
	})
	require.NoError(t, err)
	*p = *out
}
