package platefit_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaltszeimer/plantry/backend/internal/domain/platefit"
)

func TestPhrase(t *testing.T) {
	tests := []struct {
		name string
		r    platefit.Reading
		want string
	}{
		{"nothing raised", platefit.Reading{}, "fits"},
		{"minor gaps only", platefit.Reading{Gaps: []platefit.Gap{{Dimension: platefit.Texture, Minor: true}}}, "fits"},
		{"clashes and gaps", platefit.Reading{
			Clashes: []platefit.Dimension{platefit.Richness, platefit.Proportions},
			Gaps:    []platefit.Gap{{Dimension: platefit.Fresh}, {Dimension: platefit.Colour, Minor: true}, {Dimension: platefit.Texture}},
		}, "clash: two starches or two mains, too rich; lacks: a fresh element, crunch"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, platefit.Phrase(tt.r))
		})
	}
}
