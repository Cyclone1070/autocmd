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
			_, err := tl.Prepare(tt.params)
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

	params := `{"command": ["echo", "hello"], "comment": "say hello"}`

	inv, err := tl.Prepare(params)
	require.NoError(t, err)
	require.NotNil(t, inv)

	disp := inv.Display()
	shellDisp := disp.(domain.ShellDisplay)
	assert.Equal(t, "echo hello", shellDisp.Command)
	assert.Equal(t, "say hello", shellDisp.Comment)
	assert.Empty(t, shellDisp.CapturedOutput)
	si := inv.(domain.StreamableInvocation)
	assert.NotNil(t, si.Stream())
}

func TestShellTool_Prepare_ExecutorError(t *testing.T) {
	mockPR := new(mockPathResolver)
	mockCE := new(mockCommandExecutor)

	tl := NewShellTool(mockCE, mockPR)

	mockPR.On("Root").Return("/workspace")

	mockCE.On("RunStreaming", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(nil, errors.New("command not found"))

	params := `{"command": ["nonexistent"], "comment": "fail"}`

	inv, err := tl.Prepare(params)
	require.NoError(t, err)
	require.NotNil(t, inv)

	// The executor is now invoked during Execute (so cancellation can be driven by ctx).
	si := inv.(domain.StreamableInvocation)
	_ = si.Stream()

	ctx := context.Background()
	_, disp, err := inv.(domain.ExecutableInvocation).Execute(ctx)
	require.Error(t, err)
	assert.Contains(t, disp.GetError(), "command not found")
}

func TestShellTool_Prepare_EmptyWorkspaceRoot(t *testing.T) {
	mockPR := new(mockPathResolver)
	mockCE := new(mockCommandExecutor)

	tl := NewShellTool(mockCE, mockPR)

	mockPR.On("Root").Return("")

	params := `{"command": ["echo"], "comment": "test"}`

	_, err := tl.Prepare(params)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "workspace root not set")
}

func TestShellTool_Execute_FinalDisplay(t *testing.T) {
	mockPR := new(mockPathResolver)
	mockCE := new(mockCommandExecutor)

	tl := NewShellTool(mockCE, mockPR)

	mockPR.On("Root").Return("/workspace")
	streamCmd := newTestStreamingCmd("hello world", &executor.Result{Stdout: "hello world", ExitCode: 0}, nil)
	mockCE.On("RunStreaming", mock.Anything, []string{"echo", "hello"}, "/workspace", mock.Anything).
		Return(streamCmd, nil)

	ctx := context.Background()
	params := `{"command": ["echo", "hello"], "comment": "test"}`

	inv, err := tl.Prepare(params)
	require.NoError(t, err)

	si := inv.(domain.StreamableInvocation)
	si.Stream()
	go func() { _, _ = io.Copy(io.Discard, si.Stream()) }()

	llm, disp, err := inv.(domain.ExecutableInvocation).Execute(ctx)
	require.NoError(t, err)
	sh := disp.(domain.ShellDisplay)
	assert.Equal(t, "hello world", sh.CapturedOutput)
	assert.Empty(t, sh.GetError())
	assert.Contains(t, llm, "(Exit code: 0)")
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

	inv, err := tl.Prepare(params)
	require.NoError(t, err)

	si := inv.(domain.StreamableInvocation)
	si.Stream()
	go func() { _, _ = io.Copy(io.Discard, si.Stream()) }()

	output, _, err := inv.(domain.ExecutableInvocation).Execute(ctx)
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

	inv, err := tl.Prepare(params)
	require.NoError(t, err)

	si := inv.(domain.StreamableInvocation)
	si.Stream()
	go func() { _, _ = io.Copy(io.Discard, si.Stream()) }()

	output, _, err := inv.(domain.ExecutableInvocation).Execute(ctx)
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

	inv, err := tl.Prepare(params)
	require.NoError(t, err)

	si := inv.(domain.StreamableInvocation)
	si.Stream()
	go func() { _, _ = io.Copy(io.Discard, si.Stream()) }()

	cancel()

	_, disp, err := inv.(domain.ExecutableInvocation).Execute(ctx)
	assert.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, domain.ToolErrorCancelled, disp.GetError())
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

	inv, err := tl.Prepare(params)
	require.NoError(t, err)

	si := inv.(domain.StreamableInvocation)
	si.Stream()
	go func() { _, _ = io.Copy(io.Discard, si.Stream()) }()

	output, _, err := inv.(domain.ExecutableInvocation).Execute(ctx)
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

	inv, err := tl.Prepare(params)
	require.NoError(t, err)

	si := inv.(domain.StreamableInvocation)
	si.Stream() // Guarantee stable pipeWriter state before concurrency

	var buf strings.Builder
	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(&buf, si.Stream())
		close(done)
	}()

	_, _, _ = inv.(domain.ExecutableInvocation).Execute(ctx)
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

	inv, err := tl.Prepare(params)
	require.NoError(t, err)

	disp := inv.Display().(domain.ShellDisplay)
	assert.Empty(t, disp.CapturedOutput)

	out, _, err := inv.(domain.ExecutableInvocation).Execute(ctx)
	require.NoError(t, err)

	assert.Contains(t, out, "captured")
	assert.Contains(t, out, "(Exit code: 0)")
}
