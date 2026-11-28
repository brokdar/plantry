package chefcanary

import (
	"context"
	"fmt"
	"slices"
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
		if c.Expect.Recipe {
			return f.Ingredient < 0
		}
		return f.Ingredient == c.Expect.Ingredient
	})
}

// wrongChanges are the applied changes that count as a wrong fix: a kind the
// set does not allow, or an edit of the planted ingredient line.
func (c ImportCase) wrongChanges(changes []importer.Change, allowed []string) []string {
	var wrong []string
	for _, ch := range changes {
		bad := !slices.Contains(allowed, ch.Kind)
		for _, e := range ch.Edits {
			if c.Planted != nil && e.Field == importer.FieldIngredient && e.Index == c.Expect.Ingredient && !c.Expect.Recipe {
				bad = true
			}
		}
		if bad {
			wrong = append(wrong, ch.Kind)
		}
	}
	return wrong
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
	var planted, detected, wrongFix, notRun, clean, cleanChanged, cleanFlagged int
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
		wrong := c.wrongChanges(chk.Changes, cfg.Set.AllowedChanges)
		if c.Planted != nil {
			planted++
			res.OK = c.found(chk.Flags)
			if res.OK {
				detected++
			}
			if len(wrong) > 0 {
				wrongFix++
			}
			res.Outcome = map[bool]string{true: "detected", false: "missed"}[res.OK]
		} else {
			clean++
			res.OK = len(wrong) == 0
			if len(chk.Changes) > 0 {
				cleanChanged++
			}
			if len(chk.Flags) > 0 {
				cleanFlagged++
			}
			res.Outcome = map[bool]string{true: "unchanged_or_allowed", false: "wrong_change"}[res.OK]
		}
		res.Detail = fmt.Sprintf("changes=%d wrong=%v flags=%d", len(chk.Changes), wrong, len(chk.Flags))
		rep.Results = append(rep.Results, res)
	}
	l.finish(&rep)

	rep.add(Metric{Name: "detection_rate", Value: pct(detected, planted), Unit: "%", Note: fmt.Sprintf("%d/%d planted errors", detected, planted)}.target(">= 80 %", true, 80))
	rep.add(Metric{Name: "wrong_fix_rate", Value: pct(wrongFix, planted), Unit: "%", Note: fmt.Sprintf("%d/%d planted recipes", wrongFix, planted)}.target("<= 5 %", false, 5))
	byGroup(&rep, "detection_rate.kind.", func(c CaseResult) string {
		if len(c.Groups) == 0 {
			return ""
		}
		return c.Groups[0]
	})
	rep.add(Metric{Name: "clean_recipes_changed", Value: float64(cleanChanged), Note: fmt.Sprintf("of %d clean (allowed kinds %v)", clean, cfg.Set.AllowedChanges)})
	rep.add(Metric{Name: "clean_recipes_flagged", Value: float64(cleanFlagged), Note: fmt.Sprintf("of %d clean (false alarms)", clean)})
	rep.add(Metric{Name: "check_not_run", Value: float64(notRun), Note: "calls that failed or were refused"}.target("0", false, 0))
	return rep, nil
}
