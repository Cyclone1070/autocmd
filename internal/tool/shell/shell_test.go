package shell

import (
	"context"
	"encoding/json"
	"io"
	"os"
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

// realPathResolver uses the actual file system for tests
type realPathResolver struct {
	baseDir string
}

func (r *realPathResolver) Abs(path string) (string, error) {
	if path == "." || path == "" {
		return r.baseDir, nil
	}
	return path, nil
}

func (r *realPathResolver) Rel(path string) (string, error) {
	return ".", nil
}

// consumeOutput reads the output stream from the display to prevent deadlock if IO pipes are used.
func consumeOutput(inv tool.Invocation) {
	if d, ok := inv.Display().(tool.ShellDisplay); ok {
		go io.Copy(io.Discard, d.Output)
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

// Integration tests use real OSCommandExecutor
func TestShellTool_Prepare_Success(t *testing.T) {
	cfg := config.DefaultConfig()
	cwd, _ := os.Getwd()
	realPR := &realPathResolver{baseDir: cwd}
	realCE := executor.NewOSCommandExecutor(cfg)

	tl := NewShellTool(&mockEnvFileOps{}, realCE, cfg, realPR)

	ctx := context.Background()
	params := `{"command": ["echo", "hello"], "description": "say hello"}`

	inv, err := tl.Prepare(ctx, json.RawMessage(params))
	require.NoError(t, err)
	require.NotNil(t, inv)

	// Consume output to prevent blocking
	consumeOutput(inv)

	// Verify Display
	disp := inv.Display()
	shellDisp, ok := disp.(tool.ShellDisplay)
	require.True(t, ok)
	assert.Equal(t, "[echo hello]", shellDisp.Command)
	assert.Equal(t, "say hello", shellDisp.Description)
	assert.Equal(t, ".", shellDisp.WorkingDir)
	assert.NotNil(t, shellDisp.Output)
	assert.NotNil(t, shellDisp.Wait)

	// Clean up: wait for command to finish
	_, _ = inv.Execute(ctx)
}

func TestShellTool_Display_Wait(t *testing.T) {
	cfg := config.DefaultConfig()
	cwd, _ := os.Getwd()
	realPR := &realPathResolver{baseDir: cwd}
	realCE := executor.NewOSCommandExecutor(cfg)

	tl := NewShellTool(&mockEnvFileOps{}, realCE, cfg, realPR)

	ctx := context.Background()
	params := `{"command": ["sleep", "0.5"], "description": "wait"}`

	inv, err := tl.Prepare(ctx, json.RawMessage(params))
	require.NoError(t, err)

	disp := inv.Display().(tool.ShellDisplay)

	// Wait should block initially
	timeout := time.After(100 * time.Millisecond)
	done := make(chan struct{})
	go func() {
		disp.Wait()
		close(done)
	}()

	select {
	case <-done:
		t.Fatal("Wait should have blocked")
	case <-timeout:
		// expected to block
	}

	// Consume output in background to allow execution to proceed
	go io.Copy(io.Discard, disp.Output)

	// Execute should unblock Wait
	_, err = inv.Execute(ctx)
	require.NoError(t, err)

	select {
	case <-done:
		// success - Wait was called from the goroutine or Execute completed
	case <-time.After(1 * time.Second):
		t.Fatal("Wait should have unblocked after Execute")
	}
}

func TestShellTool_Execute_Echo(t *testing.T) {
	cfg := config.DefaultConfig()
	cwd, _ := os.Getwd()
	realPR := &realPathResolver{baseDir: cwd}
	realCE := executor.NewOSCommandExecutor(cfg)

	tl := NewShellTool(&mockEnvFileOps{}, realCE, cfg, realPR)

	ctx := context.Background()
	params := `{"command": ["echo", "test_output"], "description": "test"}`

	inv, err := tl.Prepare(ctx, json.RawMessage(params))
	require.NoError(t, err)

	// Stream reader check
	disp := inv.Display().(tool.ShellDisplay)

	// Start a goroutine to read streaming output
	var streamContent []byte
	done := make(chan struct{})
	go func() {
		streamContent, _ = io.ReadAll(disp.Output)
		close(done)
	}()

	output, err := inv.Execute(ctx)
	require.NoError(t, err)

	<-done // Wait for stream to finish

	expected := "test_output"
	// Check streaming output
	assert.Contains(t, string(streamContent), expected)

	// Check returned output
	assert.Contains(t, output, expected)
	assert.Contains(t, output, "(Exit code: 0)")
}

func TestShellTool_Execute_Failure(t *testing.T) {
	cfg := config.DefaultConfig()
	cwd, _ := os.Getwd()
	realPR := &realPathResolver{baseDir: cwd}
	realCE := executor.NewOSCommandExecutor(cfg)

	tl := NewShellTool(&mockEnvFileOps{}, realCE, cfg, realPR)

	ctx := context.Background()
	params := `{"command": ["ls", "non_existent_file_xyz"], "description": "fail"}`

	inv, err := tl.Prepare(ctx, json.RawMessage(params))
	require.NoError(t, err)

	consumeOutput(inv) // Prevent deadlock

	output, err := inv.Execute(ctx)
	// Execute returns correct output string with exit code, error should be nil for command failure
	require.NoError(t, err)
	assert.Contains(t, output, "No such file") // Standard ls error
	assert.Contains(t, output, "(Exit code:")
	assert.NotContains(t, output, "(Exit code: 0)")
}

func TestShellTool_Execute_Timeout(t *testing.T) {
	cfg := config.DefaultConfig()
	cwd, _ := os.Getwd()
	realPR := &realPathResolver{baseDir: cwd}
	realCE := executor.NewOSCommandExecutor(cfg)

	tl := NewShellTool(&mockEnvFileOps{}, realCE, cfg, realPR)

	// Sleep for 2s, timeout 1s
	ctx := context.Background()
	params := `{"command": ["sleep", "2"], "timeout_seconds": 1, "description": "sleep"}`

	inv, err := tl.Prepare(ctx, json.RawMessage(params))
	require.NoError(t, err)

	consumeOutput(inv) // Prevent deadlock

	start := time.Now()
	output, err := inv.Execute(ctx)
	elapsed := time.Since(start)

	assert.ErrorIs(t, err, executor.ErrTimeout)
	assert.Contains(t, output, "(Command timed out)")
	assert.GreaterOrEqual(t, elapsed.Milliseconds(), int64(900))
}
