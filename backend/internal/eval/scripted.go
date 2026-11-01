package eval

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/fake"
	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
)

// fakeProvider is the run log's provider of scripted runs.
const fakeProvider = "fake"

// scriptedLLM is the LLM chooser of an offline run: the real LLMChooser
// over the fake client, scripted per choice with one submit_plan turn that
// holds the heuristic's proposal for the same input. So the run goes
// through the prompt, the forced tool call, parsing and the checker with a
// deterministic answer, and the fake client records every request the
// chooser sent (spec §8.4).
type scriptedLLM struct {
	llm   *planning.LLMChooser
	meter func(llm.Client) llm.Client
	model string

	mu       sync.Mutex
	client   *fake.Client
	requests []llm.Request
}

func newScriptedLLM(meter func(llm.Client) llm.Client, model string) *scriptedLLM {
	s := &scriptedLLM{meter: meter, model: model, client: fake.NewFromScript(&fake.Script{})}
	s.llm = planning.NewLLMChooser(s)
	return s
}

// Current implements llm.Resolver with the client of the current choice.
func (s *scriptedLLM) Current(context.Context) (llm.Client, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.meter(s.client), s.model, nil
}

// PromptVersion is the LLM chooser's.
func (s *scriptedLLM) PromptVersion() string { return s.llm.PromptVersion() }

// Choose scripts the fake client with the heuristic's proposal and lets
// the LLM chooser plan with it.
func (s *scriptedLLM) Choose(ctx context.Context, in planning.ChooseInput) (planning.Proposal, error) {
	p, err := planning.Heuristic{}.Choose(ctx, in)
	if err != nil {
		return planning.Proposal{}, err
	}
	input, err := json.Marshal(submitInput(p))
	if err != nil {
		return planning.Proposal{}, err
	}
	client := fake.NewFromScript(&fake.Script{Turns: []fake.Turn{{
		StopReason: "tool_use",
		Message: fake.ScriptedMessage{Content: []fake.ScriptedBlock{{
			Type: "tool_use", ToolUseID: "tu_eval", ToolName: "submit_plan", ToolInput: input,
		}}},
		Usage: fake.ScriptedUsage{
			InputTokens:  planning.EstimateTokens(planning.RenderPrompt(in)),
			OutputTokens: planning.EstimateTokens(string(input)),
		},
	}}})
	s.mu.Lock()
	s.client = client
	s.mu.Unlock()
	prop, err := s.llm.Choose(ctx, in)
	s.mu.Lock()
	s.requests = append(s.requests, client.Requests()...)
	s.mu.Unlock()
	return prop, err
}

// takeRequests returns the requests recorded since the last call.
func (s *scriptedLLM) takeRequests() []llm.Request {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := s.requests
	s.requests = nil
	return out
}

// submitPlan mirrors the submit_plan tool's input schema.
type submitPlan struct {
	Sessions []submitSession `json:"sessions"`
	Plates   []submitPlate   `json:"plates"`
	Notes    string          `json:"notes"`
}

type submitSession struct {
	Key            string  `json:"key"`
	FoodID         int64   `json:"food_id"`
	CookDate       string  `json:"cook_date"`
	CookSlotID     int64   `json:"cook_slot_id"`
	Portions       float64 `json:"portions"`
	FrozenPortions float64 `json:"frozen_portions"`
	Reason         string  `json:"reason"`
}

type submitPlate struct {
	PlateID    int64             `json:"plate_id,omitempty"`
	Date       string            `json:"date"`
	SlotID     int64             `json:"slot_id"`
	Eaters     float64           `json:"eaters,omitempty"`
	Components []submitComponent `json:"components"`
	Reason     string            `json:"reason"`
}

type submitComponent struct {
	FoodID     int64   `json:"food_id"`
	Portions   float64 `json:"portions"`
	SessionKey string  `json:"session_key,omitempty"`
	SessionID  int64   `json:"session_id,omitempty"`
}

// submitInput is p as a submit_plan call.
func submitInput(p planning.Proposal) submitPlan {
	out := submitPlan{Sessions: []submitSession{}, Plates: []submitPlate{}, Notes: p.Notes}
	for _, s := range p.Sessions {
		out.Sessions = append(out.Sessions, submitSession{
			Key: s.Key, FoodID: s.FoodID, CookDate: s.CookDate.Format(time.DateOnly), CookSlotID: s.CookSlotID,
			Portions: s.Portions, FrozenPortions: s.FrozenPortions, Reason: s.Reason,
		})
	}
	for _, pl := range p.Plates {
		sp := submitPlate{PlateID: pl.PlateID, Date: pl.Date.Format(time.DateOnly), SlotID: pl.SlotID, Eaters: pl.Eaters, Reason: pl.Reason}
		for _, c := range pl.Components {
			sp.Components = append(sp.Components, submitComponent{FoodID: c.FoodID, Portions: c.Portions, SessionKey: c.SessionKey, SessionID: c.SessionID})
		}
		out.Plates = append(out.Plates, sp)
	}
	return out
}
