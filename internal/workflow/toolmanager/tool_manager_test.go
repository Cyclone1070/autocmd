package toolmanager

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/Cyclone1070/iav/internal/provider"
	"github.com/Cyclone1070/iav/internal/tool"
	"github.com/Cyclone1070/iav/internal/workflow"
	"github.com/stretchr/testify/assert"
)

// -- Mocks --

type mockInvocation struct {
	llmContent string
	display    tool.ToolDisplay
	err        error
}

func (i *mockInvocation) Display() tool.ToolDisplay { return i.display }
func (i *mockInvocation) Execute(ctx context.Context) (string, error) {
	return i.llmContent, i.err
}

type mockTool struct {
	name        string
	declaration tool.Declaration
	prepareFunc func(ctx context.Context, params json.RawMessage) (tool.Invocation, error)
}

func (m *mockTool) Name() string                  { return m.name }
func (m *mockTool) Declaration() tool.Declaration { return m.declaration }
func (m *mockTool) Prepare(ctx context.Context, params json.RawMessage) (tool.Invocation, error) {
	if m.prepareFunc != nil {
		return m.prepareFunc(ctx, params)
	}
	return &mockInvocation{llmContent: "ok", display: tool.StringDisplay("ok")}, nil
}

// -- Tests --

func TestRegister_AddsTool(t *testing.T) {
	tm := NewToolManager()
	mt := &mockTool{name: "test-tool", declaration: tool.Declaration{Name: "test-tool"}}
	tm.Register(mt)

	decls := tm.Declarations()
	assert.Len(t, decls, 1)
	assert.Equal(t, "test-tool", decls[0].Name)
}

func TestRegister_DuplicateName(t *testing.T) {
	tm := NewToolManager()
	mt1 := &mockTool{name: "test-tool", declaration: tool.Declaration{Name: "test-tool", Description: "v1"}}
	mt2 := &mockTool{name: "test-tool", declaration: tool.Declaration{Name: "test-tool", Description: "v2"}}

	tm.Register(mt1)
	tm.Register(mt2)

	decls := tm.Declarations()
	assert.Len(t, decls, 1)
	assert.Equal(t, "v2", decls[0].Description)
}

func TestDeclarations_SortedByName(t *testing.T) {
	tm := NewToolManager()
	tm.Register(&mockTool{name: "z", declaration: tool.Declaration{Name: "z"}})
	tm.Register(&mockTool{name: "a", declaration: tool.Declaration{Name: "a"}})
	tm.Register(&mockTool{name: "m", declaration: tool.Declaration{Name: "m"}})

	decls := tm.Declarations()
	assert.Len(t, decls, 3)
	assert.Equal(t, "a", decls[0].Name)
	assert.Equal(t, "m", decls[1].Name)
	assert.Equal(t, "z", decls[2].Name)
}

func TestExecute_UnknownTool_ReturnsMessageToLLM(t *testing.T) {
	tm := NewToolManager()
	res, err := tm.Execute(context.Background(), provider.ToolCall{
		ID:       "tc-123",
		Function: provider.FunctionCall{Name: "unknown"},
	}, nil)

	assert.NoError(t, err)
	assert.Equal(t, "tc-123", res.ToolCallID)
	assert.Contains(t, res.Content, "Error: tool \"unknown\" does not exist")
}

func TestExecute_ValidJSON_ParsesCorrectly(t *testing.T) {
	tm := NewToolManager()
	var capturedParams json.RawMessage
	tm.Register(&mockTool{
		name: "test",
		prepareFunc: func(ctx context.Context, params json.RawMessage) (tool.Invocation, error) {
			capturedParams = params
			return &mockInvocation{llmContent: "ok"}, nil
		},
	})

	_, err := tm.Execute(context.Background(), provider.ToolCall{
		ID: "tc-456",
		Function: provider.FunctionCall{
			Name:      "test",
			Arguments: json.RawMessage(`{"value": "hello"}`),
		},
	}, nil)

	assert.NoError(t, err)
	assert.Equal(t, `{"value": "hello"}`, string(capturedParams))
}

func TestExecute_PrepareFail_ReturnsMessageToLLM(t *testing.T) {
	tm := NewToolManager()
	tm.Register(&mockTool{
		name: "test",
		prepareFunc: func(ctx context.Context, params json.RawMessage) (tool.Invocation, error) {
			return nil, fmt.Errorf("bad params")
		},
	})

	res, err := tm.Execute(context.Background(), provider.ToolCall{
		ID: "tc-789",
		Function: provider.FunctionCall{
			Name: "test",
		},
	}, nil)

	assert.NoError(t, err)
	assert.Contains(t, res.Content, "Error: failed to prepare tool \"test\": bad params")
}

func TestExecute_EmitsToolEvents(t *testing.T) {
	tm := NewToolManager()
	tm.Register(&mockTool{
		name: "test",
		prepareFunc: func(ctx context.Context, params json.RawMessage) (tool.Invocation, error) {
			return &mockInvocation{
				llmContent: "result",
				display:    tool.StringDisplay("display output"),
			}, nil
		},
	})

	events := make(chan workflow.Event, 10)
	_, err := tm.Execute(context.Background(), provider.ToolCall{
		ID: "tc-1",
		Function: provider.FunctionCall{
			Name: "test",
		},
	}, events)

	assert.NoError(t, err)

	e1 := <-events
	start, ok := e1.(workflow.ToolStartEvent)
	assert.True(t, ok)
	assert.Equal(t, "tc-1", start.CallID)
	assert.Equal(t, "test", start.ToolName)
	assert.Equal(t, tool.StringDisplay("display output"), start.Display)

	e2 := <-events
	end, ok := e2.(workflow.ToolEndEvent)
	assert.True(t, ok)
	assert.Equal(t, "tc-1", end.CallID)
	assert.Empty(t, end.Error)
}

func TestExecute_Shell_StreamsAndEnds(t *testing.T) {
	tm := NewToolManager()
	tm.Register(&mockTool{
		name: "shell",
		prepareFunc: func(ctx context.Context, params json.RawMessage) (tool.Invocation, error) {
			return &mockInvocation{
				llmContent: "Command finished",
				display: tool.ShellDisplay{
					Command: "ls",
					Output:  strings.NewReader("file1\nfile2\n"),
					Wait:    func() {},
				},
			}, nil
		},
	})

	events := make(chan workflow.Event, 10)
	_, err := tm.Execute(context.Background(), provider.ToolCall{
		ID: "tc-shell",
		Function: provider.FunctionCall{
			Name: "shell",
		},
	}, events)

	assert.NoError(t, err)

	// ToolStartEvent
	<-events

	var streamOutput strings.Builder
loop:
	for {
		e := <-events
		switch ev := e.(type) {
		case workflow.ToolStreamEvent:
			assert.Equal(t, "tc-shell", ev.CallID)
			streamOutput.WriteString(ev.Chunk)
		case workflow.ToolEndEvent:
			assert.Equal(t, "tc-shell", ev.CallID)
			assert.Empty(t, ev.Error)
			break loop
		}
	}

	assert.Equal(t, "file1\nfile2\n", streamOutput.String())
}

func TestExecute_ExecuteFail_EmitsErrorEvent(t *testing.T) {
	tm := NewToolManager()
	tm.Register(&mockTool{
		name: "fail",
		prepareFunc: func(ctx context.Context, params json.RawMessage) (tool.Invocation, error) {
			return &mockInvocation{
				llmContent: "Detailed error in content",
				err:        fmt.Errorf("infra failure"),
			}, nil
		},
	})

	events := make(chan workflow.Event, 10)
	res, err := tm.Execute(context.Background(), provider.ToolCall{
		ID:       "tc-fail",
		Function: provider.FunctionCall{Name: "fail"},
	}, events)

	assert.NoError(t, err)
	assert.Equal(t, "Detailed error in content", res.Content)

	// ToolStartEvent
	<-events

	// ToolEndEvent
	e2 := <-events
	end, ok := e2.(workflow.ToolEndEvent)
	assert.True(t, ok)
	assert.Equal(t, "tc-fail", end.CallID)
	assert.Equal(t, "Execution failed", end.Error)
}

func TestExecute_ConcurrentCalls_NoRace(t *testing.T) {
	tm := NewToolManager()
	tm.Register(&mockTool{name: "tool"})

	results := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(id int) {
			_, err := tm.Execute(context.Background(), provider.ToolCall{
				ID:       fmt.Sprintf("tc-%d", id),
				Function: provider.FunctionCall{Name: "tool", Arguments: json.RawMessage(`{}`)},
			}, nil)
			results <- (err == nil)
		}(i)
	}

	for i := 0; i < 10; i++ {
		assert.True(t, <-results)
	}
}

func TestDeclarations_Sorted(t *testing.T) {
	tm := NewToolManager()
	tm.Register(&mockTool{name: "z"})
	tm.Register(&mockTool{name: "a"})
	tm.Register(&mockTool{name: "m"})

	decls := tm.Declarations()
	var names []string
	for _, d := range decls {
		names = append(names, d.Name)
	}
	assert.True(t, sort.StringsAreSorted(names))
}
