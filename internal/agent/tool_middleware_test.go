package agent

import (
	"context"
	"fmt"
	"testing"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/permission"
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

func (m *mockPreviewTool) Name() string                                     { return m.name }
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
	tl := &mockPreviewTool{name: "read_file"}
	reg := &mockToolRegistry{tools: map[string]tool.BaseTool{"read_file": tl}}
	events := &mockEventSender{}
	mw := newPreviewStartMiddleware(events, reg)

	nextCalled := 0
	next := func(ctx context.Context, in *compose.ToolInput) (*compose.ToolOutput, error) {
		nextCalled++
		return &compose.ToolOutput{Result: "from_next"}, nil
	}

	ctx := context.Background()
	out, err := mw.Invokable(next)(ctx, &compose.ToolInput{
		Name:      "read_file",
		Arguments: `{"file_path":"/tmp/x"}`,
		CallID:    "call_1",
	})
	require.NoError(t, err)
	require.Equal(t, "from_next", out.Result)

	require.Equal(t, 1, tl.previewN)
	require.Equal(t, 1, nextCalled)
	require.Len(t, events.updates, 1)
	start, ok := events.updates[0].(domain.ToolStartEvent)
	require.True(t, ok)
	require.Equal(t, "call_1", start.CallID)
}

func TestPermissionMiddleware_PermissionDenied_DoesNotCallNext(t *testing.T) {
	events := &mockEventSender{}
	waiter := &mockWaiter{act: domain.PermissionDecisionAction{Approved: false}}
	resolver := permission.NewResolver("ask", map[string]string{"read_file": "ask"})
	tl := &mockPreviewTool{name: "read_file"}
	reg := &mockToolRegistry{tools: map[string]tool.BaseTool{"read_file": tl}}
	mw := newPermissionMiddleware(resolver, waiter, events, reg)

	nextCalled := 0
	next := func(ctx context.Context, in *compose.ToolInput) (*compose.ToolOutput, error) {
		nextCalled++
		return &compose.ToolOutput{Result: "from_next"}, nil
	}

	ctx := context.Background()
	out, err := mw.Invokable(next)(ctx, &compose.ToolInput{
		Name:      "read_file",
		Arguments: `{"file_path":"/tmp/x"}`,
		CallID:    "call_1",
	})
	require.NoError(t, err)
	require.Equal(t, "Tool execution was denied by the user.", out.Result)
	require.Equal(t, 0, nextCalled)
	require.Len(t, events.updates, 2)
	_, ok := events.updates[0].(domain.ToolApprovalRequestEvent)
	require.True(t, ok)
	end, ok := events.updates[1].(domain.ToolEndEvent)
	require.True(t, ok)
	require.Equal(t, domain.ToolErrorPermissionDenied, end.Display.GetError())
	sd, ok := end.Display.(domain.StringDisplay)
	require.True(t, ok)
	require.Equal(t, "preview", sd.Description)
}

func TestPermissionMiddleware_AskQuestion_StillRespectsResolverPolicy(t *testing.T) {
	events := &mockEventSender{}
	waiter := &mockWaiter{act: domain.PermissionDecisionAction{Approved: false}}
	resolver := permission.NewResolver("allow", map[string]string{"ask_question": "ask"})
	tl := &mockPreviewTool{name: "ask_question"}
	reg := &mockToolRegistry{tools: map[string]tool.BaseTool{"ask_question": tl}}
	mw := newPermissionMiddleware(resolver, waiter, events, reg)

	nextCalled := 0
	next := func(ctx context.Context, in *compose.ToolInput) (*compose.ToolOutput, error) {
		nextCalled++
		return &compose.ToolOutput{Result: "from_next"}, nil
	}

	ctx := context.Background()
	out, err := mw.Invokable(next)(ctx, &compose.ToolInput{
		Name:      "ask_question",
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
	require.Equal(t, domain.ToolErrorPermissionDenied, end.Display.GetError())
}

func TestPermissionMiddleware_WaiterNil_EmitsToolEnd(t *testing.T) {
	events := &mockEventSender{}
	resolver := permission.NewResolver("ask", map[string]string{"read_file": "ask"})
	tl := &mockPreviewTool{name: "read_file"}
	reg := &mockToolRegistry{tools: map[string]tool.BaseTool{"read_file": tl}}
	mw := newPermissionMiddleware(resolver, nil, events, reg)

	nextCalled := 0
	next := func(ctx context.Context, in *compose.ToolInput) (*compose.ToolOutput, error) {
		nextCalled++
		return &compose.ToolOutput{Result: "from_next"}, nil
	}

	ctx := context.Background()
	out, err := mw.Invokable(next)(ctx, &compose.ToolInput{
		Name:      "read_file",
		Arguments: `{"file_path":"/tmp/x"}`,
		CallID:    "call_1",
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
	resolver := permission.NewResolver("ask", map[string]string{"read_file": "ask"})
	tl := &mockPreviewTool{name: "read_file"}
	reg := &mockToolRegistry{tools: map[string]tool.BaseTool{"read_file": tl}}
	mw := newPermissionMiddleware(resolver, waiter, events, reg)

	nextCalled := 0
	next := func(ctx context.Context, in *compose.ToolInput) (*compose.ToolOutput, error) {
		nextCalled++
		return &compose.ToolOutput{Result: "from_next"}, nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	out, err := mw.Invokable(next)(ctx, &compose.ToolInput{
		Name:      "read_file",
		Arguments: `{"file_path":"/tmp/x"}`,
		CallID:    "call_1",
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
	tl := &mockPreviewValidateTool{mockPreviewTool: mockPreviewTool{name: "read_file"}}
	reg := &mockToolRegistry{tools: map[string]tool.BaseTool{"read_file": tl}}
	events := &mockEventSender{}
	mw := newPreflightValidationMiddleware(events, reg)

	nextCalled := 0
	next := func(ctx context.Context, in *compose.ToolInput) (*compose.ToolOutput, error) {
		nextCalled++
		return &compose.ToolOutput{Result: "ok"}, nil
	}

	out, err := mw.Invokable(next)(context.Background(), &compose.ToolInput{
		Name:      "read_file",
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
