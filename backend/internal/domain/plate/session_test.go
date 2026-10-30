package plate_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

func TestBuildView(t *testing.T) {
	s := sunDinner(8, 0)
	v := plate.BuildView(s, "Chili", 3, []plate.Link{
		link(2, cell("2026-11-02", lunch), 1, 2),
		link(1, cell("2026-11-01", dinner), 1, 2),
	}, plate.BatchSettings{DefaultKeepDays: 3, FreezerKeepDays: 90}, order)
	assert.Equal(t, "Chili", v.FoodName)
	assert.Equal(t, day("2026-11-04"), v.UseBy)
	assert.Equal(t, 2, v.Meals)
	assert.InDelta(t, 4.0, v.Consumed, 1e-9)
	assert.InDelta(t, 4.0, v.Surplus, 1e-9)
	assert.Equal(t, int64(1), v.Links[0].PlateID)
	assert.Empty(t, v.Warnings)
}
