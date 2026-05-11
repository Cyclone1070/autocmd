package workflow

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/eventbus"
	"github.com/Cyclone1070/iav/internal/state"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type mockSessionStore struct {
	mock.Mock
}

func (m *mockSessionStore) Create() (*domain.Session, error) {
	args := m.Called()
	return args.Get(0).(*domain.Session), args.Error(1)
}

func (m *mockSessionStore) Get(id string) (*domain.Session, error) {
	args := m.Called(id)
	return args.Get(0).(*domain.Session), args.Error(1)
}

func (m *mockSessionStore) Save(s *domain.Session) error {
	args := m.Called(s)
	return args.Error(0)
}

func (m *mockSessionStore) GenerateName(ctx context.Context, llm domain.LLM, target string) (string, error) {
	args := m.Called(ctx, llm, target)
	return args.String(0), args.Error(1)
}

type mockLLM struct {
	mock.Mock
}

func (m *mockLLM) ID() string {
	args := m.Called()
	return args.String(0)
}

func (m *mockLLM) DisplayName() string {
	args := m.Called()
	return args.String(0)
}

func (m *mockLLM) ComputeTokens(ctx context.Context, messages []*schema.Message) (int, error) {
	args := m.Called(ctx, messages)
	return args.Int(0), args.Error(1)
}

func (m *mockLLM) Model() model.ToolCallingChatModel {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(model.ToolCallingChatModel)
}

func (m *mockLLM) ContextWindow() int {
	args := m.Called()
	return args.Int(0)
}

type mockAgent struct {
	mock.Mock
}

func (m *mockAgent) Run(ctx context.Context, sess *domain.Session, input string) error {
	args := m.Called(ctx, sess, input)
	return args.Error(0)
}

type mockActionForwarder struct {
	mock.Mock
}

func (m *mockActionForwarder) Deliver(act domain.Action) {
	m.Called(act)
}

func TestRunPrompt_ActionForwarding(t *testing.T) {
	ctx := t.Context()

	store := new(mockSessionStore)
	llm := new(mockLLM)
	agent := new(mockAgent)
	bus := eventbus.New()
	forwarder := new(mockActionForwarder)

	deps := &PromptDeps{
		Store:        store,
		LLM:          llm,
		State:        &state.State{},
		ToolRegistry: nil,
		Agent:        agent,
		Bus:          bus,
		Forwarder:    forwarder,
	}

	store.On("Create").Return(&domain.Session{ID: "id"}, nil)
	store.On("Save", mock.Anything).Return(nil)
	store.On("GenerateName", mock.Anything, mock.Anything, mock.Anything).Return("Name", nil)

	// Keep the agent running so we can send an action
	agentRunDone := make(chan struct{})
	agent.On("Run", mock.Anything, mock.Anything, mock.Anything).Run(func(_ mock.Arguments) {
		<-agentRunDone
	}).Return(nil)

	RunPrompt(ctx, "hello", deps)

	// Send an action that should be forwarded
	act := domain.QuestionAnswerAction{CallID: "call-1"}
	forwarder.On("Deliver", act).Return()

	bus.SendAction(act)

	// Give the goroutine time to process
	time.Sleep(50 * time.Millisecond)

	forwarder.AssertExpectations(t)
	close(agentRunDone)
}

func TestRunPrompt_GREEN(t *testing.T) {
	ctx := t.Context()

	store := new(mockSessionStore)
	llm := new(mockLLM)

	appState := &state.State{}
	// cfg is not needed by PromptDeps

	agent := new(mockAgent)
	bus := eventbus.New()

	deps := &PromptDeps{
		Store:        store,
		LLM:          llm,
		State:        appState,
		ToolRegistry: nil,
		Agent:        agent,
		Bus:          bus,
	}

	// 1. Session Lifecycle Expectations
	store.On("Create").Return(&domain.Session{ID: "new-id"}, nil)
	store.On("Save", mock.Anything).Return(nil)

	// 2. Auto-naming expectations
	store.On("GenerateName", mock.Anything, mock.Anything, "hello").Return("New Session", nil)

	// 3. Agent Loop expectations
	agent.On("Run", mock.MatchedBy(func(ctx context.Context) bool {
		id, ok := domain.GetSessionID(ctx)
		return ok && id == "new-id"
	}), mock.Anything, "hello").Return(nil)

	done := RunPrompt(ctx, "hello", deps)
	err := <-done

	assert.NoError(t, err)
	assert.Equal(t, "new-id", appState.CurrentSessionID())
	store.AssertExpectations(t)
	llm.AssertExpectations(t)
}

func TestRunPrompt_ExistingNamedSession_DoesNotHang(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	store := new(mockSessionStore)
	llm := new(mockLLM)
	agent := new(mockAgent)
	bus := eventbus.New()

	appState := &state.State{}
	appState.SetCurrentSessionID("existing-id")

	sess := &domain.Session{ID: "existing-id", Name: "Existing Session"}

	deps := &PromptDeps{
		Store:        store,
		LLM:          llm,
		State:        appState,
		ToolRegistry: nil,
		Agent:        agent,
		Bus:          bus,
	}

	store.On("Get", "existing-id").Return(sess, nil)
	store.On("Save", mock.Anything).Return(nil)

	agent.On("Run", mock.Anything, sess, "hello").Return(nil)

	done := RunPrompt(ctx, "hello", deps)

	select {
	case err := <-done:
		assert.NoError(t, err)
	case <-ctx.Done():
		t.Fatal("RunPrompt did not complete for existing named session (possible deadlock on nameChan)")
	}

	store.AssertNotCalled(t, "GenerateName", mock.Anything, mock.Anything, mock.Anything)
}

type trackableBus struct {
	*eventbus.EventBus
	closed bool
}

func (b *trackableBus) Close() {
	b.closed = true
	b.EventBus.Close()
}

func TestRunPrompt_DoesNotCloseBus(t *testing.T) {
	ctx := t.Context()

	store := new(mockSessionStore)
	llm := new(mockLLM)
	agent := new(mockAgent)

	eb := eventbus.New()
	bus := &trackableBus{EventBus: eb}

	deps := &PromptDeps{
		Store:        store,
		LLM:          llm,
		State:        &state.State{},
		ToolRegistry: nil,
		Agent:        agent,
		Bus:          bus.EventBus,
	}

	store.On("Create").Return(&domain.Session{ID: "id"}, nil)
	store.On("Save", mock.Anything).Return(nil)
	store.On("GenerateName", mock.Anything, mock.Anything, mock.Anything).Return("New Session", nil)
	agent.On("Run", mock.Anything, mock.Anything, mock.Anything).Return(nil)
	done := RunPrompt(ctx, "hello", deps)
	err := <-done
	assert.NoError(t, err)

	// In the NEW architecture, the bus should still be open here
	// because RunPrompt no longer calls Close().
	assert.False(t, bus.closed, "RunPrompt should NOT close the bus; that is now wiring responsibility")
}

func TestRunPrompt_NamingRace(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	store := new(mockSessionStore)
	llm := new(mockLLM)
	agent := new(mockAgent)
	bus := eventbus.New()

	deps := &PromptDeps{
		Store:        store,
		LLM:          llm,
		State:        &state.State{},
		ToolRegistry: nil,
		Agent:        agent,
		Bus:          bus,
	}

	sess := &domain.Session{ID: "race-id"}
	store.On("Create").Return(sess, nil)
	store.On("Save", mock.Anything).Return(nil)

	// Sync channels to force interleaving
	agentStartedAppending := make(chan struct{})

	// Simulate GenerateName waiting for agent to start appending
	store.On("GenerateName", mock.Anything, mock.Anything, "hello").
		Run(func(_ mock.Arguments) {
			<-agentStartedAppending // Wait until agent says it started appending
		}).
		Return("Named Session", nil)

	// Simulate Agent.Run appending messages and then signaling
	agent.On("Run", mock.Anything, mock.Anything, "hello").
		Run(func(args mock.Arguments) {
			s := args.Get(1).(*domain.Session)
			// Append some messages to simulate work and race
			for i := range 10 {
				s.Messages = append(s.Messages, &schema.Message{Role: schema.User, Content: "msg"})
				if i == 5 {
					close(agentStartedAppending) // Signal that we've started appending
				}
			}
		}).
		Return(nil)

	done := RunPrompt(ctx, "hello", deps)

	select {
	case err := <-done:
		assert.NoError(t, err)
	case <-ctx.Done():
		t.Fatal("Test timed out - possible deadlock in deterministic synchronization")
	}
}

func TestRunPrompt_MissingSession_ShouldFallbackToCreate(t *testing.T) {
	ctx := t.Context()

	store := new(mockSessionStore)
	llm := new(mockLLM)
	agent := new(mockAgent)
	bus := eventbus.New()

	appState := &state.State{}
	appState.SetCurrentSessionID("non-existent-id")

	deps := &PromptDeps{
		Store:        store,
		LLM:          llm,
		State:        appState,
		ToolRegistry: nil,
		Agent:        agent,
		Bus:          bus,
	}

	// Mocking the "not found" error - wrapped as it is in the real store
	store.On("Get", "non-existent-id").Return((*domain.Session)(nil), fmt.Errorf("read session info: %w", os.ErrNotExist))

	// Fallback expectations
	store.On("Create").Return(&domain.Session{ID: "new-id"}, nil)
	store.On("Save", mock.Anything).Return(nil)
	store.On("GenerateName", mock.Anything, mock.Anything, mock.Anything).Return("New Session", nil)
	agent.On("Run", mock.Anything, mock.Anything, "hello").Return(nil)

	done := RunPrompt(ctx, "hello", deps)
	err := <-done

	assert.NoError(t, err)
	assert.Equal(t, "new-id", appState.CurrentSessionID(), "Should have fallen back to a new session ID")
	store.AssertExpectations(t)
}
