package workflow

import (
	"context"
	"testing"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/state"
	tea "github.com/charmbracelet/bubbletea"
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

func (m *mockSessionStore) GenerateName(ctx context.Context, llm domain.LLM, sess *domain.Session, input string) (string, error) {
	args := m.Called(ctx, llm, sess, input)
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

type mockRunner struct {
	mock.Mock
}

func (m *mockRunner) Run(model tea.Model) error {
	args := m.Called(model)
	return args.Error(0)
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
	runner := new(mockRunner)

	appState := &state.State{}
	// cfg is not needed by PromptDeps

	agent := new(mockAgent)
	bus := NewEventBus()

	deps := &PromptDeps{
		Store:        store,
		LLM:          llm,
		State:        appState,
		ToolRegistry: registry,
		Runner:       runner,
		Agent:        agent,
		UI:           nil, // Model can be nil in this mock test
		Bus:          bus,
	}

	// 1. Session Lifecycle Expectations
	store.On("Create").Return(&domain.Session{ID: "new-id"}, nil)
	store.On("Save", mock.Anything).Return(nil)

	// 2. Auto-naming expectations
	store.On("GenerateName", mock.Anything, mock.Anything, mock.Anything, "hello").Return("New Session", nil)

	// 3. Agent Loop expectations
	agent.On("Run", mock.Anything, mock.Anything, "hello").Return(nil)

	// 4. UI expectations (mock Run to return immediately)
	runner.On("Run", mock.Anything).Return(nil)

	err := RunPrompt(ctx, "hello", deps)

	assert.NoError(t, err)
	assert.Equal(t, "new-id", appState.CurrentSessionID())
	store.AssertExpectations(t)
	llm.AssertExpectations(t)
	runner.AssertExpectations(t)
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
	*EventBus
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
	runner := new(mockRunner)
	agent := new(mockAgent)
	
	eb := NewEventBus()
	bus := &trackableBus{EventBus: eb}

	deps := &PromptDeps{
		Store:        store,
		LLM:          llm,
		State:        &state.State{},
		ToolRegistry: registry,
		Runner:       runner,
		Agent:        agent,
		Bus:          bus.EventBus, // Current PromptDeps expects *EventBus
	}

	store.On("Create").Return(&domain.Session{ID: "id"}, nil)
	store.On("Save", mock.Anything).Return(nil)
	store.On("GenerateName", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return("New Session", nil)
	agent.On("Run", mock.Anything, mock.Anything, mock.Anything).Return(nil)
	runner.On("Run", mock.Anything).Return(nil)

	err := RunPrompt(ctx, "hello", deps)
	assert.NoError(t, err)

	// In the NEW architecture, the bus should still be open here
	// because RunPrompt no longer calls Close().
	assert.False(t, bus.closed, "RunPrompt should NOT close the bus; that is now wiring responsibility")
}
