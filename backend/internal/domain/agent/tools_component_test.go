package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

func TestComponentByID(t *testing.T) {
	p := &plate.Plate{ID: 3, Components: []plate.PlateComponent{{ID: 7, PlateID: 3, FoodID: 1}, {ID: 8, PlateID: 3, FoodID: 2}}}

	pc, err := componentByID(p, 8)
	require.NoError(t, err)
	assert.Equal(t, int64(2), pc.FoodID)

	pc, err = componentByID(p, 9)
	assert.Nil(t, pc, "no fabricated component")
	assert.ErrorIs(t, err, domain.ErrNotFound)
}
