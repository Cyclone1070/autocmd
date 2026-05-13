package agent

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/permission"
	"github.com/Cyclone1070/iav/internal/runtimectx"
	"github.com/Cyclone1070/iav/internal/testutil"
	"github.com/Cyclone1070/iav/internal/tool"
	"github.com/Cyclone1070/iav/internal/tool/read"
	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/require"
)

type capturingBus struct {
	updates []domain.UIUpdate
}

func (b *capturingBus) SendUIUpdate(u domain.UIUpdate) { b.updates = append(b.updates, u) }

type memReadFS struct{}

func (memReadFS) ReadFile(path string) ([]byte, error) { return []byte("hello\nworld\n"), nil }
func (memReadFS) Stat(path string) (os.FileInfo, error) {
	return memFileInfo{name: path, size: 12, isDir: false}, nil
}

type memFileInfo struct {
	name  string
	size  int64
	isDir bool
}

func (m memFileInfo) Name() string       { return m.name }
func (m memFileInfo) Size() int64        { return m.size }
func (m memFileInfo) Mode() os.FileMode  { return 0o644 }
func (m memFileInfo) ModTime() time.Time { return time.Time{} }
func (m memFileInfo) IsDir() bool        { return m.isDir }
func (m memFileInfo) Sys() any           { return nil }

type memChecksum struct{}

func (memChecksum) Compute(data []byte) string          { return string(data) }
func (memChecksum) Update(path string, checksum string) {}

type memPathResolver struct{}

func (memPathResolver) ValidateAbs(path string) (string, error) { return path, nil }
func (memPathResolver) DisplayPath(path string) string          { return path }

func TestToolExecution_EmitsToolEndEvent_AndPersistsDisplay_StrictSplit(t *testing.T) {
	// Use real read tool with mocks from its test helpers via minimal deps.
	// For integration purposes, we just need Prepare/Execute to run without touching disk.
	readTool := read.NewTool(memReadFS{}, memChecksum{}, memPathResolver{})

	tools := []einotool.BaseTool{readTool}
	reg := tool.NewRegistry(tools)
	waiter := &mockWaiter{act: domain.PermissionDecisionAction{Approved: true}}
	bus := &capturingBus{}
	resolver := permission.NewResolver("allow", nil)

	toolsNode, err := compose.NewToolNode(context.Background(), &compose.ToolsNodeConfig{
		Tools:               reg.Tools(),
		ExecuteSequentially: true,
		ToolCallMiddlewares: []compose.ToolMiddleware{
			newPreviewStartMiddleware(bus, reg),
			newPermissionMiddleware(resolver, waiter, bus, reg),
		},
	})
	require.NoError(t, err)

	displays := make(map[string]domain.ToolDisplay)
	ctx := context.Background()
	ctx = runtimectx.WithEventSender(ctx, bus)
	ctx = runtimectx.WithToolDisplaySink(ctx, func(callID string, d domain.ToolDisplay) { displays[callID] = d })

	assistant := &schema.Message{
		Role: schema.Assistant,
		ToolCalls: []schema.ToolCall{{
			ID: testToolCallID1,
			Function: schema.FunctionCall{
				Name:      testToolNameReadFile,
				Arguments: `{"file_path":"` + testutil.TestWorkspaceRoot + `/a.txt"}`,
			},
		}},
	}

	_, invokeErr := toolsNode.Invoke(ctx, assistant)
	require.NoError(t, invokeErr)

	foundEnd := false
	for _, u := range bus.updates {
		if e, ok := u.(domain.ToolEndEvent); ok {
			foundEnd = true
			require.Equal(t, testToolCallID1, e.CallID)
		}
	}
	require.True(t, foundEnd, "expected ToolEndEvent from tool execution")
	require.NotNil(t, displays[testToolCallID1], "expected ToolDisplay persisted by call id")
}
