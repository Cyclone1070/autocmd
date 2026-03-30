package shell

import (
	"context"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/tool/service/executor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// --- Mocks ---

type mockPathResolver struct {
	mock.Mock
}

func (m *mockPathResolver) Root() string {
	args := m.Called()
	return args.String(0)
}

type mockCommandExecutor struct {
	mock.Mock
}

func (m *mockCommandExecutor) RunStreaming(ctx context.Context, command []string, dir string, env []string) (*executor.StreamingCmd, error) {
	args := m.Called(ctx, command, dir, env)
	if res := args.Get(0); res != nil {
		return res.(*executor.StreamingCmd), args.Error(1)
	}
	return nil, args.Error(1)
}

func newTestStreamingCmd(output string, result *executor.Result, waitErr error) *executor.StreamingCmd {
	pr, pw := io.Pipe()

	go func() {
		_, _ = pw.Write([]byte(output))
		_ = pw.Close()
	}()

	return executor.NewStreamingCmd(pr, func() (*executor.Result, error) {
		return result, waitErr
	})
}

func TestShellTool_Name(t *testing.T) {
	tl := NewShellTool(&mockCommandExecutor{}, &mockPathResolver{})
	assert.Equal(t, "shell", tl.Name())
}

func TestShellTool_Declaration(t *testing.T) {
	tl := NewShellTool(&mockCommandExecutor{}, &mockPathResolver{})
	info := tl.Definition()
	assert.Equal(t, "shell", info.Name)
	assert.Contains(t, info.Desc, "shell command")
	assert.NotNil(t, info.ParamsOneOf)
}

func TestShellTool_Definition_OnlyCommandAndComment(t *testing.T) {
	tl := NewShellTool(&mockCommandExecutor{}, &mockPathResolver{})
	js, err := tl.Definition().ToJSONSchema()
	require.NoError(t, err)
	require.NotNil(t, js.Properties)
	keys := make([]string, 0, js.Properties.Len())
	for pair := js.Properties.Oldest(); pair != nil; pair = pair.Next() {
		keys = append(keys, pair.Key)
	}
	slices.Sort(keys)
	assert.Equal(t, []string{"command", "comment"}, keys)
}

func TestShellTool_Prepare_Validation(t *testing.T) {
	tl := NewShellTool(&mockCommandExecutor{}, &mockPathResolver{})
	ctx := context.Background()

	tests := []struct {
		name    string
		params  string
		wantErr string
	}{
		{
			name:    "Missing command",
			params:  `{"comment": "foo"}`,
			wantErr: "command is required",
		},
		{
			name:    "Empty command",
			params:  `{"command": [], "comment": "foo"}`,
			wantErr: "command is required",
		},
		{
			name:    "Missing comment",
			params:  `{"command": ["echo"]}`,
			wantErr: "comment is required",
		},
		{
			name:    "Empty comment",
			params:  `{"command": ["echo"], "comment": ""}`,
			wantErr: "comment is required",
		},
		{
			name:    "Whitespace-only comment",
			params:  `{"command": ["echo"], "comment": "   "}`,
			wantErr: "comment is required",
		},
		{
			name:    "Invalid JSON",
			params:  `{invalid`,
			wantErr: "failed to parse arguments",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tl.Prepare(ctx, tt.params)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestShellTool_Prepare_Success(t *testing.T) {
	mockPR := new(mockPathResolver)
	mockCE := new(mockCommandExecutor)

	tl := NewShellTool(mockCE, mockPR)

	mockPR.On("Root").Return("/workspace")

	streamCmd := newTestStreamingCmd("hello", &executor.Result{Stdout: "hello", ExitCode: 0}, nil)
	mockCE.On("RunStreaming", mock.Anything, []string{"echo", "hello"}, "/workspace", mock.Anything).
		Return(streamCmd, nil)

	ctx := context.Background()
	params := `{"command": ["echo", "hello"], "comment": "say hello"}`

	inv, err := tl.Prepare(ctx, params)
	require.NoError(t, err)
	require.NotNil(t, inv)

	disp := inv.Display()
	shellDisp := disp.(domain.ShellDisplay)
	assert.Equal(t, "echo hello", shellDisp.Command)
	assert.Equal(t, "say hello", shellDisp.Comment)
	assert.NotNil(t, shellDisp.Output)
}

func TestShellTool_Prepare_ExecutorError(t *testing.T) {
	mockPR := new(mockPathResolver)
	mockCE := new(mockCommandExecutor)

	tl := NewShellTool(mockCE, mockPR)

	mockPR.On("Root").Return("/workspace")

	mockCE.On("RunStreaming", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(nil, errors.New("command not found"))

	ctx := context.Background()
	params := `{"command": ["nonexistent"], "comment": "fail"}`

	_, err := tl.Prepare(ctx, params)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "command not found")
}

func TestShellTool_Prepare_EmptyWorkspaceRoot(t *testing.T) {
	mockPR := new(mockPathResolver)
	mockCE := new(mockCommandExecutor)

	tl := NewShellTool(mockCE, mockPR)

	mockPR.On("Root").Return("")

	ctx := context.Background()
	params := `{"command": ["echo"], "comment": "test"}`

	_, err := tl.Prepare(ctx, params)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "workspace root not set")
}

func TestShellTool_Execute_Success(t *testing.T) {
	mockPR := new(mockPathResolver)
	mockCE := new(mockCommandExecutor)

	tl := NewShellTool(mockCE, mockPR)

	mockPR.On("Root").Return("/workspace")
	streamCmd := newTestStreamingCmd("hello world", &executor.Result{Stdout: "hello world", ExitCode: 0}, nil)
	mockCE.On("RunStreaming", mock.Anything, []string{"echo", "hello"}, "/workspace", mock.Anything).
		Return(streamCmd, nil)

	ctx := context.Background()
	params := `{"command": ["echo", "hello"], "comment": "test"}`

	inv, err := tl.Prepare(ctx, params)
	require.NoError(t, err)

	disp := inv.Display().(domain.ShellDisplay)
	go func() { _, _ = io.Copy(io.Discard, disp.Output) }()

	output, err := inv.Execute(ctx)
	require.NoError(t, err)

	assert.Contains(t, output, "hello world")
	assert.Contains(t, output, "(Exit code: 0)")
}

func TestShellTool_Execute_NonZeroExit(t *testing.T) {
	mockPR := new(mockPathResolver)
	mockCE := new(mockCommandExecutor)

	tl := NewShellTool(mockCE, mockPR)

	mockPR.On("Root").Return("/workspace")
	streamCmd := newTestStreamingCmd("error output", &executor.Result{Stdout: "error output", ExitCode: 1}, nil)
	mockCE.On("RunStreaming", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(streamCmd, nil)

	ctx := context.Background()
	params := `{"command": ["false"], "comment": "fail"}`

	inv, err := tl.Prepare(ctx, params)
	require.NoError(t, err)

	disp := inv.Display().(domain.ShellDisplay)
	go func() { _, _ = io.Copy(io.Discard, disp.Output) }()

	output, err := inv.Execute(ctx)
	require.NoError(t, err)
	assert.Contains(t, output, "(Exit code: 1)")
}

func TestShellTool_Execute_ContextCancelled(t *testing.T) {
	mockPR := new(mockPathResolver)
	mockCE := new(mockCommandExecutor)

	tl := NewShellTool(mockCE, mockPR)

	mockPR.On("Root").Return("/workspace")
	streamCmd := newTestStreamingCmd("", &executor.Result{}, context.Canceled)
	mockCE.On("RunStreaming", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(streamCmd, nil)

	ctx, cancel := context.WithCancel(context.Background())
	params := `{"command": ["sleep", "10"], "comment": "sleep"}`

	inv, err := tl.Prepare(ctx, params)
	require.NoError(t, err)

	disp := inv.Display().(domain.ShellDisplay)
	go func() { _, _ = io.Copy(io.Discard, disp.Output) }()

	cancel()

	_, err = inv.Execute(ctx)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestShellTool_Execute_Truncation(t *testing.T) {
	mockPR := new(mockPathResolver)
	mockCE := new(mockCommandExecutor)

	tl := NewShellTool(mockCE, mockPR)

	mockPR.On("Root").Return("/workspace")
	streamCmd := newTestStreamingCmd("output", &executor.Result{Stdout: "output", ExitCode: 0, Truncated: true}, nil)
	mockCE.On("RunStreaming", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(streamCmd, nil)

	ctx := context.Background()
	params := `{"command": ["cat", "bigfile"], "comment": "cat"}`

	inv, err := tl.Prepare(ctx, params)
	require.NoError(t, err)

	disp := inv.Display().(domain.ShellDisplay)
	go func() { _, _ = io.Copy(io.Discard, disp.Output) }()

	output, err := inv.Execute(ctx)
	require.NoError(t, err)
	assert.Contains(t, output, "(Output truncated)")
}

func TestShellTool_Display_StreamingOutput(t *testing.T) {
	mockPR := new(mockPathResolver)
	mockCE := new(mockCommandExecutor)

	tl := NewShellTool(mockCE, mockPR)

	mockPR.On("Root").Return("/workspace")
	streamCmd := newTestStreamingCmd("streaming_test", &executor.Result{Stdout: "streaming_test", ExitCode: 0}, nil)
	mockCE.On("RunStreaming", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(streamCmd, nil)

	ctx := context.Background()
	params := `{"command": ["echo", "streaming"], "comment": "stream"}`

	inv, err := tl.Prepare(ctx, params)
	require.NoError(t, err)

	disp := inv.Display().(domain.ShellDisplay)

	var buf strings.Builder
	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(&buf, disp.Output)
		close(done)
	}()

	_, _ = inv.Execute(ctx)
	<-done

	assert.Contains(t, buf.String(), "streaming_test")
}

func TestShellTool_CapturedOutput(t *testing.T) {
	mockExec := &mockCommandExecutor{}
	res := &executor.Result{Stdout: "captured\n", ExitCode: 0}
	mockExec.On("RunStreaming", mock.Anything, []string{"echo", "captured"}, mock.Anything, mock.Anything).
		Return(newTestStreamingCmd("captured\n", res, nil), nil)

	mockPath := &mockPathResolver{}
	mockPath.On("Root").Return(".")

	tl := NewShellTool(mockExec, mockPath)
	ctx := context.Background()
	params := `{"command": ["echo", "captured"], "comment": "test capture"}`

	inv, err := tl.Prepare(ctx, params)
	require.NoError(t, err)

	disp := inv.Display().(domain.ShellDisplay)
	require.NotNil(t, disp.CapturedOutput)
	assert.Equal(t, "", *disp.CapturedOutput)

	_, err = inv.Execute(ctx)
	require.NoError(t, err)

	assert.Equal(t, "captured\n", *disp.CapturedOutput)
}
