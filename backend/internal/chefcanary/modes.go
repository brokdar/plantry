package chefcanary

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain/airun"
	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
)

// The measurement modes of the paid harness (F1, SPEC section 10). Each runs
// one set through the service the product uses and reports one-line metrics;
// every call is in the run log of the database it runs on.

// Env is what every mode runs on.
type Env struct {
	// DB is a migrated database; the run log and the finds are written to it,
	// so it is a copy.
	DB       *sql.DB
	Client   llm.Client
	Provider string
	Model    string
	// Prices are ai.price_overrides for the cost (nil: the built-in table).
	Prices map[string]airun.Price
	Logger *slog.Logger
	// SpendCap stops the run before the next case once the run log's cost of
	// this run reaches it (USD); 0 is no cap.
	SpendCap float64
}

func (e Env) logger() *slog.Logger {
	if e.Logger != nil {
		return e.Logger
	}
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// Metric is one summary line.
type Metric struct {
	Name  string  `json:"name"`
	Value float64 `json:"value"`
	Unit  string  `json:"unit,omitempty"`
	// Target is the gate the metric is read against, Met whether it holds
	// (nil: no target).
	Target string `json:"target,omitempty"`
	Met    *bool  `json:"met,omitempty"`
	Note   string `json:"note,omitempty"`
}

// CaseResult is one case of a mode.
type CaseResult struct {
	ID          string   `json:"id"`
	Groups      []string `json:"groups,omitempty"`
	Outcome     string   `json:"outcome"`
	OK          bool     `json:"ok"`
	CostUSD     float64  `json:"cost_usd"`
	LatencyMS   int64    `json:"latency_ms"`
	WebSearches int64    `json:"web_searches,omitempty"`
	Detail      string   `json:"detail,omitempty"`
}

// ModeReport is a mode's outcome.
type ModeReport struct {
	Mode  string `json:"mode"`
	Model string `json:"model,omitempty"`
	// Cases is the size of the set, Ran how many were run.
	Cases int `json:"cases"`
	Ran   int `json:"ran"`
	// StoppedBySpendCap: the run ended before the set did because the run
	// log's cost reached SpendCapUSD.
	StoppedBySpendCap bool         `json:"stopped_by_spend_cap"`
	SpendCapUSD       float64      `json:"spend_cap_usd,omitempty"`
	CostUSD           float64      `json:"cost_usd"`
	Unpriced          bool         `json:"unpriced"`
	Metrics           []Metric     `json:"metrics"`
	Results           []CaseResult `json:"results"`
}

// Summary renders one line per metric.
func (r ModeReport) Summary() string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "mode %s: %d/%d cases run, cost ", r.Mode, r.Ran, r.Cases)
	if r.Unpriced {
		sb.WriteString("unknown (unpriced model)")
	} else {
		fmt.Fprintf(&sb, "$%.4f", r.CostUSD)
	}
	if r.StoppedBySpendCap {
		fmt.Fprintf(&sb, ", STOPPED by the spend cap ($%.2f)", r.SpendCapUSD)
	}
	sb.WriteString("\n")
	for _, m := range r.Metrics {
		fmt.Fprintf(&sb, "  %-34s %9.2f %-4s", m.Name, m.Value, m.Unit)
		if m.Target != "" {
			verdict := "met"
			if m.Met != nil && !*m.Met {
				verdict = "NOT MET"
			}
			fmt.Fprintf(&sb, " target %s: %s", m.Target, verdict)
		}
		if m.Note != "" {
			fmt.Fprintf(&sb, "  (%s)", m.Note)
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

func (r *ModeReport) add(m Metric) { r.Metrics = append(r.Metrics, m) }

// target sets Met by comparing value with the bound: atLeast true is
// value >= bound.
func (m Metric) target(text string, atLeast bool, bound float64) Metric {
	m.Target = text
	met := m.Value <= bound
	if atLeast {
		met = m.Value >= bound
	}
	m.Met = &met
	return m
}

// ledger reads what the run log gained since the last settle and keeps the
// run's spend for the cap.
type ledger struct {
	env      Env
	runs     *sqlite.AIRunRepo
	rec      *airun.Recorder
	last     int64
	spent    float64
	unpriced bool
}

func newLedger(ctx context.Context, env Env) (*ledger, error) {
	runs := sqlite.NewAIRunRepo(env.DB)
	l := &ledger{env: env, runs: runs, rec: airun.NewRecorder(runs, env.logger(), time.Now)}
	rows, err := runs.ListRuns(ctx, airun.RunQuery{Limit: 1})
	if err == nil && len(rows) > 0 {
		l.last = rows[0].ID
	}
	return l, err
}

// capReached says the next case must not start.
func (l *ledger) capReached() bool { return l.env.SpendCap > 0 && l.spent >= l.env.SpendCap }

// usage is what the runs since the last settle cost.
type usage struct {
	Runs        int
	CostUSD     float64
	WebSearches int64
	Unpriced    bool
	Rows        []airun.RunRow
}

// settle returns the runs logged since the last call and adds their cost to
// the spend.
func (l *ledger) settle(ctx context.Context) (usage, error) {
	rows, err := l.runs.ListRuns(ctx, airun.RunQuery{Limit: 200})
	if err != nil {
		return usage{}, err
	}
	var u usage
	maxID := l.last
	for _, r := range rows {
		if r.ID <= l.last {
			continue
		}
		maxID = max(maxID, r.ID)
		u.Runs++
		u.Rows = append(u.Rows, r)
		u.WebSearches += r.Tokens.WebSearchRequests
		if c := airun.Cost(r.Model, r.Provider, r.Tokens, l.env.Prices); c != nil {
			u.CostUSD += *c
		} else {
			u.Unpriced = true
		}
	}
	l.last = maxID
	l.spent += u.CostUSD
	l.unpriced = l.unpriced || u.Unpriced
	return u, nil
}

// finish fills the report's totals.
func (l *ledger) finish(r *ModeReport) {
	r.Model, r.CostUSD, r.Unpriced, r.SpendCapUSD = l.env.Model, l.spent, l.unpriced, l.env.SpendCap
}

// percentile is the nearest-rank percentile of values (0 for none).
func percentile(values []float64, p float64) float64 {
	if len(values) == 0 {
		return 0
	}
	s := slices.Clone(values)
	slices.Sort(s)
	return s[max(0, int(math.Ceil(p/100*float64(len(s))))-1)]
}

func pct(n, total int) float64 {
	if total == 0 {
		return 0
	}
	return 100 * float64(n) / float64(total)
}

// byGroup adds one rate metric per group value (theme, constraint ...).
func byGroup(r *ModeReport, prefix string, groups func(CaseResult) string) {
	total, ok := map[string]int{}, map[string]int{}
	for _, c := range r.Results {
		g := groups(c)
		if g == "" {
			continue
		}
		total[g]++
		if c.OK {
			ok[g]++
		}
	}
	for _, g := range slices.Sorted(maps.Keys(total)) {
		r.add(Metric{Name: prefix + g, Value: pct(ok[g], total[g]), Unit: "%", Note: fmt.Sprintf("%d/%d", ok[g], total[g])})
	}
}
