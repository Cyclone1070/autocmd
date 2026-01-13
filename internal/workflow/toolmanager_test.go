package workflow

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

// --- Mocks for toolManager tests ---

type tmMockTool struct {
	name        string
	description string
	prepare     func(ctx context.Context, params json.RawMessage) (domain.Invocation, error)
}

func (mt *tmMockTool) Name() string { return mt.name }
func (mt *tmMockTool) Declaration() domain.Declaration {
	return domain.Declaration{Name: mt.name, Description: mt.description}
}
func (mt *tmMockTool) Prepare(ctx context.Context, params json.RawMessage) (domain.Invocation, error) {
	if mt.prepare != nil {
		return mt.prepare(ctx, params)
	}
	return &tmMockInvocation{content: "ok", display: domain.StringDisplay("ok")}, nil
}

type tmMockInvocation struct {
	content string
	err     error
	display domain.ToolDisplay
}

func (m *tmMockInvocation) Execute(ctx context.Context) (string, error) {
	return m.content, m.err
}
func (m *tmMockInvocation) Display() domain.ToolDisplay { return m.display }

// --- Tests ---

func TestRegister_DuplicateName(t *testing.T) {
	mt1 := &tmMockTool{name: "test-tool", description: "v1"}
	mt2 := &tmMockTool{name: "test-tool", description: "v2"}

	tm := newToolManager([]Tool{mt1, mt2})

	decls := tm.declarations()
	assert.Len(t, decls, 1)
	assert.Equal(t, "v2", decls[0].Description)
}

func TestDeclarations_SortedByName(t *testing.T) {
	tm := newToolManager([]Tool{
		&tmMockTool{name: "z"},
		&tmMockTool{name: "a"},
		&tmMockTool{name: "m"},
	})

	decls := tm.declarations()
	assert.Len(t, decls, 3)
	assert.Equal(t, "a", decls[0].Name)
	assert.Equal(t, "m", decls[1].Name)
	assert.Equal(t, "z", decls[2].Name)
}

func TestExecute_UnknownTool_ReturnsMessageToLLM(t *testing.T) {
	tm := newToolManager([]Tool{})
	res, err := tm.execute(context.Background(), domain.ToolCall{
		ID:       "tc-123",
		Function: domain.FunctionCall{Name: "unknown"},
	}, nil)

	assert.NoError(t, err)
	assert.Equal(t, "tc-123", res.ToolCallID)
	assert.Contains(t, res.Content, "Error: tool \"unknown\" does not exist")
}

func TestExecute_ValidJSON_ParsesCorrectly(t *testing.T) {
	var capturedParams []byte
	mt := &tmMockTool{
		name: "test",
		prepare: func(ctx context.Context, params json.RawMessage) (domain.Invocation, error) {
			capturedParams = params
			return &tmMockInvocation{content: "ok"}, nil
		},
	}
	tm := newToolManager([]Tool{mt})

	_, err := tm.execute(context.Background(), domain.ToolCall{
		ID: "tc-456",
		Function: domain.FunctionCall{
			Name:      "test",
			Arguments: json.RawMessage(`{"value": "hello"}`),
		},
	}, nil)

	assert.NoError(t, err)
	assert.Equal(t, `{"value": "hello"}`, string(capturedParams))
}

func TestExecute_PrepareFail_ReturnsMessageToLLM(t *testing.T) {
	mt := &tmMockTool{
		name: "test",
		prepare: func(ctx context.Context, params json.RawMessage) (domain.Invocation, error) {
			return nil, fmt.Errorf("bad params")
		},
	}
	tm := newToolManager([]Tool{mt})

	res, err := tm.execute(context.Background(), domain.ToolCall{
		ID: "tc-789",
		Function: domain.FunctionCall{
			Name: "test",
		},
	}, nil)

	assert.NoError(t, err)
	assert.Contains(t, res.Content, "Error: failed to prepare tool \"test\": bad params")
}

func TestExecute_EmitsToolEvents(t *testing.T) {
	mt := &tmMockTool{
		name: "test",
		prepare: func(ctx context.Context, params json.RawMessage) (domain.Invocation, error) {
			return &tmMockInvocation{
				content: "result",
				display: domain.StringDisplay("display output"),
			}, nil
		},
	}
	tm := newToolManager([]Tool{mt})

	events := make(chan Event, 10)
	_, err := tm.execute(context.Background(), domain.ToolCall{
		ID: "tc-1",
		Function: domain.FunctionCall{
			Name: "test",
		},
	}, events)

	assert.NoError(t, err)

	e1 := <-events
	start, ok := e1.(ToolStartEvent)
	assert.True(t, ok)
	assert.Equal(t, "tc-1", start.CallID)
	assert.Equal(t, "test", start.ToolName)
	assert.Equal(t, domain.StringDisplay("display output"), start.Display)

	e2 := <-events
	end, ok := e2.(ToolEndEvent)
	assert.True(t, ok)
	assert.Equal(t, "tc-1", end.CallID)
	assert.Empty(t, end.Error)
}

func TestExecute_Shell_StreamsAndEnds(t *testing.T) {
	mt := &tmMockTool{
		name: "shell",
		prepare: func(ctx context.Context, params json.RawMessage) (domain.Invocation, error) {
			return &tmMockInvocation{
				content: "Command finished",
				display: domain.ShellDisplay{
					Command: "ls",
					Output:  strings.NewReader("file1\nfile2\n"),
					Wait:    func() {},
				},
			}, nil
		},
	}
	tm := newToolManager([]Tool{mt})

	events := make(chan Event, 10)
	_, err := tm.execute(context.Background(), domain.ToolCall{
		ID: "tc-shell",
		Function: domain.FunctionCall{
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
		case ToolStreamEvent:
			assert.Equal(t, "tc-shell", ev.CallID)
			streamOutput.WriteString(ev.Chunk)
		case ToolEndEvent:
			assert.Equal(t, "tc-shell", ev.CallID)
			assert.Empty(t, ev.Error)
			break loop
		}
	}

	assert.Equal(t, "file1\nfile2\n", streamOutput.String())
}

func TestExecute_ExecuteFail_EmitsErrorEvent(t *testing.T) {
	mt := &tmMockTool{
		name: "fail",
		prepare: func(ctx context.Context, params json.RawMessage) (domain.Invocation, error) {
			return &tmMockInvocation{
				content: "Detailed error in content",
				err:     fmt.Errorf("infra failure"),
			}, nil
		},
	}
	tm := newToolManager([]Tool{mt})

	events := make(chan Event, 10)
	res, err := tm.execute(context.Background(), domain.ToolCall{
		ID:       "tc-fail",
		Function: domain.FunctionCall{Name: "fail"},
	}, events)

	assert.NoError(t, err)
	assert.Equal(t, "Detailed error in content", res.Content)

	// ToolStartEvent
	<-events

	// ToolEndEvent
	e2 := <-events
	end, ok := e2.(ToolEndEvent)
	assert.True(t, ok)
	assert.Equal(t, "tc-fail", end.CallID)
	assert.Equal(t, "Execution failed", end.Error)
}

func TestExecute_ConcurrentCalls_NoRace(t *testing.T) {
	tm := newToolManager([]Tool{&tmMockTool{name: "tool"}})

	results := make(chan bool, 10)
	for i := range 10 {
		go func(id int) {
			_, err := tm.execute(context.Background(), domain.ToolCall{
				ID:       fmt.Sprintf("tc-%d", id),
				Function: domain.FunctionCall{Name: "tool", Arguments: json.RawMessage(`{}`)},
			}, nil)
			results <- (err == nil)
		}(i)
	}

	for range 10 {
		assert.True(t, <-results)
	}
}

func TestDeclarations_Sorted(t *testing.T) {
	tm := newToolManager([]Tool{
		&tmMockTool{name: "z"},
		&tmMockTool{name: "a"},
		&tmMockTool{name: "m"},
	})

	decls := tm.declarations()
	var names []string
	for _, d := range decls {
		names = append(names, d.Name)
	}
	assert.True(t, sort.StringsAreSorted(names))
}
