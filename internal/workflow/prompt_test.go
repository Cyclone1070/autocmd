package workflow

import (
	"context"
	"testing"
	"time"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/eventbus"
	"github.com/Cyclone1070/iav/internal/state"
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

func (m *mockLLM) Stream(ctx context.Context, messages domain.Messages, tools []domain.Declaration) (domain.Stream, error) {
	args := m.Called(ctx, messages, tools)
	return args.Get(0).(domain.Stream), args.Error(1)
}

func (m *mockLLM) ComputeTokens(ctx context.Context, messages domain.Messages) (int, error) {
	args := m.Called(ctx, messages)
	return args.Int(0), args.Error(1)
}

func (m *mockLLM) ContextWindow() int {
	args := m.Called()
	return args.Int(0)
}

type mockToolRegistry struct {
	mock.Mock
}

func (m *mockToolRegistry) Declarations() []domain.Declaration {
	args := m.Called()
	return args.Get(0).([]domain.Declaration)
}

func (m *mockToolRegistry) Get(name string) (domain.Tool, bool) {
	args := m.Called(name)
	return args.Get(0).(domain.Tool), args.Bool(1)
}

type mockAgent struct {
	mock.Mock
}

func (m *mockAgent) Run(ctx context.Context, sess *domain.Session, input string) error {
	args := m.Called(ctx, sess, input)
	return args.Error(0)
}

func TestRunPrompt_GREEN(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store := new(mockSessionStore)
	llm := new(mockLLM)
	registry := new(mockToolRegistry)

	appState := &state.State{}
	// cfg is not needed by PromptDeps

	agent := new(mockAgent)
	bus := eventbus.New()

	deps := &PromptDeps{
		Store:        store,
		LLM:          llm,
		State:        appState,
		ToolRegistry: registry,
		Agent:        agent,
		Bus:          bus,
	}

	// 1. Session Lifecycle Expectations
	store.On("Create").Return(&domain.Session{ID: "new-id"}, nil)
	store.On("Save", mock.Anything).Return(nil)

	// 2. Auto-naming expectations
	store.On("GenerateName", mock.Anything, mock.Anything, "hello").Return("New Session", nil)

	// 3. Agent Loop expectations
	agent.On("Run", mock.Anything, mock.Anything, "hello").Return(nil)

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
	registry := new(mockToolRegistry)
	agent := new(mockAgent)
	bus := eventbus.New()

	appState := &state.State{}
	appState.SetCurrentSessionID("existing-id")

	sess := &domain.Session{ID: "existing-id", Name: "Existing Session"}

	deps := &PromptDeps{
		Store:        store,
		LLM:          llm,
		State:        appState,
		ToolRegistry: registry,
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

type mockStream struct {
	mock.Mock
}

func newMockStream() *mockStream {
	m := &mockStream{}
	m.On("Next").Return(false)
	m.On("Err").Return(nil)
	return m
}

func (m *mockStream) Next() bool {
	args := m.Called()
	return args.Bool(0)
}

func (m *mockStream) Chunk() domain.StreamChunk {
	return nil
}

func (m *mockStream) Err() error {
	args := m.Called()
	return args.Error(0)
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
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store := new(mockSessionStore)
	llm := new(mockLLM)
	registry := new(mockToolRegistry)
	agent := new(mockAgent)
	
	eb := eventbus.New()
	bus := &trackableBus{EventBus: eb}

	deps := &PromptDeps{
		Store:        store,
		LLM:          llm,
		State:        &state.State{},
		ToolRegistry: registry,
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
	registry := new(mockToolRegistry)
	agent := new(mockAgent)
	bus := eventbus.New()

	deps := &PromptDeps{
		Store:        store,
		LLM:          llm,
		State:        &state.State{},
		ToolRegistry: registry,
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
		Run(func(args mock.Arguments) {
			<-agentStartedAppending // Wait until agent says it started appending
		}).
		Return("Named Session", nil)

	// Simulate Agent.Run appending messages and then signaling
	agent.On("Run", mock.Anything, mock.Anything, "hello").
		Run(func(args mock.Arguments) {
			s := args.Get(1).(*domain.Session)
			// Append some messages to simulate work and race
			for i := 0; i < 10; i++ {
				s.Messages = append(s.Messages, domain.UserMessage{Content: "msg"})
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
