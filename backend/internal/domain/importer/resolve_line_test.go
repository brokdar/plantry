package importer_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/importer"
)

type queryRecorder struct{ queries []string }

func (q *queryRecorder) Lookup(_ context.Context, _, query, _ string, _ int) ([]food.Candidate, int, error) {
	q.queries = append(q.queries, query)
	return nil, -1, nil
}

// F4: the lookup of a review row matches the food, not its preparation.
func TestResolveLine_StripsPrepText(t *testing.T) {
	cases := map[string]string{
		"onion roughly chopped":                    "onion",
		"garlic roughly chopped":                   "garlic",
		"medium-sized floury potatoes":             "floury potatoes",
		"Zwiebel":                                  "Zwiebel",
		"chopped tomatoes":                         "chopped tomatoes", // "chopped" first: it is the food
		"light coconut or natural yogurt to serve": "light coconut or natural yogurt",
	}
	for in, want := range cases {
		rec := &queryRecorder{}
		svc := importer.NewService(nil, nil, nil, rec, nil)
		_, err := svc.ResolveLine(context.Background(), in, "en")
		require.NoError(t, err)
		assert.Equal(t, []string{want}, rec.queries, in)
	}
}
