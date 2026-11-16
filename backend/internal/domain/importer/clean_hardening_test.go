package importer

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCleanName_KeepsAdjectivesOfTheName(t *testing.T) {
	cases := []struct{ in, name, prep string }{
		{"Frozen chopped spinach", "Frozen chopped spinach", ""},
		{"Canned chopped tomatoes", "Canned chopped tomatoes", ""},
		{"Fresh grated parmesan", "Fresh grated parmesan", ""},
		{"Dried sliced mushrooms", "Dried sliced mushrooms", ""},
		{"onion roughly chopped", "onion", "roughly chopped"},
		{"onion, chopped", "onion,", "chopped"},
		{"butter to grease", "butter", "to grease"},
		{"Zwiebel fein gehackt", "Zwiebel", "fein gehackt"},
	}
	for _, tc := range cases {
		name, prep := CleanName(tc.in)
		assert.Equal(t, tc.name, name, tc.in)
		assert.Equal(t, tc.prep, prep, tc.in)
	}
}

func TestMinutesFromSteps(t *testing.T) {
	cases := []struct {
		name  string
		steps []string
		want  int
	}{
		{"sum of steps", []string{"20 Min backen", "1 Stunde ruhen"}, 80},
		{"nested duration is not stacked", []string{"30 Min backen, nach 15 Min wenden"}, 30},
		{"or alternative", []string{"20 min on high or 90 min on low"}, 20},
		{"oder alternative", []string{"30 Min kochen oder 10 Min im Schnellkochtopf"}, 30},
		{"bis zu", []string{"10 Min braten, bis zu 20 Min ziehen lassen"}, 10},
		{"over 24h dropped", []string{"180 h ruhen"}, 0},
		{"none", []string{"Alles mischen."}, 0},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, minutesFromSteps(tc.steps), tc.name)
	}
}

func TestCleanRecipeName_Edges(t *testing.T) {
	// Intended edges: the credit and the "Rezept" suffix are cut even when
	// they belong to a title like these.
	assert.Equal(t, "Hühnersuppe", cleanRecipeName("Hühnersuppe - Rezept von Oma"))
	assert.Equal(t, "Omas", cleanRecipeName("Omas Rezept"))
}

func TestStripHTML_NBSP(t *testing.T) {
	assert.Equal(t, "a b", stripHTML("a&nbsp;&nbsp;b"))
}
