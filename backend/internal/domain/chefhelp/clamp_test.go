package chefhelp

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestClamp_NeverCutsMidWordOrSentence(t *testing.T) {
	t.Parallel()
	sentence := "Stir the sauce over low heat until it coats the back of a spoon."
	long := strings.Repeat(sentence+" ", 5) // well over the cap
	got := clamp([]string{long})
	assert.Len(t, got, 1)
	assert.LessOrEqual(t, len([]rune(got[0])), MaxLineRunes)
	assert.True(t, strings.HasSuffix(got[0], "spoon."), got[0])

	clauses := strings.Repeat("keep the pan on low heat, stir slowly, ", 8)
	got = clamp([]string{clauses})
	assert.True(t, strings.HasSuffix(got[0], "."), got[0])
	assert.NotContains(t, got[0], "…")
	assert.LessOrEqual(t, len([]rune(got[0])), MaxLineRunes)

	whole := strings.Repeat("a", 10) + " " + strings.Repeat("b", 150) // under the cap: kept whole
	assert.Equal(t, []string{whole}, clamp([]string{whole}))
}

func TestClamp_DropsSelfTalk(t *testing.T) {
	t.Parallel()
	got := clamp([]string{"No, the eggs are still in the jug.", "Wait, that is wrong.", "Actually add salt.", "Whisk the eggs now."})
	assert.Equal(t, []string{"Whisk the eggs now."}, got)
}
