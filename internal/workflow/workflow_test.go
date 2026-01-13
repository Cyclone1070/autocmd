package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/Cyclone1070/iav/internal/config"
	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/stretchr/testify/assert"
)

// --- Mocks for Workflow tests ---

type mockProvider struct {
	generateFunc func(ctx context.Context, model string, messages []domain.Message, tools []domain.Declaration) (*domain.Message, error)
}

func (m *mockProvider) Generate(ctx context.Context, model string, messages []domain.Message, tools []domain.Declaration) (*domain.Message, error) {
	if m.generateFunc != nil {
		return m.generateFunc(ctx, model, messages, tools)
	}
	return nil, nil
}

func (m *mockProvider) ListModels(ctx context.Context) ([]string, error) {
	return []string{"model-1"}, nil
}

type mockTool struct {
	name    string
	prepare func(ctx context.Context, params json.RawMessage) (domain.Invocation, error)
}

func (mt *mockTool) Name() string { return mt.name }
func (mt *mockTool) Declaration() domain.Declaration {
	return domain.Declaration{Name: mt.name}
}
func (mt *mockTool) Prepare(ctx context.Context, params json.RawMessage) (domain.Invocation, error) {
	if mt.prepare != nil {
		return mt.prepare(ctx, params)
	}
	return nil, fmt.Errorf("not implemented")
}

type mockInvocation struct {
	content string
	err     error
}

func (m *mockInvocation) Execute(ctx context.Context) (string, error) {
	return m.content, m.err
}
func (m *mockInvocation) Display() domain.ToolDisplay { return nil }

type mockSessionStore struct {
	sessions map[string]*domain.Session
	current  *domain.Session
}

func newMockSessionStore() *mockSessionStore {
	return &mockSessionStore{sessions: make(map[string]*domain.Session)}
}

func (m *mockSessionStore) Create() (*domain.Session, error) {
	s := &domain.Session{
		ID:       fmt.Sprintf("test-%d", len(m.sessions)),
		Messages: []domain.Message{},
	}
	m.sessions[s.ID] = s
	m.current = s
	return s, nil
}

func (m *mockSessionStore) Get(id string) (*domain.Session, error) {
	if s, ok := m.sessions[id]; ok {
		return s, nil
	}
	return nil, fmt.Errorf("not found")
}

func (m *mockSessionStore) Save(s *domain.Session) error {
	m.sessions[s.ID] = s
	return nil
}

func (m *mockSessionStore) List() ([]domain.SessionSummary, error) {
	return nil, nil
}

func (m *mockSessionStore) Delete(id string) error {
	delete(m.sessions, id)
	return nil
}

// --- Helper to create test workflow ---

func newTestWorkflow(t *testing.T, mp llmProvider, tools []Tool, events chan Event) *Workflow {
	cfg := &config.Config{Tools: config.ToolsConfig{MaxIterations: 5}}
	return NewWorkflow(mp, newMockSessionStore(), cfg, events, tools)
}

// --- Tests ---

func TestRun_SingleTurn_TextOnly(t *testing.T) {
	ctx := context.Background()
	events := make(chan Event, 10)

	mp := &mockProvider{
		generateFunc: func(ctx context.Context, model string, messages []domain.Message, tools []domain.Declaration) (*domain.Message, error) {
			return &domain.Message{Role: domain.RoleAssistant, Content: "Hello!"}, nil
		},
	}

	w := newTestWorkflow(t, mp, []Tool{}, events)
	err := w.Run(ctx, "Hi")

	assert.NoError(t, err)

	sess := w.CurrentSession()
	assert.Equal(t, 2, len(sess.Messages))
	assert.Equal(t, "Hi", sess.Messages[0].Content)
	assert.Equal(t, "Hello!", sess.Messages[1].Content)

	assert.IsType(t, ThinkingEvent{}, <-events)
	assert.Equal(t, TextEvent{Text: "Hello!"}, <-events)
	assert.IsType(t, DoneEvent{}, <-events)
}

func TestRun_SingleToolCall(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	events := make(chan Event, 10)

	callCount := 0
	mp := &mockProvider{
		generateFunc: func(ctx context.Context, model string, messages []domain.Message, tools []domain.Declaration) (*domain.Message, error) {
			callCount++
			if callCount == 1 {
				return &domain.Message{
					Role: domain.RoleAssistant,
					ToolCalls: []domain.ToolCall{
						{ID: "tc-1", Function: domain.FunctionCall{Name: "get_weather"}},
					},
				}, nil
			}
			return &domain.Message{Role: domain.RoleAssistant, Content: "It's sunny!"}, nil
		},
	}

	mt := &mockTool{
		name: "get_weather",
		prepare: func(ctx context.Context, params json.RawMessage) (domain.Invocation, error) {
			return &mockInvocation{content: "Sunny"}, nil
		},
	}

	w := newTestWorkflow(t, mp, []Tool{mt}, events)
	err := w.Run(ctx, "Weather?")

	assert.NoError(t, err)
	assert.Equal(t, 2, callCount)

	sess := w.CurrentSession()
	assert.Equal(t, 4, len(sess.Messages)) // User, Assist(ToolCall), ToolResp, Assist(Text)

	// Consume events
	assert.IsType(t, ThinkingEvent{}, <-events)  // First thinking
	assert.IsType(t, ToolStartEvent{}, <-events) // Tool start
	assert.IsType(t, ToolEndEvent{}, <-events)   // Tool end
	assert.IsType(t, ThinkingEvent{}, <-events)  // Second thinking
	assert.Equal(t, TextEvent{Text: "It's sunny!"}, <-events)
	assert.IsType(t, DoneEvent{}, <-events)
}

func TestRun_MaxIterationsExceeded_ReturnsError(t *testing.T) {
	mp := &mockProvider{
		generateFunc: func(ctx context.Context, model string, messages []domain.Message, tools []domain.Declaration) (*domain.Message, error) {
			return &domain.Message{
				Role: domain.RoleAssistant,
				ToolCalls: []domain.ToolCall{
					{ID: "tc-inf", Function: domain.FunctionCall{Name: "infinite"}},
				},
			}, nil
		},
	}

	mt := &mockTool{
		name: "infinite",
		prepare: func(ctx context.Context, params json.RawMessage) (domain.Invocation, error) {
			return &mockInvocation{content: "kept going"}, nil
		},
	}

	cfg := &config.Config{Tools: config.ToolsConfig{MaxIterations: 3}}
	w := NewWorkflow(mp, newMockSessionStore(), cfg, nil, []Tool{mt})

	err := w.Run(context.Background(), "go")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "max iterations (3) reached")

	sess := w.CurrentSession()
	messages := sess.Messages
	assert.Equal(t, "[Max iterations reached]", messages[len(messages)-1].Content)
}

func TestRun_ProviderError_ReturnsError(t *testing.T) {
	mp := &mockProvider{
		generateFunc: func(ctx context.Context, model string, messages []domain.Message, tools []domain.Declaration) (*domain.Message, error) {
			return nil, fmt.Errorf("provider fail")
		},
	}

	w := newTestWorkflow(t, mp, []Tool{}, make(chan Event, 10))
	err := w.Run(context.Background(), "hi")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "provider.Generate")
}

func TestRun_ToolError_ReturnsError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	callCount := 0
	mp := &mockProvider{
		generateFunc: func(ctx context.Context, model string, messages []domain.Message, tools []domain.Declaration) (*domain.Message, error) {
			callCount++
			if callCount == 1 {
				return &domain.Message{
					Role: domain.RoleAssistant,
					ToolCalls: []domain.ToolCall{
						{ID: "tc-err", Function: domain.FunctionCall{Name: "tool"}},
					},
				}, nil
			}
			// After tool execution, return text to end loop
			return &domain.Message{Role: domain.RoleAssistant, Content: "Done"}, nil
		},
	}

	mt := &mockTool{
		name: "tool",
		prepare: func(ctx context.Context, params json.RawMessage) (domain.Invocation, error) {
			return &mockInvocation{content: "error content", err: fmt.Errorf("tool fail")}, nil
		},
	}

	// Use nil events to avoid channel overflow
	w := newTestWorkflow(t, mp, []Tool{mt}, nil)
	err := w.Run(ctx, "hi")

	// Tool errors are NOT propagated as Run() errors in new design
	// They are returned to LLM as message content, loop continues
	assert.NoError(t, err)
}

func TestRun_ContextCancelled_DuringThinking_ReturnsError(t *testing.T) {
	mp := &mockProvider{
		generateFunc: func(ctx context.Context, model string, messages []domain.Message, tools []domain.Declaration) (*domain.Message, error) {
			return &domain.Message{Role: domain.RoleAssistant, Content: "ok"}, nil
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	w := newTestWorkflow(t, mp, []Tool{}, nil)
	err := w.Run(ctx, "hi")

	assert.ErrorIs(t, err, context.Canceled)

	sess := w.CurrentSession()
	messages := sess.Messages
	assert.Equal(t, "[Session cancelled by user]", messages[len(messages)-1].Content)
}
