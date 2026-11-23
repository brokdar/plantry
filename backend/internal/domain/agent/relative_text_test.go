package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// RL1, RL4: the tool texts state the rule the code applies: a tenth of the
// meal (no "or 80 kcal"), and the share under target is not a protein rule.
func TestToolTexts_StateTheSpecRules(t *testing.T) {
	t.Parallel()
	for name, text := range map[string]string{"find_ways": findWaysDescription, "apply_changes": applyChangesDescription} {
		assert.Contains(t, text, "at least 10% of the meal", name)
		assert.NotContains(t, text, "80 kcal", name)
		assert.NotContains(t, text, "or the share under target", name)
	}
	assert.Contains(t, findWaysDescription, "0.5 to 1.5", "KR4: the portion range the way search uses")
	assert.Contains(t, findWaysDescription, "two dishes on one cooking time", "KR2")
}

// V72 T3 (tune-v2 h2-limit-04): "Thursday dinner really filling" was searched
// as a comparative (no to_limit), so the filling mark was not stored (SC3,
// C-8). The tool texts name the filling wording for to_limit, not only light.
func TestToolTexts_ToLimitCoversFilling(t *testing.T) {
	t.Parallel()
	assert.Contains(t, findWaysDescription, `"really filling"`)
	assert.Contains(t, findWaysDescription, "not just a lighter or heartier one")
}
