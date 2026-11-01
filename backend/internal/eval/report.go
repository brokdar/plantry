package eval

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"slices"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
)

// Record is one scenario run: a JSONL line of the report, keyed by
// (git_sha, prompt_version, model).
type Record struct {
	GitSHA         string           `json:"git_sha"`
	PromptVersion  string           `json:"prompt_version"`
	Model          string           `json:"model"`
	Chooser        string           `json:"chooser"`
	Scenario       string           `json:"scenario"`
	Run            int              `json:"run"`
	Status         string           `json:"status"`
	ErrorKey       string           `json:"error_key,omitempty"`
	RepairAttempts int              `json:"repair_attempts"`
	Plates         int              `json:"plates"`
	Score          *planning.Score  `json:"score"`
	Violations     []planning.Issue `json:"violations"`
	Tokens         Tokens           `json:"tokens"`
	CostUSD        *float64         `json:"cost_usd"`
	LatencyMS      int64            `json:"latency_ms"`
	Expectations   []Expectation    `json:"expectations"`
}

// Tokens are a run's summed token counts.
type Tokens struct {
	Input      int64 `json:"input"`
	Output     int64 `json:"output"`
	CacheRead  int64 `json:"cache_read"`
	CacheWrite int64 `json:"cache_write"`
}

// Expectation is one check of a run; Detail says why it failed.
type Expectation struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail,omitempty"`
}

// Failed lists the record's failed expectations.
func (r Record) Failed() []Expectation {
	var out []Expectation
	for _, x := range r.Expectations {
		if !x.OK {
			out = append(out, x)
		}
	}
	return out
}

// WriteJSONL writes one record per line.
func WriteJSONL(w io.Writer, recs []Record) error {
	enc := json.NewEncoder(w)
	for _, r := range recs {
		if err := enc.Encode(r); err != nil {
			return err
		}
	}
	return nil
}

// ReadJSONL reads a report.
func ReadJSONL(r io.Reader) ([]Record, error) {
	var out []Record
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for line := 1; sc.Scan(); line++ {
		if strings.TrimSpace(sc.Text()) == "" {
			continue
		}
		var rec Record
		if err := json.Unmarshal(sc.Bytes(), &rec); err != nil {
			return nil, fmt.Errorf("line %d: %w", line, err)
		}
		out = append(out, rec)
	}
	return out, sc.Err()
}

// component is one per-scenario number of the summary and the diff: the
// mean over a scenario's runs (nil values are left out).
type component struct {
	name  string
	value func(Record) *float64
}

func f(v float64) *float64 { return &v }

var components = []component{
	{"ok_runs", func(r Record) *float64 { return f(b2f(len(r.Failed()) == 0)) }},
	{"hard_violations", func(r Record) *float64 { return f(float64(len(r.Violations))) }},
	{"repair_attempts", func(r Record) *float64 { return f(float64(r.RepairAttempts)) }},
	{"plates", func(r Record) *float64 { return f(float64(r.Plates)) }},
	{"coverage", score(func(s planning.Score) *float64 { return &s.Coverage })},
	{"macro_daily_pct", score(func(s planning.Score) *float64 { return s.MacroDeviation.DailyPct })},
	{"macro_weekly_pct", score(func(s planning.Score) *float64 { return s.MacroDeviation.WeeklyPct })},
	{"cook_occasions", score(func(s planning.Score) *float64 {
		n := 0
		for _, v := range s.CookOccasions.PerDay {
			n += v
		}
		return f(float64(n))
	})},
	{"days_with_prepared", score(func(s planning.Score) *float64 { return f(float64(s.CookOccasions.DaysWithPrepared)) })},
	{"variety_distinct", score(func(s planning.Score) *float64 { return f(float64(s.Variety.Distinct)) })},
	{"variety_repeats", score(func(s planning.Score) *float64 { return f(float64(s.Variety.Repeats)) })},
	{"new_recipes", score(func(s planning.Score) *float64 { return f(float64(s.Variety.NewRecipes)) })},
	{"preference_fit", score(func(s planning.Score) *float64 { return &s.PreferenceFit })},
	{"waste_portions", score(func(s planning.Score) *float64 { return &s.WastePortions })},
	{"unknown_data_share", score(func(s planning.Score) *float64 { return &s.UnknownDataShare })},
	{"input_tokens", func(r Record) *float64 { return f(float64(r.Tokens.Input)) }},
	{"output_tokens", func(r Record) *float64 { return f(float64(r.Tokens.Output)) }},
	{"cost_usd", func(r Record) *float64 { return r.CostUSD }},
	{"latency_ms", func(r Record) *float64 { return f(float64(r.LatencyMS)) }},
}

func score(get func(planning.Score) *float64) func(Record) *float64 {
	return func(r Record) *float64 {
		if r.Score == nil {
			return nil
		}
		return get(*r.Score)
	}
}

func b2f(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

// aggregate is a report's per-scenario means of every component.
type aggregate struct {
	keys      []string
	scenarios []string
	runs      map[string]int
	means     map[string]map[string]*float64
}

func aggregateOf(recs []Record) aggregate {
	a := aggregate{runs: map[string]int{}, means: map[string]map[string]*float64{}}
	by := map[string][]Record{}
	for _, r := range recs {
		key := fmt.Sprintf("%s %s/%s/%s", r.GitSHA, r.Chooser, r.PromptVersion, r.Model)
		if !slices.Contains(a.keys, key) {
			a.keys = append(a.keys, key)
		}
		if _, ok := by[r.Scenario]; !ok {
			a.scenarios = append(a.scenarios, r.Scenario)
		}
		by[r.Scenario] = append(by[r.Scenario], r)
	}
	for sc, rs := range by {
		a.runs[sc] = len(rs)
		a.means[sc] = map[string]*float64{}
		for _, c := range components {
			sum, n := 0.0, 0
			for _, r := range rs {
				if v := c.value(r); v != nil {
					sum += *v
					n++
				}
			}
			if n > 0 {
				a.means[sc][c.name] = f(sum / float64(n))
			}
		}
	}
	return a
}

// printer writes formatted text and keeps the first error.
type printer struct {
	w   io.Writer
	err error
}

func (p *printer) printf(format string, a ...any) {
	if p.err == nil {
		_, p.err = fmt.Fprintf(p.w, format, a...)
	}
}

// table prints rows of tab-separated cells as aligned columns.
func (p *printer) table(rows []string) {
	tw := tabwriter.NewWriter(p.w, 0, 0, 2, ' ', 0)
	t := &printer{w: tw}
	for _, r := range rows {
		t.printf("%s\n", r)
	}
	if t.err == nil {
		t.err = tw.Flush()
	}
	if p.err == nil {
		p.err = t.err
	}
}

// WriteSummary prints a report's per-scenario means and every failed
// expectation.
func WriteSummary(w io.Writer, recs []Record) error {
	a := aggregateOf(recs)
	p := &printer{w: w}
	p.printf("Report %s\n", strings.Join(a.keys, ", "))
	rows := []string{"scenario\truns\tok\tviolations\tcoverage\tmacro_daily_pct\tdistinct\trepeats\twaste\ttokens_in\tcost_usd\tlatency_ms"}
	for _, sc := range a.scenarios {
		m := a.means[sc]
		rows = append(rows, strings.Join([]string{
			sc, strconv.Itoa(a.runs[sc]),
			num(m["ok_runs"]), num(m["hard_violations"]), num(m["coverage"]), num(m["macro_daily_pct"]),
			num(m["variety_distinct"]), num(m["variety_repeats"]), num(m["waste_portions"]),
			num(m["input_tokens"]), num(m["cost_usd"]), num(m["latency_ms"]),
		}, "\t"))
	}
	p.table(rows)
	for _, r := range recs {
		for _, x := range r.Failed() {
			p.printf("FAIL %s run %d: %s %s\n", r.Scenario, r.Run, x.Name, x.Detail)
		}
	}
	return p.err
}

// WriteCompare prints, per scenario, every component's mean in the previous
// and the current report and the change.
func WriteCompare(w io.Writer, prev, cur []Record) error {
	a, b := aggregateOf(prev), aggregateOf(cur)
	p := &printer{w: w}
	p.printf("Compare %s -> %s\n", strings.Join(a.keys, ", "), strings.Join(b.keys, ", "))
	scenarios := slices.Clone(a.scenarios)
	for _, sc := range b.scenarios {
		if !slices.Contains(scenarios, sc) {
			scenarios = append(scenarios, sc)
		}
	}
	slices.Sort(scenarios)
	for _, sc := range scenarios {
		p.printf("\n%s\n", sc)
		rows := []string{"  component\tprevious\tcurrent\tdelta"}
		for _, c := range components {
			x, y := a.means[sc][c.name], b.means[sc][c.name]
			if x == nil && y == nil {
				continue
			}
			rows = append(rows, fmt.Sprintf("  %s\t%s\t%s\t%s", c.name, num(x), num(y), delta(x, y)))
		}
		p.table(rows)
	}
	return p.err
}

// num prints a mean with up to 3 decimals; nil is "-".
func num(v *float64) string {
	if v == nil {
		return "-"
	}
	s := fmt.Sprintf("%.3f", math.Round(*v*1000)/1000)
	s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	if s == "-0" {
		s = "0"
	}
	return s
}

func delta(p, q *float64) string {
	if p == nil || q == nil {
		return "-"
	}
	d := *q - *p
	s := num(&d)
	if d > 0 && s != "0" {
		s = "+" + s
	}
	return s
}
