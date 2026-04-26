package agent

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planner"
	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
	"github.com/jaltszeimer/plantry/backend/internal/domain/settings"
)

// SettingsReader is the subset of settings.Service that the agent needs.
type SettingsReader interface {
	Get(ctx context.Context, key string) (settings.Value, error)
}

// Service is the facade over conversation persistence + the agent loop. The
// HTTP handler calls this exclusively.
type Service struct {
	repo     Repository
	resolver llm.Resolver
	tools    *ToolSet
	planner  *planner.Service
	profile  *profile.Service
	settings SettingsReader
}

// NewService constructs the agent service. The resolver is consulted at the
// start of each Chat turn so that changes to the provider/model/API key take
// effect without a restart.
func NewService(
	repo Repository,
	resolver llm.Resolver,
	tools *ToolSet,
	plannerSvc *planner.Service,
	profileSvc *profile.Service,
	settingsSvc SettingsReader,
) *Service {
	return &Service{
		repo: repo, resolver: resolver, tools: tools,
		planner: plannerSvc, profile: profileSvc, settings: settingsSvc,
	}
}

// ChatRequest is what the HTTP handler receives.
type ChatRequest struct {
	ConversationID *int64
	WeekID         *int64
	Mode           string // "fill_empty" | "replace_all" | ""
	UserText       string
}

// Chat drives a single turn-taking interaction with the model, starting with
// the user's new message. Events stream on out; the caller must read them
// until the channel closes.
func (s *Service) Chat(ctx context.Context, req ChatRequest, out chan<- llm.Event) error {
	if req.UserText == "" {
		return fmt.Errorf("%w: user_text required", domain.ErrInvalidInput)
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
	prompt, err := s.buildSystemPrompt(ctx, req, conv)
	if err != nil {
		return err
	}

	// Drive the loop.
	return Run(ctx, RunRequest{
		ConversationID: conv.ID,
		WeekID:         conv.WeekID,
		Model:          model,
		SystemPrompt:   prompt,
		History:        history,
		Tools:          s.tools.Describe(),
	}, client, s.tools, s.repo, out)
}

// ListConversations returns a page of conversations, optionally scoped to a week.
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
	return s.repo.CreateConversation(ctx, req.WeekID, nil)
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

func (s *Service) buildSystemPrompt(ctx context.Context, req ChatRequest, conv *Conversation) (string, error) {
	p, err := s.profile.Get(ctx)
	if err != nil {
		return "", err
	}
	var week *planner.Week
	weekID := conv.WeekID
	if req.WeekID != nil {
		weekID = req.WeekID
	}
	if weekID != nil {
		w, err := s.planner.Get(ctx, *weekID)
		if err == nil {
			week = w
		}
	}
	var prefs *PlanningPrefs
	if s.settings != nil {
		shoppingDayVal, _ := s.settings.Get(ctx, settings.KeyPlanShoppingDay)
		anchorModeVal, _ := s.settings.Get(ctx, settings.KeyPlanAnchor)
		prefs = &PlanningPrefs{
			ShoppingDay: shoppingDayVal.Raw,
			AnchorMode:  anchorModeVal.Raw,
		}
	}
	base := ComposePrompt(p, week, nil, prefs)
	if req.Mode != "" {
		base += "\nMode hint: " + modeHint(req.Mode) + "\n"
	}
	return base, nil
}

// DebugSystemPrompt composes and returns the system prompt that would be sent
// to the LLM for a chat turn given the supplied weekID. It is a thin wrapper
// over the same logic used by Chat itself, so end-to-end tests can assert on
// what the agent actually sees. Never call this from the Chat path — it
// reloads the profile and week and is meant strictly for debug surfaces.
func (s *Service) DebugSystemPrompt(ctx context.Context, weekID *int64) (string, error) {
	p, err := s.profile.Get(ctx)
	if err != nil {
		return "", err
	}
	var week *planner.Week
	if weekID != nil {
		w, err := s.planner.Get(ctx, *weekID)
		if err == nil {
			week = w
		}
	}
	return ComposePrompt(p, week, nil, nil), nil
}

func modeHint(mode string) string {
	switch mode {
	case "fill_empty":
		return "Only create plates in empty (day, slot) cells. Do not modify existing plates unless the user asks. Never modify plates where skipped=true — treat them as immutable (the user marked that slot as eating out, canteen, or otherwise off-plan). When candidates are otherwise equivalent, prefer components where favorite=true."
	case "replace_all":
		return "You may clear the week and plan it from scratch. Confirm with the user before deleting existing plates. Respect skipped plates — do not overwrite them. When candidates are otherwise equivalent, prefer components where favorite=true."
	default:
		return mode
	}
}
