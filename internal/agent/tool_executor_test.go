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
	"github.com/Cyclone1070/iav/internal/permission"
	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testFast1 = "fast1"
	testFast2 = "fast2"
)

// ptr is defined in loop_test.go in the same package.

// --- Mocks ---

type mockTool struct {
	name          string
	description   string
	concurrent    bool
	setConcurrent bool
	prepare       func(params string) (domain.Invocation, error)
}

func (mt *mockTool) Name() string { return mt.name }
func (mt *mockTool) IsConcurrentSafe() bool {
	if !mt.setConcurrent {
		return true
	}
	return mt.concurrent
}

func (mt *mockTool) Definition() *schema.ToolInfo {
	return &schema.ToolInfo{Name: mt.name, Desc: mt.description}
}

func (mt *mockTool) Prepare(params string) (domain.Invocation, error) {
	if mt.prepare != nil {
		return mt.prepare(params)
	}
	return &mockInvocation{content: "ok", display: domain.NewStringDisplay("", "")}, nil
}

type mockInvocation struct {
	content string
	err     error
	display domain.ToolDisplay
	execute func(ctx context.Context) (string, domain.ToolDisplay)
}

func (m *mockInvocation) Execute(ctx context.Context) (string, domain.ToolDisplay) {
	if err := ctx.Err(); err != nil {
		disp := m.display
		if disp.GetError() == "" {
			disp = withToolError(disp, domain.ToolErrorCancelled)
		}
		return domain.ToolErrorCancelled, disp
	}
	if m.execute != nil {
		return m.execute(ctx)
	}
	if m.err != nil {
		return m.content, withToolError(m.display, m.err.Error())
	}
	return m.content, m.display
}
func (m *mockInvocation) Display() domain.ToolDisplay { return m.display }

func withToolError(d domain.ToolDisplay, msg string) domain.ToolDisplay {
	switch d := d.(type) {
	case domain.StringDisplay:
		d.Error = msg
		return d
	case domain.DiffDisplay:
		d.Error = msg
		return d
	case domain.BashDisplay:
		d.Error = msg
		return d
	case domain.QuestionDisplay:
		d.Error = msg
		return d
	default:
		return d
	}
}

// mockStreamInvocation implements domain.StreamableInvocation for streaming tests.
type mockStreamInvocation struct {
	stream  io.Reader
	display domain.ToolDisplay
	content string
	err     error
}

func (m *mockStreamInvocation) Stream() io.Reader           { return m.stream }
func (m *mockStreamInvocation) Display() domain.ToolDisplay { return m.display }
func (m *mockStreamInvocation) Execute(ctx context.Context) (string, domain.ToolDisplay) {
	if err := ctx.Err(); err != nil {
		disp := m.display
		if disp.GetError() == "" {
			disp = withToolError(disp, domain.ToolErrorCancelled)
		}
		return domain.ToolErrorCancelled, disp
	}
	if m.err != nil {
		return m.content, withToolError(m.display, m.err.Error())
	}
	return m.content, m.display
}

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

type mockActionWaiter struct {
	wait func(ctx context.Context, callID string) (domain.Action, error)
}

func (m *mockActionWaiter) Wait(ctx context.Context, callID string) (domain.Action, error) {
	if m.wait != nil {
		return m.wait(ctx, callID)
	}
	return nil, nil
}

type mockInteractiveInvocation struct {
	display domain.ToolDisplay
	resolve func(ctx context.Context, action domain.Action) (string, domain.ToolDisplay)
}

func (m *mockInteractiveInvocation) Display() domain.ToolDisplay { return m.display }
func (m *mockInteractiveInvocation) Resolve(ctx context.Context, action domain.Action) (string, domain.ToolDisplay) {
	if m.resolve != nil {
		return m.resolve(ctx, action)
	}
	return "ok", m.display
}

func TestExecute_InteractiveInvocation_HappyPath(t *testing.T) {
	callID := "tc-int"
	qd := domain.QuestionDisplay{TypeField: "question"}
	action := domain.QuestionAnswerAction{CallID: callID, Answers: [][]string{{"ans"}}}

	mt := &mockTool{
		name: "ask",
		prepare: func(params string) (domain.Invocation, error) {
			return &mockInteractiveInvocation{
				display: qd,
				resolve: func(ctx context.Context, act domain.Action) (string, domain.ToolDisplay) {
					assert.Equal(t, action, act)
					return "User answered ans", domain.NewStringDisplay("Done", "")
				},
			}, nil
		},
	}

	waiter := &mockActionWaiter{
		wait: func(ctx context.Context, cid string) (domain.Action, error) {
			assert.Equal(t, callID, cid)
			return action, nil
		},
	}

	registry := newMockToolRegistry([]domain.Tool{mt})
	// This will fail to compile as NewToolExecutor doesn't take 'waiter' yet
	executor := NewToolExecutor(registry, waiter)
	sender := newMockEventSender(10)

	res, disp, err := executor.execute(context.Background(), &schema.ToolCall{
		ID:       callID,
		Function: schema.FunctionCall{Name: "ask"},
	}, sender)

	assert.NoError(t, err)
	assert.Equal(t, "User answered ans", res.Content)
	assert.Equal(t, "Done", disp.(domain.StringDisplay).Description)

	assert.IsType(t, domain.ToolStartEvent{}, <-sender.events)
	assert.IsType(t, domain.ToolEndEvent{}, <-sender.events)
}

// Ensure mockToolRegistry implements local toolRegistry.
var _ toolRegistry = (*mockToolRegistry)(nil)

// --- Tests ---

func TestRegister_DuplicateName(t *testing.T) {
	mt1 := &mockTool{name: "test-tool", description: "v1"}
	mt2 := &mockTool{name: "test-tool", description: "v2"}

	registry := newMockToolRegistry([]domain.Tool{mt1, mt2})
	executor := NewToolExecutor(registry, nil)

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
	executor := NewToolExecutor(registry, nil)

	defs := executor.definitions()
	assert.Len(t, defs, 3)
	assert.Equal(t, "a", defs[0].Name)
	assert.Equal(t, "m", defs[1].Name)
	assert.Equal(t, "z", defs[2].Name)
}

func TestExecute_UnknownTool_ReturnsMessageToLLM(t *testing.T) {
	registry := newMockToolRegistry([]domain.Tool{})
	executor := NewToolExecutor(registry, nil)
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

func TestExecute_PermissionDenied_ReturnsErrorAndSkipsPrepare(t *testing.T) {
	prepareCalled := false
	mt := &mockTool{
		name: "bash",
		prepare: func(params string) (domain.Invocation, error) {
			prepareCalled = true
			return &mockInvocation{content: "ok", display: domain.NewStringDisplay("", "")}, nil
		},
	}
	registry := newMockToolRegistry([]domain.Tool{mt})
	executor := NewToolExecutor(registry, nil, permission.NewResolver("ask", map[string]string{
		"bash": "deny",
	}))
	sender := newMockEventSender(10)

	res, disp, err := executor.execute(context.Background(), &schema.ToolCall{
		ID:       "tc-deny",
		Function: schema.FunctionCall{Name: "bash"},
	}, sender)

	assert.NoError(t, err)
	assert.False(t, prepareCalled)
	assert.Equal(t, "tc-deny", res.ToolCallID)
	assert.Contains(t, res.Content, "permission denied")
	assert.Equal(t, domain.ToolErrorPermissionDenied, disp.GetError())

	e1 := <-sender.events
	_, ok := e1.(domain.ToolStartEvent)
	assert.True(t, ok)
	e2 := <-sender.events
	_, ok = e2.(domain.ToolEndEvent)
	assert.True(t, ok)
}

func TestExecute_PermissionAsk_Approve_ExecutesPreparedInvocation(t *testing.T) {
	mt := &mockTool{
		name: "bash",
		prepare: func(params string) (domain.Invocation, error) {
			return &mockInvocation{content: "ok", display: domain.NewStringDisplay("Run bash", "preview")}, nil
		},
	}
	waiter := &mockActionWaiter{
		wait: func(ctx context.Context, callID string) (domain.Action, error) {
			return domain.PermissionDecisionAction{CallID: callID, Approved: true}, nil
		},
	}
	registry := newMockToolRegistry([]domain.Tool{mt})
	executor := NewToolExecutor(registry, waiter, permission.NewResolver("ask", nil))
	sender := newMockEventSender(10)

	res, disp, err := executor.execute(context.Background(), &schema.ToolCall{
		ID:       "tc-ask-allow",
		Function: schema.FunctionCall{Name: "bash"},
	}, sender)

	assert.NoError(t, err)
	assert.Equal(t, "ok", res.Content)
	assert.Empty(t, disp.GetError())

	e1 := <-sender.events
	_, ok := e1.(domain.ToolStartEvent)
	assert.True(t, ok)
	e2 := <-sender.events
	_, ok = e2.(domain.ToolApprovalRequestEvent)
	assert.True(t, ok)
	e3 := <-sender.events
	_, ok = e3.(domain.ToolEndEvent)
	assert.True(t, ok)
}

func TestExecute_PermissionAsk_Deny_ReturnsDenied(t *testing.T) {
	prepareCalled := false
	mt := &mockTool{
		name: "bash",
		prepare: func(params string) (domain.Invocation, error) {
			prepareCalled = true
			return &mockInvocation{content: "ok", display: domain.NewStringDisplay("Run bash", "preview")}, nil
		},
	}
	waiter := &mockActionWaiter{
		wait: func(ctx context.Context, callID string) (domain.Action, error) {
			return domain.PermissionDecisionAction{CallID: callID, Approved: false}, nil
		},
	}
	registry := newMockToolRegistry([]domain.Tool{mt})
	executor := NewToolExecutor(registry, waiter, permission.NewResolver("ask", nil))
	sender := newMockEventSender(10)

	res, disp, err := executor.execute(context.Background(), &schema.ToolCall{
		ID:       "tc-ask-deny",
		Function: schema.FunctionCall{Name: "bash"},
	}, sender)

	assert.NoError(t, err)
	assert.True(t, prepareCalled)
	assert.Contains(t, res.Content, "permission denied")
	assert.Equal(t, domain.ToolErrorPermissionDenied, disp.GetError())
	sd, ok := disp.(domain.StringDisplay)
	require.True(t, ok)
	assert.Equal(t, "Run bash", sd.Description)
	assert.Equal(t, "preview", sd.Content)

	e1 := <-sender.events
	_, ok = e1.(domain.ToolStartEvent)
	assert.True(t, ok)
	e2 := <-sender.events
	_, ok = e2.(domain.ToolApprovalRequestEvent)
	assert.True(t, ok)
	e3 := <-sender.events
	_, ok = e3.(domain.ToolEndEvent)
	assert.True(t, ok)
	select {
	case extra := <-sender.events:
		t.Fatalf("unexpected extra event after ask deny: %T", extra)
	default:
	}
}

func TestExecute_InteractiveInvocation_SkipsPermissionApprovalGate(t *testing.T) {
	callID := "tc-question"
	qd := domain.QuestionDisplay{TypeField: "question", Questions: []domain.QuestionInfo{{Question: "Proceed?", Options: []string{"Yes"}}}}
	questionAnswer := domain.QuestionAnswerAction{CallID: callID, Answers: [][]string{{"Yes"}}}

	mt := &mockTool{
		name: "ask_question",
		prepare: func(params string) (domain.Invocation, error) {
			return &mockInteractiveInvocation{
				display: qd,
				resolve: func(ctx context.Context, action domain.Action) (string, domain.ToolDisplay) {
					assert.Equal(t, questionAnswer, action)
					return "answered", domain.NewStringDisplay("Question answered", "Yes")
				},
			}, nil
		},
	}
	waiter := &mockActionWaiter{
		wait: func(ctx context.Context, gotCallID string) (domain.Action, error) {
			assert.Equal(t, callID, gotCallID)
			return questionAnswer, nil
		},
	}
	executor := NewToolExecutor(newMockToolRegistry([]domain.Tool{mt}), waiter, permission.NewResolver("ask", nil))
	sender := newMockEventSender(10)

	res, disp, err := executor.execute(context.Background(), &schema.ToolCall{
		ID:       callID,
		Function: schema.FunctionCall{Name: "ask_question"},
	}, sender)

	assert.NoError(t, err)
	assert.Equal(t, "answered", res.Content)
	assert.Empty(t, disp.GetError())

	e1 := <-sender.events
	_, ok := e1.(domain.ToolStartEvent)
	assert.True(t, ok)
	e2 := <-sender.events
	_, ok = e2.(domain.ToolEndEvent)
	assert.True(t, ok)
	select {
	case extra := <-sender.events:
		t.Fatalf("unexpected extra event for interactive invocation: %T", extra)
	default:
	}
}

// mockDisplayOnlyInvocation implements Invocation but not ExecutableInvocation (for executor guard tests).
type mockDisplayOnlyInvocation struct{}

func (mockDisplayOnlyInvocation) Display() domain.ToolDisplay {
	return domain.NewStringDisplay("", "preview")
}

func TestExecute_NonExecutableInvocation_Panics(t *testing.T) {
	mt := &mockTool{
		name: "weird",
		prepare: func(params string) (domain.Invocation, error) {
			return mockDisplayOnlyInvocation{}, nil
		},
	}
	registry := newMockToolRegistry([]domain.Tool{mt})
	executor := NewToolExecutor(registry, nil)

	assert.Panics(t, func() {
		_, _, _ = executor.execute(context.Background(), &schema.ToolCall{
			ID:       "tc-ne",
			Function: schema.FunctionCall{Name: "weird"},
		}, nil)
	})
}

func TestExecute_ValidJSON_ParsesCorrectly(t *testing.T) {
	var capturedParams string
	mt := &mockTool{
		name: "test",
		prepare: func(params string) (domain.Invocation, error) {
			capturedParams = params
			return &mockInvocation{content: "ok", display: domain.NewStringDisplay("", "")}, nil
		},
	}
	registry := newMockToolRegistry([]domain.Tool{mt})
	executor := NewToolExecutor(registry, nil)

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
		prepare: func(params string) (domain.Invocation, error) {
			return nil, fmt.Errorf("bad params")
		},
	}
	registry := newMockToolRegistry([]domain.Tool{mt})
	executor := NewToolExecutor(registry, nil)

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
	expStart := domain.NewStringDisplay("", "")
	expStart.Error = "Bad TEST request"
	assert.Equal(t, expStart, start.Display)

	// Verify generic error event
	e2 := <-sender.events
	end, ok := e2.(domain.ToolEndEvent)
	assert.True(t, ok)
	assert.Equal(t, "Bad TEST request", end.Display.GetError())
}

func TestExecute_EmitsToolEvents(t *testing.T) {
	mt := &mockTool{
		name: "test",
		prepare: func(params string) (domain.Invocation, error) {
			return &mockInvocation{
				content: "result",
				display: domain.NewStringDisplay("", "display output"),
			}, nil
		},
	}
	registry := newMockToolRegistry([]domain.Tool{mt})
	executor := NewToolExecutor(registry, nil)

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
	assert.Equal(t, domain.NewStringDisplay("", "display output"), start.Display)

	e2 := <-sender.events
	end, ok := e2.(domain.ToolEndEvent)
	assert.True(t, ok)
	assert.Equal(t, "tc-1", end.CallID)
	assert.Empty(t, end.Display.GetError())
	assert.Equal(t, domain.NewStringDisplay("", "display output"), end.Display)
}

func TestExecute_Failures_ReturnsDisplay(t *testing.T) {
	registry := newMockToolRegistry([]domain.Tool{
		&mockTool{
			name: "read_file",
			prepare: func(params string) (domain.Invocation, error) {
				return nil, fmt.Errorf("failed to prepare")
			},
		},
	})
	executor := NewToolExecutor(registry, nil)
	sender := newMockEventSender(10)

	// Verify lookup failure (TC-1)
	tc1 := &schema.ToolCall{ID: "tc-1", Function: schema.FunctionCall{Name: "unknown"}}
	msg1, disp1, err1 := executor.execute(context.Background(), tc1, sender)
	assert.NoError(t, err1)
	assert.NotNil(t, disp1)
	exp1 := domain.NewStringDisplay("", "")
	exp1.Error = "Unknown tool"
	assert.Equal(t, exp1, disp1)
	assert.Equal(t, "tc-1", msg1.ToolCallID)
	assert.Contains(t, msg1.Content, "does not exist")

	// Verify prepare failure (TC-2)
	tc2 := &schema.ToolCall{ID: "tc-2", Function: schema.FunctionCall{Name: "read_file", Arguments: "invalid"}}
	msg2, disp2, err2 := executor.execute(context.Background(), tc2, sender)
	assert.NoError(t, err2)
	assert.NotNil(t, disp2)
	exp2 := domain.NewStringDisplay("", "")
	exp2.Error = "Bad READ FILE request"
	assert.Equal(t, exp2, disp2)
	assert.Equal(t, "tc-2", msg2.ToolCallID)
	assert.Contains(t, msg2.Content, "failed to prepare")
}

func TestExecute_Bash_StreamsAndEnds(t *testing.T) {
	mt := &mockTool{
		name: "bash",
		prepare: func(params string) (domain.Invocation, error) {
			return &mockStreamInvocation{
				content: "Command finished",
				stream:  strings.NewReader("file1\nfile2\n"),
				display: domain.NewBashDisplay("ls", "ls", "", ""),
			}, nil
		},
	}
	registry := newMockToolRegistry([]domain.Tool{mt})
	executor := NewToolExecutor(registry, nil)

	sender := newMockEventSender(10)
	_, _, err := executor.execute(context.Background(), &schema.ToolCall{
		ID: "tc-bash",
		Function: schema.FunctionCall{
			Name: "bash",
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
			assert.Equal(t, "tc-bash", ev.CallID)
			streamOutput.WriteString(ev.Chunk)
		case domain.ToolEndEvent:
			assert.Equal(t, "tc-bash", ev.CallID)
			assert.Empty(t, ev.Display.GetError())
			break loop
		}
	}

	assert.Equal(t, "file1\nfile2\n", streamOutput.String())
}

func TestExecute_UsesFinalDisplayFromExecute(t *testing.T) {
	mt := &mockTool{
		name: "test",
		prepare: func(params string) (domain.Invocation, error) {
			return &mockInvocation{
				content: "infra failure",
				display: domain.NewStringDisplay("", "preview").WithError(domain.ToolErrorFailed),
				err:     nil,
			}, nil
		},
	}
	registry := newMockToolRegistry([]domain.Tool{mt})
	executor := NewToolExecutor(registry, nil)

	_, disp, err := executor.execute(context.Background(), &schema.ToolCall{
		ID:       "tc-1",
		Function: schema.FunctionCall{Name: "test"},
	}, nil)
	assert.NoError(t, err)
	assert.Equal(t, domain.ToolErrorFailed, disp.GetError())
	sd, ok := disp.(domain.StringDisplay)
	assert.True(t, ok)
	assert.Equal(t, domain.ToolErrorFailed, sd.Error)
}

func TestExecute_ExecuteFail_EmitsErrorEvent(t *testing.T) {
	mt := &mockTool{
		name: "fail",
		prepare: func(params string) (domain.Invocation, error) {
			return &mockInvocation{
				content: "Detailed error in content",
				display: domain.NewStringDisplay("", "").WithError(domain.ToolErrorFailed),
				err:     nil,
			}, nil
		},
	}
	registry := newMockToolRegistry([]domain.Tool{mt})
	executor := NewToolExecutor(registry, nil)

	sender := newMockEventSender(10)
	res, _, err := executor.execute(context.Background(), &schema.ToolCall{
		ID: "tc-fail",
		Function: schema.FunctionCall{
			Name: "fail",
		},
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
	assert.Equal(t, domain.ToolErrorFailed, end.Display.GetError())
}

func TestIssue6_DoubleEndEvent_Regression(t *testing.T) {
	t.Parallel()
	output := io.NopCloser(strings.NewReader("some output"))

	mt := &mockTool{
		name: "bash",
		prepare: func(params string) (domain.Invocation, error) {
			return &mockStreamInvocation{
				stream:  output,
				display: domain.NewBashDisplay("Header", "cmd", "", "").WithError(domain.ToolErrorFailed),
				content: "specific error",
				err:     nil,
			}, nil
		},
	}

	registry := newMockToolRegistry([]domain.Tool{mt})
	executor := NewToolExecutor(registry, nil)
	sender := newMockEventSender(10)

	_, _, err := executor.execute(context.Background(), &schema.ToolCall{
		ID: "call-1",
		Function: schema.FunctionCall{
			Name: "bash",
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
	assert.Equal(t, domain.ToolErrorFailed, endEvents[0].Display.GetError(), "Completion event must retain the execution error status")
}

func TestExecute_ConcurrentCalls_NoRace(t *testing.T) {
	registry := newMockToolRegistry([]domain.Tool{&mockTool{name: "tool"}})
	executor := NewToolExecutor(registry, nil)

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
	executor := NewToolExecutor(registry, nil)

	res, _, err := executor.execute(ctx, &schema.ToolCall{
		ID: "tc-cancel",
		Function: schema.FunctionCall{
			Name: "test",
		},
	}, nil)

	assert.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, schema.Tool, res.Role)
	assert.Equal(t, "tc-cancel", res.ToolCallID)
	assert.Nil(t, res.Extra)
	assert.Equal(t, domain.ToolErrorCancelled, res.Content)
}

func TestExecuteBatch_ContextCancelled_PopulatesAllToolResponsesOnFatalError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	mt1 := &mockTool{
		name:          "t1",
		concurrent:    true,
		setConcurrent: true,
		prepare: func(params string) (domain.Invocation, error) {
			return &mockInvocation{display: domain.NewStringDisplay("", "")}, nil
		},
	}
	mt2 := &mockTool{
		name:          "t2",
		concurrent:    true,
		setConcurrent: true,
		prepare: func(params string) (domain.Invocation, error) {
			return &mockInvocation{display: domain.NewStringDisplay("", "")}, nil
		},
	}
	executor := NewToolExecutor(newMockToolRegistry([]domain.Tool{mt1, mt2}), nil)

	calls := []schema.ToolCall{
		{ID: "tc-1", Function: schema.FunctionCall{Name: "t1"}},
		{ID: "tc-2", Function: schema.FunctionCall{Name: "t2"}},
	}

	res, err := executor.executeBatch(ctx, calls, nil)
	assert.ErrorIs(t, err, context.Canceled)

	assert.Len(t, res.Responses, 2)
	assert.NotNil(t, res.Responses[0])
	assert.NotNil(t, res.Responses[1])
	assert.Equal(t, "tc-1", res.Responses[0].ToolCallID)
	assert.Equal(t, domain.ToolErrorCancelled, res.Responses[0].Content)
	assert.Equal(t, "tc-2", res.Responses[1].ToolCallID)
	assert.Equal(t, domain.ToolErrorCancelled, res.Responses[1].Content)

	assert.Nil(t, res.Displays["tc-1"])
	assert.Nil(t, res.Displays["tc-2"])
}

func TestExecuteBatch_FatalErrorStillBakesRemainingPreflightedCalls(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	executed2 := make(chan struct{})

	mt1 := &mockTool{
		name:          "t1",
		concurrent:    false,
		setConcurrent: true, // force non-concurrent safe barrier
		prepare: func(params string) (domain.Invocation, error) {
			return &mockInvocation{
				display: domain.NewStringDisplay("", ""),
				execute: func(ctx context.Context) (string, domain.ToolDisplay) {
					cancel()
					disp := domain.NewStringDisplay("", "")
					disp.Error = domain.ToolErrorCancelled
					return domain.ToolErrorCancelled, disp
				},
			}, nil
		},
	}

	mt2 := &mockTool{
		name:       "t2",
		concurrent: true,
		// concurrently safe would be executed in parallel with other safe tools,
		// but executeBatch should stop after mt1's fatal error and bake cancellation.
		setConcurrent: true,
		prepare: func(params string) (domain.Invocation, error) {
			return &mockInvocation{
				display: domain.NewStringDisplay("", ""),
				execute: func(ctx context.Context) (string, domain.ToolDisplay) {
					close(executed2)
					disp := domain.NewStringDisplay("", "")
					disp.Error = domain.ToolErrorCancelled
					return domain.ToolErrorCancelled, disp
				},
			}, nil
		},
	}

	executor := NewToolExecutor(newMockToolRegistry([]domain.Tool{mt1, mt2}), nil)
	calls := []schema.ToolCall{
		{ID: "tc-1", Function: schema.FunctionCall{Name: "t1"}},
		{ID: "tc-2", Function: schema.FunctionCall{Name: "t2"}},
	}

	res, err := executor.executeBatch(ctx, calls, nil)
	assert.ErrorIs(t, err, context.Canceled)

	// mt2 should not run, but it must still have a corresponding tool response.
	select {
	case <-executed2:
		t.Fatal("t2 invocation should not be executed after fatal error")
	default:
	}

	assert.Len(t, res.Responses, 2)
	assert.NotNil(t, res.Responses[0])
	assert.NotNil(t, res.Responses[1])
	assert.Equal(t, "tc-1", res.Responses[0].ToolCallID)
	assert.Equal(t, domain.ToolErrorCancelled, res.Responses[0].Content)
	assert.Equal(t, "tc-2", res.Responses[1].ToolCallID)
	assert.Equal(t, domain.ToolErrorCancelled, res.Responses[1].Content)

	assert.NotNil(t, res.Displays["tc-1"])
	assert.Nil(t, res.Displays["tc-2"])
}

func TestExecuteBatch_PanicsWhenPreparedInvocationDisplayIsNil(t *testing.T) {
	mt := &mockTool{
		name: "bad",
		prepare: func(params string) (domain.Invocation, error) {
			return &mockInvocation{
				display: nil,
				execute: func(ctx context.Context) (string, domain.ToolDisplay) {
					return "ok", domain.NewStringDisplay("", "ok")
				},
			}, nil
		},
	}
	executor := NewToolExecutor(newMockToolRegistry([]domain.Tool{mt}), nil)

	assert.Panics(t, func() {
		_, _ = executor.executeBatch(context.Background(), []schema.ToolCall{
			{ID: "tc-bad", Function: schema.FunctionCall{Name: "bad"}},
		}, nil)
	})
}

func TestExecuteBatch_PreservesInputOrder_WithPreflightFailure(t *testing.T) {
	registry := newMockToolRegistry([]domain.Tool{
		&mockTool{
			name:          "ok",
			concurrent:    true,
			setConcurrent: true,
			prepare: func(params string) (domain.Invocation, error) {
				return &mockInvocation{content: "ok-result", display: domain.NewStringDisplay("", "ok")}, nil
			},
		},
	})
	executor := NewToolExecutor(registry, nil)

	calls := []schema.ToolCall{
		{ID: "c-1", Function: schema.FunctionCall{Name: "unknown"}},
		{ID: "c-2", Function: schema.FunctionCall{Name: "ok"}},
	}
	res, err := executor.executeBatch(context.Background(), calls, nil)
	assert.NoError(t, err)
	assert.Len(t, res.Responses, 2)
	assert.Equal(t, "c-1", res.Responses[0].ToolCallID)
	assert.Contains(t, res.Responses[0].Content, "does not exist")
	assert.Equal(t, "c-2", res.Responses[1].ToolCallID)
	assert.Equal(t, "ok-result", res.Responses[1].Content)
}

func TestExecuteBatch_NonConcurrentSafeActsAsBarrier(t *testing.T) {
	startedFast1 := make(chan struct{}, 1)
	startedExclusive := make(chan struct{}, 1)
	startedFast2 := make(chan struct{}, 1)
	doneFast1 := make(chan struct{})
	doneExclusive := make(chan struct{})

	registry := newMockToolRegistry([]domain.Tool{
		&mockTool{
			name:          testFast1,
			concurrent:    true,
			setConcurrent: true,
			prepare: func(params string) (domain.Invocation, error) {
				return &mockInvocation{
					display: domain.NewStringDisplay("", testFast1),
					execute: func(ctx context.Context) (string, domain.ToolDisplay) {
						startedFast1 <- struct{}{}
						<-doneFast1
						return testFast1, domain.NewStringDisplay("", testFast1)
					},
				}, nil
			},
		},
		&mockTool{
			name:          "exclusive",
			concurrent:    false,
			setConcurrent: true,
			prepare: func(params string) (domain.Invocation, error) {
				return &mockInvocation{
					display: domain.NewStringDisplay("", "exclusive"),
					execute: func(ctx context.Context) (string, domain.ToolDisplay) {
						startedExclusive <- struct{}{}
						<-doneExclusive
						return "exclusive", domain.NewStringDisplay("", "exclusive")
					},
				}, nil
			},
		},
		&mockTool{
			name:          testFast2,
			concurrent:    true,
			setConcurrent: true,
			prepare: func(params string) (domain.Invocation, error) {
				return &mockInvocation{
					display: domain.NewStringDisplay("", testFast2),
					execute: func(ctx context.Context) (string, domain.ToolDisplay) {
						startedFast2 <- struct{}{}
						return testFast2, domain.NewStringDisplay("", testFast2)
					},
				}, nil
			},
		},
	})
	executor := NewToolExecutor(registry, nil)

	runDone := make(chan error, 1)
	go func() {
		_, err := executor.executeBatch(context.Background(), []schema.ToolCall{
			{ID: "c1", Function: schema.FunctionCall{Name: "fast1"}},
			{ID: "c2", Function: schema.FunctionCall{Name: "exclusive"}},
			{ID: "c3", Function: schema.FunctionCall{Name: "fast2"}},
		}, nil)
		runDone <- err
	}()

	select {
	case <-startedFast1:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("fast1 did not start")
	}
	select {
	case <-startedExclusive:
		t.Fatal("exclusive should not start before fast1 completes")
	default:
	}

	close(doneFast1)
	select {
	case <-startedExclusive:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("exclusive did not start after first concurrent batch finished")
	}
	select {
	case <-startedFast2:
		t.Fatal("fast2 should not start while exclusive call is running")
	default:
	}

	close(doneExclusive)
	select {
	case <-startedFast2:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("fast2 did not start after exclusive call finished")
	}

	assert.NoError(t, <-runDone)
}

func TestExecuteBatch_StartEventsRespectBarrierTiming(t *testing.T) {
	doneFast1 := make(chan struct{})
	doneExclusive := make(chan struct{})
	doneFast2 := make(chan struct{})

	registry := newMockToolRegistry([]domain.Tool{
		&mockTool{
			name:          "fast1",
			concurrent:    true,
			setConcurrent: true,
			prepare: func(params string) (domain.Invocation, error) {
				return &mockInvocation{
					display: domain.NewStringDisplay("", "fast1"),
					execute: func(ctx context.Context) (string, domain.ToolDisplay) {
						<-doneFast1
						return "fast1", domain.NewStringDisplay("", "fast1")
					},
				}, nil
			},
		},
		&mockTool{
			name:          "exclusive",
			concurrent:    false,
			setConcurrent: true,
			prepare: func(params string) (domain.Invocation, error) {
				return &mockInvocation{
					display: domain.NewStringDisplay("", "exclusive"),
					execute: func(ctx context.Context) (string, domain.ToolDisplay) {
						<-doneExclusive
						return "exclusive", domain.NewStringDisplay("", "exclusive")
					},
				}, nil
			},
		},
		&mockTool{
			name:          "fast2",
			concurrent:    true,
			setConcurrent: true,
			prepare: func(params string) (domain.Invocation, error) {
				return &mockInvocation{
					display: domain.NewStringDisplay("", "fast2"),
					execute: func(ctx context.Context) (string, domain.ToolDisplay) {
						<-doneFast2
						return "fast2", domain.NewStringDisplay("", "fast2")
					},
				}, nil
			},
		},
	})
	executor := NewToolExecutor(registry, nil)
	sender := newMockEventSender(20)

	runDone := make(chan error, 1)
	go func() {
		_, err := executor.executeBatch(context.Background(), []schema.ToolCall{
			{ID: "c1", Function: schema.FunctionCall{Name: "fast1"}},
			{ID: "c2", Function: schema.FunctionCall{Name: "exclusive"}},
			{ID: "c3", Function: schema.FunctionCall{Name: "fast2"}},
		}, sender)
		runDone <- err
	}()

	nextStart := func() domain.ToolStartEvent {
		t.Helper()
		for {
			ev := <-sender.events
			if s, ok := ev.(domain.ToolStartEvent); ok {
				return s
			}
		}
	}

	// Only first segment start should be emitted before fast1 is released.
	s1 := nextStart()
	assert.Equal(t, "c1", s1.CallID)

	select {
	case e := <-sender.events:
		if se, ok := e.(domain.ToolStartEvent); ok {
			t.Fatalf("unexpected early ToolStartEvent for %s before barrier release", se.CallID)
		}
	default:
	}

	close(doneFast1)
	s2 := nextStart()
	assert.Equal(t, "c2", s2.CallID)

	select {
	case e := <-sender.events:
		if se, ok := e.(domain.ToolStartEvent); ok {
			t.Fatalf("unexpected early ToolStartEvent for %s before exclusive release", se.CallID)
		}
	default:
	}

	close(doneExclusive)
	s3 := nextStart()
	assert.Equal(t, "c3", s3.CallID)

	close(doneFast2)
	assert.NoError(t, <-runDone)
}

func TestExecuteBatch_EmitsStartEventsInInputOrder_ForConcurrentCalls(t *testing.T) {
	doneFast1 := make(chan struct{})
	doneFast2 := make(chan struct{})

	registry := newMockToolRegistry([]domain.Tool{
		&mockTool{
			name:          "fast1",
			concurrent:    true,
			setConcurrent: true,
			prepare: func(params string) (domain.Invocation, error) {
				return &mockInvocation{
					display: domain.NewStringDisplay("", "fast1"),
					execute: func(ctx context.Context) (string, domain.ToolDisplay) {
						<-doneFast1
						return "fast1", domain.NewStringDisplay("", "fast1")
					},
				}, nil
			},
		},
		&mockTool{
			name:          "fast2",
			concurrent:    true,
			setConcurrent: true,
			prepare: func(params string) (domain.Invocation, error) {
				return &mockInvocation{
					display: domain.NewStringDisplay("", "fast2"),
					execute: func(ctx context.Context) (string, domain.ToolDisplay) {
						<-doneFast2
						return "fast2", domain.NewStringDisplay("", "fast2")
					},
				}, nil
			},
		},
	})
	executor := NewToolExecutor(registry, nil)
	sender := newMockEventSender(20)

	runDone := make(chan error, 1)
	go func() {
		_, err := executor.executeBatch(context.Background(), []schema.ToolCall{
			{ID: "c1", Function: schema.FunctionCall{Name: "fast1"}},
			{ID: "c2", Function: schema.FunctionCall{Name: "fast2"}},
		}, sender)
		runDone <- err
	}()

	e1 := <-sender.events
	s1, ok := e1.(domain.ToolStartEvent)
	require.True(t, ok)
	assert.Equal(t, "c1", s1.CallID)

	e2 := <-sender.events
	s2, ok := e2.(domain.ToolStartEvent)
	require.True(t, ok)
	assert.Equal(t, "c2", s2.CallID)

	close(doneFast1)
	close(doneFast2)
	assert.NoError(t, <-runDone)
}

func TestExecute_PanicsWhenFinalDisplayIsNil(t *testing.T) {
	mt := &mockTool{
		name: "bad",
		prepare: func(params string) (domain.Invocation, error) {
			return &mockInvocation{
				display: domain.NewStringDisplay("", "preview"),
				execute: func(ctx context.Context) (string, domain.ToolDisplay) {
					return "", nil
				},
			}, nil
		},
	}
	registry := newMockToolRegistry([]domain.Tool{mt})
	executor := NewToolExecutor(registry, nil)

	assert.Panics(t, func() {
		_, _, _ = executor.execute(context.Background(), &schema.ToolCall{
			ID: "tc-nil",
			Function: schema.FunctionCall{
				Name:      "bad",
				Arguments: "{}",
			},
		}, nil)
	})
}

func TestToolExecutor_Throughput_Batching(t *testing.T) {
	data := strings.Repeat("A", 8192)

	mt := &mockTool{
		name: "throughput-test",
		prepare: func(params string) (domain.Invocation, error) {
			return &mockStreamInvocation{
				content: "done",
				stream:  strings.NewReader(data),
				display: domain.NewBashDisplay("test", "test", "", ""),
			}, nil
		},
	}

	registry := newMockToolRegistry([]domain.Tool{mt})
	executor := NewToolExecutor(registry, nil)

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

func TestExecute_ContextCancelled_DuringPrepare_Silent(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	mt := &mockTool{name: "test"}
	registry := newMockToolRegistry([]domain.Tool{mt})
	executor := NewToolExecutor(registry, nil)
	sender := newMockEventSender(10)

	res, disp, err := executor.execute(ctx, &schema.ToolCall{
		ID:       "tc-1",
		Function: schema.FunctionCall{Name: "test"},
	}, sender)

	assert.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, domain.ToolErrorCancelled, res.Content)
	assert.Nil(t, disp)
	select {
	case e := <-sender.events:
		t.Fatalf("Expected no events for cancelled prepare, got %T", e)
	case <-time.After(50 * time.Millisecond):
		// Success: no events
	}
}

func (e *ToolExecutor) execute(ctx context.Context, tc *schema.ToolCall, events eventSender) (*schema.Message, domain.ToolDisplay, error) {
	inv, msg, disp := e.prepareTool(ctx, tc, events)
	if inv == nil {
		return msg, disp, ctx.Err()
	}
	resp, finalDisp := e.executeTool(ctx, tc, inv, events)
	return resp, finalDisp, ctx.Err()
}
func TestToolExecutor_LogDirInjection(t *testing.T) {

	// Create a mock invocation that is LogAware
	mt := &mockTool{
		name: "loggy",
		prepare: func(params string) (domain.Invocation, error) {
			return &mockLogAwareInvocation{
				display: domain.NewStringDisplay("", "loggy"),
			}, nil
		},
	}

	registry := newMockToolRegistry([]domain.Tool{mt})
	executor := NewToolExecutor(registry, nil)
	sender := newMockEventSender(10)

	// executeBatch now expects logDir as 4th arg
	calls := []schema.ToolCall{{ID: "tc-1", Function: schema.FunctionCall{Name: "loggy"}}}
	_, err := executor.executeBatch(context.Background(), calls, sender)
	assert.NoError(t, err)

	// Implementation note: The actual check happens inside the mock's Execute call or we check a field.
	// For this test, the mockLogAwareInvocation will be defined below.
}

type mockLogAwareInvocation struct {
	display        domain.ToolDisplay
	capturedLogDir string
}

func (m *mockLogAwareInvocation) Display() domain.ToolDisplay { return m.display }
func (m *mockLogAwareInvocation) SetLogDir(path string)       { m.capturedLogDir = path }
func (m *mockLogAwareInvocation) Execute(ctx context.Context) (string, domain.ToolDisplay) {
	if m.capturedLogDir == "" {
		return "fail: log dir not set", m.display.WithError("Log dir not set")
	}
	return "ok: " + m.capturedLogDir, m.display
}
