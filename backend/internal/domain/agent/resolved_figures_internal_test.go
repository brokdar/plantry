package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
)

// RP9 (P-4): a day issue an away resolves is listed with the figures it had
// before the write (target 1992, range 1793 to 2191), not the planned-part
// figures the day has after it (replay a1 out-wed).
func TestDeltaOf_ResolvedIssueKeepsItsBeforeFigures(t *testing.T) {
	before := planning.Issue{
		Rule: "S2", Code: planning.CodeKcalBelowBand, Severity: planning.SeveritySoft, Date: "2026-11-25",
		Params: map[string]any{"kcal": 1707.0, "target_kcal": 1992.0, "range_low_kcal": 1793.0, "range_high_kcal": 2191.0},
	}
	delta := deltaOf(issueMap([]planning.Issue{before}), issueMap(nil))
	resolved, ok := delta["resolved"].([]effectIssue)
	require.True(t, ok)
	require.Len(t, resolved, 1)
	assert.InDelta(t, 1992, resolved[0].Params["target_kcal"], 0.5)
	assert.InDelta(t, 1793, resolved[0].Params["range_low_kcal"], 0.5)
}
