package report_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/report"
)

func TestRender_EmptyDataSaysNoDataInEverySection(t *testing.T) {
	t.Parallel()
	out := report.Render(report.Data{GeneratedAt: time.Date(2026, 11, 28, 12, 0, 0, 0, time.UTC), Days: 30})
	for _, want := range []string{
		"## Problems\n\nNothing stands out.",
		"No data: no AI runs in the window.",
		"- Help answers: no data",
		"- Trial dishes: no data",
		"- Import checks: no data",
		"No data: no plan runs in the window.",
		"No data: no rated plates in the window.",
	} {
		assert.Contains(t, out, want)
	}
	assert.NotContains(t, out, "NaN")
	assert.NotContains(t, out, "Inf")
}

func TestProblems_ThresholdsAreStrict(t *testing.T) {
	t.Parallel()
	at := report.Data{
		AI: report.AI{
			CurrentMonth: "2026-11", Cap: 5,
			Kinds: []report.KindStat{
				{Kind: "chef_find", Runs: 10, Errors: 1},                         // exactly 10 %
				{Kind: "chef_help", Runs: 4, Timed: 4, P90: report.HelpP90Limit}, // exactly 10 s
			},
			Spend: []report.MonthSpend{{Month: "2026-11", USD: 4}}, // exactly 80 %
		},
		Chef: report.Chef{Jobs: []report.KeyCount{{Key: "chef_find/failed", N: 2}}},
	}
	assert.Empty(t, report.Problems(at), "at the limit is fine")

	over := at
	over.AI.Kinds = []report.KindStat{
		{Kind: "chef_find", Runs: 9, Errors: 1},
		{Kind: "chef_help", Runs: 4, Timed: 4, P90: report.HelpP90Limit + 1},
	}
	over.AI.Spend = []report.MonthSpend{{Month: "2026-11", USD: 4.01}, {Month: "2026-10", USD: 99}}
	over.AI.Untracked = 1
	over.Chef.Jobs = []report.KeyCount{{Key: "chef_find/failed", N: 2}, {Key: "chef_find/failed", N: 1}}
	ps := report.Problems(over)
	require.Len(t, ps, 5)
	joined := strings.Join(ps, "\n")
	for _, want := range []string{"chef_find: 1 of 9", "chef_help: p90 is 10.0 s", "untracked AI runs: 1", "$4.01 of the $5.00 cap", "job chef_find failed 3 times"} {
		assert.Contains(t, joined, want)
	}
	assert.NotContains(t, joined, "$99", "only this month's spend counts against the cap")
}

func TestRender_ZeroDivisionShowsDashes(t *testing.T) {
	t.Parallel()
	out := report.Render(report.Data{
		AI:      report.AI{Kinds: []report.KindStat{{Kind: "chef_help"}}},
		Chef:    report.Chef{Funnel: report.Funnel{}},
		Ratings: report.Ratings{Other: report.RatingCounts{Loved: 1}},
	})
	assert.Contains(t, out, "| chef_help | 0 | 0 (-) | - | - | $0.0000 |")
	assert.Contains(t, out, "| With a Chef dish | 0 | - | - | - |")
	assert.Contains(t, out, "| Others | 1 | 100 % | 0 % | 0 % |")
}
