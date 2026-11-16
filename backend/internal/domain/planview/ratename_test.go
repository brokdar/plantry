package planview

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

func comp(name, role string) plate.PlateComponent {
	r := role
	one := 1.0
	return plate.PlateComponent{FoodName: name, FoodKind: "composed", FoodRole: &r, Portions: &one}
}

// F7: the rate question names the plate's main; without one it names the
// plate as the board does, not a single side.
func TestRateName(t *testing.T) {
	withMain := &plate.Plate{Components: []plate.PlateComponent{comp("Steamed rice", "side_starch"), comp("Curry", "main")}}
	assert.Equal(t, "Curry", rateName(withMain, primaryComposed(withMain)))

	noMain := &plate.Plate{Components: []plate.PlateComponent{comp("Steamed rice", "side_starch"), comp("Hasselback potatoes", "side_starch")}}
	assert.Equal(t, "Steamed rice + Hasselback potatoes", rateName(noMain, primaryComposed(noMain)))

	single := &plate.Plate{Components: []plate.PlateComponent{comp("Overnight oats", "standalone")}}
	assert.Equal(t, "Overnight oats", rateName(single, primaryComposed(single)))
}
