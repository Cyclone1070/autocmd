package shell

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/Cyclone1070/iav/internal/config"
	"github.com/Cyclone1070/iav/internal/tool"
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

func (m *mockCommandExecutor) RunStreaming(ctx context.Context, command []string, dir string, env []string, timeout time.Duration) (streamingCommand, error) {
	args := m.Called(ctx, command, dir, env, timeout)
	if res := args.Get(0); res != nil {
		return res.(streamingCommand), args.Error(1)
	}
	return nil, args.Error(1)
}

// mockStreamingCommand implements streamingCommand for testing
type mockStreamingCommand struct {
	output io.Reader
	result *executor.Result
	err    error
}

func (m *mockStreamingCommand) Output() io.Reader {
	return m.output
}

func (m *mockStreamingCommand) Wait() (*executor.Result, error) {
	return m.result, m.err
}

// newMockStreamingCmd creates a mock streaming command for testing
func newMockStreamingCmd(output string, result *executor.Result, err error) *mockStreamingCommand {
	pr, pw := io.Pipe()
	go func() {
		pw.Write([]byte(output))
		pw.Close()
	}()
	return &mockStreamingCommand{
		output: pr,
		result: result,
		err:    err,
	}
}

// executorAdapter wraps a real OSCommandExecutor to satisfy the commandExecutor interface
type executorAdapter struct {
	exec *executor.OSCommandExecutor
}

func (a *executorAdapter) RunStreaming(ctx context.Context, cmd []string, dir string, env []string, timeout time.Duration) (streamingCommand, error) {
	return a.exec.RunStreaming(ctx, cmd, dir, env, timeout)
}

// --- Tests ---

func TestShellTool_Name(t *testing.T) {
	tl := NewShellTool(&mockEnvFileOps{}, &mockCommandExecutor{}, config.DefaultConfig(), &mockPathResolver{})
	assert.Equal(t, "shell", tl.Name())
}

func TestShellTool_Declaration(t *testing.T) {
	tl := NewShellTool(&mockEnvFileOps{}, &mockCommandExecutor{}, config.DefaultConfig(), &mockPathResolver{})
	decl := tl.Declaration()
	assert.Equal(t, "shell", decl.Name)
	assert.Contains(t, decl.Description, "shell command")
	assert.NotNil(t, decl.Parameters)
	assert.Contains(t, decl.Parameters.Properties, "command")
	assert.Contains(t, decl.Parameters.Properties, "description")
	assert.Contains(t, decl.Parameters.Required, "command")
	assert.Contains(t, decl.Parameters.Required, "description")
}

func TestShellTool_Prepare_Validation(t *testing.T) {
	tl := NewShellTool(&mockEnvFileOps{}, &mockCommandExecutor{}, config.DefaultConfig(), &mockPathResolver{})
	ctx := context.Background()

	tests := []struct {
		name    string
		params  string
		wantErr string
	}{
		{
			name:    "Missing command",
			params:  `{"description": "foo"}`,
			wantErr: "command is required",
		},
		{
			name:    "Empty command",
			params:  `{"command": [], "description": "foo"}`,
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
			_, err := tl.Prepare(ctx, json.RawMessage(tt.params))
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestShellTool_Prepare_Success(t *testing.T) {
	mockPR := new(mockPathResolver)
	mockCE := new(mockCommandExecutor)
	mockEnv := new(mockEnvFileOps)
	cfg := config.DefaultConfig()

	tl := NewShellTool(mockEnv, mockCE, cfg, mockPR)

	// Setup mocks
	mockPR.On("Abs", ".").Return("/workspace", nil)
	mockPR.On("Rel", "/workspace").Return(".", nil)

	mockStreamCmd := newMockStreamingCmd("hello", &executor.Result{Stdout: "hello", ExitCode: 0}, nil)
	mockCE.On("RunStreaming", mock.Anything, []string{"echo", "hello"}, "/workspace", mock.Anything, mock.Anything).
		Return(mockStreamCmd, nil)

	ctx := context.Background()
	params := `{"command": ["echo", "hello"], "description": "say hello"}`

	inv, err := tl.Prepare(ctx, json.RawMessage(params))
	require.NoError(t, err)
	require.NotNil(t, inv)

	// Verify Display
	disp := inv.Display()
	shellDisp, ok := disp.(tool.ShellDisplay)
	require.True(t, ok)
	assert.Equal(t, "[echo hello]", shellDisp.Command)
	assert.Equal(t, "say hello", shellDisp.Description)
	assert.Equal(t, ".", shellDisp.WorkingDir)
	assert.NotNil(t, shellDisp.Output)
	assert.NotNil(t, shellDisp.Wait)

	mockPR.AssertExpectations(t)
	mockCE.AssertExpectations(t)
}

func TestShellTool_Prepare_CustomWorkingDir(t *testing.T) {
	mockPR := new(mockPathResolver)
	mockCE := new(mockCommandExecutor)
	mockEnv := new(mockEnvFileOps)
	cfg := config.DefaultConfig()

	tl := NewShellTool(mockEnv, mockCE, cfg, mockPR)

	// Setup mocks
	mockPR.On("Abs", "/custom/path").Return("/custom/path", nil)
	mockPR.On("Rel", "/custom/path").Return("custom/path", nil)

	mockStreamCmd := newMockStreamingCmd("", &executor.Result{ExitCode: 0}, nil)
	mockCE.On("RunStreaming", mock.Anything, []string{"ls"}, "/custom/path", mock.Anything, mock.Anything).
		Return(mockStreamCmd, nil)

	ctx := context.Background()
	params := `{"command": ["ls"], "working_dir": "/custom/path", "description": "list"}`

	inv, err := tl.Prepare(ctx, json.RawMessage(params))
	require.NoError(t, err)

	disp := inv.Display().(tool.ShellDisplay)
	assert.Equal(t, "custom/path", disp.WorkingDir)

	mockCE.AssertExpectations(t)
}

func TestShellTool_Prepare_EnvFiles(t *testing.T) {
	mockPR := new(mockPathResolver)
	mockCE := new(mockCommandExecutor)
	mockEnv := new(mockEnvFileOps)
	cfg := config.DefaultConfig()

	tl := NewShellTool(mockEnv, mockCE, cfg, mockPR)

	// Setup mocks
	mockPR.On("Abs", ".").Return("/workspace", nil)
	mockPR.On("Rel", "/workspace").Return(".", nil)
	mockPR.On("Abs", ".env").Return("/workspace/.env", nil)

	// Mock env file reading
	mockEnv.On("ReadFile", "/workspace/.env").Return([]byte("KEY1=value1\nKEY2=value2"), nil)

	mockStreamCmd := newMockStreamingCmd("", &executor.Result{ExitCode: 0}, nil)
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
	}), mock.Anything).Return(mockStreamCmd, nil)

	ctx := context.Background()
	params := `{"command": ["echo"], "env_files": [".env"], "description": "test"}`

	_, err := tl.Prepare(ctx, json.RawMessage(params))
	require.NoError(t, err)

	mockEnv.AssertExpectations(t)
	mockCE.AssertExpectations(t)
}

func TestShellTool_Prepare_CustomEnvVars(t *testing.T) {
	mockPR := new(mockPathResolver)
	mockCE := new(mockCommandExecutor)
	mockEnv := new(mockEnvFileOps)
	cfg := config.DefaultConfig()

	tl := NewShellTool(mockEnv, mockCE, cfg, mockPR)

	// Setup mocks
	mockPR.On("Abs", ".").Return("/workspace", nil)
	mockPR.On("Rel", "/workspace").Return(".", nil)

	mockStreamCmd := newMockStreamingCmd("", &executor.Result{ExitCode: 0}, nil)
	mockCE.On("RunStreaming", mock.Anything, []string{"echo"}, "/workspace", mock.MatchedBy(func(env []string) bool {
		for _, e := range env {
			if e == "CUSTOM_VAR=custom_value" {
				return true
			}
		}
		return false
	}), mock.Anything).Return(mockStreamCmd, nil)

	ctx := context.Background()
	params := `{"command": ["echo"], "env": {"CUSTOM_VAR": "custom_value"}, "description": "test"}`

	_, err := tl.Prepare(ctx, json.RawMessage(params))
	require.NoError(t, err)

	mockCE.AssertExpectations(t)
}

func TestShellTool_Prepare_CustomTimeout(t *testing.T) {
	mockPR := new(mockPathResolver)
	mockCE := new(mockCommandExecutor)
	mockEnv := new(mockEnvFileOps)
	cfg := config.DefaultConfig()

	tl := NewShellTool(mockEnv, mockCE, cfg, mockPR)

	// Setup mocks
	mockPR.On("Abs", ".").Return("/workspace", nil)
	mockPR.On("Rel", "/workspace").Return(".", nil)

	mockStreamCmd := newMockStreamingCmd("", &executor.Result{ExitCode: 0}, nil)
	mockCE.On("RunStreaming", mock.Anything, []string{"sleep"}, "/workspace", mock.Anything, 30*time.Second).
		Return(mockStreamCmd, nil)

	ctx := context.Background()
	params := `{"command": ["sleep"], "timeout_seconds": 30, "description": "wait"}`

	_, err := tl.Prepare(ctx, json.RawMessage(params))
	require.NoError(t, err)

	mockCE.AssertExpectations(t)
}

func TestShellTool_Prepare_DefaultTimeout(t *testing.T) {
	mockPR := new(mockPathResolver)
	mockCE := new(mockCommandExecutor)
	mockEnv := new(mockEnvFileOps)
	cfg := config.DefaultConfig()
	cfg.Tools.DefaultShellTimeout = 60 // 60 seconds default

	tl := NewShellTool(mockEnv, mockCE, cfg, mockPR)

	// Setup mocks
	mockPR.On("Abs", ".").Return("/workspace", nil)
	mockPR.On("Rel", "/workspace").Return(".", nil)

	mockStreamCmd := newMockStreamingCmd("", &executor.Result{ExitCode: 0}, nil)
	mockCE.On("RunStreaming", mock.Anything, []string{"echo"}, "/workspace", mock.Anything, 60*time.Second).
		Return(mockStreamCmd, nil)

	ctx := context.Background()
	params := `{"command": ["echo"], "description": "test"}`

	_, err := tl.Prepare(ctx, json.RawMessage(params))
	require.NoError(t, err)

	mockCE.AssertExpectations(t)
}

func TestShellTool_Prepare_ExecutorError(t *testing.T) {
	mockPR := new(mockPathResolver)
	mockCE := new(mockCommandExecutor)
	mockEnv := new(mockEnvFileOps)
	cfg := config.DefaultConfig()

	tl := NewShellTool(mockEnv, mockCE, cfg, mockPR)

	// Setup mocks
	mockPR.On("Abs", ".").Return("/workspace", nil)
	mockPR.On("Rel", "/workspace").Return(".", nil)

	mockCE.On("RunStreaming", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(nil, errors.New("command not found"))

	ctx := context.Background()
	params := `{"command": ["nonexistent"], "description": "fail"}`

	_, err := tl.Prepare(ctx, json.RawMessage(params))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "command not found")
}

func TestShellTool_Prepare_PathResolverError(t *testing.T) {
	mockPR := new(mockPathResolver)
	mockCE := new(mockCommandExecutor)
	mockEnv := new(mockEnvFileOps)
	cfg := config.DefaultConfig()

	tl := NewShellTool(mockEnv, mockCE, cfg, mockPR)

	mockPR.On("Abs", ".").Return("", errors.New("path error"))

	ctx := context.Background()
	params := `{"command": ["echo"], "description": "test"}`

	_, err := tl.Prepare(ctx, json.RawMessage(params))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "path error")
}

func TestShellTool_Prepare_EnvFileError(t *testing.T) {
	mockPR := new(mockPathResolver)
	mockCE := new(mockCommandExecutor)
	mockEnv := new(mockEnvFileOps)
	cfg := config.DefaultConfig()

	tl := NewShellTool(mockEnv, mockCE, cfg, mockPR)

	// Setup mocks
	mockPR.On("Abs", ".").Return("/workspace", nil)
	mockPR.On("Rel", "/workspace").Return(".", nil)
	mockPR.On("Abs", ".env").Return("/workspace/.env", nil)

	mockEnv.On("ReadFile", "/workspace/.env").Return(nil, errors.New("file not found"))

	ctx := context.Background()
	params := `{"command": ["echo"], "env_files": [".env"], "description": "test"}`

	_, err := tl.Prepare(ctx, json.RawMessage(params))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "file not found")
}

func TestShellTool_Execute_Success(t *testing.T) {
	mockPR := new(mockPathResolver)
	mockCE := new(mockCommandExecutor)
	mockEnv := new(mockEnvFileOps)
	cfg := config.DefaultConfig()

	tl := NewShellTool(mockEnv, mockCE, cfg, mockPR)

	// Setup mocks
	mockPR.On("Abs", ".").Return("/workspace", nil)
	mockPR.On("Rel", "/workspace").Return(".", nil)

	mockStreamCmd := newMockStreamingCmd("hello world", &executor.Result{Stdout: "hello world", ExitCode: 0}, nil)
	mockCE.On("RunStreaming", mock.Anything, []string{"echo", "hello"}, "/workspace", mock.Anything, mock.Anything).
		Return(mockStreamCmd, nil)

	ctx := context.Background()
	params := `{"command": ["echo", "hello"], "description": "test"}`

	inv, err := tl.Prepare(ctx, json.RawMessage(params))
	require.NoError(t, err)

	// Consume output
	disp := inv.Display().(tool.ShellDisplay)
	go io.Copy(io.Discard, disp.Output)

	output, err := inv.Execute(ctx)
	require.NoError(t, err)

	assert.Contains(t, output, "hello world")
	assert.Contains(t, output, "(Exit code: 0)")
}

func TestShellTool_Execute_NonZeroExit(t *testing.T) {
	mockPR := new(mockPathResolver)
	mockCE := new(mockCommandExecutor)
	mockEnv := new(mockEnvFileOps)
	cfg := config.DefaultConfig()

	tl := NewShellTool(mockEnv, mockCE, cfg, mockPR)

	// Setup mocks
	mockPR.On("Abs", ".").Return("/workspace", nil)
	mockPR.On("Rel", "/workspace").Return(".", nil)

	mockStreamCmd := newMockStreamingCmd("error output", &executor.Result{Stdout: "error output", ExitCode: 1}, nil)
	mockCE.On("RunStreaming", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(mockStreamCmd, nil)

	ctx := context.Background()
	params := `{"command": ["false"], "description": "fail"}`

	inv, err := tl.Prepare(ctx, json.RawMessage(params))
	require.NoError(t, err)

	disp := inv.Display().(tool.ShellDisplay)
	go io.Copy(io.Discard, disp.Output)

	output, err := inv.Execute(ctx)
	require.NoError(t, err) // No error for non-zero exit
	assert.Contains(t, output, "(Exit code: 1)")
}

func TestShellTool_Execute_Timeout(t *testing.T) {
	mockPR := new(mockPathResolver)
	mockCE := new(mockCommandExecutor)
	mockEnv := new(mockEnvFileOps)
	cfg := config.DefaultConfig()

	tl := NewShellTool(mockEnv, mockCE, cfg, mockPR)

	// Setup mocks
	mockPR.On("Abs", ".").Return("/workspace", nil)
	mockPR.On("Rel", "/workspace").Return(".", nil)

	mockStreamCmd := newMockStreamingCmd("partial", &executor.Result{Stdout: "partial", ExitCode: -1}, executor.ErrTimeout)
	mockCE.On("RunStreaming", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(mockStreamCmd, nil)

	ctx := context.Background()
	params := `{"command": ["sleep", "10"], "timeout_seconds": 1, "description": "sleep"}`

	inv, err := tl.Prepare(ctx, json.RawMessage(params))
	require.NoError(t, err)

	disp := inv.Display().(tool.ShellDisplay)
	go io.Copy(io.Discard, disp.Output)

	output, err := inv.Execute(ctx)
	assert.ErrorIs(t, err, executor.ErrTimeout)
	assert.Contains(t, output, "(Command timed out)")
}

func TestShellTool_Execute_ContextCancelled(t *testing.T) {
	mockPR := new(mockPathResolver)
	mockCE := new(mockCommandExecutor)
	mockEnv := new(mockEnvFileOps)
	cfg := config.DefaultConfig()

	tl := NewShellTool(mockEnv, mockCE, cfg, mockPR)

	// Setup mocks
	mockPR.On("Abs", ".").Return("/workspace", nil)
	mockPR.On("Rel", "/workspace").Return(".", nil)

	mockStreamCmd := newMockStreamingCmd("", &executor.Result{}, context.Canceled)
	mockCE.On("RunStreaming", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(mockStreamCmd, nil)

	ctx, cancel := context.WithCancel(context.Background())
	params := `{"command": ["sleep", "10"], "description": "sleep"}`

	inv, err := tl.Prepare(ctx, json.RawMessage(params))
	require.NoError(t, err)

	disp := inv.Display().(tool.ShellDisplay)
	go io.Copy(io.Discard, disp.Output)

	cancel() // Cancel the context

	_, err = inv.Execute(ctx)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestShellTool_Execute_Truncation(t *testing.T) {
	mockPR := new(mockPathResolver)
	mockCE := new(mockCommandExecutor)
	mockEnv := new(mockEnvFileOps)
	cfg := config.DefaultConfig()

	tl := NewShellTool(mockEnv, mockCE, cfg, mockPR)

	// Setup mocks
	mockPR.On("Abs", ".").Return("/workspace", nil)
	mockPR.On("Rel", "/workspace").Return(".", nil)

	mockStreamCmd := newMockStreamingCmd("output", &executor.Result{Stdout: "output", ExitCode: 0, Truncated: true}, nil)
	mockCE.On("RunStreaming", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(mockStreamCmd, nil)

	ctx := context.Background()
	params := `{"command": ["cat", "bigfile"], "description": "cat"}`

	inv, err := tl.Prepare(ctx, json.RawMessage(params))
	require.NoError(t, err)

	disp := inv.Display().(tool.ShellDisplay)
	go io.Copy(io.Discard, disp.Output)

	output, err := inv.Execute(ctx)
	require.NoError(t, err)
	assert.Contains(t, output, "(Output truncated)")
}

func TestShellTool_Display_StreamingOutput(t *testing.T) {
	mockPR := new(mockPathResolver)
	mockCE := new(mockCommandExecutor)
	mockEnv := new(mockEnvFileOps)
	cfg := config.DefaultConfig()

	tl := NewShellTool(mockEnv, mockCE, cfg, mockPR)

	// Setup mocks
	mockPR.On("Abs", ".").Return("/workspace", nil)
	mockPR.On("Rel", "/workspace").Return(".", nil)

	mockStreamCmd := newMockStreamingCmd("streaming_test", &executor.Result{Stdout: "streaming_test", ExitCode: 0}, nil)
	mockCE.On("RunStreaming", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(mockStreamCmd, nil)

	ctx := context.Background()
	params := `{"command": ["echo", "streaming"], "description": "stream"}`

	inv, err := tl.Prepare(ctx, json.RawMessage(params))
	require.NoError(t, err)

	disp := inv.Display().(tool.ShellDisplay)

	// Read streaming output
	var buf strings.Builder
	done := make(chan struct{})
	go func() {
		io.Copy(&buf, disp.Output)
		close(done)
	}()

	_, _ = inv.Execute(ctx)
	<-done

	assert.Contains(t, buf.String(), "streaming_test")
}

func TestShellTool_Display_Wait(t *testing.T) {
	mockPR := new(mockPathResolver)
	mockCE := new(mockCommandExecutor)
	mockEnv := new(mockEnvFileOps)
	cfg := config.DefaultConfig()

	tl := NewShellTool(mockEnv, mockCE, cfg, mockPR)

	// Setup mocks
	mockPR.On("Abs", ".").Return("/workspace", nil)
	mockPR.On("Rel", "/workspace").Return(".", nil)

	// Create a mock that blocks on Wait
	pr, pw := io.Pipe()
	waitCalled := make(chan struct{})
	mockStreamCmd := &mockStreamingCommand{
		output: pr,
		result: &executor.Result{Stdout: "done", ExitCode: 0},
	}
	// Override Wait to block until channel is closed
	originalWait := mockStreamCmd
	blockingMock := &blockingMockStreamingCommand{
		mockStreamingCommand: originalWait,
		waitCh:               waitCalled,
	}

	mockCE.On("RunStreaming", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(blockingMock, nil)

	ctx := context.Background()
	params := `{"command": ["sleep", "1"], "description": "wait"}`

	inv, err := tl.Prepare(ctx, json.RawMessage(params))
	require.NoError(t, err)

	disp := inv.Display().(tool.ShellDisplay)

	// Wait should block
	done := make(chan struct{})
	go func() {
		disp.Wait()
		close(done)
	}()

	select {
	case <-done:
		t.Fatal("Wait should have blocked")
	case <-time.After(50 * time.Millisecond):
		// expected to block
	}

	// Signal that Wait can return
	close(waitCalled)
	pw.Close()

	select {
	case <-done:
		// success
	case <-time.After(1 * time.Second):
		t.Fatal("Wait should have unblocked")
	}
}

// blockingMockStreamingCommand wraps mockStreamingCommand and blocks on Wait
type blockingMockStreamingCommand struct {
	*mockStreamingCommand
	waitCh <-chan struct{}
}

func (m *blockingMockStreamingCommand) Wait() (*executor.Result, error) {
	<-m.waitCh
	return m.mockStreamingCommand.result, m.mockStreamingCommand.err
}
