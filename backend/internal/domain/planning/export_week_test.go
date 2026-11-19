package planning

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
)

// GuardWeek exposes the last guard to the external tests.
var GuardWeek = guardWeek

// BalancePlan exposes the balancing pass to the external tests.
var BalancePlan = balancePlan

// SameFamily exposes the dish family test.
var SameFamily = sameFamily

// PlannerView exposes the evaluate_plan result of a proposal (E020): the
// result map and the finished proposal.
func PlannerView(in ChooseInput, raw []byte, offered bool) (map[string]any, Proposal, error) {
	prop, err := parseProposal(raw, in)
	if err != nil {
		return nil, Proposal{}, err
	}
	prop, _ = autofix(in, prop)
	view, _, done := plannerView(in, prop, offered)
	return view, done, nil
}

// LegacyView is what evaluate_plan returned before E020: the evaluation of
// the plan as proposed.
func LegacyView(in ChooseInput, raw []byte) (map[string]any, error) {
	prop, err := parseProposal(raw, in)
	if err != nil {
		return nil, err
	}
	prop, _ = autofix(in, prop)
	ev := EvaluateWeek(Merge(in.State, in.Target, prop), in.Target)
	return ev.Compact(ev.ProposedView()), nil
}

// FinishProposal exposes what the service does to a model's plan after it is
// submitted.
var FinishProposal = finishProposal

// PlannerViewOf exposes the evaluate_plan result of a proposal.
func PlannerViewOf(in ChooseInput, prop Proposal) (map[string]any, Evaluation, Proposal) {
	return plannerView(in, prop, false)
}

// PlannerViewAgain is PlannerViewOf for a later evaluation of the run.
func PlannerViewAgain(in ChooseInput, prop Proposal) (map[string]any, Evaluation, Proposal) {
	return plannerView(in, prop, true)
}

// GuardDayMains exposes the day's distinct-mains guard.
var GuardDayMains = guardDayMains

// ParseForProof exposes the parsed model proposal.
func ParseForProof(raw []byte, in ChooseInput) (Proposal, error) {
	p, err := parseProposal(raw, in)
	if err != nil {
		return Proposal{}, err
	}
	p, _ = autofix(in, p)
	return p, nil
}

// SavedPlanNotes exposes the code-written lines of a saved plan's note.
var SavedPlanNotes = savedPlanNotes

// FixCookLimit exposes the cooking-limit move of the finishing step.
var FixCookLimit = fixCookLimit

// PromptSections measures the planner prompt by section, in characters.
func PromptSections(in ChooseInput) [][2]any {
	s := in.State
	x := newIndex(s, in.Target)
	var out [][2]any
	measure := func(name string, f func(b *strings.Builder)) {
		var b strings.Builder
		f(&b)
		out = append(out, [2]any{name, utf8.RuneCountInString(b.String())})
	}
	measure("rules", func(b *strings.Builder) { writeRules(b, s) })
	measure("dates", func(b *strings.Builder) { writeDates(b, s) })
	measure("settings", func(b *strings.Builder) { writeSettings(b, in) })
	measure("profile", func(b *strings.Builder) { writeProfile(b, s.Profile) })
	measure("day budgets", func(b *strings.Builder) { writeDayBudgets(b, x) })
	measure("meat budget", func(b *strings.Builder) { writeMeatBudget(b, in) })
	measure("cells", func(b *strings.Builder) { writeCells(b, x) })
	measure("sessions", func(b *strings.Builder) { writeSessions(b, x) })
	measure("fixed cooks", func(b *strings.Builder) { writeFixedCooks(b, x) })
	measure("keep table", func(b *strings.Builder) { writeKeepTable(b, in) })
	measure("corrections", func(b *strings.Builder) { writeCorrections(b, s) })
	measure("task", func(b *strings.Builder) { writeTask(b, in) })
	cands := candidates(in)
	n := 0
	for _, f := range cands {
		n += utf8.RuneCountInString(candidateRow(f, s.Settings.DefaultKeepDays)) + 1
	}
	out = append(out, [2]any{fmt.Sprintf("candidate table (%d rows)", len(cands)), n})
	for _, tl := range []llm.Tool{evaluatePlanTool(), submitPlanTool()} {
		out = append(out, [2]any{"tool " + tl.Name + " (description+schema)", utf8.RuneCountInString(tl.Description) + len(tl.Schema)})
	}
	out = append(out, [2]any{"TOTAL rendered prompt", utf8.RuneCountInString(RenderPrompt(in))})
	return out
}

// CandidateColumns is the characters per column of the candidate table.
func CandidateColumns(in ChooseInput) map[string]int {
	names := strings.Split("id|name|role|suits|diet|meat|meat_g|fish_g|kcal|protein_g|fat_g|carbs_g|nutrition_complete|minutes|reference_portions|keep_days|freezable|last_eaten|eaten_8_weeks|favorite|like_score|explore", "|")
	out := map[string]int{}
	for _, f := range candidates(in) {
		for i, v := range strings.Split(candidateRow(f, in.State.Settings.DefaultKeepDays), "|") {
			if i < len(names) {
				out[names[i]] += utf8.RuneCountInString(v) + 1
			}
		}
	}
	return out
}
