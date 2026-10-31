package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/airun"
	"github.com/jaltszeimer/plantry/backend/internal/domain/availability"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
	"github.com/jaltszeimer/plantry/backend/internal/domain/settings"
	"github.com/jaltszeimer/plantry/backend/internal/domain/slot"
)

// PromptVersion identifies the chat system prompt template. Bump it on every
// change to the prompt.
const PromptVersion = "v4"

// SettingsReader is the subset of settings.Service that the agent needs.
type SettingsReader interface {
	Get(ctx context.Context, key string) (settings.Value, error)
}

// PlateRanger loads the plates in an inclusive date range.
type PlateRanger interface {
	Range(ctx context.Context, from, to time.Time) ([]plate.Plate, error)
}

// SlotLister lists the time slots.
type SlotLister interface {
	List(ctx context.Context, activeOnly bool) ([]slot.TimeSlot, error)
}

// FoodLookup loads food headers by id.
type FoodLookup interface {
	LookupByIDs(ctx context.Context, ids []int64) (map[int64]*food.Food, error)
}

// AvailabilityResolver resolves the availability of every cell in an
// inclusive date range.
type AvailabilityResolver interface {
	Resolve(ctx context.Context, from, to time.Time) ([]availability.Cell, error)
}

// WindowSources read the planning window shown in the system prompt.
// Availability may be nil: every cell is then "plan".
type WindowSources struct {
	Plates       PlateRanger
	Slots        SlotLister
	Foods        FoodLookup
	Availability AvailabilityResolver
}

// Service is the facade over conversation persistence + the agent loop. The
// HTTP handler calls this exclusively.
type Service struct {
	repo     Repository
	resolver llm.Resolver
	tools    *ToolSet
	profile  *profile.Service
	settings SettingsReader
	window   WindowSources
	now      func() time.Time
	recorder *airun.Recorder // may be nil: chat turns are then not recorded
}

// NewService constructs the agent service. The resolver is consulted at the
// start of each Chat turn so that changes to the provider/model/API key take
// effect without a restart. now supplies today's date when a request has
// none.
func NewService(
	repo Repository,
	resolver llm.Resolver,
	tools *ToolSet,
	profileSvc *profile.Service,
	settingsSvc SettingsReader,
	window WindowSources,
	now func() time.Time,
) *Service {
	return &Service{
		repo: repo, resolver: resolver, tools: tools,
		profile: profileSvc, settings: settingsSvc,
		window: window, now: now,
	}
}

// WithRecorder records every Chat turn as a chat AI run.
func (s *Service) WithRecorder(rec *airun.Recorder) *Service {
	s.recorder = rec
	return s
}

// ChatRequest is what the HTTP handler receives.
type ChatRequest struct {
	ConversationID *int64
	UserText       string
	// Today is the user's current date; zero falls back to the server clock.
	Today time.Time
	// From and To bound the planning window (inclusive, required).
	From time.Time
	To   time.Time
}

// Chat drives a single turn-taking interaction with the model, starting with
// the user's new message. Events stream on out; the caller must read them
// until the channel closes.
func (s *Service) Chat(ctx context.Context, req ChatRequest, out chan<- llm.Event) (err error) {
	if req.UserText == "" {
		return fmt.Errorf("%w: user_text required", domain.ErrInvalidInput)
	}
	if err := checkWindow(req); err != nil {
		return err
	}

	client, model, err := s.resolver.Current(ctx)
	if err != nil {
		return err
	}

	// Resolve conversation.
	conv, err := s.resolveConversation(ctx, req)
	if err != nil {
		return err
	}

	// Publish the conversation id immediately so the client can capture it
	// before any model output arrives.
	out <- llm.Event{
		Type:    llm.EventConversationReady,
		Payload: llm.ConversationReadyPayload{ConversationID: conv.ID},
	}

	// Load history + persist the new user message.
	history, err := s.loadHistory(ctx, conv.ID)
	if err != nil {
		return err
	}
	userBlocks := []llm.ContentBlock{{Type: llm.ContentTypeText, Text: req.UserText}}
	userContent, err := json.Marshal(userBlocks)
	if err != nil {
		return err
	}
	if err := s.repo.AppendMessage(ctx, &Message{
		ConversationID: conv.ID,
		Role:           RoleUser,
		Content:        userContent,
	}); err != nil {
		return err
	}
	history = append(history, llm.Message{Role: llm.RoleUser, Content: userBlocks})

	// Compose system prompt.
	prompt, err := s.buildSystemPrompt(ctx, req)
	if err != nil {
		return err
	}

	// Record the turn from here on, so setup failures leave no run behind.
	if s.recorder != nil {
		var run *airun.Run
		ctx, run = s.recorder.Start(ctx, airun.Start{
			Kind:           airun.KindChat,
			PromptVersion:  PromptVersion,
			WindowStart:    &req.From,
			WindowEnd:      &req.To,
			Instruction:    &req.UserText,
			ConversationID: &conv.ID,
		})
		defer func() {
			status, errKey := chatOutcome(ctx, err)
			run.Finish(ctx, status, errKey)
		}()
	}

	// Drive the loop. Tools read today from ctx (D25).
	ctx = withToday(ctx, s.today(req))
	return Run(ctx, RunRequest{
		ConversationID: conv.ID,
		Model:          model,
		SystemPrompt:   prompt,
		History:        history,
		Tools:          s.tools.Describe(),
	}, client, s.tools, s.repo, out)
}

// chatOutcome maps the result of a Chat turn to the run status and, for
// failures, the i18n key the client was shown.
func chatOutcome(ctx context.Context, err error) (airun.Status, string) {
	var keyed *keyedError
	switch {
	case err == nil:
		return airun.StatusOK, ""
	case errors.Is(err, ErrIterationsExceeded):
		return airun.StatusIterationsExceeded, ""
	case errors.Is(err, context.Canceled) || ctx.Err() != nil:
		return airun.StatusCancelled, ""
	case errors.As(err, &keyed):
		return airun.StatusError, keyed.key
	default:
		// Errors outside the loop are surfaced by the chat handler under
		// this key.
		return airun.StatusError, "error.ai.stream_interrupted"
	}
}

// ListConversations returns a page of conversations.
func (s *Service) ListConversations(ctx context.Context, q ListQuery) (*ListResult, error) {
	return s.repo.ListConversations(ctx, q)
}

// GetConversation returns a conversation with its full message history.
func (s *Service) GetConversation(ctx context.Context, id int64) (*Conversation, error) {
	return s.repo.GetConversation(ctx, id)
}

// DeleteConversation removes a conversation and cascades to its messages.
func (s *Service) DeleteConversation(ctx context.Context, id int64) error {
	return s.repo.DeleteConversation(ctx, id)
}

// ---------------------------------------------------------------------------

func (s *Service) resolveConversation(ctx context.Context, req ChatRequest) (*Conversation, error) {
	if req.ConversationID != nil {
		return s.repo.GetConversation(ctx, *req.ConversationID)
	}
	return s.repo.CreateConversation(ctx, nil)
}

// loadHistory translates persisted agent.Message rows back into llm.Message
// values suitable for the provider. system / error rows are skipped — the
// system prompt is recomposed fresh every turn, and error rows are only for
// the transcript UI.
func (s *Service) loadHistory(ctx context.Context, convID int64) ([]llm.Message, error) {
	rows, err := s.repo.ListMessages(ctx, convID)
	if err != nil {
		return nil, err
	}
	out := make([]llm.Message, 0, len(rows))
	for _, m := range rows {
		var blocks []llm.ContentBlock
		if len(m.Content) > 0 {
			if err := json.Unmarshal(m.Content, &blocks); err != nil {
				// Skip unreadable rows rather than fail the entire chat.
				continue
			}
		}
		switch m.Role {
		case RoleUser:
			out = append(out, llm.Message{Role: llm.RoleUser, Content: blocks})
		case RoleAssistant:
			out = append(out, llm.Message{Role: llm.RoleAssistant, Content: blocks})
		case RoleTool:
			// Provider protocols expect tool results inside a user-role message.
			out = append(out, llm.Message{Role: llm.RoleUser, Content: blocks})
		case RoleSystem, RoleError:
			continue
		}
	}
	return out, nil
}

// checkWindow rejects a request without a planning window.
func checkWindow(req ChatRequest) error {
	if req.From.IsZero() || req.To.IsZero() || req.To.Before(req.From) {
		return fmt.Errorf("%w: from and to required, to not before from", domain.ErrInvalidInput)
	}
	return nil
}

func (s *Service) buildSystemPrompt(ctx context.Context, req ChatRequest) (string, error) {
	p, err := s.profile.Get(ctx)
	if err != nil {
		return "", err
	}
	slotNames, err := s.slotNames(ctx)
	if err != nil {
		return "", err
	}
	plates, err := s.windowPlates(ctx, req.From, req.To, slotNames)
	if err != nil {
		return "", err
	}
	cells, err := s.windowCells(ctx, req.From, req.To, slotNames)
	if err != nil {
		return "", err
	}
	today := s.today(req)
	var prefs *PlanningPrefs
	if s.settings != nil {
		shoppingDayVal, _ := s.settings.Get(ctx, settings.KeyPlanShoppingDay)
		anchorModeVal, _ := s.settings.Get(ctx, settings.KeyPlanAnchor)
		prefs = &PlanningPrefs{
			ShoppingDay: weekdayName(shoppingDayVal.Raw),
			AnchorMode:  anchorModeVal.Raw,
		}
	}
	return ComposePrompt(p, plates, cells, &DateRange{Today: today, From: req.From, To: req.To}, prefs), nil
}

// today is the request's date, or the server's local date when it has none.
func (s *Service) today(req ChatRequest) time.Time {
	if !req.Today.IsZero() {
		return req.Today
	}
	return localDate(s.now())
}

// localDate is t's calendar date at midnight UTC.
func localDate(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

type todayKey struct{}

// withToday carries the user's date to the tools of a chat turn.
func withToday(ctx context.Context, today time.Time) context.Context {
	return context.WithValue(ctx, todayKey{}, today)
}

// todayFrom returns the date set by withToday, else the server's local date.
func todayFrom(ctx context.Context) time.Time {
	if t, ok := ctx.Value(todayKey{}).(time.Time); ok {
		return t
	}
	return localDate(time.Now())
}

// weekdayName turns a plan.shopping_day value (0 = Monday … 6 = Sunday) into
// its English weekday name. Any other value is returned unchanged.
func weekdayName(raw string) string {
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 || n > 6 {
		return raw
	}
	return time.Weekday((n + 1) % 7).String()
}

// slotNames maps every slot id to its name key.
func (s *Service) slotNames(ctx context.Context) (map[int64]string, error) {
	slots, err := s.window.Slots.List(ctx, false)
	if err != nil {
		return nil, err
	}
	names := make(map[int64]string, len(slots))
	for _, sl := range slots {
		names[sl.ID] = sl.NameKey
	}
	return names, nil
}

// windowPlates loads the plates in [from, to] with their slot and food names.
func (s *Service) windowPlates(ctx context.Context, from, to time.Time, slotNames map[int64]string) ([]PromptPlate, error) {
	plates, err := s.window.Plates.Range(ctx, from, to)
	if err != nil {
		return nil, err
	}
	if len(plates) == 0 {
		return nil, nil
	}
	var foodIDs []int64
	for _, p := range plates {
		for _, c := range p.Components {
			foodIDs = append(foodIDs, c.FoodID)
		}
	}
	foods, err := s.window.Foods.LookupByIDs(ctx, foodIDs)
	if err != nil {
		return nil, err
	}
	out := make([]PromptPlate, len(plates))
	for i, p := range plates {
		names := make([]string, 0, len(p.Components))
		for _, c := range p.Components {
			if f, ok := foods[c.FoodID]; ok {
				names = append(names, f.Name)
			}
		}
		out[i] = PromptPlate{
			ID: p.ID, Date: p.Date, SlotID: p.SlotID, Slot: slotNames[p.SlotID],
			Foods: names,
		}
	}
	return out, nil
}

// windowCells loads the "none" and "routine" cells in [from, to].
func (s *Service) windowCells(ctx context.Context, from, to time.Time, slotNames map[int64]string) ([]PromptCell, error) {
	if s.window.Availability == nil {
		return nil, nil
	}
	cells, err := s.window.Availability.Resolve(ctx, from, to)
	if err != nil {
		return nil, err
	}
	var out []PromptCell
	for _, c := range cells {
		if c.Mode == availability.ModePlan {
			continue
		}
		pc := PromptCell{Date: c.Date, SlotID: c.SlotID, Slot: slotNames[c.SlotID], Availability: string(c.Mode)}
		if c.PresetName != nil {
			pc.PresetName = *c.PresetName
		}
		if c.Note != nil {
			pc.Note = *c.Note
		}
		out = append(out, pc)
	}
	return out, nil
}

// DebugSystemPrompt composes and returns the system prompt that would be sent
// to the LLM for a chat turn with req's dates. It goes through the
// same path as Chat, so end-to-end tests can assert on what the agent
// actually sees. Never call this from the Chat path.
func (s *Service) DebugSystemPrompt(ctx context.Context, req ChatRequest) (string, error) {
	if err := checkWindow(req); err != nil {
		return "", err
	}
	return s.buildSystemPrompt(ctx, req)
}
