package bash

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Cyclone1070/autocmd/internal/domain"
	"github.com/Cyclone1070/autocmd/internal/tool/service/executor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type mockFileSystem struct {
	mock.Mock
}

func (m *mockFileSystem) Open(path string) (domain.File, error) {
	args := m.Called(path)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(domain.File), args.Error(1)
}

func (m *mockFileSystem) CreateAtomic(path string) (io.WriteCloser, error) {
	args := m.Called(path)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(io.WriteCloser), args.Error(1)
}

type mockFile struct {
	mock.Mock
}

func (m *mockFile) Read(p []byte) (n int, err error) {
	args := m.Called(p)
	return args.Int(0), args.Error(1)
}

func (m *mockFile) Seek(offset int64, whence int) (int64, error) {
	args := m.Called(offset, whence)
	return args.Get(0).(int64), args.Error(1)
}

func (m *mockFile) Close() error {
	args := m.Called()
	return args.Error(0)
}

func (m *mockFile) Stat() (os.FileInfo, error) {
	args := m.Called()
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(os.FileInfo), args.Error(1)
}

type mockFileInfo struct {
	os.FileInfo
	size int64
}

func (m *mockFileInfo) Size() int64 { return m.size }

type mockExecutor struct {
	mock.Mock
}

func (m *mockExecutor) RunStreaming(ctx context.Context, command string, dir string, enableLogging bool) (*executor.StreamingCmd, error) {
	args := m.Called(ctx, command, dir, enableLogging)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*executor.StreamingCmd), args.Error(1)
}

type mockTaskManager struct {
	mock.Mock
}

type captureEventSender struct {
	updates []domain.UIUpdate
	mu      sync.Mutex
}

func (c *captureEventSender) SendUIUpdate(u domain.UIUpdate) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.updates = append(c.updates, u)
}

func (m *mockTaskManager) Register(id string, cmd *executor.StreamingCmd, logPath string, cancel context.CancelFunc, description, command string, cwd string) error {
	args := m.Called(id, cmd, logPath, cancel, description, command, cwd)
	return args.Error(0)
}

func TestBashTool_Validate_AllowedCommands(t *testing.T) {
	mockFS := &mockFileSystem{}
	mockExec := &mockExecutor{}
	mockResolver := &mockPathResolver{root: "/workspace"}
	tool := NewTool(mockFS, mockExec, mockResolver, nil)

	allowed := []string{
		"ssh user@host 'ls'",
		"scp file user@host:/path",
		"echo hi | less",
		"top -n 1; echo done",
		"watch -n 1 date",
		"htop",
		"telnet host 80",
		"ftp host",
		"vim",
		"ls && vim",
		"ls &&vim",
		"git commit && nvim",
		"nano -w file",
		"tmux new -s test",
		"git status | grep 'modified'",
		"cat file | grep 'pattern'",
		"cd dir; grep foo bar",
		"find . -name '*.go' | xargs grep foo",
	}
	for _, cmd := range allowed {
		t.Run(cmd, func(t *testing.T) {
			params := fmt.Sprintf(`{"command": "%s", "description": "test"}`, cmd)
			_, err := tool.validate(params)
			if err != nil {
				t.Errorf("Expected command %q to be allowed, but it was blocked: %v", cmd, err)
			}
		})
	}
}

func TestBashTool_Validate_BlockedCommands(t *testing.T) {
	mockFS := &mockFileSystem{}
	mockExec := &mockExecutor{}
	mockResolver := &mockPathResolver{root: "/workspace"}
	tool := NewTool(mockFS, mockExec, mockResolver, nil)

	blocked := map[string]string{
		"find .":            "glob",
		"fd foo":            "glob",
		"grep foo file":     "grep",
		"rg foo":            "grep",
		"ag foo":            "grep",
		"ack foo":           "grep",
		"cat file":          "read_file",
		"head -n 10 file":   "read_file",
		"tail -f file":      "read_file",
		"/usr/bin/cat file": "read_file",
		"./bin/rg pattern":  "grep",
		"find":              "glob",
		" grep":             "grep",
	}

	for cmd, expectedTool := range blocked {
		t.Run(cmd, func(t *testing.T) {
			params := fmt.Sprintf(`{"command": "%s", "description": "test"}`, cmd)
			_, err := tool.validate(params)
			if err == nil {
				t.Errorf("Expected command %q to be blocked, but it was allowed", cmd)
			} else {
				assert.Contains(t, err.Error(), fmt.Sprintf("use the %q tool instead", expectedTool))
			}
		})
	}
}

func (m *mockTaskManager) List() []TaskInfo {
	args := m.Called()
	return args.Get(0).([]TaskInfo)
}

func (m *mockTaskManager) Stop(id string) error {
	args := m.Called(id)
	return args.Error(0)
}

func (m *mockTaskManager) StopAll() {
	m.Called()
}

func (m *mockTaskManager) NotifyChan() <-chan struct{} {
	args := m.Called()
	return args.Get(0).(<-chan struct{})
}

func (m *mockTaskManager) Drain() []domain.TaskResult {
	args := m.Called()
	return args.Get(0).([]domain.TaskResult)
}

type mockPathResolver struct {
	root string
}

func (m *mockPathResolver) Root() string {
	return m.root
}

func (m *mockPathResolver) DisplayPath(path string) string {
	return path
}

func TestBashTool_Execute(t *testing.T) {
	fs := &mockFileSystem{}
	exec := &mockExecutor{}
	resolver := &mockPathResolver{root: "/tmp"}
	tool := NewTool(fs, exec, resolver, nil)

	params := `{"command": "echo test", "description": "test command"}`
	req, err := tool.validate(params)
	assert.NoError(t, err)

	// Mock streaming command
	output := strings.NewReader("test output")
	waitFn := func() (*executor.Result, error) {
		return &executor.Result{Stdout: "test output", ExitCode: 0}, nil
	}
	streamCmd := executor.NewStreamingCmd("t1", output, waitFn, "/tmp/test.log")

	exec.On("RunStreaming", mock.Anything, mock.Anything, "/tmp", true).Return(streamCmd, nil)

	ctx := context.Background()
	llmContent, display := tool.executeBash(ctx, req, nil, "")

	assert.Contains(t, llmContent, "test output")
	assert.Contains(t, llmContent, "<cwd>/tmp</cwd>")
	assert.Equal(t, "test output", display.(domain.BashDisplay).CapturedOutput)
	assert.Equal(t, "test command", display.(domain.BashDisplay).Description)
	assert.Equal(t, "echo test", display.(domain.BashDisplay).Command)
	assert.Equal(t, "/tmp", display.(domain.BashDisplay).Cwd)
}

func TestBashTool_Stream(t *testing.T) {
	fs := &mockFileSystem{}
	exec := &mockExecutor{}
	resolver := &mockPathResolver{root: "/tmp"}
	tool := NewTool(fs, exec, resolver, nil)

	params := `{"command": "echo test", "description": "test stream"}`
	req, _ := tool.validate(params)

	// Start execution in background
	output := strings.NewReader("real time output")
	waitDone := make(chan struct{})
	waitFn := func() (*executor.Result, error) {
		<-waitDone // Wait until we explicitly signal we are done reading
		return &executor.Result{Stdout: "full output", ExitCode: 0}, nil
	}
	streamCmd := executor.NewStreamingCmd("t1", output, waitFn, "")
	exec.On("RunStreaming", mock.Anything, mock.Anything, "/tmp", true).Return(streamCmd, nil)

	events := &captureEventSender{}
	go tool.executeBash(context.Background(), req, events, "call-1")
	time.Sleep(20 * time.Millisecond)

	close(waitDone) // Now allow Execute to finish
	time.Sleep(20 * time.Millisecond)
	events.mu.Lock()
	defer events.mu.Unlock()
	assert.NotEmpty(t, events.updates)
	var combined strings.Builder
	for _, update := range events.updates {
		if streamEvent, ok := update.(domain.ToolStreamEvent); ok {
			combined.WriteString(streamEvent.Chunk)
		}
	}
	assert.Contains(t, combined.String(), "real time output")
}

func TestBashTool_Execute_AlignWithClaudeCode(t *testing.T) {
	t.Run("promotes to background on timeout without killing the process", func(t *testing.T) {
		mockFS := &mockFileSystem{}
		mockExec := &mockExecutor{}
		mockTM := &syncTM{
			done:           make(chan struct{}),
			registeredCmds: make(map[string]*executor.StreamingCmd),
		}
		mockResolver := &mockPathResolver{root: "/workspace"}

		tool := NewTool(mockFS, mockExec, mockResolver, mockTM)

		req, _ := tool.validate(`{"command": "long_running", "description": "test", "timeout": 100}`)

		mockExec.On("RunStreaming", mock.Anything, "long_running", "/workspace", true).Return(
			executor.NewStreamingCmd("task1", strings.NewReader("some output"), func() (*executor.Result, error) {
				time.Sleep(150 * time.Millisecond) // Longer than 100ms timeout
				return &executor.Result{Stdout: "done", ExitCode: 0}, nil
			}, "/tmp/task1.log"),
			nil,
		)

		// No mock expectation for Register here, handled by manual mock

		start := time.Now()
		resp, display := tool.executeBash(context.Background(), req, nil, "")
		duration := time.Since(start)

		assert.Contains(t, resp, "command ran in the background")
		assert.Contains(t, resp, "<cwd>/workspace</cwd>")
		bd := display.(domain.BashDisplay)
		assert.Contains(t, bd.CapturedOutput, "(Command ran in the background")
		assert.Equal(t, "/workspace", bd.Cwd)
		assert.Less(t, duration, 150*time.Millisecond)

		<-mockTM.done // Ensure background goroutine finishes Wait() and sync.Once Unlock
		mockExec.AssertExpectations(t)
		mockTM.AssertExpectations(t)

		mockTM.mu.Lock()
		assert.Contains(t, mockTM.registeredCmds, "task1")
		mockTM.mu.Unlock()
	})
}

type syncTM struct {
	done           chan struct{}
	registeredCmds map[string]*executor.StreamingCmd
	mockTaskManager
	mu sync.Mutex
}

func (m *syncTM) Register(id string, cmd *executor.StreamingCmd, _ string, _ context.CancelFunc, _, _ string, _ string) error {
	m.mu.Lock()
	m.registeredCmds[id] = cmd
	m.mu.Unlock()

	go func() {
		_, _ = cmd.Wait()
		close(m.done)
	}()
	return nil
}

type testStubTaskManager struct {
	mockTaskManager
	capturedCmd *executor.StreamingCmd
}

func (s *testStubTaskManager) Register(_ string, cmd *executor.StreamingCmd, _ string, _ context.CancelFunc, _, _ string, _ string) error {
	s.capturedCmd = cmd
	return nil
}

func TestBashTool_ZeroForegroundTimeout(t *testing.T) {
	fs := &mockFileSystem{}
	exec := &mockExecutor{}
	resolver := &mockPathResolver{root: "/tmp"}
	taskMgr := &testStubTaskManager{}

	// Tool with 0 foreground timeout
	tool := NewTool(fs, exec, resolver, taskMgr)

	params := `{"command": "long_command", "description": "test backgrounding"}`
	req, _ := tool.validate(params)

	waitCh := make(chan struct{})
	waitFn := func() (*executor.Result, error) {
		<-waitCh // Block until we say
		return &executor.Result{Stdout: "done", ExitCode: 0}, nil
	}
	output := strings.NewReader("some output")
	streamCmd := executor.NewStreamingCmd("t1", output, waitFn, "/tmp/log")

	exec.On("RunStreaming", mock.Anything, mock.Anything, "/tmp", true).Return(streamCmd, nil)

	ctx := context.Background()
	llmContent, display := tool.executeBash(ctx, req, nil, "")

	// Should have backgrounded immediately
	assert.Contains(t, llmContent, "<background_task_id>")
	assert.Contains(t, llmContent, "<cwd>/tmp</cwd>")
	assert.Contains(t, display.(domain.BashDisplay).CapturedOutput, "(Command ran in the background")
	assert.Equal(t, "/tmp", display.(domain.BashDisplay).Cwd)

	// Verify that command was registered
	assert.NotNil(t, taskMgr.capturedCmd, "Command should be registered")

	exec.AssertExpectations(t)
	close(waitCh)                     // Cleanup
	time.Sleep(10 * time.Millisecond) // Give the goroutine a moment to finish
}

func TestBashTool_IsConcurrentSafe(t *testing.T) {
	tool := NewTool(&mockFileSystem{}, &mockExecutor{}, &mockPathResolver{}, nil)
	assert.True(t, tool.IsConcurrentSafe())
}

func TestBashTool_Validate_EmptyDescription(t *testing.T) {
	fs := &mockFileSystem{}
	exec := &mockExecutor{}
	resolver := &mockPathResolver{root: "/tmp"}
	tool := NewTool(fs, exec, resolver, nil)

	params := `{"command": "ls"}`
	_, err := tool.validate(params)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "description is required")
}

func TestBashTool_DeadlineExceeded_TreatedAsFailure(t *testing.T) {
	fs := &mockFileSystem{}
	exec := &mockExecutor{}
	resolver := &mockPathResolver{root: "/tmp"}

	// foreground timeout 1s, but we'll trigger a hard timeout via context or timeoutMS earlier (e.g. 100ms)
	tool := NewTool(fs, exec, resolver, nil)

	params := `{"command": "sleep 10", "description": "testing hard timeout", "timeout": 100}`
	req, _ := tool.validate(params)

	output := strings.NewReader("some partial output")
	waitFn := func() (*executor.Result, error) {
		return &executor.Result{Stdout: "some partial output", ExitCode: -1}, context.DeadlineExceeded
	}
	streamCmd := executor.NewStreamingCmd("t1", output, waitFn, "/tmp/log")

	exec.On("RunStreaming", mock.Anything, mock.Anything, "/tmp", true).Return(streamCmd, nil)

	ctx := context.Background()
	llmContent, display := tool.executeBash(ctx, req, nil, "")

	assert.Contains(t, llmContent, "Error: command failed:")
	assert.Contains(t, llmContent, "<cwd>/tmp</cwd>")
	assert.Equal(t, domain.ToolErrorFailed, display.GetError())
}

func TestBashTool_Cancellation(t *testing.T) {
	fs := &mockFileSystem{}
	exec := &mockExecutor{}
	resolver := &mockPathResolver{root: "/tmp"}
	tool := NewTool(fs, exec, resolver, nil)

	params := `{"command": "ls", "description": "testing cancellation"}`
	req, _ := tool.validate(params)

	output := strings.NewReader("")
	waitFn := func() (*executor.Result, error) {
		return &executor.Result{ExitCode: -1}, context.Canceled
	}
	streamCmd := executor.NewStreamingCmd("t1", output, waitFn, "")

	exec.On("RunStreaming", mock.Anything, mock.Anything, "/tmp", true).Return(streamCmd, nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	llmContent, display := tool.executeBash(ctx, req, nil, "")

	assert.Equal(t, domain.ToolErrorCancelled, llmContent)
	assert.Equal(t, domain.ToolErrorCancelled, display.GetError())
}

func TestBashTool_LargeOutput(t *testing.T) {
	fs := &mockFileSystem{}
	exec := &mockExecutor{}
	resolver := &mockPathResolver{root: "/tmp"}
	tool := NewTool(fs, exec, resolver, nil)

	params := `{"command": "large_command", "description": "testing large output"}`
	req, _ := tool.validate(params)

	output := strings.NewReader("")
	waitFn := func() (*executor.Result, error) {
		return &executor.Result{Stdout: "", LogPath: "/tmp/large.log", ExitCode: 0}, nil
	}
	streamCmd := executor.NewStreamingCmd("t1", output, waitFn, "/tmp/large.log")

	// Mock file for tail read
	mFile := &mockFile{}
	mFileInfo := &mockFileInfo{size: 3000}
	mFile.On("Stat").Return(mFileInfo, nil)
	mFile.On("Seek", int64(3000-2048), io.SeekStart).Return(int64(3000-2048), nil)
	mFile.On("Read", mock.Anything).Return(10, io.EOF).Run(func(args mock.Arguments) {
		buf := args.Get(0).([]byte)
		copy(buf, "tail data")
	})
	mFile.On("Close").Return(nil)
	fs.On("Open", "/tmp/large.log").Return(mFile, nil)

	exec.On("RunStreaming", mock.Anything, mock.Anything, "/tmp", true).Return(streamCmd, nil)

	ctx := context.Background()
	llmContent, display := tool.executeBash(ctx, req, nil, "")

	assert.Contains(t, llmContent, "too large")
	assert.Contains(t, llmContent, "tail data")
	assert.Equal(t, "(Output too large, saved to /tmp/large.log)", display.(domain.BashDisplay).CapturedOutput)
}

func TestBashTool_DeadlineExceeded_LargeOutput_TreatedAsFailure(t *testing.T) {
	fs := &mockFileSystem{}
	exec := &mockExecutor{}
	resolver := &mockPathResolver{root: "/tmp"}
	tool := NewTool(fs, exec, resolver, nil)

	params := `{"command": "sleep 10", "description": "testing timeout + large", "timeout": 100}`
	req, _ := tool.validate(params)

	output := strings.NewReader("")
	waitFn := func() (*executor.Result, error) {
		return &executor.Result{Stdout: "", LogPath: "/tmp/large.log", ExitCode: -1}, context.DeadlineExceeded
	}
	streamCmd := executor.NewStreamingCmd("t1", output, waitFn, "/tmp/large.log")

	// Mock file for tail read
	mFile := &mockFile{}
	mFileInfo := &mockFileInfo{size: 100}
	mFile.On("Stat").Return(mFileInfo, nil)
	mFile.On("Seek", int64(0), io.SeekStart).Return(int64(0), nil)
	mFile.On("Read", mock.Anything).Return(10, io.EOF).Run(func(args mock.Arguments) {
		buf := args.Get(0).([]byte)
		copy(buf, "small tail")
	})
	mFile.On("Close").Return(nil)
	fs.On("Open", "/tmp/large.log").Return(mFile, nil)

	exec.On("RunStreaming", mock.Anything, mock.Anything, "/tmp", true).Return(streamCmd, nil)

	ctx := context.Background()
	llmContent, display := tool.executeBash(ctx, req, nil, "")

	assert.Contains(t, llmContent, "Error: command failed:")
	assert.Contains(t, llmContent, "too large")
	assert.Contains(t, llmContent, "small tail")
	assert.Contains(t, llmContent, "<cwd>/tmp</cwd>")
	assert.Equal(t, "(Output too large, saved to /tmp/large.log)", display.(domain.BashDisplay).CapturedOutput)
	assert.Equal(t, domain.ToolErrorFailed, display.GetError())
}

func TestBashTool_NonZeroExit_SuccessUI(t *testing.T) {
	fs := &mockFileSystem{}
	exec := &mockExecutor{}
	resolver := &mockPathResolver{root: "/tmp"}
	tool := NewTool(fs, exec, resolver, nil)

	params := `{"command": "false", "description": "testing non-zero exit"}`
	req, _ := tool.validate(params)

	output := strings.NewReader("some error output")
	waitFn := func() (*executor.Result, error) {
		return &executor.Result{Stdout: "some error output", ExitCode: 1}, nil
	}
	streamCmd := executor.NewStreamingCmd("t1", output, waitFn, "")

	exec.On("RunStreaming", mock.Anything, mock.Anything, "/tmp", true).Return(streamCmd, nil)

	ctx := context.Background()
	llmContent, display := tool.executeBash(ctx, req, nil, "")

	assert.Contains(t, llmContent, "some error output")
	assert.Contains(t, llmContent, "<exit_code>1</exit_code>")
	assert.Empty(t, display.GetError(), "Display should NOT have an error for non-zero exit codes")
}
