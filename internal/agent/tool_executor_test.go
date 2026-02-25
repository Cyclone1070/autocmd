package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/stretchr/testify/assert"
)

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
	res, err := executor.execute(context.Background(), domain.ToolCall{
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

	_, err := executor.execute(context.Background(), domain.ToolCall{
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

	res, err := executor.execute(context.Background(), domain.ToolCall{
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
				display: domain.StringDisplay("display output"),
			}, nil
		},
	}
	registry := newMockToolRegistry([]domain.Tool{mt})
	executor := newToolExecutor(registry)

	events := make(chan domain.Event, 10)
	_, err := executor.execute(context.Background(), domain.ToolCall{
		ID:   "tc-1",
		Name: "test",
	}, events)

	assert.NoError(t, err)

	e1 := <-events
	start, ok := e1.(domain.ToolStartEvent)
	assert.True(t, ok)
	assert.Equal(t, "tc-1", start.CallID)
	assert.Equal(t, "test", start.ToolName)
	assert.Equal(t, domain.StringDisplay("display output"), start.Display)

	e2 := <-events
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
				display: domain.ShellDisplay{
					Command: "ls",
					Output:  strings.NewReader("file1\nfile2\n"),
					Wait:    func() {},
				},
			}, nil
		},
	}
	registry := newMockToolRegistry([]domain.Tool{mt})
	executor := newToolExecutor(registry)

	events := make(chan domain.Event, 10)
	_, err := executor.execute(context.Background(), domain.ToolCall{
		ID:   "tc-shell",
		Name: "shell",
	}, events)

	assert.NoError(t, err)

	// ToolStartEvent
	<-events

	var streamOutput strings.Builder
loop:
	for {
		e := <-events
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

	events := make(chan domain.Event, 10)
	res, err := executor.execute(context.Background(), domain.ToolCall{
		ID:   "tc-fail",
		Name: "fail",
	}, events)

	assert.NoError(t, err)
	assert.Equal(t, "Detailed error in content", res.Content)

	// ToolStartEvent
	<-events

	// ToolEndEvent
	e2 := <-events
	end, ok := e2.(domain.ToolEndEvent)
	assert.True(t, ok)
	assert.Equal(t, "tc-fail", end.CallID)
	assert.Equal(t, "Execution failed", end.Error)
}

func TestExecute_ConcurrentCalls_NoRace(t *testing.T) {
	registry := newMockToolRegistry([]domain.Tool{&mockTool{name: "tool"}})
	executor := newToolExecutor(registry)

	results := make(chan bool, 10)
	for i := range 10 {
		go func(id int) {
			_, err := executor.execute(context.Background(), domain.ToolCall{
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

func TestDeclarations_Sorted(t *testing.T) {
	registry := newMockToolRegistry([]domain.Tool{
		&mockTool{name: "z"},
		&mockTool{name: "a"},
		&mockTool{name: "m"},
	})
	executor := newToolExecutor(registry)

	decls := executor.declarations()
	var names []string
	for _, d := range decls {
		names = append(names, d.Name)
	}
	assert.True(t, sort.StringsAreSorted(names))
}
