package plate_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

// plateWith is a plate whose composed components eat from sessions (0: a
// leaf component without a session).
func plateWith(id int64, sessions ...int64) plate.Plate {
	p := plate.Plate{ID: id}
	for _, sid := range sessions {
		c := plate.PlateComponent{FoodID: 1}
		if sid != 0 {
			c.CookSessionID = i64(sid)
		}
		p.Components = append(p.Components, c)
	}
	return p
}

func TestClosure(t *testing.T) {
	// Session 10 (Chili) feeds plates 1, 2, 3; plate 3 also eats session 20
	// (Rice), which feeds plate 4. Session 30 feeds plate 5 alone; plate 6
	// has only a leaf. Session 40 has no loaded link.
	plates := []plate.Plate{
		plateWith(1, 10),
		plateWith(2, 10, 0),
		plateWith(3, 10, 20),
		plateWith(4, 20),
		plateWith(5, 30),
		plateWith(6, 0),
	}
	tests := []struct {
		name         string
		seedPlates   []int64
		seedSessions []int64
		want         plate.RowSet
	}{
		{
			"leftover plate reaches its batch and the batch's other plates",
			[]int64{1},
			nil,
			plate.RowSet{PlateIDs: []int64{1, 2, 3, 4}, SessionIDs: []int64{10, 20}},
		},
		{
			"side batch joins through a shared plate",
			[]int64{4},
			nil,
			plate.RowSet{PlateIDs: []int64{1, 2, 3, 4}, SessionIDs: []int64{10, 20}},
		},
		{
			"session seed", nil,
			[]int64{30},
			plate.RowSet{PlateIDs: []int64{5}, SessionIDs: []int64{30}},
		},
		{
			"plate without sessions is alone",
			[]int64{6},
			nil,
			plate.RowSet{PlateIDs: []int64{6}, SessionIDs: []int64{}},
		},
		{
			"session without loaded links is alone", nil,
			[]int64{40},
			plate.RowSet{PlateIDs: []int64{}, SessionIDs: []int64{40}},
		},
		{
			"several seeds merge and sort",
			[]int64{5, 6},
			[]int64{20},
			plate.RowSet{PlateIDs: []int64{1, 2, 3, 4, 5, 6}, SessionIDs: []int64{10, 20, 30}},
		},
		{"no seeds", nil, nil, plate.RowSet{PlateIDs: []int64{}, SessionIDs: []int64{}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, plate.Closure(plates, tt.seedPlates, tt.seedSessions))
		})
	}
}

func TestRowSet_Empty(t *testing.T) {
	assert.True(t, plate.RowSet{}.Empty())
	assert.False(t, plate.RowSet{SessionIDs: []int64{1}}.Empty())
}
