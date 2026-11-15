package importer_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/importer"
)

func pageWithSteps(steps ...string) string {
	items := ""
	for i, s := range steps {
		if i > 0 {
			items += ","
		}
		items += `{"@type":"HowToStep","text":"` + s + `"}`
	}
	return `<script type="application/ld+json">{"@type":"Recipe","name":"R",
"recipeYield":"2","recipeIngredient":["200 g Reis"],"recipeInstructions":[` + items + `]}</script>`
}

// A closing pleasantry is not a cooking step. A real last step that merely
// mentions serving is kept.
func TestExtract_DropsClosingPleasantry(t *testing.T) {
	cases := []struct {
		name  string
		steps []string
		want  []string
	}{
		{"german", []string{"Reis kochen.", "Ich wünsche dir einen guten Appetit!"}, []string{"Reis kochen."}},
		{"german short", []string{"Reis kochen.", "Guten Appetit!"}, []string{"Reis kochen."}},
		{"english", []string{"Cook the rice.", "Enjoy!"}, []string{"Cook the rice."}},
		{"bon appetit", []string{"Cook the rice.", "Bon appétit!"}, []string{"Cook the rice."}},
		{"real last step stays", []string{"Reis kochen.", "Mit Koriander servieren und guten Appetit wünschen, sobald alle sitzen und das Essen noch richtig heiß ist."}, []string{"Reis kochen.", "Mit Koriander servieren und guten Appetit wünschen, sobald alle sitzen und das Essen noch richtig heiß ist."}},
		{"only step stays", []string{"Guten Appetit!"}, []string{"Guten Appetit!"}},
		{"middle stays", []string{"Guten Appetit!", "Reis kochen."}, []string{"Guten Appetit!", "Reis kochen."}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d, err := newFinalizeEnv(t).importer.Extract(context.Background(),
				importer.ExtractInput{HTML: pageWithSteps(tc.steps...)})
			require.NoError(t, err)
			assert.Equal(t, tc.want, d.Instructions)
		})
	}
}
