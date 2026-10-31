package airun

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"
)

// RunRow is one ai_runs row as the insights read it. Empty strings stand for
// NULL columns.
type RunRow struct {
	ID             int64
	Kind           Kind
	StartedAt      time.Time
	FinishedAt     *time.Time
	Status         Status
	Provider       string
	Model          string
	PromptVersion  string
	Chooser        Chooser
	Tokens         Tokens
	ScoreJSON      string
	ViolationsJSON string
	Outcome        string
}

// RunDetailRow adds what only the run detail shows.
type RunDetailRow struct {
	RunRow
	ScopeJSON   string
	Instruction string
	Notes       string
	ErrorKey    string
	PromptText  string
}

// StepView is one step of a run's detail. Args and Result are cut to 8192
// characters (Truncated says so); for an llm_call they hold the stored
// request and response. Empty means none.
type StepView struct {
	Seq             int
	Type            StepType
	Name            string
	Args            string
	ArgsTruncated   bool
	Result          string
	ResultTruncated bool
	IsError         bool
	DurationMs      int64
	InputTokens     int64
	OutputTokens    int64
}

// Correction counts a run's generated plates and those the user changed
// before accepting them (D24).
type Correction struct {
	Generated int
	Corrected int
}

// Rate is Corrected / Generated, or nil when the run generated no plates.
func (c Correction) Rate() *float64 {
	if c.Generated == 0 {
		return nil
	}
	r := float64(c.Corrected) / float64(c.Generated)
	return &r
}

// RunQuery selects runs: Kind and the started_at range [From, To) are
// optional; BeforeID > 0 lists runs with smaller ids; Limit < 0 lists all.
type RunQuery struct {
	Kind     Kind
	From     *time.Time
	To       *time.Time
	BeforeID int64
	Limit    int
}

// InsightsStore reads the run log. Every method is one query.
type InsightsStore interface {
	ListRuns(ctx context.Context, q RunQuery) ([]RunRow, error)
	// GetRun returns domain.ErrNotFound for an unknown id.
	GetRun(ctx context.Context, id int64) (RunDetailRow, error)
	ListSteps(ctx context.Context, runID int64) ([]StepView, error)
	// Corrections returns D24 counts for the runs with ids in [minID, maxID]
	// that generated plates.
	Corrections(ctx context.Context, minID, maxID int64) (map[int64]Correction, error)
}

// RunSummary is a run with its estimated cost (nil when unpriced) and
// correction rate (nil when it generated no plates).
type RunSummary struct {
	RunRow
	CostUSD        *float64
	CorrectionRate *float64
}

// RunDetail is a run's summary, detail fields and steps.
type RunDetail struct {
	RunSummary
	ScopeJSON   string
	Instruction string
	Notes       string
	ErrorKey    string
	PromptText  string
	Steps       []StepView
}

// RunsPage is one page of runs; NextCursor is nil on the last page.
type RunsPage struct {
	Items      []RunSummary
	NextCursor *int64
}

// WeekBucket aggregates the runs started in the week from Start (a Monday).
// Accepted counts accepted and partially accepted runs. CostUSD sums the
// priced runs; UnpricedRuns are left out of it.
type WeekBucket struct {
	Start           time.Time
	Runs            int
	Accepted        int
	Discarded       int
	CorrectionRate  *float64
	CostUSD         float64
	CostPerAccepted *float64
	UnpricedRuns    int
}

// VersionGroup aggregates runs by kind, prompt version and model. ScoreAvg
// averages every numeric leaf of score_json (nested keys joined with "."),
// over the runs that have it; Violations counts violations by rule.
type VersionGroup struct {
	Kind          Kind
	PromptVersion string
	Model         string
	Runs          int
	ScoreAvg      map[string]float64
	Violations    map[string]int
}

// Report is the insights over a date range.
type Report struct {
	Weeks      []WeekBucket
	ByVersion  []VersionGroup
	PricesAsOf string
}

// PriceInfo is a model's effective price.
type PriceInfo struct {
	Model  string
	Price  *Price
	Source PriceSource
	AsOf   string
}

// Insights is the read side of the run log: runs, run detail, weekly trends
// and prices. Costs are computed on read, so a changed price re-prices
// history.
type Insights struct {
	store  InsightsStore
	prices func(ctx context.Context) (map[string]Price, error)
}

// NewInsights builds the read service. prices returns the current
// ai.price_overrides.
func NewInsights(store InsightsStore, prices func(ctx context.Context) (map[string]Price, error)) *Insights {
	return &Insights{store: store, prices: prices}
}

// Runs lists one page of runs, newest first. q.Limit must be positive.
func (s *Insights) Runs(ctx context.Context, q RunQuery) (RunsPage, error) {
	limit := q.Limit
	q.Limit = limit + 1
	rows, err := s.store.ListRuns(ctx, q)
	if err != nil {
		return RunsPage{}, fmt.Errorf("list runs: %w", err)
	}
	var page RunsPage
	if len(rows) > limit {
		rows = rows[:limit]
		next := rows[limit-1].ID
		page.NextCursor = &next
	}
	page.Items, _, err = s.summarize(ctx, rows)
	return page, err
}

// Run returns one run with its steps in seq order.
func (s *Insights) Run(ctx context.Context, id int64) (RunDetail, error) {
	row, err := s.store.GetRun(ctx, id)
	if err != nil {
		return RunDetail{}, fmt.Errorf("get run %d: %w", id, err)
	}
	sums, _, err := s.summarize(ctx, []RunRow{row.RunRow})
	if err != nil {
		return RunDetail{}, err
	}
	steps, err := s.store.ListSteps(ctx, id)
	if err != nil {
		return RunDetail{}, fmt.Errorf("list steps of run %d: %w", id, err)
	}
	return RunDetail{
		RunSummary:  sums[0],
		ScopeJSON:   row.ScopeJSON,
		Instruction: row.Instruction,
		Notes:       row.Notes,
		ErrorKey:    row.ErrorKey,
		PromptText:  row.PromptText,
		Steps:       steps,
	}, nil
}

// summarize adds costs and correction rates to rows with one corrections
// query over the rows' id range, and returns the corrections too.
func (s *Insights) summarize(ctx context.Context, rows []RunRow) ([]RunSummary, map[int64]Correction, error) {
	out := make([]RunSummary, len(rows))
	if len(rows) == 0 {
		return out, nil, nil
	}
	overrides, err := s.prices(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("price overrides: %w", err)
	}
	minID, maxID := idRange(rows)
	corr, err := s.store.Corrections(ctx, minID, maxID)
	if err != nil {
		return nil, nil, fmt.Errorf("corrections: %w", err)
	}
	for i, r := range rows {
		out[i] = RunSummary{
			RunRow:         r,
			CostUSD:        Cost(r.Model, r.Provider, r.Tokens, overrides),
			CorrectionRate: corr[r.ID].Rate(),
		}
	}
	return out, corr, nil
}

func idRange(rows []RunRow) (minID, maxID int64) {
	minID, maxID = rows[0].ID, rows[0].ID
	for _, r := range rows[1:] {
		minID, maxID = min(minID, r.ID), max(maxID, r.ID)
	}
	return minID, maxID
}

// Weekly reports the runs started on the dates [from, to] in Monday-based
// weeks (every week of the range, empty ones included) and grouped by kind,
// prompt version and model.
func (s *Insights) Weekly(ctx context.Context, from, to time.Time) (Report, error) {
	end := to.AddDate(0, 0, 1)
	rows, err := s.store.ListRuns(ctx, RunQuery{From: &from, To: &end, Limit: -1})
	if err != nil {
		return Report{}, fmt.Errorf("list runs: %w", err)
	}
	sums, corr, err := s.summarize(ctx, rows)
	if err != nil {
		return Report{}, err
	}
	return Report{
		Weeks:      weekBuckets(from, to, sums, corr),
		ByVersion:  versionGroups(rows),
		PricesAsOf: PricesAsOf,
	}, nil
}

func monday(t time.Time) time.Time {
	d := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	return d.AddDate(0, 0, -((int(d.Weekday()) + 6) % 7))
}

func weekBuckets(from, to time.Time, sums []RunSummary, corr map[int64]Correction) []WeekBucket {
	var weeks []WeekBucket
	index := map[time.Time]int{}
	for w := monday(from); !w.After(to); w = w.AddDate(0, 0, 7) {
		index[w] = len(weeks)
		weeks = append(weeks, WeekBucket{Start: w})
	}
	generated := make([]Correction, len(weeks))
	for _, r := range sums {
		i, ok := index[monday(r.StartedAt)]
		if !ok {
			continue
		}
		b := &weeks[i]
		b.Runs++
		switch r.Outcome {
		case "accepted", "partially_accepted":
			b.Accepted++
		case "discarded":
			b.Discarded++
		}
		if r.CostUSD == nil {
			b.UnpricedRuns++
		} else {
			b.CostUSD += *r.CostUSD
		}
		c := corr[r.ID]
		generated[i].Generated += c.Generated
		generated[i].Corrected += c.Corrected
	}
	for i := range weeks {
		weeks[i].CorrectionRate = generated[i].Rate()
		if weeks[i].Accepted > 0 {
			v := weeks[i].CostUSD / float64(weeks[i].Accepted)
			weeks[i].CostPerAccepted = &v
		}
	}
	return weeks
}

func versionGroups(rows []RunRow) []VersionGroup {
	type key struct {
		kind           Kind
		version, model string
	}
	type acc struct {
		g      VersionGroup
		sums   map[string]float64
		counts map[string]int
	}
	groups := map[key]*acc{}
	var order []key
	for _, r := range rows {
		k := key{r.Kind, r.PromptVersion, r.Model}
		a, ok := groups[k]
		if !ok {
			a = &acc{
				g:    VersionGroup{Kind: r.Kind, PromptVersion: r.PromptVersion, Model: r.Model, ScoreAvg: map[string]float64{}, Violations: map[string]int{}},
				sums: map[string]float64{}, counts: map[string]int{},
			}
			groups[k] = a
			order = append(order, k)
		}
		a.g.Runs++
		if r.ScoreJSON != "" {
			var score any
			if json.Unmarshal([]byte(r.ScoreJSON), &score) == nil {
				numericLeaves("", score, func(path string, v float64) {
					a.sums[path] += v
					a.counts[path]++
				})
			}
		}
		if r.ViolationsJSON != "" {
			var vs []struct {
				Rule string `json:"rule"`
			}
			if json.Unmarshal([]byte(r.ViolationsJSON), &vs) == nil {
				for _, v := range vs {
					if v.Rule != "" {
						a.g.Violations[v.Rule]++
					}
				}
			}
		}
	}
	sort.SliceStable(order, func(i, j int) bool {
		a, b := order[i], order[j]
		if a.kind != b.kind {
			return a.kind < b.kind
		}
		if a.version != b.version {
			return a.version < b.version
		}
		return a.model < b.model
	})
	out := make([]VersionGroup, len(order))
	for i, k := range order {
		a := groups[k]
		for path, sum := range a.sums {
			a.g.ScoreAvg[path] = sum / float64(a.counts[path])
		}
		out[i] = a.g
	}
	return out
}

// numericLeaves calls fn for every number in v, keyed by its dotted path.
// Arrays are skipped.
func numericLeaves(path string, v any, fn func(string, float64)) {
	switch x := v.(type) {
	case float64:
		if path != "" {
			fn(path, x)
		}
	case map[string]any:
		for k, child := range x {
			p := k
			if path != "" {
				p = path + "." + k
			}
			numericLeaves(p, child, fn)
		}
	}
}

// Price returns model's effective price: an override, a built-in entry or
// none.
func (s *Insights) Price(ctx context.Context, model string) (PriceInfo, error) {
	overrides, err := s.prices(ctx)
	if err != nil {
		return PriceInfo{}, fmt.Errorf("price overrides: %w", err)
	}
	info := PriceInfo{Model: model, AsOf: PricesAsOf}
	p, src := LookupPrice(model, overrides)
	info.Source = src
	if src != PriceNone {
		info.Price = &p
	}
	return info, nil
}
