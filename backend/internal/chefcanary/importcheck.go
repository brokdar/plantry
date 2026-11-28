package chefcanary

import (
	"context"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/jsonld"
	"github.com/jaltszeimer/plantry/backend/internal/chefsets"
	"github.com/jaltszeimer/plantry/backend/internal/domain/importer"
	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
)

// ImportCheckConfig is the importcheck mode: planted-error recipes and clean
// ones through the B3 import check (importer.Checker, as the import uses it).
type ImportCheckConfig struct {
	Env
	Set chefsets.ImportCheckSet
}

// expectation says where a flag has to be to count as finding the planted
// error: on the ingredient line, or (a missing step, mis-scaled servings: the
// error belongs to the whole recipe) on the recipe.
type expectation struct {
	Recipe     bool
	Ingredient int
}

// ImportCase is one draft the check is run on: clean, or with one planted error.
type ImportCase struct {
	ID      string
	Planted *chefsets.Planted
	Expect  expectation
	Draft   *importer.Draft
	// Original is the planted ingredient line before the error was planted:
	// the value a correct fix puts back.
	Original *importer.DraftIngredient
}

// ImportCases builds the clean drafts (one per base recipe) and the planted
// ones. Every planted case has an expectation by construction.
func ImportCases(set chefsets.ImportCheckSet) []ImportCase {
	base := map[string]chefsets.CheckRecipe{}
	for _, r := range set.Recipes {
		base[r.ID] = r
	}
	var out []ImportCase
	for _, r := range set.Recipes {
		out = append(out, ImportCase{ID: r.ID + "-clean", Draft: draftOf(r)})
	}
	for _, p := range set.Planted {
		d := draftOf(base[p.Base])
		c := ImportCase{ID: p.ID, Planted: &p, Draft: d}
		switch p.Kind {
		case chefsets.PlantAmountX10, chefsets.PlantWrongUnit, chefsets.PlantSwapIngr:
			orig := d.Ingredients[p.Ingredient]
			c.Original = &orig
		}
		switch p.Kind {
		case chefsets.PlantAmountX10:
			in := &d.Ingredients[p.Ingredient]
			in.Amount *= 10
			in.RawText = fmt.Sprintf("%g %s %s", in.Amount, in.Unit, in.Name)
			c.Expect = expectation{Ingredient: p.Ingredient}
		case chefsets.PlantWrongUnit:
			in := &d.Ingredients[p.Ingredient]
			in.Unit, in.OriginalUnit = p.ToUnit, p.ToUnit
			in.RawText = fmt.Sprintf("%g %s %s", in.Amount, in.Unit, in.Name)
			c.Expect = expectation{Ingredient: p.Ingredient}
		case chefsets.PlantSwapIngr:
			in := &d.Ingredients[p.Ingredient]
			in.Name = p.ToName
			in.RawText = fmt.Sprintf("%g %s %s", in.Amount, in.Unit, in.Name)
			c.Expect = expectation{Ingredient: p.Ingredient}
		case chefsets.PlantDropStep:
			d.Instructions = slices.Delete(d.Instructions, p.Step, p.Step+1)
			c.Expect = expectation{Recipe: true}
		case chefsets.PlantScaleServe:
			d.ReferencePortions *= p.Factor
			c.Expect = expectation{Recipe: true}
		}
		out = append(out, c)
	}
	return out
}

func draftOf(r chefsets.CheckRecipe) *importer.Draft {
	prep, cook := r.Prep, r.Cook
	total := prep + cook
	d := &importer.Draft{
		Name: r.Name, ReferencePortions: r.Portions, Language: r.Language, ExtractMethod: "jsonld",
		PrepMinutes: &prep, CookMinutes: &cook, TotalMinutes: &total,
		Instructions: slices.Clone(r.Steps),
	}
	for _, in := range r.Ingredients {
		d.Ingredients = append(d.Ingredients, importer.DraftIngredient{
			RawText: in.Text, Amount: in.Amount, Unit: in.Unit, OriginalUnit: in.Unit, Name: in.Name, Confidence: importer.ConfidenceParsed,
		})
	}
	return d
}

// found reports whether a flag of the check is where the planted error is.
func (c ImportCase) found(flags []importer.Flag) bool {
	return slices.ContainsFunc(flags, func(f importer.Flag) bool {
		if f.Clarity {
			return false
		}
		if c.Expect.Recipe {
			return f.Ingredient < 0
		}
		return f.Ingredient == c.Expect.Ingredient
	})
}

// Verdicts of an applied change.
const (
	verdictAllowed  = "allowed"     // a kind the set allows
	verdictCorrect  = "correct_fix" // restores the planted line
	verdictWrong    = "wrong"
	verdictToJudge  = "to_judge" // a designed change whose quality needs a reader
	judgeFileName   = "importcheck-changes.md"
	amountTolerance = 0.01
)

// ChangeRecord is one applied change of the check, as the result file keeps it.
type ChangeRecord struct {
	Kind    string `json:"kind"`
	Reason  string `json:"reason,omitempty"`
	Field   string `json:"field"`
	Index   int    `json:"index"` // the step or ingredient, -1 for the recipe
	Before  any    `json:"before"`
	After   any    `json:"after"`
	Verdict string `json:"verdict"`
}

// restores reports whether an ingredient edit puts the planted line back to
// its original amount and unit.
func (c ImportCase) restores(e importer.Edit) bool {
	v, ok := e.After.(importer.IngredientValue)
	if !ok || c.Original == nil {
		return false
	}
	return v.Unit == c.Original.Unit && math.Abs(v.Amount-c.Original.Amount) <= amountTolerance*c.Original.Amount
}

// classify records every edit of the applied changes with its verdict. A kind
// the set allows is fine; clarity rewrites, safety cues and the other designed
// metadata changes go to the judge file; a change of an ingredient line is a
// correct fix only when it restores the planted line, any other one is wrong.
func (c ImportCase) classify(changes []importer.Change, allowed []string) []ChangeRecord {
	var out []ChangeRecord
	for _, ch := range changes {
		for _, e := range ch.Edits {
			r := ChangeRecord{Kind: ch.Kind, Reason: ch.Reason, Field: e.Field, Index: e.Index, Before: e.Before, After: e.After}
			switch {
			case e.Field == importer.FieldIngredient:
				planted := c.Planted != nil && !c.Expect.Recipe && e.Index == c.Expect.Ingredient
				r.Verdict = verdictWrong
				if planted && c.restores(e) {
					r.Verdict = verdictCorrect
				}
			case slices.Contains(allowed, ch.Kind):
				r.Verdict = verdictAllowed
			default:
				r.Verdict = verdictToJudge
			}
			out = append(out, r)
		}
	}
	return out
}

// legacyWrong is the first definition of a wrong fix (as at 73c4b8b1), kept so
// earlier runs stay comparable: any applied change of a kind the set does not
// allow, or any edit of the planted line, in a planted case.
func (c ImportCase) legacyWrong(changes []importer.Change, allowed []string) bool {
	if c.Planted == nil {
		return false
	}
	for _, ch := range changes {
		if !slices.Contains(allowed, ch.Kind) {
			return true
		}
		for _, e := range ch.Edits {
			if e.Field == importer.FieldIngredient && e.Index == c.Expect.Ingredient && !c.Expect.Recipe {
				return true
			}
		}
	}
	return false
}

func countVerdict(recs []ChangeRecord, verdict string) int {
	return len(slices.DeleteFunc(slices.Clone(recs), func(r ChangeRecord) bool { return r.Verdict != verdict }))
}

// judgeFile lists the changes to judge: for each one the recipe step (or field)
// before and after, so a reader can mark it good or wrong.
func judgeFile(results []CaseResult) string {
	var sb strings.Builder
	sb.WriteString("# Import check changes to judge\n\nMark each change good or wrong; wrong ones count against the 5 % target.\n")
	n := 0
	for _, res := range results {
		for _, r := range res.Changes {
			if r.Verdict != verdictToJudge {
				continue
			}
			n++
			fmt.Fprintf(&sb, "\n## %d. %s: %s (%s", n, res.ID, r.Kind, r.Field)
			if r.Index >= 0 {
				fmt.Fprintf(&sb, " %d", r.Index)
			}
			sb.WriteString(")\n\n")
			if r.Reason != "" {
				fmt.Fprintf(&sb, "Reason: %s\n\n", r.Reason)
			}
			fmt.Fprintf(&sb, "- Before: %v\n- After: %v\n- Verdict: \n", r.Before, r.After)
		}
	}
	if n == 0 {
		sb.WriteString("\nNo changes to judge.\n")
	}
	sb.WriteString("\n# Clarity proposals (shown to the user as flags, never applied)\n")
	m := 0
	for _, res := range results {
		for _, t := range res.ClarityFlags {
			m++
			fmt.Fprintf(&sb, "\n## %d. %s\n\n- %s\n- Verdict: \n", m, res.ID, t)
		}
	}
	if m == 0 {
		sb.WriteString("\nNo clarity proposals.\n")
	}
	return sb.String()
}

// RunImportCheck runs the check on every case and reports the detection rate
// of the planted errors, the wrong-fix rate and what it did to clean recipes.
func RunImportCheck(ctx context.Context, cfg ImportCheckConfig) (ModeReport, error) {
	l, err := newLedger(ctx, cfg.Env)
	if err != nil {
		return ModeReport{}, err
	}
	resolver := llm.StaticResolver(l.rec.Meter(cfg.Client, cfg.Provider), cfg.Model)
	svc := importer.NewService(nil, jsonld.Extractor{}, resolver, nil, nil).WithChecker(importer.NewChecker(resolver, l.rec,
		func(context.Context, bool) (importer.CheckPermit, error) { return importer.CheckPermit{Run: true}, nil }))

	cases := ImportCases(cfg.Set)
	rep := ModeReport{Mode: "importcheck", Cases: len(cases)}
	var planted, detected, wrongFix, legacyWrongFix, notRun, clean, cleanChanged, cleanFlagged, fixed, toJudge, ran int
	for _, c := range cases {
		if l.capReached() {
			rep.StoppedBySpendCap = true
			break
		}
		start := time.Now()
		chk, err := svc.Check(ctx, c.Draft, c.Draft.Language)
		elapsed := time.Since(start)
		u, serr := l.settle(ctx)
		if serr != nil {
			return ModeReport{}, serr
		}
		res := CaseResult{ID: c.ID, CostUSD: u.CostUSD, LatencyMS: elapsed.Milliseconds()}
		if c.Planted != nil {
			res.Groups = []string{c.Planted.Kind}
		}
		rep.Ran++
		if err != nil || !chk.Ran {
			notRun++
			res.Outcome, res.Detail = "check_not_run", fmt.Sprint(err)
			rep.Results = append(rep.Results, res)
			if c.Planted != nil {
				planted++
			} else {
				clean++
			}
			continue
		}
		res.Changes = c.classify(chk.Changes, cfg.Set.AllowedChanges)
		// A clarity proposal is a flag the reader judges, not a change and not a false alarm.
		var flags []importer.Flag
		for _, f := range chk.Flags {
			if f.Clarity {
				res.ClarityFlags = append(res.ClarityFlags, f.Text)
			} else {
				flags = append(flags, f)
			}
		}
		chk.Flags = flags
		ran++
		wrong := countVerdict(res.Changes, verdictWrong)
		toJudge += countVerdict(res.Changes, verdictToJudge)
		if wrong > 0 {
			wrongFix++
		}
		if c.legacyWrong(chk.Changes, cfg.Set.AllowedChanges) {
			legacyWrongFix++
		}
		if c.Planted != nil {
			planted++
			res.OK = c.found(chk.Flags)
			if res.OK {
				detected++
			}
			if countVerdict(res.Changes, verdictCorrect) > 0 {
				fixed++
			}
			res.Outcome = map[bool]string{true: "detected", false: "missed"}[res.OK]
		} else {
			clean++
			res.OK = wrong == 0
			if len(chk.Changes) > 0 {
				cleanChanged++
			}
			if len(chk.Flags) > 0 {
				cleanFlagged++
			}
			res.Outcome = map[bool]string{true: "unchanged_or_allowed", false: "wrong_change"}[res.OK]
		}
		res.Detail = fmt.Sprintf("changes=%d wrong=%d to_judge=%d flags=%d", len(res.Changes), wrong, countVerdict(res.Changes, verdictToJudge), len(chk.Flags))
		rep.Results = append(rep.Results, res)
	}
	l.finish(&rep)
	rep.Artifacts = map[string]string{judgeFileName: judgeFile(rep.Results)}

	rep.add(Metric{Name: "detection_rate", Value: pct(detected, planted), Unit: "%", Note: fmt.Sprintf("%d/%d planted errors", detected, planted)}.target(">= 80 %", true, 80))
	rep.add(Metric{Name: "wrong_fix_rate", Value: pct(wrongFix, ran), Unit: "%", Note: fmt.Sprintf("%d/%d checked recipes with a wrong change (an ingredient line changed other than back to the original); judged-wrong designed changes add to it", wrongFix, ran)}.target("<= 5 %", false, 5))
	rep.add(Metric{Name: "wrong_fix_rate.legacy", Value: pct(legacyWrongFix, planted), Unit: "%", Note: fmt.Sprintf("%d/%d planted recipes with any change of a kind the set does not allow or any edit of the planted line (the first definition, for comparing earlier runs)", legacyWrongFix, planted)})
	rep.add(Metric{Name: "planted_fixed_correctly", Value: float64(fixed), Note: fmt.Sprintf("of %d planted: the planted line was put back to its original", planted)})
	rep.add(Metric{Name: "changes_to_judge", Value: float64(toJudge), Note: "safety cue and other designed changes, in " + judgeFileName})
	byGroup(&rep, "detection_rate.kind.", func(c CaseResult) string {
		if len(c.Groups) == 0 {
			return ""
		}
		return c.Groups[0]
	})
	rep.add(Metric{Name: "clean_recipes_changed", Value: float64(cleanChanged), Note: fmt.Sprintf("of %d clean (any change; allowed kinds %v)", clean, cfg.Set.AllowedChanges)})
	rep.add(Metric{Name: "clean_recipes_flagged", Value: float64(cleanFlagged), Note: fmt.Sprintf("of %d clean (false alarms)", clean)})
	rep.add(Metric{Name: "check_not_run", Value: float64(notRun), Note: "calls that failed or were refused"}.target("0", false, 0))
	return rep, nil
}
