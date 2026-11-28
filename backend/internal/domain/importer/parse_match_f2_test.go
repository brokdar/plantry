package importer_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/crypto"
	"github.com/jaltszeimer/plantry/backend/internal/adapters/fdc"
	"github.com/jaltszeimer/plantry/backend/internal/adapters/fdclocal"
	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/importer"
	"github.com/jaltszeimer/plantry/backend/internal/domain/settings"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
)

// f2Row is what one parsed row of an F2 find line must come to.
type f2Row struct {
	name   string // the parsed name contains
	amount float64
	unit   string
	// unmatched: the food is in no catalogue the matcher has; the line
	// parses right and stays for the review.
	unmatched bool
}

// TestParseAndMatch_F2FindLines: the exact ingredient lines of the F2 paid
// run's needs_check and failed finds (parser and matching gaps) parse into
// the rows named and every row matches a food.
func TestParseAndMatch_F2FindLines(t *testing.T) {
	ctx := context.Background()
	db := testhelper.NewTestDB(t)
	svc := settings.NewService(sqlite.NewAppSettingsRepo(db), settings.NewEnvSnapshot(nil), crypto.NilCipher{})
	m := food.NewMatcher(sqlite.NewFoodRepo(db), fdc.NewDynamicProvider(svc).WithFallback(fdclocal.New()), nil)

	tests := []struct {
		line string
		lang string
		rows []f2Row
	}{
		{"1 tsp dried rosemary", "en", []f2Row{{name: "rosemary", amount: 5, unit: "ml"}}},
		{"8 asparagus spears ((5 oz, 140 g))", "en", []f2Row{{name: "asparagus", amount: 8, unit: "piece"}}},
		{"1 box vegetarian Japanese curry roux", "en", []f2Row{{name: "vegetarian Japanese curry roux", amount: 1, unit: "piece", unmatched: true}}},
		{"1 batch mild taco seasoning (cumin, paprika, garlic powder, oregano, salt; no chili or cayenne)", "en", []f2Row{{name: "taco seasoning", amount: 1, unit: "piece"}}},
		{"3½ to 4 lbs. chicken pieces ($7.48***)", "en", []f2Row{{name: "chicken pieces", amount: 1700, unit: "g"}}},
		{"1 tablespoon Shaoxing wine", "en", []f2Row{{name: "Shaoxing", amount: 15, unit: "ml"}}},
		{"1 tsp doubanjiang (spicy chili bean paste)", "en", []f2Row{{name: "doubanjiang", amount: 5, unit: "ml"}}},
		{"2 tbsp tonkatsu sauce", "en", []f2Row{{name: "tonkatsu", amount: 30, unit: "ml"}}},
		{"4 Vollkorn-Tortillafladen", "de", []f2Row{{name: "Tortilla", amount: 4, unit: "piece"}}},
		{"1 Stück frischer Ingwer (ca. 2 cm)", "de", []f2Row{{name: "Ingwer", amount: 5, unit: "g"}}},
		{"je 1 rote und gelbe Paprikaschote", "de", []f2Row{{name: "rote Paprikaschote", amount: 1, unit: "piece"}, {name: "gelbe Paprikaschote", amount: 1, unit: "piece"}}},
		{"je 1 kleine rote und gelbe Paprikaschote", "de", []f2Row{{name: "rote Paprikaschote", amount: 1, unit: "piece"}, {name: "gelbe Paprikaschote", amount: 1, unit: "piece"}}},
		{"je 1 kleine rote, gelbe Paprikaschote", "de", []f2Row{{name: "rote Paprikaschote", amount: 1, unit: "piece"}, {name: "gelbe Paprikaschote", amount: 1, unit: "piece"}}},
	}
	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			t.Parallel()
			rows := importer.ParseLine(tt.line, tt.lang)
			require.Len(t, rows, len(tt.rows), "%+v", rows)
			for i, want := range tt.rows {
				r := rows[i]
				assert.Contains(t, r.Name, want.name, "%+v", r)
				assert.False(t, r.ToTaste, "%+v", r)
				assert.Equal(t, want.unit, r.Unit, "%+v", r)
				assert.InDelta(t, want.amount, r.Amount, want.amount*0.2, "%+v", r)
				res, err := m.Match(ctx, food.MatchRequest{Lang: tt.lang, Lines: []food.MatchLine{importer.MatchLineOf(r)}})
				require.NoError(t, err)
				l := res.Lines[0]
				if want.unmatched {
					assert.Equal(t, food.MatchUnmatched, l.Status, "%+v", r)
					continue
				}
				assert.Equal(t, food.MatchMatched, l.Status, "row %+v reason %s tried %v", r, l.Reason, l.Tried)
			}
		})
	}
}
