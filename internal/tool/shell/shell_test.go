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

func (m *mockCommandExecutor) RunStreaming(ctx context.Context, command []string, dir string, env []string, timeout time.Duration) (*executor.StreamingCmd, error) {
	args := m.Called(ctx, command, dir, env, timeout)
	if res := args.Get(0); res != nil {
		return res.(*executor.StreamingCmd), args.Error(1)
	}
	return nil, args.Error(1)
}

// createMockStreamingCmd creates a mock StreamingCmd for testing.
// The provided result and err are returned when Wait() is called.
func createMockStreamingCmd(output string, result *executor.Result, err error) *executor.StreamingCmd {
	pr, pw := io.Pipe()
	go func() {
		pw.Write([]byte(output))
		pw.Close()
	}()

	return &executor.StreamingCmd{
		Output: pr,
		// Note: We cannot set the internal wait func directly since it's private.
		// We need to use the real executor for this, or refactor StreamingCmd.
	}
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

	// Create a mock StreamingCmd
	pr, pw := io.Pipe()
	go func() {
		pw.Write([]byte("hello"))
		pw.Close()
	}()
	mockStreamCmd := &executor.StreamingCmd{Output: pr}

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

	pr, pw := io.Pipe()
	go func() { pw.Close() }()
	mockStreamCmd := &executor.StreamingCmd{Output: pr}

	// Assert command is run in custom directory
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

	pr, pw := io.Pipe()
	go func() { pw.Close() }()
	mockStreamCmd := &executor.StreamingCmd{Output: pr}

	// Capture the env slice to verify it contains our env vars
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

	pr, pw := io.Pipe()
	go func() { pw.Close() }()
	mockStreamCmd := &executor.StreamingCmd{Output: pr}

	// Verify custom env vars are passed
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

	pr, pw := io.Pipe()
	go func() { pw.Close() }()
	mockStreamCmd := &executor.StreamingCmd{Output: pr}

	// Verify custom timeout is passed (30 seconds)
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

	pr, pw := io.Pipe()
	go func() { pw.Close() }()
	mockStreamCmd := &executor.StreamingCmd{Output: pr}

	// Verify default timeout is used
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

	// Executor returns error
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

	// Path resolution fails
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

	// Env file reading fails
	mockEnv.On("ReadFile", "/workspace/.env").Return(nil, errors.New("file not found"))

	ctx := context.Background()
	params := `{"command": ["echo"], "env_files": [".env"], "description": "test"}`

	_, err := tl.Prepare(ctx, json.RawMessage(params))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "file not found")
}

func TestShellTool_Execute_Success(t *testing.T) {
	// For Execute tests, we need to use the real executor since StreamingCmd's
	// Wait() function is private and cannot be mocked directly.
	// This is an integration test by necessity.
	cfg := config.DefaultConfig()
	realCE := executor.NewOSCommandExecutor(cfg)

	mockPR := new(mockPathResolver)
	mockEnv := new(mockEnvFileOps)

	tl := NewShellTool(mockEnv, realCE, cfg, mockPR)

	mockPR.On("Abs", ".").Return("/tmp", nil)
	mockPR.On("Rel", "/tmp").Return(".", nil)

	ctx := context.Background()
	params := `{"command": ["echo", "test"], "description": "echo test"}`

	inv, err := tl.Prepare(ctx, json.RawMessage(params))
	require.NoError(t, err)

	// Consume output
	disp := inv.Display().(tool.ShellDisplay)
	go io.Copy(io.Discard, disp.Output)

	output, err := inv.Execute(ctx)
	require.NoError(t, err)

	assert.Contains(t, output, "test")
	assert.Contains(t, output, "(Exit code: 0)")
}

func TestShellTool_Execute_Timeout(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Tools.DockerGracefulShutdownMs = 100
	realCE := executor.NewOSCommandExecutor(cfg)

	mockPR := new(mockPathResolver)
	mockEnv := new(mockEnvFileOps)

	tl := NewShellTool(mockEnv, realCE, cfg, mockPR)

	mockPR.On("Abs", ".").Return("/tmp", nil)
	mockPR.On("Rel", "/tmp").Return(".", nil)

	ctx := context.Background()
	params := `{"command": ["sleep", "5"], "timeout_seconds": 1, "description": "sleep"}`

	inv, err := tl.Prepare(ctx, json.RawMessage(params))
	require.NoError(t, err)

	// Consume output
	disp := inv.Display().(tool.ShellDisplay)
	go io.Copy(io.Discard, disp.Output)

	output, err := inv.Execute(ctx)
	assert.ErrorIs(t, err, executor.ErrTimeout)
	assert.Contains(t, output, "(Command timed out)")
}

func TestShellTool_Execute_ContextCancellation(t *testing.T) {
	cfg := config.DefaultConfig()
	realCE := executor.NewOSCommandExecutor(cfg)

	mockPR := new(mockPathResolver)
	mockEnv := new(mockEnvFileOps)

	tl := NewShellTool(mockEnv, realCE, cfg, mockPR)

	mockPR.On("Abs", ".").Return("/tmp", nil)
	mockPR.On("Rel", "/tmp").Return(".", nil)

	ctx, cancel := context.WithCancel(context.Background())
	params := `{"command": ["sleep", "10"], "description": "sleep"}`

	inv, err := tl.Prepare(ctx, json.RawMessage(params))
	require.NoError(t, err)

	// Consume output
	disp := inv.Display().(tool.ShellDisplay)
	go io.Copy(io.Discard, disp.Output)

	// Cancel after a short delay
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	_, err = inv.Execute(ctx)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestShellTool_Execute_NonZeroExit(t *testing.T) {
	cfg := config.DefaultConfig()
	realCE := executor.NewOSCommandExecutor(cfg)

	mockPR := new(mockPathResolver)
	mockEnv := new(mockEnvFileOps)

	tl := NewShellTool(mockEnv, realCE, cfg, mockPR)

	mockPR.On("Abs", ".").Return("/tmp", nil)
	mockPR.On("Rel", "/tmp").Return(".", nil)

	ctx := context.Background()
	params := `{"command": ["false"], "description": "fail"}`

	inv, err := tl.Prepare(ctx, json.RawMessage(params))
	require.NoError(t, err)

	// Consume output
	disp := inv.Display().(tool.ShellDisplay)
	go io.Copy(io.Discard, disp.Output)

	output, err := inv.Execute(ctx)
	require.NoError(t, err) // No error for non-zero exit
	assert.Contains(t, output, "(Exit code: 1)")
}

func TestShellTool_Execute_Truncation(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Tools.DefaultMaxCommandOutputSize = 10 // Very small to trigger truncation
	realCE := executor.NewOSCommandExecutor(cfg)

	mockPR := new(mockPathResolver)
	mockEnv := new(mockEnvFileOps)

	tl := NewShellTool(mockEnv, realCE, cfg, mockPR)

	mockPR.On("Abs", ".").Return("/tmp", nil)
	mockPR.On("Rel", "/tmp").Return(".", nil)

	ctx := context.Background()
	// Generate output longer than 10 bytes
	params := `{"command": ["echo", "this is a very long output that will be truncated"], "description": "long"}`

	inv, err := tl.Prepare(ctx, json.RawMessage(params))
	require.NoError(t, err)

	// Consume output
	disp := inv.Display().(tool.ShellDisplay)
	go io.Copy(io.Discard, disp.Output)

	output, err := inv.Execute(ctx)
	require.NoError(t, err)
	assert.Contains(t, output, "(Output truncated)")
}

func TestShellTool_Display_Wait(t *testing.T) {
	cfg := config.DefaultConfig()
	realCE := executor.NewOSCommandExecutor(cfg)

	mockPR := new(mockPathResolver)
	mockEnv := new(mockEnvFileOps)

	tl := NewShellTool(mockEnv, realCE, cfg, mockPR)

	mockPR.On("Abs", ".").Return("/tmp", nil)
	mockPR.On("Rel", "/tmp").Return(".", nil)

	ctx := context.Background()
	params := `{"command": ["sleep", "0.3"], "description": "wait"}`

	inv, err := tl.Prepare(ctx, json.RawMessage(params))
	require.NoError(t, err)

	disp := inv.Display().(tool.ShellDisplay)

	// Wait should block initially
	done := make(chan struct{})
	go func() {
		disp.Wait()
		close(done)
	}()

	select {
	case <-done:
		t.Fatal("Wait should have blocked")
	case <-time.After(100 * time.Millisecond):
		// expected to block
	}

	// Consume output
	go io.Copy(io.Discard, disp.Output)

	// Execute unblocks Wait
	_, _ = inv.Execute(ctx)

	select {
	case <-done:
		// success
	case <-time.After(1 * time.Second):
		t.Fatal("Wait should have unblocked")
	}
}

func TestShellTool_Display_StreamingOutput(t *testing.T) {
	cfg := config.DefaultConfig()
	realCE := executor.NewOSCommandExecutor(cfg)

	mockPR := new(mockPathResolver)
	mockEnv := new(mockEnvFileOps)

	tl := NewShellTool(mockEnv, realCE, cfg, mockPR)

	mockPR.On("Abs", ".").Return("/tmp", nil)
	mockPR.On("Rel", "/tmp").Return(".", nil)

	ctx := context.Background()
	params := `{"command": ["echo", "streaming_test"], "description": "stream"}`

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
