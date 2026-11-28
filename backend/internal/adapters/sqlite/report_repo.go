package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite/sqlcgen"
	"github.com/jaltszeimer/plantry/backend/internal/domain/chef"
	"github.com/jaltszeimer/plantry/backend/internal/domain/settings"
	"github.com/jaltszeimer/plantry/backend/internal/report"
)

// ReportOpts is what Collect needs besides the database.
type ReportOpts struct {
	Now     time.Time
	Days    int
	Version string
	DBBytes int64
}

// reportSettings are the settings the report shows (none holds a secret).
var reportSettings = []string{
	settings.KeyChefEnabled, settings.KeyAIProvider, settings.KeyAIModel, settings.KeyChefMonthlyCap,
	settings.KeyAIRunRetentionDays, settings.KeyChefLibraryReview, settings.KeyChefNewDishesPerWeek,
	settings.KeyDiscoveryDailyEnabled, settings.KeyAIProgrammaticTools,
}

// defaultMonthlyCap is chef.monthly_cap while the setting is unset.
const defaultMonthlyCap = 5.0

// ReportRepo reads the aggregates of `plantry report`. It only reads.
type ReportRepo struct {
	conn
	db *sql.DB
}

// NewReportRepo creates a ReportRepo.
func NewReportRepo(db *sql.DB) *ReportRepo { return &ReportRepo{conn: newConn(db), db: db} }

// Collect gathers the report data of the last o.Days days.
func (r *ReportRepo) Collect(ctx context.Context, o ReportOpts) (report.Data, error) {
	q := r.queriesFor(ctx)
	from := o.Now.AddDate(0, 0, -o.Days)
	fromTs := formatTime(from)
	d := report.Data{GeneratedAt: o.Now, Days: o.Days}

	build, capUSD, err := r.collectBuild(ctx, q, o)
	if err != nil {
		return d, err
	}
	d.Build = build
	stats, err := q.ReportRunStats(ctx, fromTs)
	if err != nil {
		return d, fmt.Errorf("report run stats: %w", err)
	}
	if d.AI, err = r.collectAI(ctx, q, o, fromTs, capUSD, stats); err != nil {
		return d, err
	}
	if d.Chef, err = r.collectChef(ctx, q, from, o.Now, fromTs, stats); err != nil {
		return d, err
	}
	if d.Planning, err = r.collectPlanning(ctx, q, fromTs, stats); err != nil {
		return d, err
	}
	if d.Ratings, err = r.collectRatings(ctx, q, fromTs); err != nil {
		return d, err
	}
	return d, nil
}

func (r *ReportRepo) collectBuild(ctx context.Context, q *sqlcgen.Queries, o ReportOpts) (report.Build, float64, error) {
	b := report.Build{Version: o.Version, DBBytes: o.DBBytes}
	var v sql.NullInt64
	if err := r.db.QueryRowContext(ctx, `SELECT MAX(version_id) FROM goose_db_version WHERE is_applied = 1`).Scan(&v); err != nil {
		return b, 0, fmt.Errorf("report schema version: %w", err)
	}
	b.SchemaVersion = int(v.Int64)
	rows, err := q.ListSettings(ctx)
	if err != nil {
		return b, 0, fmt.Errorf("report settings: %w", err)
	}
	stored := map[string]string{}
	for _, s := range rows {
		if s.Encrypted == 0 && slices.Contains(reportSettings, s.Key) {
			stored[s.Key] = s.Value
		}
	}
	for _, k := range reportSettings {
		b.Settings = append(b.Settings, report.Setting{Key: k, Value: stored[k]})
	}
	capUSD := defaultMonthlyCap
	if raw, ok := stored[settings.KeyChefMonthlyCap]; ok {
		if c, err := settings.ParseMonthlyCap(raw); err == nil {
			capUSD = c
		}
	}
	return b, capUSD, nil
}

func (r *ReportRepo) collectAI(ctx context.Context, q *sqlcgen.Queries, o ReportOpts, fromTs string, capUSD float64, stats []sqlcgen.ReportRunStatsRow) (report.AI, error) {
	a := report.AI{Cap: capUSD, CurrentMonth: o.Now.UTC().Format("2006-01")}
	byKind := map[string]*report.KindStat{}
	for _, s := range stats {
		k := byKind[s.Kind]
		if k == nil {
			k = &report.KindStat{Kind: s.Kind}
			byKind[s.Kind] = k
		}
		k.Runs += int(s.N)
		k.USD += s.Usd
		if s.Status == "error" {
			k.Errors += int(s.N)
		}
		if s.Kind == "untracked" {
			a.Untracked += int(s.N)
		}
	}
	errs, err := q.ReportRunErrors(ctx, fromTs)
	if err != nil {
		return a, fmt.Errorf("report run errors: %w", err)
	}
	for _, e := range errs {
		k := byKind[e.Kind]
		if k == nil { // a run that finished between the queries
			continue
		}
		k.TopErrors = append(k.TopErrors, report.KeyCount{Key: e.ErrorKey, N: int(e.N)})
	}
	durs, err := q.ReportRunDurations(ctx, fromTs)
	if err != nil {
		return a, fmt.Errorf("report run durations: %w", err)
	}
	perKind := map[string][]int64{}
	for _, d := range durs {
		perKind[d.Kind] = append(perKind[d.Kind], d.DurationMs.Int64)
	}
	for kind, ms := range perKind { // sorted ascending by the query
		k := byKind[kind]
		if k == nil {
			continue
		}
		k.Timed, k.P50, k.P90 = len(ms), percentile(ms, 0.5), percentile(ms, 0.9)
	}
	for _, kind := range slices.Sorted(mapKeys(byKind)) {
		a.Kinds = append(a.Kinds, *byKind[kind])
	}
	models, err := q.ReportRunModels(ctx, fromTs)
	if err != nil {
		return a, fmt.Errorf("report run models: %w", err)
	}
	for _, m := range models {
		a.Models = append(a.Models, report.ModelUse{Provider: m.Provider, Model: m.Model, Runs: int(m.N)})
	}
	kinds := make([]string, len(chef.CappedKinds))
	for i, k := range chef.CappedKinds {
		kinds[i] = string(k)
	}
	spend, err := q.ReportChefSpendByMonth(ctx, sqlcgen.ReportChefSpendByMonthParams{
		FromMonth: o.Now.AddDate(0, 0, -o.Days).UTC().Format("2006-01"), Kinds: kinds,
	})
	if err != nil {
		return a, fmt.Errorf("report chef spend: %w", err)
	}
	for _, s := range spend {
		a.Spend = append(a.Spend, report.MonthSpend{Month: s.Month, USD: s.Usd})
	}
	return a, nil
}

func (r *ReportRepo) collectChef(ctx context.Context, q *sqlcgen.Queries, from, now time.Time, fromTs string, stats []sqlcgen.ReportRunStatsRow) (report.Chef, error) {
	var c report.Chef
	verdicts, err := q.ReportHelpVerdicts(ctx, fromTs)
	if err != nil {
		return c, fmt.Errorf("report help verdicts: %w", err)
	}
	for _, v := range verdicts {
		switch v.Verdict {
		case "helped":
			c.HelpHelped = int(v.N)
		case "not_helped":
			c.HelpNotHelped = int(v.N)
		default:
			c.HelpNoVerdict = int(v.N)
		}
	}
	states, err := q.ReportFindStates(ctx, fromTs)
	if err != nil {
		return c, fmt.Errorf("report find states: %w", err)
	}
	for _, s := range states {
		key := s.Status
		if s.State != "" {
			key = s.State + "/" + s.Status
		}
		c.FindStates = append(c.FindStates, report.KeyCount{Key: key, N: int(s.N)})
	}
	reasons, err := q.ReportFindRejectReasons(ctx, fromTs)
	if err != nil {
		return c, fmt.Errorf("report find reasons: %w", err)
	}
	for _, s := range reasons {
		c.FindRejectReasons = append(c.FindRejectReasons, report.KeyCount{Key: s.Reason, N: int(s.N)})
	}
	f, err := q.ReportFindFunnel(ctx, fromTs)
	if err != nil {
		return c, fmt.Errorf("report find funnel: %w", err)
	}
	c.Funnel = report.Funnel{Accepted: int(f.Accepted), InLibrary: int(f.InLibrary), Cooked: int(f.Cooked), Rated: int(f.Rated)}
	t, err := q.ReportTrialFoods(ctx)
	if err != nil {
		return c, fmt.Errorf("report trial foods: %w", err)
	}
	c.Trial = report.Trial{Total: int(t.Total), Archived: int(t.Archived), Cooked: int(t.Cooked)}
	tips, err := q.CountChefTipsCreatedBetween(ctx, sqlcgen.CountChefTipsCreatedBetweenParams{
		CreatedAt: formatTime(from), CreatedAt_2: formatTime(now.AddDate(0, 0, 1)),
	})
	if err != nil {
		return c, fmt.Errorf("report tips: %w", err)
	}
	for _, s := range tips {
		c.Tips = append(c.Tips, report.KeyCount{Key: s.State, N: int(s.N)})
	}
	if c.Import, err = r.collectImport(ctx, q, fromTs, stats); err != nil {
		return c, err
	}
	jobs, err := q.ReportJobs(ctx, fromTs)
	if err != nil {
		return c, fmt.Errorf("report jobs: %w", err)
	}
	for _, j := range jobs {
		c.Jobs = append(c.Jobs, report.KeyCount{Key: j.Kind + "/" + j.Status, N: int(j.N)})
	}
	return c, nil
}

func (r *ReportRepo) collectImport(ctx context.Context, q *sqlcgen.Queries, fromTs string, stats []sqlcgen.ReportRunStatsRow) (report.ImportChecks, error) {
	var im report.ImportChecks
	for _, s := range stats {
		if s.Kind == "chef_import_check" && s.Status == "ok" {
			im.Checks += int(s.N)
		}
	}
	rows, err := q.ReportImportOutcomes(ctx, fromTs)
	if err != nil {
		return im, fmt.Errorf("report import outcomes: %w", err)
	}
	kept, undone := map[string]int{}, map[string]int{}
	for _, o := range rows {
		im.Saved++
		im.FlagsIgnored += int(o.Flags)
		switch o.Touch {
		case "added":
			im.TouchAdded++
		case "skipped":
			im.TouchSkip++
		}
		for _, p := range []struct {
			raw string
			to  map[string]int
		}{{o.Kept, kept}, {o.Undone, undone}} {
			var kinds []string
			if err := json.Unmarshal([]byte(p.raw), &kinds); err != nil {
				return im, fmt.Errorf("report import outcome kinds: %w", err)
			}
			for _, k := range kinds {
				p.to[k]++
			}
		}
	}
	im.Checks = max(im.Checks, im.Saved)
	im.Kept, im.Undone = countsOf(kept), countsOf(undone)
	return im, nil
}

func (r *ReportRepo) collectPlanning(ctx context.Context, q *sqlcgen.Queries, fromTs string, stats []sqlcgen.ReportRunStatsRow) (report.Planning, error) {
	var p report.Planning
	for _, s := range stats {
		switch s.Kind {
		case "generate":
			p.Generate += int(s.N)
		case "regenerate":
			p.Regenerate += int(s.N)
		default:
			continue
		}
		switch s.Chooser {
		case "heuristic":
			p.Heuristic += int(s.N)
		case "llm":
			p.LLM += int(s.N)
		}
	}
	evals, err := q.ReportClosingEvaluations(ctx, fromTs)
	if err != nil {
		return p, fmt.Errorf("report closing evaluations: %w", err)
	}
	codes := map[string]int{}
	for _, e := range evals {
		var ev struct {
			Issues []struct {
				Code string `json:"code"`
			} `json:"issues"`
		}
		if err := json.Unmarshal([]byte(e.ResultJson.String), &ev); err != nil {
			continue // not an evaluation (an encode error result): not counted
		}
		p.Evaluated++
		for _, is := range ev.Issues {
			codes[is.Code]++
		}
	}
	p.Violations = countsOf(codes)
	return p, nil
}

func (r *ReportRepo) collectRatings(ctx context.Context, q *sqlcgen.Queries, fromTs string) (report.Ratings, error) {
	var out report.Ratings
	rows, err := q.ReportRatings(ctx, fromTs)
	if err != nil {
		return out, fmt.Errorf("report ratings: %w", err)
	}
	for _, row := range rows {
		c := &out.Other
		if row.ChefOrigin == 1 {
			c = &out.Chef
		}
		switch row.Status {
		case "loved":
			c.Loved += int(row.N)
		case "disliked":
			c.Disliked += int(row.N)
		case "not_eaten":
			c.NotEaten += int(row.N)
		}
	}
	return out, nil
}

// percentile is the nearest-rank p-quantile of ascending xs (0 when empty).
func percentile(xs []int64, p float64) int64 {
	if len(xs) == 0 {
		return 0
	}
	i := int(math.Ceil(p*float64(len(xs)))) - 1
	return xs[max(i, 0)]
}

// countsOf lists a count map, most frequent first, then by key.
func countsOf(m map[string]int) []report.KeyCount {
	out := make([]report.KeyCount, 0, len(m))
	for k, n := range m {
		out = append(out, report.KeyCount{Key: k, N: n})
	}
	slices.SortFunc(out, func(a, b report.KeyCount) int {
		if a.N != b.N {
			return b.N - a.N
		}
		return strings.Compare(a.Key, b.Key)
	})
	return out
}

func mapKeys[V any](m map[string]V) func(yield func(string) bool) {
	return func(yield func(string) bool) {
		for k := range m {
			if !yield(k) {
				return
			}
		}
	}
}
