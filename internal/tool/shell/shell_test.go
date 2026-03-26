package shell

import (
	"context"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"
	"time"

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

func (m *mockPathResolver) Abs(path string) (string, error) {
	args := m.Called(path)
	return args.String(0), args.Error(1)
}

func (m *mockPathResolver) Rel(path string) (string, error) {
	args := m.Called(path)
	return args.String(0), args.Error(1)
}

type mockEnvFileOps struct {
	mock.Mock
}

func (m *mockEnvFileOps) ReadFile(filename string) ([]byte, error) {
	args := m.Called(filename)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]byte), args.Error(1)
}

type mockCommandExecutor struct {
	mock.Mock
}

func (m *mockCommandExecutor) RunStreaming(ctx context.Context, command []string, dir string, env []string, timeout time.Duration) (*executor.StreamingCmd, error) {
	args := m.Called(ctx, command, dir, env, timeout)
	if res := args.Get(0); res != nil {
		return res.(*executor.StreamingCmd), args.Error(1)
	}
	return nil, args.Error(1)
}

// newTestStreamingCmd creates a real StreamingCmd for testing using executor's constructor
func newTestStreamingCmd(output string, result *executor.Result, waitErr error) *executor.StreamingCmd {
	pr, pw := io.Pipe()

	// Write output in background
	go func() {
		_, _ = pw.Write([]byte(output))
		_ = pw.Close()
	}()

	return executor.NewStreamingCmd(pr, func() (*executor.Result, error) {
		return result, waitErr
	})
}

// --- Tests ---

func TestShellTool_Name(t *testing.T) {
	tl := NewShellTool(&mockEnvFileOps{}, &mockCommandExecutor{}, time.Second, &mockPathResolver{})
	assert.Equal(t, "shell", tl.Name())
}

func TestShellTool_Declaration(t *testing.T) {
	tl := NewShellTool(&mockEnvFileOps{}, &mockCommandExecutor{}, time.Second, &mockPathResolver{})
	info := tl.Definition()
	assert.Equal(t, "shell", info.Name)
	assert.Contains(t, info.Desc, "shell command")
	assert.NotNil(t, info.ParamsOneOf)
}

func TestShellTool_Prepare_Validation(t *testing.T) {
	tl := NewShellTool(&mockEnvFileOps{}, &mockCommandExecutor{}, time.Second, &mockPathResolver{})
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
	mockEnv := new(mockEnvFileOps)

	tl := NewShellTool(mockEnv, mockCE, 10*time.Second, mockPR)

	// Setup mocks
	mockPR.On("Abs", ".").Return("/workspace", nil)

	streamCmd := newTestStreamingCmd("hello", &executor.Result{Stdout: "hello", ExitCode: 0}, nil)
	mockCE.On("RunStreaming", mock.Anything, []string{"echo", "hello"}, "/workspace", mock.Anything, mock.Anything).
		Return(streamCmd, nil)

	ctx := context.Background()
	params := `{"command": ["echo", "hello"], "comment": "say hello"}`

	inv, err := tl.Prepare(ctx, params)
	require.NoError(t, err)
	require.NotNil(t, inv)

	// Verify Display
	disp := inv.Display()
	shellDisp := disp.(domain.ShellDisplay)
	assert.Equal(t, "echo hello", shellDisp.Command)
	assert.Equal(t, "say hello", shellDisp.Comment)
	assert.NotNil(t, shellDisp.Output)
}

func TestShellTool_Prepare_CustomWorkingDir(t *testing.T) {
	mockPR := new(mockPathResolver)
	mockCE := new(mockCommandExecutor)
	mockEnv := new(mockEnvFileOps)

	tl := NewShellTool(mockEnv, mockCE, 10*time.Second, mockPR)

	// Setup mocks
	mockPR.On("Abs", "/custom/path").Return("/custom/path", nil)
	mockPR.On("Rel", "/custom/path").Return("custom/path", nil)

	streamCmd := newTestStreamingCmd("", &executor.Result{ExitCode: 0}, nil)
	mockCE.On("RunStreaming", mock.Anything, []string{"ls"}, "/custom/path", mock.Anything, mock.Anything).
		Return(streamCmd, nil)

	ctx := context.Background()
	params := `{"command": ["ls"], "working_dir": "/custom/path", "comment": "list"}`

	_, err := tl.Prepare(ctx, params)
	require.NoError(t, err)

	// custom/path is passed to executor in mocks, but not exposed in Display anymore
	mockCE.AssertExpectations(t)
}

func TestShellTool_Prepare_EnvFiles(t *testing.T) {
	mockPR := new(mockPathResolver)
	mockCE := new(mockCommandExecutor)
	mockEnv := new(mockEnvFileOps)

	tl := NewShellTool(mockEnv, mockCE, 10*time.Second, mockPR)

	// Setup mocks
	mockPR.On("Abs", ".").Return("/workspace", nil)
	mockPR.On("Rel", "/workspace").Return(".", nil)
	mockPR.On("Abs", ".env").Return("/workspace/.env", nil)

	// Mock env file reading
	mockEnv.On("ReadFile", "/workspace/.env").Return([]byte("KEY1=value1\nKEY2=value2"), nil)

	streamCmd := newTestStreamingCmd("", &executor.Result{ExitCode: 0}, nil)
	mockCE.On("RunStreaming", mock.Anything, []string{"echo"}, "/workspace", mock.MatchedBy(func(env []string) bool {
		hasKey1 := false
		hasKey2 := false
		for _, e := range env {
			if e == "KEY1=value1" {
				hasKey1 = true
			}
			if e == "KEY2=value2" {
				hasKey2 = true
			}
		}
		return hasKey1 && hasKey2
	}), mock.Anything).Return(streamCmd, nil)

	ctx := context.Background()
	params := `{"command": ["echo"], "env_files": [".env"], "comment": "test"}`

	_, err := tl.Prepare(ctx, params)
	require.NoError(t, err)

	mockEnv.AssertExpectations(t)
	mockCE.AssertExpectations(t)
}

func TestShellTool_Prepare_CustomEnvVars(t *testing.T) {
	mockPR := new(mockPathResolver)
	mockCE := new(mockCommandExecutor)
	mockEnv := new(mockEnvFileOps)

	tl := NewShellTool(mockEnv, mockCE, 10*time.Second, mockPR)

	// Setup mocks
	mockPR.On("Abs", ".").Return("/workspace", nil)
	mockPR.On("Rel", "/workspace").Return(".", nil)

	streamCmd := newTestStreamingCmd("", &executor.Result{ExitCode: 0}, nil)
	mockCE.On("RunStreaming", mock.Anything, []string{"echo"}, "/workspace", mock.MatchedBy(func(env []string) bool {
		return slices.Contains(env, "CUSTOM_VAR=custom_value")
	}), mock.Anything).Return(streamCmd, nil)

	ctx := context.Background()
	params := `{"command": ["echo"], "env": {"CUSTOM_VAR": "custom_value"}, "comment": "test"}`

	_, err := tl.Prepare(ctx, params)
	require.NoError(t, err)

	mockCE.AssertExpectations(t)
}

func TestShellTool_Prepare_CustomTimeout(t *testing.T) {
	mockPR := new(mockPathResolver)
	mockCE := new(mockCommandExecutor)
	mockEnv := new(mockEnvFileOps)

	tl := NewShellTool(mockEnv, mockCE, 10*time.Second, mockPR)

	// Setup mocks
	mockPR.On("Abs", ".").Return("/workspace", nil)
	mockPR.On("Rel", "/workspace").Return(".", nil)

	streamCmd := newTestStreamingCmd("", &executor.Result{ExitCode: 0}, nil)
	mockCE.On("RunStreaming", mock.Anything, []string{"sleep"}, "/workspace", mock.Anything, 30*time.Second).
		Return(streamCmd, nil)

	ctx := context.Background()
	params := `{"command": ["sleep"], "timeout_seconds": 30, "comment": "wait"}`

	_, err := tl.Prepare(ctx, params)
	require.NoError(t, err)

	mockCE.AssertExpectations(t)
}

func TestShellTool_Prepare_DefaultTimeout(t *testing.T) {
	mockPR := new(mockPathResolver)
	mockCE := new(mockCommandExecutor)
	mockEnv := new(mockEnvFileOps)

	tl := NewShellTool(mockEnv, mockCE, 60*time.Second, mockPR)

	// Setup mocks
	mockPR.On("Abs", ".").Return("/workspace", nil)
	mockPR.On("Rel", "/workspace").Return(".", nil)

	streamCmd := newTestStreamingCmd("", &executor.Result{ExitCode: 0}, nil)
	mockCE.On("RunStreaming", mock.Anything, []string{"echo"}, "/workspace", mock.Anything, 60*time.Second).
		Return(streamCmd, nil)

	ctx := context.Background()
	params := `{"command": ["echo"], "comment": "test"}`

	_, err := tl.Prepare(ctx, params)
	require.NoError(t, err)

	mockCE.AssertExpectations(t)
}

func TestShellTool_Prepare_ExecutorError(t *testing.T) {
	mockPR := new(mockPathResolver)
	mockCE := new(mockCommandExecutor)
	mockEnv := new(mockEnvFileOps)

	tl := NewShellTool(mockEnv, mockCE, 10*time.Second, mockPR)

	// Setup mocks
	mockPR.On("Abs", ".").Return("/workspace", nil)
	mockPR.On("Rel", "/workspace").Return(".", nil)

	mockCE.On("RunStreaming", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(nil, errors.New("command not found"))

	ctx := context.Background()
	params := `{"command": ["nonexistent"], "comment": "fail"}`

	_, err := tl.Prepare(ctx, params)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "command not found")
}

func TestShellTool_Prepare_PathResolverError(t *testing.T) {
	mockPR := new(mockPathResolver)
	mockCE := new(mockCommandExecutor)
	mockEnv := new(mockEnvFileOps)

	tl := NewShellTool(mockEnv, mockCE, 10*time.Second, mockPR)

	mockPR.On("Abs", ".").Return("", errors.New("path error"))

	ctx := context.Background()
	params := `{"command": ["echo"], "comment": "test"}`

	_, err := tl.Prepare(ctx, params)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "path error")
}

func TestShellTool_Prepare_EnvFileError(t *testing.T) {
	mockPR := new(mockPathResolver)
	mockCE := new(mockCommandExecutor)
	mockEnv := new(mockEnvFileOps)

	tl := NewShellTool(mockEnv, mockCE, 10*time.Second, mockPR)

	// Setup mocks
	mockPR.On("Abs", ".").Return("/workspace", nil)
	mockPR.On("Rel", "/workspace").Return(".", nil)
	mockPR.On("Abs", ".env").Return("/workspace/.env", nil)

	mockEnv.On("ReadFile", "/workspace/.env").Return(nil, errors.New("file not found"))

	ctx := context.Background()
	params := `{"command": ["echo"], "env_files": [".env"], "comment": "test"}`

	_, err := tl.Prepare(ctx, params)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "file not found")
}

func TestShellTool_Execute_Success(t *testing.T) {
	mockPR := new(mockPathResolver)
	mockCE := new(mockCommandExecutor)
	mockEnv := new(mockEnvFileOps)

	tl := NewShellTool(mockEnv, mockCE, 10*time.Second, mockPR)

	// Setup mocks
	mockPR.On("Abs", ".").Return("/workspace", nil)
	mockPR.On("Rel", "/workspace").Return(".", nil)

	streamCmd := newTestStreamingCmd("hello world", &executor.Result{Stdout: "hello world", ExitCode: 0}, nil)
	mockCE.On("RunStreaming", mock.Anything, []string{"echo", "hello"}, "/workspace", mock.Anything, mock.Anything).
		Return(streamCmd, nil)

	ctx := context.Background()
	params := `{"command": ["echo", "hello"], "comment": "test"}`

	inv, err := tl.Prepare(ctx, params)
	require.NoError(t, err)

	// Consume output
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
	mockEnv := new(mockEnvFileOps)

	tl := NewShellTool(mockEnv, mockCE, 10*time.Second, mockPR)

	// Setup mocks
	mockPR.On("Abs", ".").Return("/workspace", nil)
	mockPR.On("Rel", "/workspace").Return(".", nil)

	streamCmd := newTestStreamingCmd("error output", &executor.Result{Stdout: "error output", ExitCode: 1}, nil)
	mockCE.On("RunStreaming", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(streamCmd, nil)

	ctx := context.Background()
	params := `{"command": ["false"], "comment": "fail"}`

	inv, err := tl.Prepare(ctx, params)
	require.NoError(t, err)

	disp := inv.Display().(domain.ShellDisplay)
	go func() { _, _ = io.Copy(io.Discard, disp.Output) }()

	output, err := inv.Execute(ctx)
	require.NoError(t, err) // No error for non-zero exit
	assert.Contains(t, output, "(Exit code: 1)")
}

func TestShellTool_Execute_Timeout(t *testing.T) {
	mockPR := new(mockPathResolver)
	mockCE := new(mockCommandExecutor)
	mockEnv := new(mockEnvFileOps)

	tl := NewShellTool(mockEnv, mockCE, 10*time.Second, mockPR)

	// Setup mocks
	mockPR.On("Abs", ".").Return("/workspace", nil)
	mockPR.On("Rel", "/workspace").Return(".", nil)

	streamCmd := newTestStreamingCmd("partial", &executor.Result{Stdout: "partial", ExitCode: -1}, executor.ErrTimeout)
	mockCE.On("RunStreaming", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(streamCmd, nil)

	ctx := context.Background()
	params := `{"command": ["sleep", "10"], "timeout_seconds": 1, "comment": "sleep"}`

	inv, err := tl.Prepare(ctx, params)
	require.NoError(t, err)

	disp := inv.Display().(domain.ShellDisplay)
	go func() { _, _ = io.Copy(io.Discard, disp.Output) }()

	output, err := inv.Execute(ctx)
	assert.ErrorIs(t, err, executor.ErrTimeout)
	assert.Contains(t, output, "(Command timed out)")
}

func TestShellTool_Execute_ContextCancelled(t *testing.T) {
	mockPR := new(mockPathResolver)
	mockCE := new(mockCommandExecutor)
	mockEnv := new(mockEnvFileOps)

	tl := NewShellTool(mockEnv, mockCE, 10*time.Second, mockPR)

	// Setup mocks
	mockPR.On("Abs", ".").Return("/workspace", nil)
	mockPR.On("Rel", "/workspace").Return(".", nil)

	streamCmd := newTestStreamingCmd("", &executor.Result{}, context.Canceled)
	mockCE.On("RunStreaming", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(streamCmd, nil)

	ctx, cancel := context.WithCancel(context.Background())
	params := `{"command": ["sleep", "10"], "comment": "sleep"}`

	inv, err := tl.Prepare(ctx, params)
	require.NoError(t, err)

	disp := inv.Display().(domain.ShellDisplay)
	go func() { _, _ = io.Copy(io.Discard, disp.Output) }()

	cancel() // Cancel the context

	_, err = inv.Execute(ctx)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestShellTool_Execute_Truncation(t *testing.T) {
	mockPR := new(mockPathResolver)
	mockCE := new(mockCommandExecutor)
	mockEnv := new(mockEnvFileOps)

	tl := NewShellTool(mockEnv, mockCE, 10*time.Second, mockPR)

	// Setup mocks
	mockPR.On("Abs", ".").Return("/workspace", nil)
	mockPR.On("Rel", "/workspace").Return(".", nil)

	streamCmd := newTestStreamingCmd("output", &executor.Result{Stdout: "output", ExitCode: 0, Truncated: true}, nil)
	mockCE.On("RunStreaming", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
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
	mockEnv := new(mockEnvFileOps)

	tl := NewShellTool(mockEnv, mockCE, 10*time.Second, mockPR)

	// Setup mocks
	mockPR.On("Abs", ".").Return("/workspace", nil)
	mockPR.On("Rel", "/workspace").Return(".", nil)

	streamCmd := newTestStreamingCmd("streaming_test", &executor.Result{Stdout: "streaming_test", ExitCode: 0}, nil)
	mockCE.On("RunStreaming", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(streamCmd, nil)

	ctx := context.Background()
	params := `{"command": ["echo", "streaming"], "comment": "stream"}`

	inv, err := tl.Prepare(ctx, params)
	require.NoError(t, err)

	disp := inv.Display().(domain.ShellDisplay)

	// Read streaming output
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
	mockExec.On("RunStreaming", mock.Anything, []string{"echo", "captured"}, mock.Anything, mock.Anything, mock.Anything).
		Return(newTestStreamingCmd("captured\n", res, nil), nil)

	mockPath := &mockPathResolver{}
	mockPath.On("Abs", mock.Anything).Return(".", nil)

	tl := NewShellTool(&mockEnvFileOps{}, mockExec, 10*time.Second, mockPath)
	ctx := context.Background()
	params := `{"command": ["echo", "captured"], "comment": "test capture"}`

	inv, err := tl.Prepare(ctx, params)
	require.NoError(t, err)

	disp := inv.Display().(domain.ShellDisplay)
	require.NotNil(t, disp.CapturedOutput)
	assert.Equal(t, "", *disp.CapturedOutput)

	// Execute should populate the pointer
	_, err = inv.Execute(ctx)
	require.NoError(t, err)

	assert.Equal(t, "captured\n", *disp.CapturedOutput)
}
