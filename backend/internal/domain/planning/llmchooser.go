package planning

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
)

// LLM call shape (D12).
const (
	llmTemperature = 0.4
	llmMaxTokens   = 16000
	// maxRepairs is how many times a proposal with violations, or a reply
	// without a readable submit_plan call, is sent back (spec §6.1).
	maxRepairs = 2
)

// LLMChooser proposes a plan with one forced submit_plan call per attempt
// and repairs it up to twice with the checker's violations (D11–D14). It
// calls the client under the run's ctx, so the metered client records every
// call as a step of the run.
type LLMChooser struct {
	resolver llm.Resolver
}

// NewLLMChooser returns the LLM chooser over the configured provider.
func NewLLMChooser(r llm.Resolver) *LLMChooser { return &LLMChooser{resolver: r} }

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
	req := llm.Request{
		Model:       model,
		System:      RenderPrompt(in),
		Messages:    []llm.Message{textMessage("Plan the cells marked fill and call submit_plan with the complete plan.")},
		Tools:       []llm.Tool{submitPlanTool()},
		ToolChoice:  &llm.ToolChoice{Name: submitPlanName},
		Temperature: llmTemperature,
		MaxTokens:   llmMaxTokens,
	}
	var last *Proposal
	for attempt := 0; ; attempt++ {
		if attempt > 0 && in.Stage != nil {
			in.Stage(StageRepairing)
		}
		resp, err := call(ctx, client, req)
		if err != nil {
			return Proposal{}, fmt.Errorf("%w: %w", domain.ErrAIProviderFailed, err)
		}
		input, perr := submitted(resp)
		var prop Proposal
		if perr == nil {
			prop, perr = parseProposal(input, in)
		}
		var feedback string
		if perr == nil {
			prop.RepairAttempts = attempt
			violations := violationsOf(Check(Merge(in.State, in.Target, prop), in.Target))
			if len(violations) == 0 || attempt == maxRepairs {
				return prop, nil
			}
			last, feedback = &prop, describeViolations(violations)
		} else {
			if attempt == maxRepairs {
				if last != nil {
					last.RepairAttempts = attempt
					return *last, nil
				}
				return Proposal{}, fmt.Errorf("%w: %w", domain.ErrAIInvalidPlan, perr)
			}
			feedback = "error: " + perr.Error() + "\nCall submit_plan again with the complete plan as valid JSON matching its schema."
		}
		if len(resp.Message.Content) > 0 {
			req.Messages = append(req.Messages, resp.Message)
		}
		req.Messages = append(req.Messages, repairMessage(resp.Message, feedback))
	}
}

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
		if b.Type == llm.ContentTypeToolUse && b.ToolUseName == submitPlanName {
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
			SessionKey *string `json:"session_key"`
			SessionID  *int64  `json:"session_id"`
		} `json:"components"`
		Reason string `json:"reason"`
	} `json:"plates"`
	Notes string `json:"notes"`
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
	p := Proposal{Notes: strings.TrimSpace(sp.Notes)}
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
			pc := ProposedComponent{FoodID: c.FoodID, Portions: c.Portions}
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
	var out []Issue
	for _, is := range issues {
		if is.Severity == SeverityViolation {
			out = append(out, is)
		}
	}
	return out
}

// describeViolations is the repair feedback: one line per violation with
// its rule, code, place and parameters.
func describeViolations(vs []Issue) string {
	var b strings.Builder
	b.WriteString("violations:\n")
	for _, v := range vs {
		fmt.Fprintf(&b, "- %s %s", v.Rule, v.Code)
		if v.Date != "" {
			fmt.Fprintf(&b, " on %s", v.Date)
		}
		if v.SlotID != nil {
			fmt.Fprintf(&b, " slot_id %d", *v.SlotID)
		}
		if v.PlateID != nil {
			fmt.Fprintf(&b, " plate_id %d", *v.PlateID)
		}
		if v.SessionID != nil {
			fmt.Fprintf(&b, " session_id %d", *v.SessionID)
		}
		for _, k := range slices.Sorted(maps.Keys(v.Params)) {
			fmt.Fprintf(&b, " %s=%v", k, v.Params[k])
		}
		b.WriteByte('\n')
	}
	b.WriteString("Fix every violation and call submit_plan again with the complete plan (every session and plate, not only the changes).")
	return b.String()
}
