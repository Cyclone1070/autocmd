package agent

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/assert"
)

// ptr is defined in loop_test.go in the same package.

// --- Mocks ---

type mockTool struct {
	name        string
	description string
	prepare     func(ctx context.Context, params string) (domain.Invocation, error)
}

func (mt *mockTool) Name() string { return mt.name }
func (mt *mockTool) Definition() *schema.ToolInfo {
	return &schema.ToolInfo{Name: mt.name, Desc: mt.description}
}
func (mt *mockTool) Prepare(ctx context.Context, params string) (domain.Invocation, error) {
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

func (m *mockToolRegistry) Definitions() []*schema.ToolInfo {
	var defs []*schema.ToolInfo
	for _, t := range m.tools {
		defs = append(defs, t.Definition())
	}
	sort.Slice(defs, func(i, j int) bool {
		return defs[i].Name < defs[j].Name
	})
	return defs
}

func (m *mockToolRegistry) Get(name string) (domain.Tool, bool) {
	t, ok := m.tools[name]
	return t, ok
}

// Ensure mockToolRegistry implements local toolRegistry
var _ toolRegistry = (*mockToolRegistry)(nil)

// --- Tests ---

func TestRegister_DuplicateName(t *testing.T) {
	mt1 := &mockTool{name: "test-tool", description: "v1"}
	mt2 := &mockTool{name: "test-tool", description: "v2"}

	registry := newMockToolRegistry([]domain.Tool{mt1, mt2})
	executor := newToolExecutor(registry)

	defs := executor.definitions()
	assert.Len(t, defs, 1)
	assert.Equal(t, "v2", defs[0].Desc)
}

func TestDeclarations_SortedByName(t *testing.T) {
	registry := newMockToolRegistry([]domain.Tool{
		&mockTool{name: "z"},
		&mockTool{name: "a"},
		&mockTool{name: "m"},
	})
	executor := newToolExecutor(registry)

	defs := executor.definitions()
	assert.Len(t, defs, 3)
	assert.Equal(t, "a", defs[0].Name)
	assert.Equal(t, "m", defs[1].Name)
	assert.Equal(t, "z", defs[2].Name)
}

func TestExecute_UnknownTool_ReturnsMessageToLLM(t *testing.T) {
	registry := newMockToolRegistry([]domain.Tool{})
	executor := newToolExecutor(registry)
	res, _, err := executor.execute(context.Background(), &schema.ToolCall{
		ID: "tc-123",
		Function: schema.FunctionCall{
			Name: "unknown",
		},
	}, nil)

	assert.NoError(t, err)
	assert.Equal(t, "tc-123", res.ToolCallID)
	assert.Contains(t, res.Content, "Error: tool \"unknown\" does not exist")
}

func TestExecute_ValidJSON_ParsesCorrectly(t *testing.T) {
	var capturedParams string
	mt := &mockTool{
		name: "test",
		prepare: func(ctx context.Context, params string) (domain.Invocation, error) {
			capturedParams = params
			return &mockInvocation{content: "ok"}, nil
		},
	}
	registry := newMockToolRegistry([]domain.Tool{mt})
	executor := newToolExecutor(registry)

	_, _, err := executor.execute(context.Background(), &schema.ToolCall{
		ID: "tc-456",
		Function: schema.FunctionCall{
			Name:      "test",
			Arguments: `{"value": "hello"}`,
		},
	}, nil)

	assert.NoError(t, err)
	assert.Equal(t, `{"value": "hello"}`, capturedParams)
}

func TestExecute_PrepareFail_ReturnsMessageToLLM(t *testing.T) {
	mt := &mockTool{
		name: "test",
		prepare: func(ctx context.Context, params string) (domain.Invocation, error) {
			return nil, fmt.Errorf("bad params")
		},
	}
	registry := newMockToolRegistry([]domain.Tool{mt})
	executor := newToolExecutor(registry)

	sender := newMockEventSender(10)
	res, _, _ := executor.execute(context.Background(), &schema.ToolCall{
		ID: "tc-789",
		Function: schema.FunctionCall{
			Name: "test",
		},
	}, sender)

	assert.Equal(t, schema.Tool, res.Role)
	assert.Contains(t, res.Content, "failed to prepare tool \"test\": bad params")

	// Verify start event
	e1 := <-sender.events
	start, ok := e1.(domain.ToolStartEvent)
	assert.True(t, ok)
	assert.Equal(t, "tc-789", start.CallID)
	assert.Equal(t, domain.NewStringDisplay("", "Tool call failed"), start.Display)

	// Verify generic error event
	e2 := <-sender.events
	end, ok := e2.(domain.ToolEndEvent)
	assert.True(t, ok)
	assert.Equal(t, "Bad TEST request", end.Error)
}

func TestExecute_EmitsToolEvents(t *testing.T) {
	mt := &mockTool{
		name: "test",
		prepare: func(ctx context.Context, params string) (domain.Invocation, error) {
			return &mockInvocation{
				content: "result",
				display: domain.NewStringDisplay("", "display output"),
			}, nil
		},
	}
	registry := newMockToolRegistry([]domain.Tool{mt})
	executor := newToolExecutor(registry)

	sender := newMockEventSender(10)
	_, _, err := executor.execute(context.Background(), &schema.ToolCall{
		ID: "tc-1",
		Function: schema.FunctionCall{
			Name: "test",
		},
	}, sender)

	assert.NoError(t, err)

	e1 := <-sender.events
	start, ok := e1.(domain.ToolStartEvent)
	assert.True(t, ok)
	assert.Equal(t, "tc-1", start.CallID)
	assert.Equal(t, "test", start.ToolName)
	assert.Equal(t, domain.NewStringDisplay("", "display output"), start.Display)

	e2 := <-sender.events
	end, ok := e2.(domain.ToolEndEvent)
	assert.True(t, ok)
	assert.Equal(t, "tc-1", end.CallID)
	assert.Empty(t, end.Error)
}

func TestExecute_Shell_StreamsAndEnds(t *testing.T) {
	mt := &mockTool{
		name: "shell",
		prepare: func(ctx context.Context, params string) (domain.Invocation, error) {
			return &mockInvocation{
				content: "Command finished",
				display: domain.NewShellDisplay("ls", "ls", strings.NewReader("file1\nfile2\n"), nil),
			}, nil
		},
	}
	registry := newMockToolRegistry([]domain.Tool{mt})
	executor := newToolExecutor(registry)

	sender := newMockEventSender(10)
	_, _, err := executor.execute(context.Background(), &schema.ToolCall{
		ID: "tc-shell",
		Function: schema.FunctionCall{
			Name: "shell",
		},
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
		prepare: func(ctx context.Context, params string) (domain.Invocation, error) {
			return &mockInvocation{
				content: "Detailed error in content",
				err:     fmt.Errorf("infra failure"),
			}, nil
		},
	}
	registry := newMockToolRegistry([]domain.Tool{mt})
	executor := newToolExecutor(registry)

	sender := newMockEventSender(10)
	res, _, err := executor.execute(context.Background(), &schema.ToolCall{
		ID: "tc-fail",
		Function: schema.FunctionCall{
			Name: "fail",
		},
	}, sender)

	assert.NoError(t, err)
	assert.Equal(t, "Error: execution failed: infra failure", res.Content)

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
	output := io.NopCloser(strings.NewReader("some output"))

	mt := &mockTool{
		name: "shell",
		prepare: func(ctx context.Context, params string) (domain.Invocation, error) {
			return &mockInvocation{
				display: domain.NewShellDisplay("Header", "cmd", output, nil),
				execute: func(ctx context.Context) (string, error) {
					return "specific error", fmt.Errorf("execution failed")
				},
			}, nil
		},
	}

	registry := newMockToolRegistry([]domain.Tool{mt})
	executor := newToolExecutor(registry)
	sender := newMockEventSender(10)

	_, _, err := executor.execute(context.Background(), &schema.ToolCall{
		ID: "call-1",
		Function: schema.FunctionCall{
			Name: "shell",
		},
	}, sender)

	assert.NoError(t, err)

	var endEvents []domain.ToolEndEvent
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

	assert.Equal(t, 1, len(endEvents), "Must receive exactly ONE completion event (avoid race override). Got: %v", endEvents)
	assert.Equal(t, "Execution failed", endEvents[0].Error, "Completion event must retain the execution error status")
}

func TestExecute_ConcurrentCalls_NoRace(t *testing.T) {
	registry := newMockToolRegistry([]domain.Tool{&mockTool{name: "tool"}})
	executor := newToolExecutor(registry)

	results := make(chan bool, 10)
	for i := range 10 {
		go func(id int) {
			_, _, err := executor.execute(context.Background(), &schema.ToolCall{
				ID: fmt.Sprintf("tc-%d", id),
				Function: schema.FunctionCall{
					Name:      "tool",
					Arguments: "{}",
				},
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

	res, _, err := executor.execute(ctx, &schema.ToolCall{
		ID: "tc-cancel",
		Function: schema.FunctionCall{
			Name: "test",
		},
	}, nil)

	assert.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, schema.Tool, res.Role)
	assert.Equal(t, "tc-cancel", res.ToolCallID)
	assert.True(t, res.Extra["tool_error"].(bool))
	assert.Equal(t, "execution cancelled", res.Content)
}

func TestToolExecutor_Throughput_Batching(t *testing.T) {
	data := strings.Repeat("A", 8192)

	mt := &mockTool{
		name: "throughput-test",
		prepare: func(ctx context.Context, params string) (domain.Invocation, error) {
			return &mockInvocation{
				content: "done",
				display: domain.NewShellDisplay("test", "test", strings.NewReader(data), nil),
			}, nil
		},
	}

	registry := newMockToolRegistry([]domain.Tool{mt})
	executor := newToolExecutor(registry)

	sender := newMockEventSender(100)

	_, _, err := executor.execute(context.Background(), &schema.ToolCall{
		ID: "tc-1",
		Function: schema.FunctionCall{
			Name: "throughput-test",
		},
	}, sender)

	assert.NoError(t, err)

	eventCount := 0
	totalReceived := 0
loop:
	for {
		select {
		case ev := <-sender.events:
			if se, ok := ev.(domain.ToolStreamEvent); ok {
				eventCount++
				totalReceived += len(se.Chunk)
			}
			if _, ok := ev.(domain.ToolEndEvent); ok {
				break loop
			}
		case <-time.After(1 * time.Second):
			t.Fatal("timed out waiting for events")
		}
	}

	assert.Equal(t, 8192, totalReceived, "Should receive all 8KB of data")
	assert.Equal(t, 1, eventCount, "Should receive all 8KB in a single event when buffer is 1MB")
}
