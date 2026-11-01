package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

// The key names a set of rows, whatever order the closure lists them in.
func TestKeyOf_IgnoresIDOrder(t *testing.T) {
	a := keyOf(1, plate.RowSet{PlateIDs: []int64{3, 1, 2}, SessionIDs: []int64{9, 4}})
	b := keyOf(1, plate.RowSet{PlateIDs: []int64{1, 2, 3}, SessionIDs: []int64{4, 9}})
	assert.Equal(t, a, b)
	assert.NotEqual(t, a, keyOf(1, plate.RowSet{PlateIDs: []int64{1, 2}, SessionIDs: []int64{3, 4, 9}}), "ids do not migrate between lists")

	in := plate.RowSet{PlateIDs: []int64{3, 1}}
	keyOf(1, in)
	assert.Equal(t, []int64{3, 1}, in.PlateIDs, "the caller's slice is not reordered")
}
