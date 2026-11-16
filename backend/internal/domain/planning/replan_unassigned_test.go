package planning

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

func TestUnassignedOf(t *testing.T) {
	batch := func(p, f, after float64, freezable, mutable bool) spareBatch {
		return spareBatch{
			sess:  plate.CookSession{Portions: p, FrozenPortions: f},
			after: after, freezable: freezable, mutable: mutable,
		}
	}
	cases := []struct {
		name string
		b    spareBatch
		mode SpareMode
		want float64
	}{
		{"cooked freezable overflows the batch", batch(6, 3, 1, true, false), SpareFreeze, 2},
		{"mutable freezable overflows the batch", batch(6, 3, 1, true, true), SpareFreeze, 2},
		{"cooked freezable fits", batch(6, 0, 3, true, false), SpareFreeze, 0},
		{"mutable freezable fits", batch(6, 0, 3, true, true), SpareFreeze, 0},
		{"cooked, keep mode", batch(6, 3, 1, true, false), SpareKeep, 5},
		{"cooked, not freezable", batch(6, 0, 1, false, false), SpareFreeze, 5},
		{"mutable shrinks, keep mode", batch(6, 3, 1, true, true), SpareKeep, 0},
		{"mutable shrinks, not freezable", batch(6, 0, 1, false, true), SpareFreeze, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.InDelta(t, tc.want, unassignedOf(tc.b, tc.mode), 1e-9)
		})
	}
}
