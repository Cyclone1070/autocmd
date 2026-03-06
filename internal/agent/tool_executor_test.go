package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/stretchr/testify/assert"
)

// --- Mocks ---

type mockTool struct {
	name        string
	description string
	prepare     func(ctx context.Context, params json.RawMessage) (domain.Invocation, error)
}

func (mt *mockTool) Name() string { return mt.name }
func (mt *mockTool) Declaration() domain.Declaration {
	return domain.Declaration{Name: mt.name, Description: mt.description}
}
func (mt *mockTool) Prepare(ctx context.Context, params json.RawMessage) (domain.Invocation, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if mt.prepare != nil {
		return mt.prepare(ctx, params)
	}
	return &mockInvocation{content: "ok"}, nil
}

type mockInvocation struct {
	content string
	err     error
	display domain.ToolDisplay
	execute func(ctx context.Context) (string, error)
}

func (m *mockInvocation) Execute(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if m.execute != nil {
		return m.execute(ctx)
	}
	return m.content, m.err
}
func (m *mockInvocation) Display() domain.ToolDisplay { return m.display }

type mockToolRegistry struct {
	tools map[string]domain.Tool
}

func newMockToolRegistry(tools []domain.Tool) *mockToolRegistry {
	m := &mockToolRegistry{tools: make(map[string]domain.Tool)}
	for _, t := range tools {
		if t != nil {
			m.tools[t.Name()] = t
		}
	}
	return m
}

func (m *mockToolRegistry) Declarations() []domain.Declaration {
	var decls []domain.Declaration
	for _, t := range m.tools {
		decls = append(decls, t.Declaration())
	}
	sort.Slice(decls, func(i, j int) bool {
		return decls[i].Name < decls[j].Name
	})
	return decls
}

func (m *mockToolRegistry) Get(name string) (domain.Tool, bool) {
	t, ok := m.tools[name]
	return t, ok
}

// --- Tests ---

func TestRegister_DuplicateName(t *testing.T) {
	mt1 := &mockTool{name: "test-tool", description: "v1"}
	mt2 := &mockTool{name: "test-tool", description: "v2"}

	registry := newMockToolRegistry([]domain.Tool{mt1, mt2})
	executor := newToolExecutor(registry)

	decls := executor.declarations()
	assert.Len(t, decls, 1)
	assert.Equal(t, "v2", decls[0].Description)
}

func TestDeclarations_SortedByName(t *testing.T) {
	registry := newMockToolRegistry([]domain.Tool{
		&mockTool{name: "z"},
		&mockTool{name: "a"},
		&mockTool{name: "m"},
	})
	executor := newToolExecutor(registry)

	decls := executor.declarations()
	assert.Len(t, decls, 3)
	assert.Equal(t, "a", decls[0].Name)
	assert.Equal(t, "m", decls[1].Name)
	assert.Equal(t, "z", decls[2].Name)
}

func TestExecute_UnknownTool_ReturnsMessageToLLM(t *testing.T) {
	registry := newMockToolRegistry([]domain.Tool{})
	executor := newToolExecutor(registry)
	res, _, err := executor.execute(context.Background(), domain.ToolCall{
		ID:   "tc-123",
		Name: "unknown",
	}, nil)

	assert.NoError(t, err)
	assert.Equal(t, "tc-123", res.ToolCallID)
	assert.Contains(t, res.Content, "Error: tool \"unknown\" does not exist")
}

func TestExecute_ValidJSON_ParsesCorrectly(t *testing.T) {
	var capturedParams []byte
	mt := &mockTool{
		name: "test",
		prepare: func(ctx context.Context, params json.RawMessage) (domain.Invocation, error) {
			capturedParams = params
			return &mockInvocation{content: "ok"}, nil
		},
	}
	registry := newMockToolRegistry([]domain.Tool{mt})
	executor := newToolExecutor(registry)

	_, _, err := executor.execute(context.Background(), domain.ToolCall{
		ID:        "tc-456",
		Name:      "test",
		Arguments: json.RawMessage(`{"value": "hello"}`),
	}, nil)

	assert.NoError(t, err)
	assert.Equal(t, `{"value": "hello"}`, string(capturedParams))
}

func TestExecute_PrepareFail_ReturnsMessageToLLM(t *testing.T) {
	mt := &mockTool{
		name: "test",
		prepare: func(ctx context.Context, params json.RawMessage) (domain.Invocation, error) {
			return nil, fmt.Errorf("bad params")
		},
	}
	registry := newMockToolRegistry([]domain.Tool{mt})
	executor := newToolExecutor(registry)

	res, _, err := executor.execute(context.Background(), domain.ToolCall{
		ID:   "tc-789",
		Name: "test",
	}, nil)

	assert.NoError(t, err)
	assert.Contains(t, res.Content, "Error: failed to prepare tool \"test\": bad params")
}

func TestExecute_EmitsToolEvents(t *testing.T) {
	mt := &mockTool{
		name: "test",
		prepare: func(ctx context.Context, params json.RawMessage) (domain.Invocation, error) {
			return &mockInvocation{
				content: "result",
				display: domain.NewStringDisplay("display output"),
			}, nil
		},
	}
	registry := newMockToolRegistry([]domain.Tool{mt})
	executor := newToolExecutor(registry)

	sender := newMockEventSender(10)
	_, _, err := executor.execute(context.Background(), domain.ToolCall{
		ID:   "tc-1",
		Name: "test",
	}, sender)

	assert.NoError(t, err)

	e1 := <-sender.events
	start, ok := e1.(domain.ToolStartEvent)
	assert.True(t, ok)
	assert.Equal(t, "tc-1", start.CallID)
	assert.Equal(t, "test", start.ToolName)
	assert.Equal(t, domain.NewStringDisplay("display output"), start.Display)

	e2 := <-sender.events
	end, ok := e2.(domain.ToolEndEvent)
	assert.True(t, ok)
	assert.Equal(t, "tc-1", end.CallID)
	assert.Empty(t, end.Error)
}

func TestExecute_Shell_StreamsAndEnds(t *testing.T) {
	mt := &mockTool{
		name: "shell",
		prepare: func(ctx context.Context, params json.RawMessage) (domain.Invocation, error) {
			return &mockInvocation{
				content: "Command finished",
				display: domain.NewShellDisplay("ls", "ls", strings.NewReader("file1\nfile2\n"), nil),
			}, nil
		},
	}
	registry := newMockToolRegistry([]domain.Tool{mt})
	executor := newToolExecutor(registry)

	sender := newMockEventSender(10)
	_, _, err := executor.execute(context.Background(), domain.ToolCall{
		ID:   "tc-shell",
		Name: "shell",
	}, sender)

	assert.NoError(t, err)

	// ToolStartEvent
	<-sender.events

	var streamOutput strings.Builder
loop:
	for {
		e := <-sender.events
		switch ev := e.(type) {
		case domain.ToolStreamEvent:
			assert.Equal(t, "tc-shell", ev.CallID)
			streamOutput.WriteString(ev.Chunk)
		case domain.ToolEndEvent:
			assert.Equal(t, "tc-shell", ev.CallID)
			assert.Empty(t, ev.Error)
			break loop
		}
	}

	assert.Equal(t, "file1\nfile2\n", streamOutput.String())
}

func TestExecute_ExecuteFail_EmitsErrorEvent(t *testing.T) {
	mt := &mockTool{
		name: "fail",
		prepare: func(ctx context.Context, params json.RawMessage) (domain.Invocation, error) {
			return &mockInvocation{
				content: "Detailed error in content",
				err:     fmt.Errorf("infra failure"),
			}, nil
		},
	}
	registry := newMockToolRegistry([]domain.Tool{mt})
	executor := newToolExecutor(registry)

	sender := newMockEventSender(10)
	res, _, err := executor.execute(context.Background(), domain.ToolCall{
		ID:   "tc-fail",
		Name: "fail",
	}, sender)

	assert.NoError(t, err)
	assert.Equal(t, "Detailed error in content", res.Content)

	// ToolStartEvent
	<-sender.events

	// ToolEndEvent
	e2 := <-sender.events
	end, ok := e2.(domain.ToolEndEvent)
	assert.True(t, ok)
	assert.Equal(t, "tc-fail", end.CallID)
	assert.Equal(t, "Execution failed", end.Error)
}

func TestIssue6_DoubleEndEvent_Regression(t *testing.T) {
	t.Parallel()
	// Mock an invocation that acts like a shell tool:
	// It has an output stream and it returns an error in Execute.
	output := io.NopCloser(strings.NewReader("some output"))

	mt := &mockTool{
		name: "shell",
		prepare: func(ctx context.Context, params json.RawMessage) (domain.Invocation, error) {
			return &mockInvocation{
				display: domain.NewShellDisplay("Header", "cmd", output, nil),
				execute: func(ctx context.Context) (string, error) {
					// Simulate execution failure
					return "error content", fmt.Errorf("execution failed")
				},
			}, nil
		},
	}

	registry := newMockToolRegistry([]domain.Tool{mt})
	executor := newToolExecutor(registry)
	sender := newMockEventSender(10)

	_, _, err := executor.execute(context.Background(), domain.ToolCall{
		ID:   "call-1",
		Name: "shell",
	}, sender)

	assert.NoError(t, err)

	// Collect events sent to the UI
	var endEvents []domain.ToolEndEvent
	// Drain the sender channel to see what we received
loop:
	for {
		select {
		case ev := <-sender.events:
			if ee, ok := ev.(domain.ToolEndEvent); ok {
				endEvents = append(endEvents, ee)
			}
		case <-time.After(50 * time.Millisecond):
			break loop
		}
	}

	// REGRESSION SPECIFICATION:
	// 1. Should receive EXACTLY one completion event.
	// 2. The event MUST contain the true error status from Execute().
	assert.Equal(t, 1, len(endEvents), "Must receive exactly ONE completion event (avoid race override). Got: %v", endEvents)
	assert.Equal(t, "Execution failed", endEvents[0].Error, "Completion event must retain the execution error status")
}

func TestExecute_ConcurrentCalls_NoRace(t *testing.T) {
	registry := newMockToolRegistry([]domain.Tool{&mockTool{name: "tool"}})
	executor := newToolExecutor(registry)

	results := make(chan bool, 10)
	for i := range 10 {
		go func(id int) {
			_, _, err := executor.execute(context.Background(), domain.ToolCall{
				ID:        fmt.Sprintf("tc-%d", id),
				Name:      "tool",
				Arguments: json.RawMessage(`{}`),
			}, nil)
			results <- (err == nil)
		}(i)
	}

	for range 10 {
		assert.True(t, <-results)
	}
}

func TestExecute_ContextCancelled_ReturnsProperMessage(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	mt := &mockTool{name: "test"}
	registry := newMockToolRegistry([]domain.Tool{mt})
	executor := newToolExecutor(registry)

	res, _, err := executor.execute(ctx, domain.ToolCall{
		ID:   "tc-cancel",
		Name: "test",
	}, nil)

	assert.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, domain.RoleTool, res.Role())
	assert.Equal(t, "tc-cancel", res.ToolCallID)
	assert.True(t, res.ToolError)
	assert.Equal(t, "execution cancelled", res.Content)
}
