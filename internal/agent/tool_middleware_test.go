package agent

import (
	"context"
	"fmt"
	"testing"

	"github.com/Cyclone1070/autocmd/internal/domain"
	"github.com/Cyclone1070/autocmd/internal/permission"
	"github.com/Cyclone1070/autocmd/internal/runtimectx"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/require"
)

type mockToolRegistry struct {
	tools map[string]tool.BaseTool
}

func (m *mockToolRegistry) Tools() []tool.BaseTool { return nil }
func (m *mockToolRegistry) Get(name string) (tool.BaseTool, bool) {
	tl, ok := m.tools[name]
	return tl, ok
}

type mockEventSender struct {
	updates []domain.UIUpdate
}

func (m *mockEventSender) SendUIUpdate(u domain.UIUpdate) { m.updates = append(m.updates, u) }

type mockWaiter struct {
	act domain.Action
}

func (m *mockWaiter) Wait(ctx context.Context, callID string) (domain.Action, error) {
	_ = callID
	return m.act, ctx.Err()
}

type mockPreviewTool struct {
	name     string
	previewN int
}

func (m *mockPreviewTool) Name() string { return m.name }
func (m *mockPreviewTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{Name: m.name}, nil
}

func (m *mockPreviewTool) InvokableRun(ctx context.Context, args string, opts ...tool.Option) (string, error) {
	return "", nil
}

func (m *mockPreviewTool) Preview(input *compose.ToolInput) domain.ToolDisplay {
	m.previewN++
	return domain.NewStringDisplay("preview", "content")
}

func TestPreviewStartMiddleware_CallsPreview_EmitsToolStart_AndCallsNext(t *testing.T) {
	tl := &mockPreviewTool{name: testToolNameReadFile}
	reg := &mockToolRegistry{tools: map[string]tool.BaseTool{testToolNameReadFile: tl}}
	events := &mockEventSender{}
	mw := newPreviewStartMiddleware(events, reg)

	nextCalled := 0
	next := func(ctx context.Context, in *compose.ToolInput) (*compose.ToolOutput, error) {
		nextCalled++
		return &compose.ToolOutput{Result: testToolOutputFromNext}, nil
	}

	ctx := context.Background()
	out, err := mw.Invokable(next)(ctx, &compose.ToolInput{
		Name:      testToolNameReadFile,
		Arguments: testReadFileArgsJSON,
		CallID:    testToolCallID1,
	})
	require.NoError(t, err)
	require.Equal(t, testToolOutputFromNext, out.Result)

	require.Equal(t, 1, tl.previewN)
	require.Equal(t, 1, nextCalled)
	require.Len(t, events.updates, 1)
	start, ok := events.updates[0].(domain.ToolStartEvent)
	require.True(t, ok)
	require.Equal(t, testToolCallID1, start.CallID)
}

func TestPermissionMiddleware_PermissionDenied_DoesNotCallNext(t *testing.T) {
	events := &mockEventSender{}
	waiter := &mockWaiter{act: domain.PermissionDecisionAction{Approved: false}}
	resolver := permission.NewResolver(testPermissionModeAsk, map[string]string{testToolNameReadFile: testPermissionModeAsk})
	tl := &mockPreviewTool{name: testToolNameReadFile}
	reg := &mockToolRegistry{tools: map[string]tool.BaseTool{testToolNameReadFile: tl}}
	mw := newPermissionMiddleware(resolver, waiter, events, reg)

	nextCalled := 0
	next := func(ctx context.Context, in *compose.ToolInput) (*compose.ToolOutput, error) {
		nextCalled++
		return &compose.ToolOutput{Result: testToolOutputFromNext}, nil
	}

	ctx := context.Background()
	out, err := mw.Invokable(next)(ctx, &compose.ToolInput{
		Name:      testToolNameReadFile,
		Arguments: testReadFileArgsJSON,
		CallID:    testToolCallID1,
	})
	require.NoError(t, err)
	require.Equal(t, "Tool execution was denied by the user.", out.Result)
	require.Equal(t, 0, nextCalled)
	require.Len(t, events.updates, 2)
	_, ok := events.updates[0].(domain.ToolApprovalRequestEvent)
	require.True(t, ok)
	end, ok := events.updates[1].(domain.ToolEndEvent)
	require.True(t, ok)
	require.Equal(t, toolErrorPermissionDenied, end.Display.GetError())
	sd, ok := end.Display.(domain.StringDisplay)
	require.True(t, ok)
	require.Equal(t, "preview", sd.Description)
}

func TestPermissionMiddleware_AskQuestion_StillRespectsResolverPolicy(t *testing.T) {
	events := &mockEventSender{}
	waiter := &mockWaiter{act: domain.PermissionDecisionAction{Approved: false}}
	resolver := permission.NewResolver("allow", map[string]string{testToolNameAskQuestion: testPermissionModeAsk})
	tl := &mockPreviewTool{name: testToolNameAskQuestion}
	reg := &mockToolRegistry{tools: map[string]tool.BaseTool{testToolNameAskQuestion: tl}}
	mw := newPermissionMiddleware(resolver, waiter, events, reg)

	nextCalled := 0
	next := func(ctx context.Context, in *compose.ToolInput) (*compose.ToolOutput, error) {
		nextCalled++
		return &compose.ToolOutput{Result: testToolOutputFromNext}, nil
	}

	ctx := context.Background()
	out, err := mw.Invokable(next)(ctx, &compose.ToolInput{
		Name:      testToolNameAskQuestion,
		Arguments: `{"questions":[]}`,
		CallID:    "call_q_1",
	})
	require.NoError(t, err)
	require.Equal(t, "Tool execution was denied by the user.", out.Result)
	require.Equal(t, 0, nextCalled)
	require.Len(t, events.updates, 2)
	_, ok := events.updates[0].(domain.ToolApprovalRequestEvent)
	require.True(t, ok)
	end, ok := events.updates[1].(domain.ToolEndEvent)
	require.True(t, ok)
	require.Equal(t, toolErrorPermissionDenied, end.Display.GetError())
}

func TestPermissionMiddleware_WaiterNil_EmitsToolEnd(t *testing.T) {
	events := &mockEventSender{}
	resolver := permission.NewResolver(testPermissionModeAsk, map[string]string{testToolNameReadFile: testPermissionModeAsk})
	tl := &mockPreviewTool{name: testToolNameReadFile}
	reg := &mockToolRegistry{tools: map[string]tool.BaseTool{testToolNameReadFile: tl}}
	mw := newPermissionMiddleware(resolver, nil, events, reg)

	nextCalled := 0
	next := func(ctx context.Context, in *compose.ToolInput) (*compose.ToolOutput, error) {
		nextCalled++
		return &compose.ToolOutput{Result: testToolOutputFromNext}, nil
	}

	ctx := context.Background()
	out, err := mw.Invokable(next)(ctx, &compose.ToolInput{
		Name:      testToolNameReadFile,
		Arguments: testReadFileArgsJSON,
		CallID:    testToolCallID1,
	})
	require.NoError(t, err)
	require.Equal(t, "Internal error: permission waiter unavailable", out.Result)
	require.Equal(t, 0, nextCalled)
	require.Len(t, events.updates, 2)
	_, ok := events.updates[0].(domain.ToolApprovalRequestEvent)
	require.True(t, ok)
	end, ok := events.updates[1].(domain.ToolEndEvent)
	require.True(t, ok)
	require.Equal(t, domain.ToolErrorFailed, end.Display.GetError())
	sd, ok := end.Display.(domain.StringDisplay)
	require.True(t, ok)
	require.Equal(t, "preview", sd.Description)
}

func TestPermissionMiddleware_WaiterCancelled_EmitsToolEnd(t *testing.T) {
	events := &mockEventSender{}
	waiter := &mockWaiter{act: domain.PermissionDecisionAction{Approved: true}}
	resolver := permission.NewResolver(testPermissionModeAsk, map[string]string{testToolNameReadFile: testPermissionModeAsk})
	tl := &mockPreviewTool{name: testToolNameReadFile}
	reg := &mockToolRegistry{tools: map[string]tool.BaseTool{testToolNameReadFile: tl}}
	mw := newPermissionMiddleware(resolver, waiter, events, reg)

	nextCalled := 0
	next := func(ctx context.Context, in *compose.ToolInput) (*compose.ToolOutput, error) {
		nextCalled++
		return &compose.ToolOutput{Result: testToolOutputFromNext}, nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	out, err := mw.Invokable(next)(ctx, &compose.ToolInput{
		Name:      testToolNameReadFile,
		Arguments: testReadFileArgsJSON,
		CallID:    testToolCallID1,
	})
	require.NoError(t, err)
	require.Equal(t, domain.ToolErrorCancelled, out.Result)
	require.Equal(t, 0, nextCalled)
	require.Len(t, events.updates, 2)
	_, ok := events.updates[0].(domain.ToolApprovalRequestEvent)
	require.True(t, ok)
	end, ok := events.updates[1].(domain.ToolEndEvent)
	require.True(t, ok)
	require.Equal(t, domain.ToolErrorCancelled, end.Display.GetError())
	sd, ok := end.Display.(domain.StringDisplay)
	require.True(t, ok)
	require.Equal(t, "preview", sd.Description)
}

type mockPreviewValidateTool struct {
	mockPreviewTool
}

func (m *mockPreviewValidateTool) PreflightValidate(input *compose.ToolInput) error {
	_ = input
	return fmt.Errorf("invalid arguments")
}

func TestPreflightValidationMiddleware_ValidationError_UsesBadToolRequestDisplay(t *testing.T) {
	tl := &mockPreviewValidateTool{mockPreviewTool: mockPreviewTool{name: testToolNameReadFile}}
	reg := &mockToolRegistry{tools: map[string]tool.BaseTool{testToolNameReadFile: tl}}
	events := &mockEventSender{}
	mw := newPreflightValidationMiddleware(events, reg)

	nextCalled := 0
	next := func(ctx context.Context, in *compose.ToolInput) (*compose.ToolOutput, error) {
		nextCalled++
		return &compose.ToolOutput{Result: "ok"}, nil
	}

	out, err := mw.Invokable(next)(context.Background(), &compose.ToolInput{
		Name:      testToolNameReadFile,
		Arguments: `{}`,
		CallID:    "c1",
	})
	require.NoError(t, err)
	require.Contains(t, out.Result, "Error:")
	require.Equal(t, 0, nextCalled)

	require.Len(t, events.updates, 2)
	start, ok := events.updates[0].(domain.ToolStartEvent)
	require.True(t, ok)
	end, ok := events.updates[1].(domain.ToolEndEvent)
	require.True(t, ok)
	require.Equal(t, "c1", start.CallID)
	require.Equal(t, "c1", end.CallID)
	require.Equal(t, "Bad READ_FILE request", end.Display.GetError())
	sd, ok := end.Display.(domain.StringDisplay)
	require.True(t, ok)
	require.Equal(t, "", sd.Description)
	require.Equal(t, "", sd.Content)
}

func TestPreflightValidationMiddleware_UnknownTool_EmitsUnknownToolRequestDisplay(t *testing.T) {
	reg := &mockToolRegistry{tools: map[string]tool.BaseTool{}}
	events := &mockEventSender{}
	mw := newPreflightValidationMiddleware(events, reg)

	nextCalled := 0
	next := func(ctx context.Context, in *compose.ToolInput) (*compose.ToolOutput, error) {
		nextCalled++
		return &compose.ToolOutput{Result: "ok"}, nil
	}

	out, err := mw.Invokable(next)(context.Background(), &compose.ToolInput{
		Name:      "not_exists",
		Arguments: `{}`,
		CallID:    "u1",
	})
	require.NoError(t, err)
	require.Contains(t, out.Result, "unknown tool")
	require.Equal(t, 0, nextCalled)

	require.Len(t, events.updates, 2)
	start, ok := events.updates[0].(domain.ToolStartEvent)
	require.True(t, ok)
	end, ok := events.updates[1].(domain.ToolEndEvent)
	require.True(t, ok)
	require.Equal(t, "u1", start.CallID)
	require.Equal(t, "u1", end.CallID)

	sd, ok := end.Display.(domain.StringDisplay)
	require.True(t, ok)
	require.Equal(t, "", sd.Description)
	require.Equal(t, "", sd.Content)
	require.Equal(t, "Unknown tool request", sd.Error)
}

type mockNonPreviewTool struct {
	name string
}

func (m *mockNonPreviewTool) Name() string { return m.name }
func (m *mockNonPreviewTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{Name: m.name}, nil
}

func (m *mockNonPreviewTool) InvokableRun(ctx context.Context, args string, opts ...tool.Option) (string, error) {
	return "external tool result", nil
}

func TestExternalToolEventMiddleware_ForExternalTool(t *testing.T) {
	tl := &mockNonPreviewTool{name: "external_tool"}
	reg := &mockToolRegistry{tools: map[string]tool.BaseTool{"external_tool": tl}}
	events := &mockEventSender{}
	mw := newExternalToolEventMiddleware(events, reg)

	next := func(ctx context.Context, in *compose.ToolInput) (*compose.ToolOutput, error) {
		return &compose.ToolOutput{Result: "external tool result"}, nil
	}

	ctx := context.Background()
	out, err := mw.Invokable(next)(ctx, &compose.ToolInput{
		Name:      "external_tool",
		Arguments: "{}",
		CallID:    "c_ext",
	})
	require.NoError(t, err)
	require.Equal(t, "external tool result", out.Result)

	require.Len(t, events.updates, 2)

	start, ok := events.updates[0].(domain.ToolStartEvent)
	require.True(t, ok)
	require.Equal(t, "c_ext", start.CallID)
	sdStart, ok := start.Display.(domain.StringDisplay)
	require.True(t, ok)
	require.Equal(t, `Run "external_tool"`, sdStart.Description)

	end, ok := events.updates[1].(domain.ToolEndEvent)
	require.True(t, ok)
	require.Equal(t, "c_ext", end.CallID)
	sdEnd, ok := end.Display.(domain.StringDisplay)
	require.True(t, ok)
	require.Equal(t, `Run "external_tool"`, sdEnd.Description)
	require.Equal(t, "", sdEnd.Content)
}

func TestExternalToolEventMiddleware_ForBuiltInTool(t *testing.T) {
	tl := &mockPreviewTool{name: "builtin_tool"}
	reg := &mockToolRegistry{tools: map[string]tool.BaseTool{"builtin_tool": tl}}
	events := &mockEventSender{}
	mw := newExternalToolEventMiddleware(events, reg)

	next := func(ctx context.Context, in *compose.ToolInput) (*compose.ToolOutput, error) {
		return &compose.ToolOutput{Result: "builtin tool result"}, nil
	}

	ctx := context.Background()
	out, err := mw.Invokable(next)(ctx, &compose.ToolInput{
		Name:      "builtin_tool",
		Arguments: "{}",
		CallID:    "c_built",
	})
	require.NoError(t, err)
	require.Equal(t, "builtin tool result", out.Result)

	// Since it's a built-in tool, it implements previewer, so the middleware should not emit events
	require.Empty(t, events.updates)
}

func TestExternalToolEventMiddleware_ForExternalTool_Success_WritesToSink(t *testing.T) {
	tl := &mockNonPreviewTool{name: "external_tool"}
	reg := &mockToolRegistry{tools: map[string]tool.BaseTool{"external_tool": tl}}
	events := &mockEventSender{}
	mw := newExternalToolEventMiddleware(events, reg)

	next := func(ctx context.Context, in *compose.ToolInput) (*compose.ToolOutput, error) {
		return &compose.ToolOutput{Result: "success result"}, nil
	}

	var sinkCallID string
	var sinkDisplay domain.ToolDisplay
	ctx := runtimectx.WithToolDisplaySink(context.Background(), func(callID string, display domain.ToolDisplay) {
		sinkCallID = callID
		sinkDisplay = display
	})

	out, err := mw.Invokable(next)(ctx, &compose.ToolInput{
		Name:      "external_tool",
		Arguments: "{}",
		CallID:    "c_success",
	})
	require.NoError(t, err)
	require.Equal(t, "success result", out.Result)

	// Check UI update events
	require.Len(t, events.updates, 2)

	// Check sink persistence
	require.Equal(t, "c_success", sinkCallID)
	require.NotNil(t, sinkDisplay)
	sd, ok := sinkDisplay.(domain.StringDisplay)
	require.True(t, ok)
	require.Equal(t, `Run "external_tool"`, sd.Description)
	require.Equal(t, "", sd.Error)
}

func TestExternalToolEventMiddleware_ForExternalTool_GoError_RecoversAndWritesToSink(t *testing.T) {
	tl := &mockNonPreviewTool{name: "external_tool"}
	reg := &mockToolRegistry{tools: map[string]tool.BaseTool{"external_tool": tl}}
	events := &mockEventSender{}
	mw := newExternalToolEventMiddleware(events, reg)

	next := func(ctx context.Context, in *compose.ToolInput) (*compose.ToolOutput, error) {
		return nil, fmt.Errorf("external tool execution failed")
	}

	var sinkCallID string
	var sinkDisplay domain.ToolDisplay
	ctx := runtimectx.WithToolDisplaySink(context.Background(), func(callID string, display domain.ToolDisplay) {
		sinkCallID = callID
		sinkDisplay = display
	})

	out, err := mw.Invokable(next)(ctx, &compose.ToolInput{
		Name:      "external_tool",
		Arguments: "{}",
		CallID:    "c_fail",
	})
	// In strict TDD, we assert it does NOT return a Go error but converts it to a result string
	require.NoError(t, err)
	require.NotNil(t, out)
	require.Equal(t, "Error: external tool execution failed", out.Result)

	// UI update events should show the failure
	require.Len(t, events.updates, 2)
	endEvent, ok := events.updates[1].(domain.ToolEndEvent)
	require.True(t, ok)
	require.Equal(t, "external tool execution failed", endEvent.Display.GetError())

	// Sink persistence should have the error
	require.Equal(t, "c_fail", sinkCallID)
	require.NotNil(t, sinkDisplay)
	require.Equal(t, "external tool execution failed", sinkDisplay.GetError())
}

func TestExternalToolEventMiddleware_ForExternalTool_ContextCancelled_RecoversAndWritesToSink(t *testing.T) {
	tl := &mockNonPreviewTool{name: "external_tool"}
	reg := &mockToolRegistry{tools: map[string]tool.BaseTool{"external_tool": tl}}
	events := &mockEventSender{}
	mw := newExternalToolEventMiddleware(events, reg)

	next := func(ctx context.Context, in *compose.ToolInput) (*compose.ToolOutput, error) {
		return nil, context.Canceled
	}

	var sinkCallID string
	var sinkDisplay domain.ToolDisplay
	ctx := runtimectx.WithToolDisplaySink(context.Background(), func(callID string, display domain.ToolDisplay) {
		sinkCallID = callID
		sinkDisplay = display
	})

	out, err := mw.Invokable(next)(ctx, &compose.ToolInput{
		Name:      "external_tool",
		Arguments: "{}",
		CallID:    "c_cancel",
	})
	// Should not return a Go error but instead recover and return ToolErrorCancelled
	require.NoError(t, err)
	require.NotNil(t, out)
	require.Equal(t, domain.ToolErrorCancelled, out.Result)

	// UI events should show the cancellation
	require.Len(t, events.updates, 2)
	endEvent, ok := events.updates[1].(domain.ToolEndEvent)
	require.True(t, ok)
	require.Equal(t, domain.ToolErrorCancelled, endEvent.Display.GetError())

	// Sink persistence should show cancellation
	require.Equal(t, "c_cancel", sinkCallID)
	require.NotNil(t, sinkDisplay)
	require.Equal(t, domain.ToolErrorCancelled, sinkDisplay.GetError())
}
