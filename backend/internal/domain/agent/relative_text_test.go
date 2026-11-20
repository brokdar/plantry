package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// RL1, RL4: the tool texts state the rule the code applies: a tenth of the
// meal (no "or 80 kcal"), and the share under target is not a protein rule.
func TestToolTexts_StateTheSpecRules(t *testing.T) {
	for name, text := range map[string]string{"find_ways": findWaysDescription, "apply_changes": applyChangesDescription} {
		assert.Contains(t, text, "at least 10% of the meal", name)
		assert.NotContains(t, text, "80 kcal", name)
		assert.NotContains(t, text, "or the share under target", name)
	}
	assert.Contains(t, findWaysDescription, "0.5 to 1.5", "KR4: the portion range the way search uses")
	assert.Contains(t, findWaysDescription, "two dishes on one cooking time", "KR2")
}
