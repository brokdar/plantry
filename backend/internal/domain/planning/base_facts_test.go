package planning

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
)

// A leaf's stored role (a plate-position override) is not a planner role, so
// the leaf is no side candidate.
func TestBaseFacts_LeafRoleStaysOutOfCandidateLists(t *testing.T) {
	role := food.RoleSideVeg
	leaf := BaseFacts(&food.Food{ID: 1, Name: "Gurke", Kind: food.KindLeaf, Role: &role})
	assert.Empty(t, leaf.Role)
	assert.False(t, sideRole(leaf.Role))

	comp := BaseFacts(&food.Food{ID: 2, Name: "Salat", Kind: food.KindComposed, Role: &role})
	assert.Equal(t, food.RoleSideVeg, comp.Role)
	assert.True(t, sideRole(comp.Role))
}
