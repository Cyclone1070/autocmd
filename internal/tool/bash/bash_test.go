package bash

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/tool/service/executor"
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

func (m *mockTaskManager) Register(id string, cmd *executor.StreamingCmd, logPath string, cancel context.CancelFunc, description, command string) error {
	args := m.Called(id, cmd, logPath, cancel, description, command)
	return args.Error(0)
}

func TestBashTool_Prepare_BlockedCommand(t *testing.T) {
	mockFS := &mockFileSystem{}
	mockExec := &mockExecutor{}
	mockResolver := &mockPathResolver{root: "/workspace"}
	tool := NewBashTool(mockFS, mockExec, mockResolver, nil)

	forbidden := []string{
		"vim",
		"ls && vim",
		"ls &&vim",
		"echo hi | less",
		"top -n 1; echo done",
		"git commit && nvim",
	}
	for _, cmd := range forbidden {
		t.Run(cmd, func(t *testing.T) {
			params := fmt.Sprintf(`{"command": "%s", "comment": "test"}`, cmd)
			_, err := tool.Prepare(params)
			if err == nil {
				t.Errorf("Expected command %q to be blocked, but it was accepted", cmd)
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

func (m *mockTaskManager) NotifyChan() <-chan struct{} {
	args := m.Called()
	return args.Get(0).(<-chan struct{})
}

func (m *mockTaskManager) Drain() []string {
	args := m.Called()
	return args.Get(0).([]string)
}

type mockPathResolver struct {
	root string
}

func (m *mockPathResolver) Root() string {
	return m.root
}

func TestBashTool_Execute(t *testing.T) {
	fs := &mockFileSystem{}
	exec := &mockExecutor{}
	resolver := &mockPathResolver{root: "/tmp"}
	tool := NewBashTool(fs, exec, resolver, nil)

	params := `{"command": "echo test", "comment": "test command"}`
	inv, err := tool.Prepare(params)
	assert.NoError(t, err)

	// Mock streaming command
	output := strings.NewReader("test output")
	waitFn := func() (*executor.Result, error) {
		return &executor.Result{Stdout: "test output", ExitCode: 0}, nil
	}
	streamCmd := executor.NewStreamingCmd("t1", output, waitFn, "/tmp/test.log")

	exec.On("RunStreaming", mock.Anything, mock.Anything, "/tmp", true).Return(streamCmd, nil)

	ctx := context.Background()
	llmContent, display := inv.(domain.ExecutableInvocation).Execute(ctx)

	assert.Equal(t, "test output", llmContent)
	assert.Equal(t, "test output", display.(domain.BashDisplay).CapturedOutput)
	assert.Equal(t, "test command", display.(domain.BashDisplay).Comment)
	assert.Equal(t, "echo test", display.(domain.BashDisplay).Command)
}

func TestBashTool_Stream(t *testing.T) {
	fs := &mockFileSystem{}
	exec := &mockExecutor{}
	resolver := &mockPathResolver{root: "/tmp"}
	tool := NewBashTool(fs, exec, resolver, nil)

	params := `{"command": "echo test", "comment": "test stream"}`
	inv, _ := tool.Prepare(params)
	
	// Stream should be available BEFORE Execute
	reader := inv.(domain.StreamableInvocation).Stream()
	assert.NotNil(t, reader)

	// Start execution in background
	output := strings.NewReader("real time output")
	waitDone := make(chan struct{})
	waitFn := func() (*executor.Result, error) {
		<-waitDone // Wait until we explicitly signal we are done reading
		return &executor.Result{Stdout: "full output", ExitCode: 0}, nil
	}
	streamCmd := executor.NewStreamingCmd("t1", output, waitFn, "")
	exec.On("RunStreaming", mock.Anything, mock.Anything, "/tmp", true).Return(streamCmd, nil)

	go inv.(domain.ExecutableInvocation).Execute(context.Background())

	// Data should flow through the proxy reader
	buf := make([]byte, 100)
	n, err := reader.Read(buf)
	assert.NoError(t, err)
	assert.Equal(t, "real time output", string(buf[:n]))

	close(waitDone) // Now allow Execute to finish
}

func TestBashTool_Execute_AlignWithClaudeCode(t *testing.T) {
	t.Run("promotes to background on timeout without killing the process", func(t *testing.T) {
		mockFS := &mockFileSystem{}
		mockExec := &mockExecutor{}
		mockTM := &mockTaskManager{}
		mockResolver := &mockPathResolver{root: "/workspace"}

		tool := NewBashTool(mockFS, mockExec, mockResolver, mockTM)

		inv, _ := tool.Prepare(`{"command": "long_running", "comment": "test", "timeout": 100}`)

		mockExec.On("RunStreaming", mock.Anything, "long_running", "/workspace", true).Return(
			executor.NewStreamingCmd("task1", strings.NewReader("some output"), func() (*executor.Result, error) {
				time.Sleep(200 * time.Millisecond) // Longer than 100ms timeout
				return &executor.Result{Stdout: "done", ExitCode: 0}, nil
			}, "/tmp/task1.log"),
			nil,
		)

		mockTM.On("Register", "task1", mock.Anything, "/tmp/task1.log", mock.Anything, "test", "long_running").Return(nil)

		start := time.Now()
		resp, display := inv.(*bashInvocation).Execute(context.Background())
		duration := time.Since(start)

		assert.Contains(t, resp, "the command is running in the background")
		bd := display.(domain.BashDisplay)
		assert.Equal(t, "(command running in background)", bd.CapturedOutput)
		assert.Less(t, duration, 150*time.Millisecond)
		
		mockExec.AssertExpectations(t)
		mockTM.AssertExpectations(t)
	})
}

type testStubTaskManager struct {
	mockTaskManager
}

func (s *testStubTaskManager) Register(id string, cmd *executor.StreamingCmd, logPath string, cancel context.CancelFunc, description, command string) error {
	return nil
}

func TestBashTool_ZeroForegroundTimeout(t *testing.T) {
	fs := &mockFileSystem{}
	exec := &mockExecutor{}
	resolver := &mockPathResolver{root: "/tmp"}
	taskMgr := &testStubTaskManager{}
	
	// Tool with 0 foreground timeout
	tool := NewBashTool(fs, exec, resolver, taskMgr)

	params := `{"command": "long_command", "comment": "test backgrounding"}`
	inv, _ := tool.Prepare(params)

	waitCh := make(chan struct{})
	waitFn := func() (*executor.Result, error) {
		<-waitCh // Block until we say
		return &executor.Result{Stdout: "done", ExitCode: 0}, nil
	}
	output := strings.NewReader("some output")
	streamCmd := executor.NewStreamingCmd("t1", output, waitFn, "/tmp/log")

	exec.On("RunStreaming", mock.Anything, mock.Anything, "/tmp", true).Return(streamCmd, nil)

	ctx := context.Background()
	llmContent, display := inv.(domain.ExecutableInvocation).Execute(ctx)

	// Should have backgrounded immediately
	assert.Contains(t, llmContent, "<background_task_id>")
	assert.Contains(t, display.(domain.BashDisplay).CapturedOutput, "(command running in background)")
	
	close(waitCh) // Cleanup
}

func TestBashTool_IsConcurrentSafe(t *testing.T) {
	tool := NewBashTool(&mockFileSystem{}, &mockExecutor{}, &mockPathResolver{}, nil)
	assert.True(t, tool.IsConcurrentSafe())
}

func TestBashTool_Prepare_EmptyComment(t *testing.T) {
	fs := &mockFileSystem{}
	exec := &mockExecutor{}
	resolver := &mockPathResolver{root: "/tmp"}
	tool := NewBashTool(fs, exec, resolver, nil)

	params := `{"command": "ls"}`
	_, err := tool.Prepare(params)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "comment is required")
}
func TestBashTool_HardTimeout(t *testing.T) {
	fs := &mockFileSystem{}
	exec := &mockExecutor{}
	resolver := &mockPathResolver{root: "/tmp"}
	
	// foreground timeout 1s, but we'll trigger a hard timeout via context or timeoutMS earlier (e.g. 100ms)
	tool := NewBashTool(fs, exec, resolver, nil)

	params := `{"command": "sleep 10", "comment": "testing hard timeout", "timeout": 100}`
	inv, _ := tool.Prepare(params)

	output := strings.NewReader("some partial output")
	waitFn := func() (*executor.Result, error) {
		return &executor.Result{Stdout: "some partial output", ExitCode: -1}, context.DeadlineExceeded
	}
	streamCmd := executor.NewStreamingCmd("t1", output, waitFn, "/tmp/log")

	exec.On("RunStreaming", mock.Anything, mock.Anything, "/tmp", true).Return(streamCmd, nil)

	ctx := context.Background()
	llmContent, display := inv.(domain.ExecutableInvocation).Execute(ctx)

	assert.Contains(t, llmContent, "<execution_status>")
	assert.Contains(t, llmContent, "<timedout>true</timedout>")
	assert.Equal(t, domain.ToolErrorTimedOut, display.GetError())
}

func TestBashTool_Cancellation(t *testing.T) {
	fs := &mockFileSystem{}
	exec := &mockExecutor{}
	resolver := &mockPathResolver{root: "/tmp"}
	tool := NewBashTool(fs, exec, resolver, nil)

	params := `{"command": "ls", "comment": "testing cancellation"}`
	inv, _ := tool.Prepare(params)

	output := strings.NewReader("")
	waitFn := func() (*executor.Result, error) {
		return &executor.Result{ExitCode: -1}, context.Canceled
	}
	streamCmd := executor.NewStreamingCmd("t1", output, waitFn, "")

	exec.On("RunStreaming", mock.Anything, mock.Anything, "/tmp", true).Return(streamCmd, nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	llmContent, display := inv.(domain.ExecutableInvocation).Execute(ctx)

	assert.Equal(t, domain.ToolErrorCancelled, llmContent)
	assert.Equal(t, domain.ToolErrorCancelled, display.GetError())
}

func TestBashTool_LargeOutput(t *testing.T) {
	fs := &mockFileSystem{}
	exec := &mockExecutor{}
	resolver := &mockPathResolver{root: "/tmp"}
	tool := NewBashTool(fs, exec, resolver, nil)

	params := `{"command": "large_command", "comment": "testing large output"}`
	inv, _ := tool.Prepare(params)

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
	llmContent, display := inv.(domain.ExecutableInvocation).Execute(ctx)

	assert.Contains(t, llmContent, "too large")
	assert.Contains(t, llmContent, "tail data")
	assert.Equal(t, "(Output too large, saved to /tmp/large.log)", display.(domain.BashDisplay).CapturedOutput)
}

func TestBashTool_HardTimeout_LargeOutput(t *testing.T) {
	fs := &mockFileSystem{}
	exec := &mockExecutor{}
	resolver := &mockPathResolver{root: "/tmp"}
	tool := NewBashTool(fs, exec, resolver, nil)

	params := `{"command": "sleep 10", "comment": "testing timeout + large", "timeout": 100}`
	inv, _ := tool.Prepare(params)

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
	llmContent, display := inv.(domain.ExecutableInvocation).Execute(ctx)

	assert.Contains(t, llmContent, "<execution_status>")
	assert.Contains(t, llmContent, "<timedout>true</timedout>")
	assert.Contains(t, llmContent, "too large")
	assert.Contains(t, llmContent, "small tail")
	assert.Equal(t, "(Output too large, saved to /tmp/large.log)", display.(domain.BashDisplay).CapturedOutput)
}






