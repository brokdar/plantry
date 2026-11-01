package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/airun"
	"github.com/jaltszeimer/plantry/backend/internal/domain/availability"
	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
	"github.com/jaltszeimer/plantry/backend/internal/domain/settings"
	"github.com/jaltszeimer/plantry/backend/internal/domain/window"
)

// PromptVersion identifies the chat system prompt template. Bump it on every
// change to the prompt.
const PromptVersion = "v5"

// SettingsReader is the subset of settings.Service that the agent needs.
type SettingsReader interface {
	Get(ctx context.Context, key string) (settings.Value, error)
}

// AvailabilityResolver resolves the availability of every cell in an
// inclusive date range.
type AvailabilityResolver interface {
	Resolve(ctx context.Context, from, to time.Time) ([]availability.Cell, error)
}

// PlanContext reads the plan window the system prompt shows (D2): Build
// loads the planning state of a window (planning.Builder.Build) and Summary
// renders it with the window's issues (planning.RenderPlanSummary).
type PlanContext struct {
	Build   func(ctx context.Context, w window.Window, today, from, to time.Time) (planning.State, error)
	Summary func(planning.State, []planning.Issue) string
}

// Service is the facade over conversation persistence + the agent loop. The
// HTTP handler calls this exclusively.
type Service struct {
	repo     Repository
	resolver llm.Resolver
	tools    *ToolSet
	profile  *profile.Service
	settings SettingsReader
	plan     PlanContext
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
	plan PlanContext,
	now func() time.Time,
) *Service {
	return &Service{
		repo: repo, resolver: resolver, tools: tools,
		profile: profileSvc, settings: settingsSvc,
		plan: plan, now: now,
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
	// Start is the planning window's first day (required); it must be a
	// shopping day (D1).
	Start time.Time
}

// Chat drives a single turn-taking interaction with the model, starting with
// the user's new message. Events stream on out; the caller must read them
// until the channel closes.
func (s *Service) Chat(ctx context.Context, req ChatRequest, out chan<- llm.Event) (err error) {
	if req.UserText == "" {
		return fmt.Errorf("%w: user_text required", domain.ErrInvalidInput)
	}
	w, err := s.window(ctx, req)
	if err != nil {
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
	prompt, err := s.buildSystemPrompt(ctx, req, w)
	if err != nil {
		return err
	}

	// Record the turn from here on, so setup failures leave no run behind.
	if s.recorder != nil {
		var run *airun.Run
		ctx, run = s.recorder.Start(ctx, airun.Start{
			Kind:           airun.KindChat,
			PromptVersion:  PromptVersion,
			WindowStart:    &w.Start,
			WindowEnd:      &w.End,
			Instruction:    &req.UserText,
			ConversationID: &conv.ID,
		})
		defer func() {
			status, errKey := chatOutcome(ctx, err)
			run.Finish(ctx, status, errKey)
		}()
	}

	// Drive the loop. Tools read today from ctx (D25).
	ctx = withWindow(withToday(ctx, s.today(req)), w)
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

// window derives the planning window from the request's start (D1): it must
// be a shopping day, else domain.ErrInvalidPlanStart.
func (s *Service) window(ctx context.Context, req ChatRequest) (window.Window, error) {
	if req.Start.IsZero() {
		return window.Window{}, fmt.Errorf("%w: start required", domain.ErrInvalidInput)
	}
	sd, err := plate.IntSetting(ctx, s.settings, settings.KeyPlanShoppingDay)
	if err != nil {
		return window.Window{}, err
	}
	if !window.IsStart(req.Start, sd) {
		return window.Window{}, fmt.Errorf("%w: %s", domain.ErrInvalidPlanStart, req.Start.Format(time.DateOnly))
	}
	return window.Containing(req.Start, sd), nil
}

// CheckStart reports whether start is a valid chat window start, so a handler
// can reject a request before it opens a stream.
func (s *Service) CheckStart(ctx context.Context, start time.Time) error {
	_, err := s.window(ctx, ChatRequest{Start: start})
	return err
}

func (s *Service) buildSystemPrompt(ctx context.Context, req ChatRequest, w window.Window) (string, error) {
	p, err := s.profile.Get(ctx)
	if err != nil {
		return "", err
	}
	today := s.today(req)
	st, err := s.plan.Build(ctx, w, today, w.Start, w.End)
	if err != nil {
		return "", err
	}
	return ComposePrompt(p, s.plan.Summary(st, planning.BoardIssues(st)), today), nil
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

type windowKey struct{}

// withWindow carries the chat turn's planning window to the tools.
func withWindow(ctx context.Context, w window.Window) context.Context {
	return context.WithValue(ctx, windowKey{}, w)
}

// windowFrom returns the window set by withWindow.
func windowFrom(ctx context.Context) (window.Window, bool) {
	w, ok := ctx.Value(windowKey{}).(window.Window)
	return w, ok
}

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

// DebugSystemPrompt composes and returns the system prompt that would be sent
// to the LLM for a chat turn with req's dates. It goes through the
// same path as Chat, so end-to-end tests can assert on what the agent
// actually sees. Never call this from the Chat path.
func (s *Service) DebugSystemPrompt(ctx context.Context, req ChatRequest) (string, error) {
	w, err := s.window(ctx, req)
	if err != nil {
		return "", err
	}
	return s.buildSystemPrompt(ctx, req, w)
}
