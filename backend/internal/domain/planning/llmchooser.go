package planning

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/airun"
	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
)

// LLM call shape (D12).
const (
	llmTemperature = 0.4
	llmMaxTokens   = 32000
	// maxRepairs is how many times a proposal with violations, or a reply
	// without a readable submit_plan call, is sent back (spec §6.1).
	maxRepairs = 2
	// maxEvaluations is how many evaluate_plan dry runs a run answers; a
	// further call is told to submit.
	maxEvaluations = 8
	// maxCodeEvaluations is the same bound for evaluate_plan calls made from
	// code (programmatic tool calling), which exist to try several drafts in
	// one model turn.
	maxCodeEvaluations = 30
	// maxModelCalls is the hard bound on provider calls per run, evaluate_plan
	// replies included: every submit budget plus every dry run plus slack for
	// the free attempt. Past it the run ends with what it has.
	maxModelCalls = maxEvaluations + maxRepairs + 4
	// maxRequests is the hard bound on provider requests per run, code
	// continuations and pause_turn resends included, so nothing loops paid
	// calls unbounded. Model turns (maxModelCalls) and code continuations
	// (maxCodeEvaluations) have their own bounds below it.
	maxRequests = maxModelCalls + maxCodeEvaluations
)

// LLMChooser proposes a plan with one forced submit_plan call per attempt
// and repairs it up to twice with the checker's violations (D11–D14). It
// calls the client under the run's ctx, so the metered client records every
// call as a step of the run.
type LLMChooser struct {
	resolver llm.Resolver
	effort   func(context.Context) string
	// programmatic reports whether the model may call evaluate_plan from code.
	programmatic func(context.Context) bool
}

// WithProgrammatic sets where the chooser reads ai.programmatic_tools; without
// it evaluate_plan is only called directly.
func (c *LLMChooser) WithProgrammatic(read func(context.Context) bool) *LLMChooser {
	c.programmatic = read
	return c
}

// NewLLMChooser returns the LLM chooser over the configured provider.
func NewLLMChooser(r llm.Resolver) *LLMChooser { return &LLMChooser{resolver: r} }

// WithEffort sets where the chooser reads the reasoning effort of its calls
// (ai.reasoning_effort.generate); without it the provider default applies.
func (c *LLMChooser) WithEffort(read func(context.Context) string) *LLMChooser {
	c.effort = read
	return c
}

// PromptVersion is the run log's prompt_version of LLM runs.
func (*LLMChooser) PromptVersion() string { return PromptVersion }

// Ready fails with domain.ErrAIProviderMissing when no provider is
// configured, and with domain.ErrAIProviderFailed when it can't be built.
func (c *LLMChooser) Ready(ctx context.Context) error {
	_, _, err := c.current(ctx)
	return err
}

func (c *LLMChooser) current(ctx context.Context) (llm.Client, string, error) {
	client, model, err := c.resolver.Current(ctx)
	switch {
	case errors.Is(err, llm.ErrProviderMissing):
		return nil, "", fmt.Errorf("%w: %w", domain.ErrAIProviderMissing, err)
	case err != nil:
		return nil, "", fmt.Errorf("%w: %w", domain.ErrAIProviderFailed, err)
	}
	return client, model, nil
}

// Choose implements Chooser. A provider error fails with
// domain.ErrAIProviderFailed. A proposal still violating hard rules after
// the repairs is returned as it is (the run ends partial); when no reply
// held a readable proposal it fails with domain.ErrAIInvalidPlan.
func (c *LLMChooser) Choose(ctx context.Context, in ChooseInput) (Proposal, error) {
	client, model, err := c.current(ctx)
	if err != nil {
		return Proposal{}, err
	}
	system, trimmed := renderPrompt(in)
	recordContext(ctx, in, "llm", trimmed)
	req := llm.Request{
		Model:       model,
		System:      system,
		Messages:    []llm.Message{textMessage("Plan the cells marked fill. Draft the complete plan, check it with evaluate_plan (it saves nothing), fix what it reports, then call submit_plan.")},
		Tools:       []llm.Tool{evaluatePlanTool(), submitPlanTool()},
		Temperature: llmTemperature,
		Reasoning:   true,
		MaxTokens:   llmMaxTokens,
		CacheTail:   true,
	}
	if c.effort != nil {
		req.Effort = c.effort(ctx)
	}
	if c.programmatic != nil && c.programmatic(ctx) {
		req.CodeExecution = true
		req.Tools[0].AllowedCallers = []string{llm.CallerDirect, llm.CallerCodeExecution}
	}
	var last *Proposal
	var lastSig string
	var allFixes, runNotes []string
	// paid counts the repairs that use up the budget (maxRepairs); the one
	// free attempt for a skeleton first reply is extra, once per run.
	paid, freeUsed, evals, modelCalls := 0, false, 0, 0
	directEvals, codeEvals := 0, 0
	// requests counts every provider request; conts the ones that only
	// continue the previous reply (answers to code-called tools, a paused
	// turn resent) and are not new model turns.
	requests, conts, continuation := 0, 0, false
	fillCells := len(fillCellLabels(in))
	for attempt := 0; ; attempt++ {
		if attempt > 0 && in.Stage != nil {
			in.Stage(StageRepairing)
		}
		requests++
		var stopNote string
		switch {
		case requests > maxRequests:
			stopNote = fmt.Sprintf("The run stopped after %d provider requests.", maxRequests)
		case continuation:
			if conts++; conts > maxCodeEvaluations {
				stopNote = fmt.Sprintf("The model's code kept calling tools; the run stopped after %d code continuations.", maxCodeEvaluations)
			}
		default:
			if modelCalls++; modelCalls > maxModelCalls {
				stopNote = fmt.Sprintf("The model kept calling evaluate_plan or repairing; the run stopped after %d model calls.", maxModelCalls)
			}
		}
		if stopNote != "" {
			logRunStop(ctx, in, requests-1, stopNote)
			if last == nil {
				return Proposal{}, fmt.Errorf("%w: no plan was submitted: %s", domain.ErrAIInvalidPlan, stopNote)
			}
			last.RepairAttempts = attempt
			return withNotes(*last, append(slices.Clone(allFixes), append(runNotes, stopNote)...), nil, ""), nil
		}
		resp, err := call(ctx, client, req)
		if err == nil && resp.ContainerID != "" {
			req.Container = resp.ContainerID
		}
		if err != nil {
			// The last proposal that came before the failure goes with the
			// error, so the run log can evaluate it.
			var partial Proposal
			if last != nil {
				partial = *last
			}
			return partial, fmt.Errorf("%w: %w", domain.ErrAIProviderFailed, err)
		}
		continuation = false
		if resp.StopReason == llm.StopReasonPauseTurn && !hasToolUse(resp) {
			// The server paused a long turn (code execution): resend it as
			// it is. Not an attempt, no repair used; it counts toward the
			// request caps.
			req.Messages = append(req.Messages, resp.Message)
			attempt--
			continuation = true
			continue
		}
		if resp.StopReason == llm.StopReasonMaxTokens && hasToolUse(resp) {
			// The reply was cut at the output limit: its tool input may be
			// cut too (an empty evaluate_plan call evaluated an empty plan).
			// Nothing is run; every call is answered with an error. It is a
			// model turn but not an attempt and uses no repair.
			var results []llm.ContentBlock
			modelSeq := airun.FromContext(ctx).Seq()
			var seqs []int
			for _, b := range resp.Message.Content {
				if b.Type != llm.ContentTypeToolUse {
					continue
				}
				results = append(results, errorResult(b, cutoffMessage))
				result, _ := json.Marshal(map[string]string{"error": cutoffMessage})
				seqs = append(seqs, airun.FromContext(ctx).ToolStep(llm.WithCaller(ctx, b.Caller), b.ToolUseName, b.ToolUseInput, result, true, 0))
			}
			if note := "A reply was cut off at the output limit; its tool calls were not run."; !slices.Contains(runNotes, note) {
				runNotes = append(runNotes, note)
			}
			logRunCutoff(ctx, in, modelSeq, seqs)
			req.Messages = append(req.Messages, llm.ReplayMessage(resp.Message), llm.Message{Role: llm.RoleUser, Content: results})
			attempt--
			continue
		}
		if calls := pendingCalls(resp); len(calls) > 0 && !hasSubmit(resp) {
			// A dry run: evaluate the draft, answer, and ask again. It is
			// not an attempt and uses no repair.
			results := make([]llm.ContentBlock, len(calls))
			for i, b := range calls {
				if b.ToolUseName == submitPlanName {
					results[i] = errorResult(b, "submit_plan can't be called from code; call it directly once you have chosen the plan.")
					continue
				}
				evals++
				over := false
				if b.Caller.FromCode() {
					codeEvals++
					over = codeEvals > maxCodeEvaluations
				} else {
					directEvals++
					over = directEvals > maxEvaluations
				}
				results[i] = evaluateResult(llm.WithCaller(ctx, b.Caller), in, b, evals, over)
			}
			req.Messages = append(req.Messages, resp.Message, llm.Message{Role: llm.RoleUser, Content: results})
			attempt--
			continuation = onlyCodeCalls(resp)
			continue
		}
		input, perr := submitted(resp)
		var prop Proposal
		if perr == nil {
			prop, perr = parseProposal(input, in)
		}
		var feedback string
		if perr == nil {
			prop.RepairAttempts = attempt
			// A light request stands for the whole run: a repair that
			// forgets to repeat it keeps it.
			in.State.Light = UnionLight(in.State.Light, prop.Light)
			prop.Light = in.State.Light
			sig := signature(prop)
			var fixes []string
			prop, fixes = autofix(in, prop)
			merged := Merge(in.State, in.Target, prop)
			violations := violationsOf(Check(merged, in.Target))
			rep := newRepairer(in, prop)
			advice := rep.advisories()
			logAttempt(ctx, in.Logger, attempt, violations, nil, fixes...)
			RecordAttemptEvaluation(ctx, attempt, func() Evaluation { return Evaluate(merged, in.Target) })
			for _, f := range fixes {
				if !slices.Contains(allFixes, f) {
					allFixes = append(allFixes, f)
				}
			}
			needRepair := len(violations) > 0 || len(advice) > 0
			switch {
			case !freeUsed && attempt == 0 && skeleton(violations, fillCells):
				// An incomplete first plan is not a failed repair: the
				// extra attempt is recorded in the run notes.
				freeUsed = true
				runNotes = append(runNotes, fmt.Sprintf("The plan lacked plates for %d cell(s) and was completed in an extra attempt.", missingCount(violations)))
			case needRepair && paid < maxRepairs:
				paid++
			default:
				return withNotes(prop, append(slices.Clone(allFixes), runNotes...), advice, joinNotes(kcalNote(rep.s, in.Target), floorNote(rep.s, in.Target))), nil
			}
			noted := withNotes(prop, allFixes, advice, "")
			last = &noted
			feedback = rep.feedback(findings{violations, advice, fixes, sig == lastSig}) + evaluationFeedback(merged, in.Target)
			lastSig = sig
		} else {
			logAttempt(ctx, in.Logger, attempt, nil, perr)
			if paid == maxRepairs {
				if last != nil {
					last.RepairAttempts = attempt
					return *last, nil
				}
				return Proposal{}, fmt.Errorf("%w: %w", domain.ErrAIInvalidPlan, perr)
			}
			paid++
			feedback = "error: " + perr.Error() + "\nCall submit_plan again with the complete plan as valid JSON matching its schema."
		}
		if len(resp.Message.Content) > 0 {
			req.Messages = append(req.Messages, resp.Message)
		}
		req.Messages = append(req.Messages, repairMessage(resp.Message, feedback))
	}
}

// skeleton reports whether the first reply is a plan still to complete:
// every violation is a missing plate, or it misses plates for at least half
// of the fill cells (the model used the checker as a to-do list; its other
// violations are incidental). The caller grants the free attempt once and
// only for call 1.
func skeleton(vs []Issue, fillCells int) bool {
	missing := missingCount(vs)
	return missing > 0 && (missing == len(vs) || missing*2 >= fillCells)
}

func missingCount(vs []Issue) int {
	n := 0
	for _, v := range vs {
		if v.Rule == "H1" && v.Code == CodeMissingPlate {
			n++
		}
	}
	return n
}

// withNotes appends to the proposal's notes (the run notes the board shows)
// one line per autofix and run event, one warning per advisory the repairs
// left open and the calorie line (when set), so a correction or an
// unresolved problem never passes silently.
func withNotes(p Proposal, fixes, advice []string, kcal string) Proposal {
	lines := slices.Clone(fixes)
	for _, a := range advice {
		short, _, _ := strings.Cut(a, " Fix:")
		lines = append(lines, "Warning, not resolved: "+short)
	}
	if kcal != "" {
		lines = append(lines, kcal)
	}
	if len(lines) > 0 {
		p.Notes = strings.TrimSpace(strings.Join(append([]string{p.Notes}, lines...), "\n"))
	}
	return p
}

func joinNotes(notes ...string) string {
	return strings.Join(slices.DeleteFunc(notes, func(n string) bool { return n == "" }), "\n")
}

// cutoffMessage answers the tool calls of a reply cut off at the output limit.
const cutoffMessage = "Your reply was cut off at the output limit, so nothing was run. Keep your reasoning short and let the tools compute: put your draft into evaluate_plan."

// call streams one request and returns the assembled reply.
func call(ctx context.Context, client llm.Client, req llm.Request) (*llm.Response, error) {
	events := make(chan llm.Event, 16)
	done := make(chan struct{})
	go func() {
		for range events {
		}
		close(done)
	}()
	resp, err := client.Stream(ctx, req, events)
	<-done
	if err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, errors.New("empty response")
	}
	return resp, nil
}

func textMessage(text string) llm.Message {
	return llm.Message{Role: llm.RoleUser, Content: []llm.ContentBlock{{Type: llm.ContentTypeText, Text: text}}}
}

// repairMessage answers every tool call of the reply with the feedback as
// an error result (providers want a result per call), or sends it as text
// when the reply made no call.
func repairMessage(reply llm.Message, feedback string) llm.Message {
	content, _ := json.Marshal(feedback) // a string always marshals
	var results []llm.ContentBlock
	for _, b := range reply.Content {
		if b.Type == llm.ContentTypeToolUse {
			results = append(results, llm.ContentBlock{
				Type: llm.ContentTypeToolResult, ToolResultID: b.ToolUseID, ToolResultContent: content, ToolResultIsError: true,
			})
		}
	}
	if len(results) == 0 {
		return textMessage(feedback)
	}
	return llm.Message{Role: llm.RoleUser, Content: results}
}

// submitted returns the input of the reply's submit_plan call.
func submitted(resp *llm.Response) (json.RawMessage, error) {
	for _, b := range resp.Message.Content {
		if b.Type == llm.ContentTypeToolUse && b.ToolUseName == submitPlanName && !b.Caller.FromCode() {
			return b.ToolUseInput, nil
		}
	}
	return nil, fmt.Errorf("the reply has no submit_plan call (stop reason %s)", resp.StopReason)
}

// submitPlan is submit_plan's input (submitPlanSchema).
type submitPlan struct {
	Sessions []struct {
		Key            string  `json:"key"`
		FoodID         int64   `json:"food_id"`
		CookDate       string  `json:"cook_date"`
		CookSlotID     int64   `json:"cook_slot_id"`
		Portions       float64 `json:"portions"`
		FrozenPortions float64 `json:"frozen_portions"`
		Reason         string  `json:"reason"`
	} `json:"sessions"`
	Plates []struct {
		PlateID    int64   `json:"plate_id"`
		Date       string  `json:"date"`
		SlotID     int64   `json:"slot_id"`
		Eaters     float64 `json:"eaters"`
		Components []struct {
			FoodID     int64   `json:"food_id"`
			Portions   float64 `json:"portions"`
			Grams      float64 `json:"grams"`
			SessionKey *string `json:"session_key"`
			SessionID  *int64  `json:"session_id"`
		} `json:"components"`
		Reason string `json:"reason"`
	} `json:"plates"`
	Light []LightIntent `json:"light"`
	Notes string        `json:"notes"`
}

// parseProposal maps submit_plan's input to a Proposal. Ids are not
// checked here: an unknown one is an H7 violation the repair fixes. A plate
// without eaters gets the day's guests (D16), else the household's portions.
func parseProposal(input json.RawMessage, in ChooseInput) (Proposal, error) {
	var sp submitPlan
	if err := json.Unmarshal(input, &sp); err != nil {
		var te *json.UnmarshalTypeError
		if errors.As(err, &te) {
			return Proposal{}, fmt.Errorf("submit_plan input: %s must be %s, not %s", te.Field, jsonKind(te.Type.Kind()), te.Value)
		}
		return Proposal{}, fmt.Errorf("submit_plan input: %w", err)
	}
	light, err := ValidateLight(sp.Light)
	if err != nil {
		return Proposal{}, err
	}
	p := Proposal{Notes: strings.TrimSpace(sp.Notes), Light: light}
	for i, s := range sp.Sessions {
		if strings.TrimSpace(s.Key) == "" {
			return Proposal{}, fmt.Errorf("sessions[%d].key is required", i)
		}
		d, err := parseDate(s.CookDate, fmt.Sprintf("sessions[%d].cook_date", i))
		if err != nil {
			return Proposal{}, err
		}
		p.Sessions = append(p.Sessions, ProposedSession{
			Key: s.Key, FoodID: s.FoodID, CookDate: d, CookSlotID: s.CookSlotID,
			Portions: s.Portions, FrozenPortions: s.FrozenPortions, Reason: strings.TrimSpace(s.Reason),
		})
	}
	for i, pl := range sp.Plates {
		d, err := parseDate(pl.Date, fmt.Sprintf("plates[%d].date", i))
		if err != nil {
			return Proposal{}, err
		}
		eaters := pl.Eaters
		if eaters == 0 {
			eaters = in.EatersByDate[d.Format(time.DateOnly)]
		}
		pp := ProposedPlate{PlateID: pl.PlateID, Date: d, SlotID: pl.SlotID, Eaters: eaters, Reason: strings.TrimSpace(pl.Reason)}
		for _, c := range pl.Components {
			pc := ProposedComponent{FoodID: c.FoodID, Portions: c.Portions, Grams: c.Grams}
			if c.SessionKey != nil {
				pc.SessionKey = *c.SessionKey
			}
			if c.SessionID != nil {
				pc.SessionID = *c.SessionID
			}
			pp.Components = append(pp.Components, pc)
		}
		p.Plates = append(p.Plates, pp)
	}
	return p, nil
}

// jsonKind names a Go kind as the JSON type the schema asks for.
func jsonKind(k reflect.Kind) string {
	switch k {
	case reflect.Slice, reflect.Array:
		return "an array"
	case reflect.Struct, reflect.Map:
		return "an object"
	case reflect.String:
		return "a string"
	case reflect.Bool:
		return "a boolean"
	case reflect.Pointer:
		return "a value"
	}
	return "a number"
}

func parseDate(s, field string) (time.Time, error) {
	d, err := time.Parse(time.DateOnly, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("%s %q is not a YYYY-MM-DD date", field, s)
	}
	return d, nil
}

func violationsOf(issues []Issue) []Issue {
	out := []Issue{}
	for _, is := range issues {
		if is.Severity == SeverityViolation {
			out = append(out, is)
		}
	}
	return out
}

// hasSubmit reports a direct submit_plan call: only that is the submission
// (allowed_callers is guidance, not a boundary).
func hasSubmit(resp *llm.Response) bool {
	for _, b := range resp.Message.Content {
		if b.Type == llm.ContentTypeToolUse && b.ToolUseName == submitPlanName && !b.Caller.FromCode() {
			return true
		}
	}
	return false
}

// pendingCalls are the reply's calls the loop answers without it being an
// attempt: evaluate_plan, and a submit_plan made from code (refused).
func pendingCalls(resp *llm.Response) []llm.ContentBlock {
	var out []llm.ContentBlock
	for _, b := range resp.Message.Content {
		if b.Type != llm.ContentTypeToolUse {
			continue
		}
		if b.ToolUseName == evaluatePlanName || (b.ToolUseName == submitPlanName && b.Caller.FromCode()) {
			out = append(out, b)
		}
	}
	return out
}

func hasToolUse(resp *llm.Response) bool {
	return slices.ContainsFunc(resp.Message.Content, func(b llm.ContentBlock) bool { return b.Type == llm.ContentTypeToolUse })
}

// onlyCodeCalls reports a reply whose tool calls all come from code: the
// next request only continues that script and is not a new model turn.
func onlyCodeCalls(resp *llm.Response) bool {
	n := 0
	for _, b := range resp.Message.Content {
		if b.Type != llm.ContentTypeToolUse {
			continue
		}
		if !b.Caller.FromCode() {
			return false
		}
		n++
	}
	return n > 0
}

func errorResult(b llm.ContentBlock, msg string) llm.ContentBlock {
	content, _ := json.Marshal(msg)
	return llm.ContentBlock{Type: llm.ContentTypeToolResult, ToolResultID: b.ToolUseID, ToolResultContent: content, ToolResultIsError: true}
}

// evaluateResult answers one evaluate_plan call: the Evaluate result of the
// merged proposal (after the same autofix a submit gets, listed under
// autofix), or an error when the input is unreadable or the run used up its
// dry runs. Every answered dry run is recorded as a plan.evaluation step.
func evaluateResult(ctx context.Context, in ChooseInput, b llm.ContentBlock, n int, over bool) llm.ContentBlock {
	res := llm.ContentBlock{Type: llm.ContentTypeToolResult, ToolResultID: b.ToolUseID}
	fail := func(msg string) llm.ContentBlock {
		res.ToolResultIsError = true
		res.ToolResultContent, _ = json.Marshal(msg)
		return res
	}
	if over {
		return fail("evaluate_plan call limit reached for this run; call submit_plan with your best plan.")
	}
	prop, err := parseProposal(b.ToolUseInput, in)
	if err != nil {
		return fail("error: " + err.Error() + "\nCall evaluate_plan again with the complete plan as valid JSON matching its schema.")
	}
	prop, fixes := autofix(in, prop)
	merged := Merge(in.State, in.Target, prop)
	ev := Evaluate(merged, in.Target)
	RecordDryRun(ctx, n, func() Evaluation { return ev })
	view := ev.Compact(ev.ProposedView())
	if len(fixes) > 0 {
		view["autofix"] = fixes
	}
	out, err := json.Marshal(view)
	if err != nil {
		return fail("error: encode evaluation")
	}
	res.ToolResultContent = out
	return res
}

// evaluationFeedback is the evaluate_plan result of a submitted plan, which
// goes after the repair text so the model sees every rule, soft ones
// included.
func evaluationFeedback(merged State, t Target) string {
	ev := Evaluate(merged, t)
	out, err := json.Marshal(ev.Compact(ev.ProposedView()))
	if err != nil {
		return ""
	}
	return "\nEvaluation of this plan (the same result evaluate_plan returns): " + string(out)
}
