package planning

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
