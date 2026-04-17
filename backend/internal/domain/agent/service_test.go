package agent_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/agent"
	"github.com/jaltszeimer/plantry/backend/internal/domain/component"
	"github.com/jaltszeimer/plantry/backend/internal/domain/ingredient"
	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planner"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
	"github.com/jaltszeimer/plantry/backend/internal/domain/slot"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
)

type serviceFixture struct {
	repo   *sqlite.AIRepo
	svc    *agent.Service
	weekID int64
}

func newServiceFixture(t *testing.T, client llm.Client) *serviceFixture {
	t.Helper()
	ctx := context.Background()
	db := testhelper.NewTestDB(t)

	ingRepo := sqlite.NewIngredientRepo(db)
	compRepo := sqlite.NewComponentRepo(db)
	slotRepo := sqlite.NewSlotRepo(db)
	plateRepo := sqlite.NewPlateRepo(db)
	weekRepo := sqlite.NewWeekRepo(db)
	profileRepo := sqlite.NewProfileRepo(db)
	aiRepo := sqlite.NewAIRepo(db)
	txRunner := sqlite.NewTxRunner(db)

	compSvc := component.NewService(compRepo, ingRepo, ingRepo)
	slotSvc := slot.NewService(slotRepo)
	plateSvc := plate.NewService(plateRepo, slotRepo, compRepo)
	plannerSvc := planner.NewService(weekRepo, plateRepo, txRunner)
	profileSvc := profile.NewService(profileRepo)

	ing := &ingredient.Ingredient{Name: "Chicken", Source: "manual", Kcal100g: 165}
	require.NoError(t, ingRepo.Create(ctx, ing))
	require.NoError(t, compSvc.Create(ctx, &component.Component{
		Name: "Curry", Role: component.RoleMain, ReferencePortions: 2,
		Ingredients: []component.ComponentIngredient{
			{IngredientID: ing.ID, Amount: 200, Unit: "g", Grams: 200},
		},
	}))
	require.NoError(t, slotSvc.Create(ctx, &slot.TimeSlot{
		NameKey: "slot.dinner", Icon: "moon", Active: true,
	}))

	week, err := plannerSvc.Current(ctx, time.Now().UTC())
	require.NoError(t, err)

	tools, err := agent.NewToolSet(agent.Services{
		Components: compSvc, Planner: plannerSvc, Plates: plateSvc,
		Profile: profileSvc, Slots: slotSvc, Ingredient: ingRepo,
	})
	require.NoError(t, err)

	return &serviceFixture{
		repo:   aiRepo,
		svc:    agent.NewService(aiRepo, client, tools, plannerSvc, profileSvc, "test-model"),
		weekID: week.ID,
	}
}

func runChat(t *testing.T, svc *agent.Service, req agent.ChatRequest) ([]llm.Event, error) {
	t.Helper()
	out := make(chan llm.Event, 128)
	errCh := make(chan error, 1)
	go func() {
		errCh <- svc.Chat(context.Background(), req, out)
		close(out)
	}()
	var events []llm.Event
	for e := range out {
		events = append(events, e)
	}
	return events, <-errCh
}

func TestService_Chat_CreatesConversationAndPersistsUserAndAssistant(t *testing.T) {
	fake := &fakeLLM{turns: []fakeTurn{{
		stopReason: llm.StopReasonEndTurn,
		message:    llm.Message{Role: llm.RoleAssistant, Content: textBlocks("hi")},
	}}}
	f := newServiceFixture(t, fake)

	_, err := runChat(t, f.svc, agent.ChatRequest{WeekID: &f.weekID, UserText: "plan tuesday dinner"})
	require.NoError(t, err)

	list, err := f.svc.ListConversations(context.Background(), agent.ListQuery{})
	require.NoError(t, err)
	require.Equal(t, int64(1), list.Total)

	conv, err := f.svc.GetConversation(context.Background(), list.Items[0].ID)
	require.NoError(t, err)
	require.Len(t, conv.Messages, 2)
	assert.Equal(t, agent.RoleUser, conv.Messages[0].Role)
	assert.Equal(t, agent.RoleAssistant, conv.Messages[1].Role)

	var blocks []llm.ContentBlock
	require.NoError(t, json.Unmarshal(conv.Messages[0].Content, &blocks))
	require.Len(t, blocks, 1)
	assert.Equal(t, "plan tuesday dinner", blocks[0].Text)
}

func TestService_Chat_ResumesExistingConversation(t *testing.T) {
	fake := &fakeLLM{turns: []fakeTurn{
		{stopReason: llm.StopReasonEndTurn, message: llm.Message{Role: llm.RoleAssistant, Content: textBlocks("first")}},
		{stopReason: llm.StopReasonEndTurn, message: llm.Message{Role: llm.RoleAssistant, Content: textBlocks("second")}},
	}}
	f := newServiceFixture(t, fake)

	_, err := runChat(t, f.svc, agent.ChatRequest{UserText: "hello"})
	require.NoError(t, err)

	list, err := f.svc.ListConversations(context.Background(), agent.ListQuery{})
	require.NoError(t, err)
	convID := list.Items[0].ID

	_, err = runChat(t, f.svc, agent.ChatRequest{ConversationID: &convID, UserText: "follow up"})
	require.NoError(t, err)

	conv, err := f.svc.GetConversation(context.Background(), convID)
	require.NoError(t, err)
	require.Len(t, conv.Messages, 4)
	assert.Equal(t, agent.RoleUser, conv.Messages[0].Role)
	assert.Equal(t, agent.RoleAssistant, conv.Messages[1].Role)
	assert.Equal(t, agent.RoleUser, conv.Messages[2].Role)
	assert.Equal(t, agent.RoleAssistant, conv.Messages[3].Role)
}

func TestService_Chat_RejectsEmptyText(t *testing.T) {
	f := newServiceFixture(t, &fakeLLM{})
	out := make(chan llm.Event, 1)
	err := f.svc.Chat(context.Background(), agent.ChatRequest{UserText: ""}, out)
	close(out)
	assert.True(t, errors.Is(err, domain.ErrInvalidInput))
}

func TestService_Chat_ErrorMidStream_PersistsErrorRow(t *testing.T) {
	fake := &fakeLLM{streamErr: errors.New("upstream down")}
	f := newServiceFixture(t, fake)

	_, err := runChat(t, f.svc, agent.ChatRequest{UserText: "try"})
	require.Error(t, err)

	list, err := f.svc.ListConversations(context.Background(), agent.ListQuery{})
	require.NoError(t, err)
	conv, err := f.svc.GetConversation(context.Background(), list.Items[0].ID)
	require.NoError(t, err)
	require.Len(t, conv.Messages, 2)
	assert.Equal(t, agent.RoleUser, conv.Messages[0].Role)
	assert.Equal(t, agent.RoleError, conv.Messages[1].Role)
}

func TestService_Chat_ModeHintAppendedToSystemPrompt(t *testing.T) {
	captured := &captureClient{}
	f := newServiceFixture(t, captured)

	_, err := runChat(t, f.svc, agent.ChatRequest{UserText: "hi", Mode: "fill_empty"})
	require.NoError(t, err)

	assert.Contains(t, captured.lastReq.System, "Only create plates in empty")
}

func TestService_Chat_EmitsConversationReadyBeforeModelEvents(t *testing.T) {
	fake := &fakeLLM{turns: []fakeTurn{{
		stopReason: llm.StopReasonEndTurn,
		message:    llm.Message{Role: llm.RoleAssistant, Content: textBlocks("ok")},
	}}}
	f := newServiceFixture(t, fake)

	events, err := runChat(t, f.svc, agent.ChatRequest{UserText: "hi"})
	require.NoError(t, err)
	require.NotEmpty(t, events)
	first := events[0]
	assert.Equal(t, llm.EventConversationReady, first.Type)
	payload, ok := first.Payload.(llm.ConversationReadyPayload)
	require.True(t, ok)
	assert.NotZero(t, payload.ConversationID)
}

func TestService_Chat_HistoryPassedToProvider(t *testing.T) {
	captured := &captureClient{}
	f := newServiceFixture(t, captured)

	// First call establishes the conversation.
	_, err := runChat(t, f.svc, agent.ChatRequest{UserText: "hello"})
	require.NoError(t, err)
	assert.Len(t, captured.lastReq.Messages, 1, "only the current user message")

	list, _ := f.svc.ListConversations(context.Background(), agent.ListQuery{})
	convID := list.Items[0].ID

	// Second call should include prior turns in the history sent to the provider.
	_, err = runChat(t, f.svc, agent.ChatRequest{ConversationID: &convID, UserText: "again"})
	require.NoError(t, err)
	// Expect: user(1) + assistant(1) + user(new) = 3
	assert.Len(t, captured.lastReq.Messages, 3)
}

type captureClient struct{ lastReq llm.Request }

func (c *captureClient) Stream(_ context.Context, req llm.Request, out chan<- llm.Event) (*llm.Response, error) {
	c.lastReq = req
	close(out)
	return &llm.Response{
		Message:    llm.Message{Role: llm.RoleAssistant, Content: textBlocks("ok")},
		StopReason: llm.StopReasonEndTurn,
	}, nil
}
