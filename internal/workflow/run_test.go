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
	"github.com/stretchr/testify/require"
)

// --- Mocks for Workflow tests ---

type mockLLM struct {
	id            string
	displayName   string
	contextWindow int
	streams       []*mockStream
	streamErr     error // Error starting stream
}

func (m *mockLLM) ID() string          { return m.id }
func (m *mockLLM) DisplayName() string { return m.displayName }
func (m *mockLLM) ContextWindow() int  { return m.contextWindow }

func (m *mockLLM) ComputeTokens(ctx context.Context, msgs []domain.Message) (int, error) {
	return 100, nil
}

func (m *mockLLM) Stream(ctx context.Context, msgs []domain.Message, tools []domain.Declaration) (domain.Stream, error) {
	if m.streamErr != nil && len(m.streams) == 0 {
		return nil, m.streamErr
	}
	if len(m.streams) == 0 {
		return nil, fmt.Errorf("no more streams")
	}
	s := m.streams[0]
	m.streams = m.streams[1:]
	return s, nil
}

type mockStream struct {
	chunks []domain.StreamChunk
	err    error
	index  int
}

func (m *mockStream) Next() bool {
	if m.index < len(m.chunks) {
		m.index++
		return true
	}
	return false
}

func (m *mockStream) Chunk() domain.StreamChunk {
	return m.chunks[m.index-1]
}

func (m *mockStream) Err() error {
	return m.err
}

type mockLLMRegistry struct {
	models map[string]domain.LLM
}

func (m *mockLLMRegistry) Get(ctx context.Context, id string) (domain.LLM, error) {
	model, ok := m.models[id]
	if !ok {
		return nil, fmt.Errorf("model not found: %s", id)
	}
	return model, nil
}

func (m *mockLLMRegistry) List(ctx context.Context) ([]domain.LLMInfo, error) {
	var infos []domain.LLMInfo
	for id, model := range m.models {
		infos = append(infos, domain.LLMInfo{
			ID:          id,
			DisplayName: model.DisplayName(),
		})
	}
	return infos, nil
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

func newTestWorkflow(t *testing.T, m domain.LLM, tools []domain.Tool, events chan domain.Event) *Workflow {
	cfg := &config.Config{
		Tools: config.ToolsConfig{MaxIterations: 5},
		Model: m.ID(),
	}
	registry := newMockToolRegistry(tools)
	llmRegistry := &mockLLMRegistry{
		models: map[string]domain.LLM{
			m.ID(): m,
		},
	}
	w, err := NewWorkflow(context.Background(), llmRegistry, registry, newMockSessionStore(), cfg, events)
	require.NoError(t, err)
	return w
}

// --- Tests ---

func TestRun_SingleTurn_TextOnly(t *testing.T) {
	ctx := context.Background()
	events := make(chan domain.Event, 10)

	m := &mockLLM{
		id: "test",
		streams: []*mockStream{
			{chunks: []domain.StreamChunk{domain.TextChunk{Text: "Hello!"}}},
		},
	}

	w := newTestWorkflow(t, m, []domain.Tool{}, events)
	err := w.Run(ctx, "Hi")

	assert.NoError(t, err)

	sess := w.CurrentSession()
	assert.Equal(t, 2, len(sess.Messages))
	assert.Equal(t, "Hi", sess.Messages[0].Content)
	assert.Equal(t, "Hello!", sess.Messages[1].Content)

	assert.IsType(t, domain.ThinkingEvent{}, <-events)
	assert.Equal(t, domain.TextEvent{Text: "Hello!"}, <-events)
	assert.IsType(t, domain.DoneEvent{}, <-events)
}

func TestRun_SingleToolCall(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	events := make(chan domain.Event, 10)
	m := &mockLLM{
		id: "test",
		streams: []*mockStream{
			{chunks: []domain.StreamChunk{
				domain.ToolCall{ID: "tc-1", Name: "get_weather"},
			}},
			{chunks: []domain.StreamChunk{
				domain.TextChunk{Text: "It's sunny!"},
			}},
		},
	}

	mt := &mockTool{
		name: "get_weather",
		prepare: func(ctx context.Context, params json.RawMessage) (domain.Invocation, error) {
			return &mockInvocation{content: "Sunny"}, nil
		},
	}

	w := newTestWorkflow(t, m, []domain.Tool{mt}, events)
	err := w.Run(ctx, "Weather?")

	assert.NoError(t, err)

	sess := w.CurrentSession()
	assert.Equal(t, 4, len(sess.Messages)) // User, Assist(ToolCall), ToolResp, Assist(Text)

	// Consume events
	assert.IsType(t, domain.ThinkingEvent{}, <-events)  // First thinking
	assert.IsType(t, domain.ToolStartEvent{}, <-events) // Tool start
	assert.IsType(t, domain.ToolEndEvent{}, <-events)   // Tool end
	assert.IsType(t, domain.ThinkingEvent{}, <-events)  // Second thinking
	assert.Equal(t, domain.TextEvent{Text: "It's sunny!"}, <-events)
	assert.IsType(t, domain.DoneEvent{}, <-events)
}

func TestRun_MaxIterationsExceeded_ReturnsError(t *testing.T) {
	m := &mockLLM{
		id: "infinite",
		streams: []*mockStream{
			{chunks: []domain.StreamChunk{domain.ToolCall{ID: "tc-inf", Name: "infinite"}}},
			{chunks: []domain.StreamChunk{domain.ToolCall{ID: "tc-inf", Name: "infinite"}}},
			{chunks: []domain.StreamChunk{domain.ToolCall{ID: "tc-inf", Name: "infinite"}}},
		},
	}

	mt := &mockTool{
		name: "infinite",
		prepare: func(ctx context.Context, params json.RawMessage) (domain.Invocation, error) {
			return &mockInvocation{content: "kept going"}, nil
		},
	}

	cfg := &config.Config{
		Tools: config.ToolsConfig{MaxIterations: 3},
		Model: m.ID(),
	}
	registry := newMockToolRegistry([]domain.Tool{mt})
	llmRegistry := &mockLLMRegistry{
		models: map[string]domain.LLM{
			m.ID(): m,
		},
	}
	w, err := NewWorkflow(context.Background(), llmRegistry, registry, newMockSessionStore(), cfg, nil)
	assert.NoError(t, err)

	err = w.Run(context.Background(), "go")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "max iterations (3) reached")

	sess := w.CurrentSession()
	messages := sess.Messages
	assert.Equal(t, "[Max iterations reached]", messages[len(messages)-1].Content)
}

func TestRun_ModelError_ReturnsError(t *testing.T) {
	m := &mockLLM{
		id:        "err",
		streamErr: fmt.Errorf("model fail"),
	}

	w := newTestWorkflow(t, m, []domain.Tool{}, make(chan domain.Event, 10))
	err := w.Run(context.Background(), "hi")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "LLM.Stream")
}

func TestRun_ToolError_ReturnsError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	m := &mockLLM{
		id: "test",
		streams: []*mockStream{
			{chunks: []domain.StreamChunk{
				domain.ToolCall{ID: "tc-err", Name: "tool"},
			}},
			{chunks: []domain.StreamChunk{
				domain.TextChunk{Text: "Done"},
			}},
		},
	}

	mt := &mockTool{
		name: "tool",
		prepare: func(ctx context.Context, params json.RawMessage) (domain.Invocation, error) {
			return &mockInvocation{content: "error content", err: fmt.Errorf("tool fail")}, nil
		},
	}

	// Use nil events to avoid channel overflow
	w := newTestWorkflow(t, m, []domain.Tool{mt}, nil)
	err := w.Run(ctx, "hi")

	// Tool errors are NOT propagated as Run() errors in new design
	// They are returned to LLM as message content, loop continues
	assert.NoError(t, err)
}

func TestRun_ContextCancelled_DuringThinking_ReturnsError(t *testing.T) {
	m := &mockLLM{
		id: "test",
		streams: []*mockStream{
			{chunks: []domain.StreamChunk{domain.TextChunk{Text: "ok"}}},
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	w := newTestWorkflow(t, m, []domain.Tool{}, nil)
	err := w.Run(ctx, "hi")

	assert.ErrorIs(t, err, context.Canceled)

	sess := w.CurrentSession()
	messages := sess.Messages
	assert.Equal(t, "[Session cancelled by user]", messages[len(messages)-1].Content)
}

type customMockLLM struct {
	*mockLLM
	cancel context.CancelFunc
}

func (c *customMockLLM) Stream(ctx context.Context, msgs []domain.Message, tools []domain.Declaration) (domain.Stream, error) {
	s, err := c.mockLLM.Stream(ctx, msgs, tools)
	c.cancel() // cancel mid-stream
	return s, err
}

func TestRun_ContextCancelled_DuringStream_ReturnsError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	m := &mockLLM{
		id: "test",
		streams: []*mockStream{
			{
				chunks: []domain.StreamChunk{domain.TextChunk{Text: "half message"}},
				err:    context.Canceled, // simulate stream returning error because of cancellation
			},
		},
	}

	customM := &customMockLLM{
		mockLLM: m,
		cancel:  cancel,
	}

	w := newTestWorkflow(t, customM, []domain.Tool{}, nil)

	err := w.Run(ctx, "hello")

	assert.Error(t, err)

	sess := w.CurrentSession()
	messages := sess.Messages
	// The partially received message is kept.
	assert.Equal(t, "half message", messages[len(messages)-2].Content)
	// The cancellation message is appended.
	assert.Equal(t, "[Session cancelled by user]", messages[len(messages)-1].Content)
}

func TestRun_ContextCancelled_DuringToolExecution_ReturnsError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	m := &mockLLM{
		id: "test",
		streams: []*mockStream{
			{chunks: []domain.StreamChunk{domain.ToolCall{ID: "tc-1", Name: "long_tool"}}},
		},
	}

	mt := &mockTool{
		name: "long_tool",
		prepare: func(c context.Context, params json.RawMessage) (domain.Invocation, error) {
			// Cancel context right before execute simulates user interrupt during tool runtime
			cancel()
			return &mockInvocation{content: "", err: context.Canceled}, nil
		},
	}

	w := newTestWorkflow(t, m, []domain.Tool{mt}, nil)
	err := w.Run(ctx, "run tool")

	assert.Error(t, err)

	sess := w.CurrentSession()
	messages := sess.Messages
	assert.Equal(t, "[Session cancelled by user]", messages[len(messages)-1].Content)
}

func TestGetModel(t *testing.T) {
	m := &mockLLM{id: "test-model"}
	w := newTestWorkflow(t, m, nil, nil)

	assert.Equal(t, "test-model", w.GetModel())
}

func TestSetModel(t *testing.T) {
	m1 := &mockLLM{id: "model-1"}
	m2 := &mockLLM{id: "model-2"}

	cfg := &config.Config{Model: "model-1"}
	registry := &mockLLMRegistry{
		models: map[string]domain.LLM{
			"model-1": m1,
			"model-2": m2,
		},
	}

	w, err := NewWorkflow(context.Background(), registry, newMockToolRegistry(nil), newMockSessionStore(), cfg, nil)
	require.NoError(t, err)

	t.Run("ValidModel", func(t *testing.T) {
		err := w.SetModel(context.Background(), "model-2")
		assert.NoError(t, err)
		assert.Equal(t, "model-2", w.GetModel())
		assert.Equal(t, m1, w.llm) // Should still be the original model
	})

	t.Run("InvalidModel", func(t *testing.T) {
		err := w.SetModel(context.Background(), "non-existent")
		assert.Error(t, err)
		assert.Equal(t, "model-2", w.GetModel()) // Should remain unchanged
	})
}
