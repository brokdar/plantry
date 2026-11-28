package chefcanary

import (
	"context"
	"database/sql"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain/airun"
)

// A normal month (SPEC NFR-3): one planned window a week, New dishes 1, the
// daily idea on, plus the use the Chef gets besides that. The mode multiplies
// the mean cost per run of each kind in the run log by these counts.
const (
	// MonthFinds: 4 windows x New dishes 1 (one find each, with its matching).
	MonthFinds = 4
	// MonthLabels: the find kept each window is labelled once (chef_attributes).
	MonthLabels = 4
	// MonthDailyIdeas: the daily idea, one discovery run a day.
	MonthDailyIdeas = 30
	// MonthHelps: two cook-mode questions a week.
	MonthHelps = 8
	// MonthImportChecks: one import check a week.
	MonthImportChecks = 4
)

// MonthConfig is the month mode: the month's cost computed from the run log
// of a database that the other modes (and the labelling canary) ran on.
type MonthConfig struct {
	DB     *sql.DB
	Prices map[string]airun.Price
}

type kindCost struct {
	runs int
	cost float64
	// unpriced runs have no price: they count as runs but cost 0.
	unpriced int
}

func (k kindCost) mean() float64 {
	if k.runs == 0 {
		return 0
	}
	return k.cost / float64(k.runs)
}

// RunMonth computes the month cost:
//
//	month = MonthFinds x cost per find + MonthLabels x mean chef_attributes run
//	      + MonthDailyIdeas x mean daily discovery run + MonthHelps x mean chef_help run
//	      + MonthImportChecks x mean chef_import_check run
//
// where the cost per find is the chef_find plus chef_match cost in the log over
// the number of chef_find runs. A kind with no runs in the log counts 0 and is
// named in the note: run the other modes first.
func RunMonth(ctx context.Context, cfg MonthConfig) (ModeReport, error) {
	runs := sqlite.NewAIRunRepo(cfg.DB)
	costs := map[airun.Kind]*kindCost{}
	models := map[string]bool{}
	for _, k := range []airun.Kind{airun.KindChefFind, airun.KindChefMatch, airun.KindChefAttributes, airun.KindDiscovery, airun.KindChefHelp, airun.KindChefImportCheck} {
		kc := &kindCost{}
		costs[k] = kc
		rows, err := runs.ListRuns(ctx, airun.RunQuery{Kind: k, Limit: 200})
		if err != nil {
			return ModeReport{}, err
		}
		for _, r := range rows {
			if r.Status != airun.StatusOK {
				continue
			}
			if k == airun.KindDiscovery {
				d, err := runs.GetRun(ctx, r.ID)
				if err != nil {
					return ModeReport{}, err
				}
				if !strings.Contains(d.ScopeJSON, `"daily"`) {
					continue
				}
			}
			kc.runs++
			models[r.Model] = true
			if c := airun.Cost(r.Model, r.Provider, r.Tokens, cfg.Prices); c != nil {
				kc.cost += *c
			} else {
				kc.unpriced++
			}
		}
	}
	perFind := 0.0
	if f := costs[airun.KindChefFind]; f.runs > 0 {
		perFind = (f.cost + costs[airun.KindChefMatch].cost) / float64(f.runs)
	}
	parts := []struct {
		name  string
		count int
		each  float64
		kind  airun.Kind
	}{
		{"finds", MonthFinds, perFind, airun.KindChefFind},
		{"labels", MonthLabels, costs[airun.KindChefAttributes].mean(), airun.KindChefAttributes},
		{"daily_ideas", MonthDailyIdeas, costs[airun.KindDiscovery].mean(), airun.KindDiscovery},
		{"helps", MonthHelps, costs[airun.KindChefHelp].mean(), airun.KindChefHelp},
		{"import_checks", MonthImportChecks, costs[airun.KindChefImportCheck].mean(), airun.KindChefImportCheck},
	}
	rep := ModeReport{Mode: "month", Cases: len(parts), Ran: len(parts)}
	total := 0.0
	var missing []string
	for _, p := range parts {
		sub := float64(p.count) * p.each
		total += sub
		rep.Results = append(rep.Results, CaseResult{
			ID: p.name, OK: costs[p.kind].runs > 0, Outcome: fmt.Sprintf("%d x $%.4f", p.count, p.each), CostUSD: sub,
			Detail: fmt.Sprintf("%d runs in the log", costs[p.kind].runs),
		})
		rep.add(Metric{Name: "month." + p.name, Value: sub, Unit: "usd", Note: fmt.Sprintf("%d x $%.4f from %d runs", p.count, p.each, costs[p.kind].runs)})
		if costs[p.kind].runs == 0 {
			missing = append(missing, string(p.kind))
		}
		rep.Unpriced = rep.Unpriced || costs[p.kind].unpriced > 0
	}
	rep.CostUSD = total
	note := "formula in RunMonth"
	if len(missing) > 0 {
		note = "INCOMPLETE, no runs of " + strings.Join(missing, ", ")
	}
	mc := Metric{Name: "month_cost", Value: total, Unit: "usd", Note: note}.target("<= $5", false, 5)
	if len(missing) > 0 {
		notMet := false
		mc.Met = &notMet // a month with a kind missing proves nothing
	}
	rep.add(mc)
	rep.add(Metric{Name: "kinds_without_runs", Value: float64(len(missing)), Note: strings.Join(missing, ", ")})
	rep.Model = strings.Join(slices.Sorted(maps.Keys(models)), " ")
	return rep, nil
}
