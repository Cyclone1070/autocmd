package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/Cyclone1070/iav/internal/config"
	"github.com/Cyclone1070/iav/internal/provider"
	"github.com/Cyclone1070/iav/internal/session"
	"github.com/Cyclone1070/iav/internal/tool"
	"github.com/stretchr/testify/assert"
)

// --- Mocks for Workflow tests ---

type mockProvider struct {
	generateFunc func(ctx context.Context, model string, messages []provider.Message, tools []tool.Declaration) (*provider.Message, error)
}

func (m *mockProvider) Generate(ctx context.Context, model string, messages []provider.Message, tools []tool.Declaration) (*provider.Message, error) {
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
	prepare func(ctx context.Context, params json.RawMessage) (tool.Invocation, error)
}

func (mt *mockTool) Name() string { return mt.name }
func (mt *mockTool) Declaration() tool.Declaration {
	return tool.Declaration{Name: mt.name}
}
func (mt *mockTool) Prepare(ctx context.Context, params json.RawMessage) (tool.Invocation, error) {
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
func (m *mockInvocation) Display() tool.ToolDisplay { return nil }

// --- Helper to create test workflow ---

func newTestWorkflow(t *testing.T, mp llmProvider, tools []Tool, events chan Event) *Workflow {
	tmpDir := t.TempDir()
	store := session.NewStore(&config.Config{Session: config.SessionConfig{StorageDir: tmpDir}})
	cfg := &config.Config{Tools: config.ToolsConfig{MaxIterations: 5}}
	return NewWorkflow(mp, store, cfg, events, tools)
}

// --- Tests ---

func TestRun_SingleTurn_TextOnly(t *testing.T) {
	ctx := context.Background()
	events := make(chan Event, 10)

	mp := &mockProvider{
		generateFunc: func(ctx context.Context, model string, messages []provider.Message, tools []tool.Declaration) (*provider.Message, error) {
			return &provider.Message{Role: provider.RoleAssistant, Content: "Hello!"}, nil
		},
	}

	w := newTestWorkflow(t, mp, []Tool{}, events)
	err := w.Run(ctx, "Hi")

	assert.NoError(t, err)

	sess := w.CurrentSession()
	assert.Equal(t, 2, len(sess.Messages()))
	assert.Equal(t, "Hi", sess.Messages()[0].Content)
	assert.Equal(t, "Hello!", sess.Messages()[1].Content)

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
		generateFunc: func(ctx context.Context, model string, messages []provider.Message, tools []tool.Declaration) (*provider.Message, error) {
			callCount++
			if callCount == 1 {
				return &provider.Message{
					Role: provider.RoleAssistant,
					ToolCalls: []provider.ToolCall{
						{ID: "tc-1", Function: provider.FunctionCall{Name: "get_weather"}},
					},
				}, nil
			}
			return &provider.Message{Role: provider.RoleAssistant, Content: "It's sunny!"}, nil
		},
	}

	mt := &mockTool{
		name: "get_weather",
		prepare: func(ctx context.Context, params json.RawMessage) (tool.Invocation, error) {
			return &mockInvocation{content: "Sunny"}, nil
		},
	}

	w := newTestWorkflow(t, mp, []Tool{mt}, events)
	err := w.Run(ctx, "Weather?")

	assert.NoError(t, err)
	assert.Equal(t, 2, callCount)

	sess := w.CurrentSession()
	assert.Equal(t, 4, len(sess.Messages())) // User, Assist(ToolCall), ToolResp, Assist(Text)

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
		generateFunc: func(ctx context.Context, model string, messages []provider.Message, tools []tool.Declaration) (*provider.Message, error) {
			return &provider.Message{
				Role: provider.RoleAssistant,
				ToolCalls: []provider.ToolCall{
					{ID: "tc-inf", Function: provider.FunctionCall{Name: "infinite"}},
				},
			}, nil
		},
	}

	mt := &mockTool{
		name: "infinite",
		prepare: func(ctx context.Context, params json.RawMessage) (tool.Invocation, error) {
			return &mockInvocation{content: "kept going"}, nil
		},
	}

	tmpDir := t.TempDir()
	store := session.NewStore(&config.Config{Session: config.SessionConfig{StorageDir: tmpDir}})
	cfg := &config.Config{Tools: config.ToolsConfig{MaxIterations: 3}}
	w := NewWorkflow(mp, store, cfg, nil, []Tool{mt})

	err := w.Run(context.Background(), "go")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "max iterations (3) reached")

	sess := w.CurrentSession()
	assert.Equal(t, "[Max iterations reached]", sess.Messages()[len(sess.Messages())-1].Content)
}

func TestRun_ProviderError_ReturnsError(t *testing.T) {
	mp := &mockProvider{
		generateFunc: func(ctx context.Context, model string, messages []provider.Message, tools []tool.Declaration) (*provider.Message, error) {
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
		generateFunc: func(ctx context.Context, model string, messages []provider.Message, tools []tool.Declaration) (*provider.Message, error) {
			callCount++
			if callCount == 1 {
				return &provider.Message{
					Role: provider.RoleAssistant,
					ToolCalls: []provider.ToolCall{
						{ID: "tc-err", Function: provider.FunctionCall{Name: "tool"}},
					},
				}, nil
			}
			// After tool execution, return text to end loop
			return &provider.Message{Role: provider.RoleAssistant, Content: "Done"}, nil
		},
	}

	mt := &mockTool{
		name: "tool",
		prepare: func(ctx context.Context, params json.RawMessage) (tool.Invocation, error) {
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
		generateFunc: func(ctx context.Context, model string, messages []provider.Message, tools []tool.Declaration) (*provider.Message, error) {
			return &provider.Message{Role: provider.RoleAssistant, Content: "ok"}, nil
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	w := newTestWorkflow(t, mp, []Tool{}, nil)
	err := w.Run(ctx, "hi")

	assert.ErrorIs(t, err, context.Canceled)

	sess := w.CurrentSession()
	assert.Equal(t, "[Session cancelled by user]", sess.Messages()[len(sess.Messages())-1].Content)
}
