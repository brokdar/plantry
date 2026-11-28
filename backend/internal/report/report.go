// Package report renders `plantry report`: a Markdown summary of how the app
// ran on a server, for the developer who never sees it. It holds counts,
// rates, durations and dish names only: no keys, prompts or text the user
// typed. Collecting the numbers is the SQLite adapter's job (Data comes from
// sqlite.ReportRepo); this package renders them and picks the problems.
package report

import (
	"fmt"
	"slices"
	"strings"
	"time"
)

// Thresholds of the Problems section.
const (
	ErrorRateLimit  = 0.10
	HelpP90Limit    = 10_000 // ms
	SpendShareLimit = 0.80
	JobFailLimit    = 3
)

// Setting is one setting worth showing. Value is "" when it is not stored in
// the database (the default or an environment variable applies).
type Setting struct{ Key, Value string }

// Build is the "Build" section.
type Build struct {
	Version       string
	SchemaVersion int
	DBBytes       int64
	Settings      []Setting
}

// KeyCount is a count per key (an error key, a reason, a state).
type KeyCount struct {
	Key string
	N   int
}

// KindStat is one run kind over the window. P50 and P90 are milliseconds,
// valid when Timed > 0.
type KindStat struct {
	Kind      string
	Runs      int
	Errors    int
	TopErrors []KeyCount
	Timed     int
	P50, P90  int64
	USD       float64
}

// ModelUse is a provider and model with the runs that used it.
type ModelUse struct {
	Provider, Model string
	Runs            int
}

// MonthSpend is the capped Chef spend of one month ("2006-01").
type MonthSpend struct {
	Month string
	USD   float64
}

// AI is the "AI" section.
type AI struct {
	Kinds     []KindStat
	Models    []ModelUse
	Untracked int
	Spend     []MonthSpend
	Cap       float64
	// CurrentMonth is the month of GeneratedAt ("2006-01").
	CurrentMonth string
}

// Funnel follows the finds accepted in the window.
type Funnel struct{ Accepted, InLibrary, Cooked, Rated int }

// Trial counts the recipes the Chef put on trial.
type Trial struct{ Total, Archived, Cooked int }

// ImportChecks sums the import-check outcomes. Flags are never applied or
// dismissed, so FlagsIgnored counts those a saved recipe left alone. Checks
// is the number of checks that ran; Saved of them ended in a saved recipe.
type ImportChecks struct {
	Checks, Saved         int
	Kept, Undone          []KeyCount
	TouchAdded, TouchSkip int
	FlagsIgnored          int
}

// Chef is the "Chef outcomes" section.
type Chef struct {
	HelpHelped, HelpNotHelped, HelpNoVerdict int
	FindStates                               []KeyCount // "state/status"
	FindRejectReasons                        []KeyCount
	Funnel                                   Funnel
	Trial                                    Trial
	Tips                                     []KeyCount
	Import                                   ImportChecks
	Jobs                                     []KeyCount // "kind/status"
}

// Planning is the "Planning" section.
type Planning struct {
	Generate, Regenerate int
	Heuristic, LLM       int
	// Evaluated is the number of runs with a closing evaluation.
	Evaluated  int
	Violations []KeyCount // issue code
}

// RatingCounts counts rated plates by status.
type RatingCounts struct{ Loved, Disliked, NotEaten int }

// Ratings is the "Ratings" section.
type Ratings struct{ Chef, Other RatingCounts }

// Data is everything the report shows.
type Data struct {
	GeneratedAt time.Time
	Days        int
	Build       Build
	AI          AI
	Chef        Chef
	Planning    Planning
	Ratings     Ratings
}

// Render writes the report as Markdown.
func Render(d Data) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Plantry report\n\nGenerated %s, window: last %d days.\n", d.GeneratedAt.UTC().Format("2006-01-02 15:04 UTC"), d.Days)
	renderProblems(&b, d)
	renderBuild(&b, d.Build)
	renderAI(&b, d.AI)
	renderChef(&b, d.Chef)
	renderPlanning(&b, d.Planning)
	renderRatings(&b, d.Ratings)
	return b.String()
}

// Problems lists what to look at first; empty when nothing stands out.
func Problems(d Data) []string {
	var out []string
	for _, k := range d.AI.Kinds {
		if k.Runs > 0 && float64(k.Errors)/float64(k.Runs) > ErrorRateLimit {
			out = append(out, fmt.Sprintf("%s: %d of %d runs failed (%s)", k.Kind, k.Errors, k.Runs, pct(k.Errors, k.Runs)))
		}
		if k.Kind == "chef_help" && k.Timed > 0 && k.P90 > HelpP90Limit {
			out = append(out, fmt.Sprintf("chef_help: p90 is %s (over %s)", seconds(k.P90), seconds(HelpP90Limit)))
		}
	}
	if d.AI.Untracked > 0 {
		out = append(out, fmt.Sprintf("untracked AI runs: %d (should be 0): a model call ran without a run record", d.AI.Untracked))
	}
	for _, m := range d.AI.Spend {
		if m.Month == d.AI.CurrentMonth && d.AI.Cap > 0 && m.USD > SpendShareLimit*d.AI.Cap {
			out = append(out, fmt.Sprintf("Chef spend this month is $%.2f of the $%.2f cap (%.0f %%)", m.USD, d.AI.Cap, 100*m.USD/d.AI.Cap))
		}
	}
	failed := map[string]int{}
	for _, j := range d.Chef.Jobs {
		if kind, status, ok := strings.Cut(j.Key, "/"); ok && status == "failed" {
			failed[kind] += j.N
		}
	}
	for _, kind := range sortedKeys(failed) {
		if failed[kind] >= JobFailLimit {
			out = append(out, fmt.Sprintf("Chef job %s failed %d times", kind, failed[kind]))
		}
	}
	return out
}

func renderProblems(b *strings.Builder, d Data) {
	b.WriteString("\n## Problems\n\n")
	ps := Problems(d)
	if len(ps) == 0 {
		b.WriteString("Nothing stands out.\n")
		return
	}
	for _, p := range ps {
		fmt.Fprintf(b, "- %s\n", p)
	}
}

func renderBuild(b *strings.Builder, x Build) {
	b.WriteString("\n## Build\n\n")
	fmt.Fprintf(b, "- App version: %s\n- Schema version: %d\n- Database size: %.1f MB\n", orDash(x.Version), x.SchemaVersion, float64(x.DBBytes)/1e6)
	for _, s := range x.Settings {
		v := s.Value
		if v == "" {
			v = "(not set: default or environment)"
		}
		fmt.Fprintf(b, "- %s: %s\n", s.Key, v)
	}
}

func renderAI(b *strings.Builder, a AI) {
	b.WriteString("\n## AI\n\n")
	if len(a.Kinds) == 0 {
		b.WriteString("No data: no AI runs in the window.\n")
	} else {
		b.WriteString("| Kind | Runs | Errors | p50 | p90 | Cost |\n|---|---:|---:|---:|---:|---:|\n")
		for _, k := range a.Kinds {
			p50, p90 := "-", "-"
			if k.Timed > 0 {
				p50, p90 = seconds(k.P50), seconds(k.P90)
			}
			fmt.Fprintf(b, "| %s | %d | %d (%s) | %s | %s | $%.4f |\n", k.Kind, k.Runs, k.Errors, pct(k.Errors, k.Runs), p50, p90, k.USD)
		}
		for _, k := range a.Kinds {
			if len(k.TopErrors) > 0 {
				fmt.Fprintf(b, "\nTop errors of %s: %s\n", k.Kind, joinCounts(k.TopErrors, 3))
			}
		}
	}
	if len(a.Models) > 0 {
		parts := make([]string, len(a.Models))
		for i, m := range a.Models {
			parts[i] = fmt.Sprintf("%s/%s (%d)", m.Provider, m.Model, m.Runs)
		}
		fmt.Fprintf(b, "\nModels used: %s\n", strings.Join(parts, ", "))
	}
	fmt.Fprintf(b, "\nUntracked runs: %d\n", a.Untracked)
	b.WriteString("\nChef spend per month (capped kinds):\n\n")
	if len(a.Spend) == 0 {
		b.WriteString("No data.\n")
	}
	for _, m := range a.Spend {
		fmt.Fprintf(b, "- %s: $%.4f of a $%.2f cap\n", m.Month, m.USD, a.Cap)
	}
}

func renderChef(b *strings.Builder, c Chef) {
	b.WriteString("\n## Chef outcomes\n\n")
	if helpTotal := c.HelpHelped + c.HelpNotHelped + c.HelpNoVerdict; helpTotal == 0 {
		b.WriteString("- Help answers: no data\n")
	} else {
		fmt.Fprintf(b, "- Help answers: %d (helped %d, didn't help %d, no verdict %d; verdict rate %s)\n",
			helpTotal, c.HelpHelped, c.HelpNotHelped, c.HelpNoVerdict, pct(c.HelpHelped+c.HelpNotHelped, helpTotal))
	}
	fmt.Fprintf(b, "- Finds by state/status: %s\n", orNoData(joinCounts(c.FindStates, 0)))
	fmt.Fprintf(b, "- Find reject reasons: %s\n", orNoData(joinCounts(c.FindRejectReasons, 0)))
	f := c.Funnel
	fmt.Fprintf(b, "- Accepted finds: %d, still in the library %d, cooked %d, rated %d\n", f.Accepted, f.InLibrary, f.Cooked, f.Rated)
	if c.Trial.Total == 0 {
		b.WriteString("- Trial dishes: no data\n")
	} else {
		fmt.Fprintf(b, "- Trial dishes: %d (kept %d, removed %d, cooked %d)\n", c.Trial.Total, c.Trial.Total-c.Trial.Archived, c.Trial.Archived, c.Trial.Cooked)
	}
	fmt.Fprintf(b, "- Tips by state: %s\n", orNoData(joinCounts(c.Tips, 0)))
	im := c.Import
	if im.Checks == 0 && im.Saved == 0 {
		b.WriteString("- Import checks: no data\n")
	} else {
		fmt.Fprintf(b, "- Import checks: %d ran, %d ended in a saved recipe (no outcome: %d)\n", im.Checks, im.Saved, max(im.Checks-im.Saved, 0))
		fmt.Fprintf(b, "  - changes kept: %s; undone: %s\n", orNoData(joinCounts(im.Kept, 0)), orNoData(joinCounts(im.Undone, 0)))
		fmt.Fprintf(b, "  - finishing touch: added %d, skipped %d; flags ignored: %d\n", im.TouchAdded, im.TouchSkip, im.FlagsIgnored)
	}
	fmt.Fprintf(b, "- Chef jobs by kind/status: %s\n", orNoData(joinCounts(c.Jobs, 0)))
}

func renderPlanning(b *strings.Builder, p Planning) {
	b.WriteString("\n## Planning\n\n")
	if p.Generate+p.Regenerate == 0 {
		b.WriteString("No data: no plan runs in the window.\n")
		return
	}
	fmt.Fprintf(b, "- Runs: generate %d, regenerate %d; chooser heuristic %d, llm %d\n", p.Generate, p.Regenerate, p.Heuristic, p.LLM)
	if p.Evaluated == 0 {
		b.WriteString("- Violations in the final plans: no data (no closing evaluation stored)\n")
		return
	}
	fmt.Fprintf(b, "- Violations in the final plans (%d evaluated): %s\n", p.Evaluated, orNone(joinCounts(p.Violations, 0)))
}

func renderRatings(b *strings.Builder, r Ratings) {
	b.WriteString("\n## Ratings\n\n")
	row := func(name string, c RatingCounts) string {
		n := c.Loved + c.Disliked + c.NotEaten
		if n == 0 {
			return fmt.Sprintf("| %s | 0 | - | - | - |\n", name)
		}
		return fmt.Sprintf("| %s | %d | %s | %s | %s |\n", name, n, pct(c.Loved, n), pct(c.Disliked, n), pct(c.NotEaten, n))
	}
	if r.Chef == (RatingCounts{}) && r.Other == (RatingCounts{}) {
		b.WriteString("No data: no rated plates in the window.\n")
		return
	}
	b.WriteString("| Plates | Rated | Loved | Disliked | Not eaten |\n|---|---:|---:|---:|---:|\n")
	b.WriteString(row("With a Chef dish", r.Chef))
	b.WriteString(row("Others", r.Other))
}

func pct(n, of int) string {
	if of == 0 {
		return "-"
	}
	return fmt.Sprintf("%.0f %%", 100*float64(n)/float64(of))
}

func seconds(ms int64) string { return fmt.Sprintf("%.1f s", float64(ms)/1000) }

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func orNoData(s string) string {
	if s == "" {
		return "no data"
	}
	return s
}

func orNone(s string) string {
	if s == "" {
		return "none"
	}
	return s
}

// joinCounts renders "key n, key n"; limit > 0 keeps the first limit.
func joinCounts(cs []KeyCount, limit int) string {
	if limit > 0 && len(cs) > limit {
		cs = cs[:limit]
	}
	parts := make([]string, len(cs))
	for i, c := range cs {
		key := c.Key
		if key == "" {
			key = "(none)"
		}
		parts[i] = fmt.Sprintf("%s %d", key, c.N)
	}
	return strings.Join(parts, ", ")
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}
