package planning

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// NU3: only the meat clause of the notes goes; the user's other words stay, and a
// decimal point does not end the clause.
func TestDropMeatLimit_OnlyTheClause(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]string{
		"meat at most 2 days, no mushrooms, I hate fennel":   "no mushrooms, I hate fennel",
		"no mushrooms. meat at most 2.5 days a week. thanks": "no mushrooms. thanks",
		"I hate fennel; meat max 3 days; quick dinners":      "I hate fennel; quick dinners",
		"meat at most 2–3 days a week":                       "",
		"vegetarian on Mondays":                              "vegetarian on Mondays",
	} {
		assert.Equal(t, want, dropMeatLimit(in), in)
	}
}
