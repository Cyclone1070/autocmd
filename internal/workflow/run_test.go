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
	name      string
	models    []domain.Model
	streams   []*mockStream
	streamErr error // Error starting stream
}

func (m *mockProvider) Name() string { return m.name }
func (m *mockProvider) ListModels(ctx context.Context) ([]domain.Model, error) {
	return m.models, nil
}

func (m *mockProvider) Stream(ctx context.Context, model string, msgs []domain.Message, tools []domain.Declaration) (domain.Stream, error) {
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

type mockProviderRegistry struct {
	providers map[string]domain.Provider
}

func (m *mockProviderRegistry) Get(name string) (domain.Provider, bool) {
	p, ok := m.providers[name]
	return p, ok
}

func (m *mockProviderRegistry) List() []string {
	var names []string
	for n := range m.providers {
		names = append(names, n)
	}
	return names
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

func newTestWorkflow(t *testing.T, p domain.Provider, tools []domain.Tool, events chan Event) *Workflow {
	cfg := &config.Config{Tools: config.ToolsConfig{MaxIterations: 5}}
	registry := newMockToolRegistry(tools)
	provRegistry := &mockProviderRegistry{
		providers: map[string]domain.Provider{
			p.Name(): p,
		},
	}
	w := NewWorkflow(provRegistry, registry, newMockSessionStore(), cfg, events)
	w.SetProvider(p.Name())
	return w
}

// --- Tests ---

func TestRun_SingleTurn_TextOnly(t *testing.T) {
	ctx := context.Background()
	events := make(chan Event, 10)

	p := &mockProvider{
		name: "test",
		streams: []*mockStream{
			{chunks: []domain.StreamChunk{domain.TextChunk{Text: "Hello!"}}},
		},
	}

	w := newTestWorkflow(t, p, []domain.Tool{}, events)
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
	p := &mockProvider{
		name: "test",
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

	w := newTestWorkflow(t, p, []domain.Tool{mt}, events)
	err := w.Run(ctx, "Weather?")

	assert.NoError(t, err)

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
	p := &mockProvider{
		name: "infinite",
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

	cfg := &config.Config{Tools: config.ToolsConfig{MaxIterations: 3}}
	registry := newMockToolRegistry([]domain.Tool{mt})
	provRegistry := &mockProviderRegistry{
		providers: map[string]domain.Provider{
			p.Name(): p,
		},
	}
	w := NewWorkflow(provRegistry, registry, newMockSessionStore(), cfg, nil)
	w.SetProvider(p.Name())

	err := w.Run(context.Background(), "go")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "max iterations (3) reached")

	sess := w.CurrentSession()
	messages := sess.Messages
	assert.Equal(t, "[Max iterations reached]", messages[len(messages)-1].Content)
}

func TestRun_ProviderError_ReturnsError(t *testing.T) {
	p := &mockProvider{
		name:      "err",
		streamErr: fmt.Errorf("provider fail"),
	}

	w := newTestWorkflow(t, p, []domain.Tool{}, make(chan Event, 10))
	err := w.Run(context.Background(), "hi")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "provider.Stream")
}

func TestRun_ToolError_ReturnsError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	p := &mockProvider{
		name: "test",
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
	w := newTestWorkflow(t, p, []domain.Tool{mt}, nil)
	err := w.Run(ctx, "hi")

	// Tool errors are NOT propagated as Run() errors in new design
	// They are returned to LLM as message content, loop continues
	assert.NoError(t, err)
}

func TestRun_ContextCancelled_DuringThinking_ReturnsError(t *testing.T) {
	p := &mockProvider{
		name: "test",
		streams: []*mockStream{
			{chunks: []domain.StreamChunk{domain.TextChunk{Text: "ok"}}},
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	w := newTestWorkflow(t, p, []domain.Tool{}, nil)
	err := w.Run(ctx, "hi")

	assert.ErrorIs(t, err, context.Canceled)

	sess := w.CurrentSession()
	messages := sess.Messages
	assert.Equal(t, "[Session cancelled by user]", messages[len(messages)-1].Content)
}
