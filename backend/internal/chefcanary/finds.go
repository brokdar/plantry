package chefcanary

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/fdclocal"
	"github.com/jaltszeimer/plantry/backend/internal/adapters/jsonld"
	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/chefsets"
	"github.com/jaltszeimer/plantry/backend/internal/domain/diet"
	"github.com/jaltszeimer/plantry/backend/internal/domain/finds"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/importer"
	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
	"github.com/jaltszeimer/plantry/backend/internal/domain/settings"
)

// FindsConfig is the finds mode: the real find pipeline (finds.Service, as
// the API uses it) run once per request on the database.
type FindsConfig struct {
	Env
	Requests []chefsets.FindRequest
	// Fetcher loads the recipe pages (the polite HTTP fetcher live).
	Fetcher finds.Fetcher
	// Matcher matches the lines of a find; nil: the catalogue matcher over
	// the database with the local FDC set and the chef_match pick.
	Matcher importer.Matcher
	// Sites are the searchable sites (nil: the default chef.sites).
	Sites []settings.ChefSite
}

// household is what a request's constraint says about the household: the
// diet, never items, a mild palate, a time limit or German.
func household(r chefsets.FindRequest) finds.Household {
	h := finds.Household{
		Lang: "en", Portions: 4,
		Kitchen: profile.Kitchen{MinutesWeekday: 60, MinutesWeekend: 90, Equipment: []string{"oven", "stovetop"}},
	}
	if r.Lang != "" {
		h.Lang = r.Lang
	}
	if r.Diet != "" {
		h.Diet, h.Gates.Diet = r.Diet, diet.Class(r.Diet)
	}
	h.Gates.Allergens, h.Gates.NeverNames, h.Never, h.RatherNot = r.Allergens, r.Never, r.Never, r.RatherNot
	if r.MaxMinutes > 0 {
		h.Kitchen.MinutesWeekday, h.Kitchen.MinutesWeekend = r.MaxMinutes, r.MaxMinutes
	}
	return h
}

// RunFinds asks for every request and records the outcome, gates, unmatched
// lines, cost, latency and web searches of each find.
func RunFinds(ctx context.Context, cfg FindsConfig) (ModeReport, error) {
	l, err := newLedger(ctx, cfg.Env)
	if err != nil {
		return ModeReport{}, err
	}
	client := l.rec.Meter(cfg.Client, cfg.Provider)
	resolver := llm.StaticResolver(client, cfg.Model)
	foods := sqlite.NewFoodRepo(cfg.DB)
	matcher := cfg.Matcher
	if matcher == nil {
		matcher = food.NewMatcher(foods, fdclocal.New(), nil).WithPicker(resolver, l.rec,
			func(context.Context, bool) (food.MatchPermit, error) { return food.MatchPermit{Run: true}, nil })
	}
	sites := cfg.Sites
	if sites == nil {
		sites = settings.DefaultChefSites()
	}
	var current finds.Household
	svc := finds.NewService(finds.Deps{
		Store: sqlite.NewFindRepo(cfg.DB), LLM: resolver, Runs: l.rec, Fetcher: cfg.Fetcher,
		Extractor: importer.NewService(nil, jsonld.Extractor{}, resolver, nil, nil).WithRecorder(l.rec),
		Matcher:   matcher,
		Household: func(context.Context) (finds.Household, error) { return current, nil },
		Sites:     func(context.Context) ([]settings.ChefSite, error) { return sites, nil },
		Enqueue:   func(context.Context, int64, bool) error { return nil },
		Logger:    cfg.logger(),
	})

	rep := ModeReport{Mode: "finds", Cases: len(cfg.Requests)}
	var costs, latencies []float64
	readyUnmatched := 0
	for _, req := range cfg.Requests {
		if l.capReached() {
			rep.StoppedBySpendCap = true
			break
		}
		current = household(req)
		start := time.Now()
		f, err := svc.Request(ctx, req.Dish, finds.TriggerChat)
		if err != nil {
			return ModeReport{}, fmt.Errorf("request %s: %w", req.ID, err)
		}
		runErr := svc.Run(ctx, f.ID)
		elapsed := time.Since(start)
		f, err = svc.Get(ctx, f.ID)
		if err != nil {
			return ModeReport{}, err
		}
		u, err := l.settle(ctx)
		if err != nil {
			return ModeReport{}, err
		}
		res := CaseResult{
			ID: req.ID, Groups: []string{req.Theme, req.Constraint}, Outcome: string(f.State), OK: f.State == finds.StateReady,
			CostUSD: u.CostUSD, LatencyMS: elapsed.Milliseconds(), WebSearches: u.WebSearches,
		}
		blocks, flags := codes(f)
		unmatched := 0
		for _, b := range blocks {
			if b == "unmatched_ingredient" {
				unmatched++
			}
		}
		if f.State == finds.StateReady {
			unmatched += unmatchedLines(f.Draft)
			if unmatched > 0 {
				readyUnmatched++
			}
		}
		res.Detail = fmt.Sprintf("reason=%s blocks=%v flags=%v unmatched=%d", reason(f, runErr), blocks, flags, unmatched)
		rep.Results = append(rep.Results, res)
		rep.Ran++
		costs, latencies = append(costs, u.CostUSD), append(latencies, elapsed.Seconds())
	}
	l.finish(&rep)

	ready := 0
	for _, c := range rep.Results {
		if c.OK {
			ready++
		}
	}
	rep.add(Metric{Name: "ready_rate", Value: pct(ready, rep.Ran), Unit: "%", Note: fmt.Sprintf("%d/%d", ready, rep.Ran)})
	byGroup(&rep, "ready_rate.theme.", func(c CaseResult) string { return c.Groups[0] })
	byGroup(&rep, "ready_rate.constraint.", func(c CaseResult) string { return c.Groups[1] })
	rep.add(Metric{Name: "ready_finds_with_unmatched_lines", Value: float64(readyUnmatched), Note: "must be 0"}.target("0", false, 0))
	rep.add(Metric{Name: "cost_per_find.p50", Value: percentile(costs, 50), Unit: "usd"})
	rep.add(Metric{Name: "cost_per_find.p90", Value: percentile(costs, 90), Unit: "usd"})
	rep.add(Metric{Name: "latency.p90", Value: percentile(latencies, 90), Unit: "s"})
	rep.add(Metric{Name: "web_searches_per_find", Value: webMean(rep.Results)})
	return rep, nil
}

func webMean(rs []CaseResult) float64 {
	if len(rs) == 0 {
		return 0
	}
	var n int64
	for _, r := range rs {
		n += r.WebSearches
	}
	return float64(n) / float64(len(rs))
}

func codes(f *finds.Find) (blocks, flags []string) {
	for _, b := range f.Gates.Blocks {
		blocks = append(blocks, b.Code)
	}
	for _, g := range f.Gates.Flags {
		flags = append(flags, g.Code)
	}
	return blocks, flags
}

// unmatchedLines counts the draft's lines (not to taste) with no matched food.
func unmatchedLines(d *importer.Draft) int {
	if d == nil {
		return 0
	}
	n := 0
	for _, in := range d.Ingredients {
		if !in.ToTaste && (in.Match == nil || in.Match.Status != food.MatchMatched) {
			n++
		}
	}
	return n
}

// reason is why a find did not end ready: the run's error, the first gate
// block, else the last pipeline step.
func reason(f *finds.Find, runErr error) string {
	switch {
	case f.State == finds.StateReady:
		return ""
	case runErr != nil:
		return "run_error"
	case len(f.Gates.Blocks) > 0:
		return f.Gates.Blocks[0].Code
	case len(f.Steps) > 0:
		return f.Steps[len(f.Steps)-1].Key
	}
	return strings.TrimSpace(string(f.State))
}
