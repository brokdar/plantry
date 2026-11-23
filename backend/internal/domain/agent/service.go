package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sync/atomic"
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

// chatMaxTokens is the output cap of a chat reply (thinking included); billing
// is per generated token, the cap only prevents cut-offs.
const chatMaxTokens = 16000

// chatProgrammatic is false (D4): the chat agent never gets the code
// execution tool or allowed callers, whatever ai.programmatic_tools says. The
// definitions cost about 21 % of every request and 0 of 30 audited chats used
// them; the planner keeps programmatic tool calling. The loop's plumbing stays
// for a chat use case that needs it.
const chatProgrammatic = false

// PromptVersion identifies the chat system prompt template. Bump it on every
// change to the prompt.
const PromptVersion = "v45"

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
	// turns numbers the Chat calls; unlocks holds the unlock previews (D6).
	turns   atomic.Uint64
	unlocks *pendingUnlocks
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
		unlocks: newPendingUnlocks(now),
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
	// Today is the user's current date (required, D25).
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
	if req.Today.IsZero() {
		return fmt.Errorf("%w: today required", domain.ErrInvalidInput)
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
	prompt, cut, st, err := s.buildSystemPrompt(ctx, req, w, earlierUserTexts(history))
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
		// The plan before the turn is the state the prompt was built from;
		// the plan after is read again when the turn ends, also on error or
		// cancel (detached from ctx).
		planning.RecordEvaluation(ctx, planning.PhaseBefore, func() planning.Evaluation { return evaluateAsSeen(st) })
		defer func() {
			dctx := context.WithoutCancel(ctx)
			if after, berr := s.plan.Build(dctx, w, req.Today, w.Start, w.End); berr != nil {
				planning.RecordEvaluationError(dctx, planning.PhaseAfter, berr)
			} else {
				planning.RecordEvaluation(dctx, planning.PhaseAfter, func() planning.Evaluation { return evaluateAsSeen(after) })
			}
			status, errKey := chatOutcome(ctx, err)
			run.Finish(ctx, status, errKey)
		}()
	}

	if cut > 0 {
		// Never cut silently: the model works with a damaged prompt then.
		slog.WarnContext(ctx, "agent.prompt.truncated", append(airun.FromContext(ctx).LogAttrs(), "conversation_id", conv.ID,
			"prompt_chars", len(prompt), "cut_chars", cut, "budget_chars", promptCharBudget)...)
	}

	// Drive the loop. Tools read today (D25), the window and the turn from
	// ctx.
	ctx = withWindow(withToday(ctx, req.Today), w)
	cells := namedCells(req.UserText, w, st.SlotNames)
	dayKcal := namedDayKcal(req.UserText, w, req.Today)
	ctx = withTurn(ctx, chatTurn{conversationID: conv.ID, seq: s.turns.Add(1), unlocks: s.unlocks, named: namedFoods(req.UserText, st.Foods), cells: cells, dayKcal: dayKcal})
	runReq := RunRequest{
		ConversationID: conv.ID,
		Model:          model,
		SystemPrompt:   prompt,
		UserText:       req.UserText,
		Locale:         st.Profile.Locale,
		FoodNames:      foodNamesOf(st),
		History:        history,
		Tools:          s.tools.Describe(),
		Effort:         s.effort(ctx),
		MaxTokens:      chatMaxTokens,
		unmet:          unmetChange(req.UserText, cells, st.SlotNames, st),
		underFloor:     underFloorBlock(dayKcal, st),
	}
	if chatProgrammatic {
		runReq.CodeExecution = true
		switch {
		case conv.ContainerID == "":
		case conv.ContainerExpiresAt.After(s.now()):
			runReq.Container = conv.ContainerID
		default:
			slog.InfoContext(ctx, "agent.container.expired", "conversation_id", conv.ID,
				"container_id", conv.ContainerID, "expired_at", conv.ContainerExpiresAt)
		}
		runReq.SaveContainer = func(ctx context.Context, id string, expires time.Time) {
			if err := s.repo.SetContainer(context.WithoutCancel(ctx), conv.ID, id, expires); err != nil {
				slog.WarnContext(ctx, "agent.container.save", "conversation_id", conv.ID, "container_id", id, "error", err)
			}
		}
	} else {
		runReq.Tools = withoutCallers(runReq.Tools)
	}
	return Run(ctx, runReq, client, s.tools, s.repo, out)
}

// effort reads ai.reasoning_effort.chat; an unreadable setting means the
// provider default.
func (s *Service) effort(ctx context.Context) string {
	v, err := s.settings.Get(ctx, settings.KeyAIReasoningEffortChat)
	if err != nil {
		slog.WarnContext(ctx, "agent.effort.setting", "error", err)
		return ""
	}
	return v.Raw
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

// buildSystemPrompt renders the system prompt and returns the bytes its
// budget cut off (0: none) and the window's state it was built from.
func (s *Service) buildSystemPrompt(ctx context.Context, req ChatRequest, w window.Window, earlier []string) (string, int, planning.State, error) {
	p, err := s.profile.Get(ctx)
	if err != nil {
		return "", 0, planning.State{}, err
	}
	today := req.Today
	st, err := s.plan.Build(ctx, w, today, w.Start, w.End)
	if err != nil {
		return "", 0, planning.State{}, err
	}
	prompt, cut := ComposePrompt(p, chatFacts(st), s.plan.Summary(st, chatIssues(st)), today)
	// A language nothing says is not asserted (round 8 P1): no line.
	if lg, ok := messageLang(append([]string{req.UserText}, earlier...), foodNamesOf(st), st.Profile.Locale); ok {
		prompt += languageLine(lg)
	}
	return prompt, cut, st, nil
}

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

// withToday carries the user's date to the tools of a chat turn and to the
// plate service they write through (plate.WithToday), so the turn's guards,
// plan state and write path share one today.
func withToday(ctx context.Context, today time.Time) context.Context {
	return plate.WithToday(ctx, today)
}

// todayFrom returns the date set by withToday; the zero time when a chat
// turn did not set one (Chat rejects such a request).
func todayFrom(ctx context.Context) time.Time {
	return plate.TodayFrom(ctx)
}

// DebugSystemPrompt composes and returns the system prompt that would be sent
// to the LLM for a chat turn with req's dates. It goes through the
// same path as Chat, so end-to-end tests can assert on what the agent
// actually sees. Never call this from the Chat path.
func (s *Service) DebugSystemPrompt(ctx context.Context, req ChatRequest) (string, error) {
	if req.Today.IsZero() {
		return "", fmt.Errorf("%w: today required", domain.ErrInvalidInput)
	}
	w, err := s.window(ctx, req)
	if err != nil {
		return "", err
	}
	prompt, _, _, err := s.buildSystemPrompt(ctx, req, w, nil)
	return prompt, err
}

// foodNamesOf are the names of the plan's foods, for the reply lint.
func foodNamesOf(st planning.State) []string {
	out := make([]string, 0, len(st.Foods))
	for _, f := range st.Foods {
		if f.Name != "" {
			out = append(out, f.Name)
		}
	}
	slices.Sort(out) // one order per plan: the lint and the language read them in turn
	return out
}
