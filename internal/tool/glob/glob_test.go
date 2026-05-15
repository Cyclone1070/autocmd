package glob

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/testutil"
	"github.com/Cyclone1070/iav/internal/tool/service/executor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestGlob_Definition(t *testing.T) {
	tool := NewTool(&mockFileSystem{}, &mockCommandExecutor{}, &mockPathResolver{})
	def, err := tool.Info(context.Background())
	require.NoError(t, err)

	assert.Equal(t, "glob", def.Name)
	assert.Contains(t, def.Desc, "Fast file pattern matching")
	assert.Contains(t, def.Desc, "Supports glob patterns")
}

func TestGlob_Basic(t *testing.T) {
	fs := &mockFileSystem{}
	exec := &mockCommandExecutor{}
	pathResolver := &mockPathResolver{}
	setupMockResolver(pathResolver)

	pathResolver.On("Abs", testutil.TestWorkspaceRoot).Return(testutil.TestWorkspaceRoot, nil).Maybe()

	fs.On("Stat", testutil.TestWorkspaceRoot).Return(&toolMockFileInfo{name: "workspace", isDir: true}, nil)

	output := "/workspace/a.go\n"
	exec.On("Run", mock.Anything,
		mock.MatchedBy(func(s string) bool {
			return strings.Contains(s, "rg") && strings.Contains(s, "*.go") && strings.Contains(s, testutil.TestWorkspaceRoot)
		}),
		testutil.TestWorkspaceRoot,
		true,
	).Return(&executor.Result{Stdout: output, ExitCode: 0}, nil)

	tool := NewTool(fs, exec, pathResolver)
	req := &Request{Pattern: "*.go", Path: testutil.TestWorkspaceRoot}

	result, err := executeFind(t, tool, req)
	assert.NoError(t, err)
	assert.Equal(t, "/workspace/a.go\n\n<exit_code>0</exit_code>", result)
}

func TestGlob_NoMatches(t *testing.T) {
	fs := &mockFileSystem{}
	exec := &mockCommandExecutor{}
	pathResolver := &mockPathResolver{}
	setupMockResolver(pathResolver)

	fs.On("Stat", testutil.TestWorkspaceRoot).Return(&toolMockFileInfo{name: "workspace", isDir: true}, nil)

	exec.On("Run", mock.Anything, mock.Anything, testutil.TestWorkspaceRoot, true).
		Return(&executor.Result{Stdout: "", ExitCode: 0}, nil)

	tool := NewTool(fs, exec, pathResolver)
	req := &Request{Pattern: "*.go", Path: testutil.TestWorkspaceRoot}

	result, err := executeFind(t, tool, req)
	assert.NoError(t, err)
	assert.Equal(t, "No files found\n\n<exit_code>0</exit_code>", result)
}

func TestGlob_Offloaded(t *testing.T) {
	fs := &mockFileSystem{}
	exec := &mockCommandExecutor{}
	pathResolver := &mockPathResolver{}
	setupMockResolver(pathResolver)

	fs.On("Stat", testutil.TestWorkspaceRoot).Return(&toolMockFileInfo{name: "workspace", isDir: true}, nil)

	// Simulate offloaded output
	exec.On("Run", mock.Anything, mock.Anything, testutil.TestWorkspaceRoot, true).
		Return(&executor.Result{
			Stdout:   "tail-output",
			ExitCode: 0,
			LogPath:  "/tmp/offloaded.log",
		}, nil)

	// Mock file for analyzer
	mf := &mockFile{}
	mf.On("Read", mock.Anything).Return(0, io.EOF)
	mf.On("Close").Return(nil)
	fs.On("Open", "/tmp/offloaded.log").Return(mf, nil)
	fs.On("Stat", "/tmp/offloaded.log").Return(&toolMockFileInfo{name: "offloaded.log"}, nil).Maybe()

	tool := NewTool(fs, exec, pathResolver)
	req := &Request{Pattern: "*.go", Path: testutil.TestWorkspaceRoot}

	result, err := executeFind(t, tool, req)
	assert.NoError(t, err)
	assert.Contains(t, result, "Output too large")
	assert.Contains(t, result, "read_file")
}

func TestGlob_ExitCode2_SuccessUI(t *testing.T) {
	fs := &mockFileSystem{}
	exec := &mockCommandExecutor{}
	pathResolver := &mockPathResolver{}
	setupMockResolver(pathResolver)

	fs.On("Stat", testutil.TestWorkspaceRoot).Return(&toolMockFileInfo{name: "workspace", isDir: true}, nil).Maybe()

	// Exit code 2 (e.g. ripgrep error) should NOT cause 'execution failed'
	exec.On("Run", mock.Anything, mock.Anything, testutil.TestWorkspaceRoot, true).
		Return(&executor.Result{Stdout: "ripgrep error", ExitCode: 2}, nil)

	tool := NewTool(fs, exec, pathResolver)
	req := &Request{Pattern: "*.go", Path: testutil.TestWorkspaceRoot}

	_, display := tool.executeGlob(context.Background(), &validatedRequest{pattern: req.Pattern, absPath: testutil.TestWorkspaceRoot})

	assert.Empty(t, display.GetError(), "Display should NOT have an error for exit code 2")
}

func TestGlob_ExecuteCancelled_ReturnsToolErrorCancelledDisplay(t *testing.T) {
	fs := &mockFileSystem{}
	exec := &mockCommandExecutor{}
	pathResolver := &mockPathResolver{}
	setupMockResolver(pathResolver)

	fs.On("Stat", testutil.TestWorkspaceRoot).Return(&toolMockFileInfo{name: "workspace", isDir: true}, nil)

	tool := NewTool(fs, exec, pathResolver)
	req := &Request{Pattern: "*.go", Path: testutil.TestWorkspaceRoot}
	params, _ := json.Marshal(req)
	validated, err := tool.validate(string(params))
	assert.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, disp := tool.executeGlob(ctx, validated)
	assert.ErrorIs(t, ctx.Err(), context.Canceled)
	assert.NotNil(t, disp)
	assert.Equal(t, domain.ToolErrorCancelled, disp.GetError())
}

func TestGlob_RejectsRelativePath(t *testing.T) {
	fs := &mockFileSystem{}
	exec := &mockCommandExecutor{}
	pathResolver := &mockPathResolver{}
	setupMockResolver(pathResolver)

	tool := NewTool(fs, exec, pathResolver)
	req := &Request{Pattern: "*.go", Path: "relative/path"}

	_, err := executeFind(t, tool, req)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "absolute path required")
}

// Shared mocks/helpers used by search package tests.
type mockFileSystem struct {
	mock.Mock
}

func (m *mockFileSystem) Stat(path string) (os.FileInfo, error) {
	args := m.Called(path)
	if len(args) > 0 {
		if args.Get(0) == nil {
			return nil, args.Error(1)
		}
		return args.Get(0).(os.FileInfo), args.Error(1)
	}
	return &toolMockFileInfo{name: filepath.Base(path)}, nil
}

func (m *mockFileSystem) ReadFile(path string) ([]byte, error) {
	args := m.Called(path)
	if len(args) > 0 {
		if args.Get(0) == nil {
			return nil, args.Error(1)
		}
		return args.Get(0).([]byte), args.Error(1)
	}
	return []byte("dummy content"), nil
}

func (m *mockFileSystem) Remove(path string) error {
	args := m.Called(path)
	return args.Error(0)
}

func (m *mockFileSystem) Open(path string) (domain.File, error) {
	args := m.Called(path)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(domain.File), args.Error(1)
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
	return args.Get(0).(os.FileInfo), args.Error(1)
}

type mockCommandExecutor struct {
	mock.Mock
}

func (m *mockCommandExecutor) Run(ctx context.Context, cmd string, dir string, enableLogging bool) (*executor.Result, error) {
	args := m.MethodCalled("Run", ctx, cmd, dir, enableLogging)
	if len(args) > 0 {
		if args.Get(0) == nil {
			return nil, args.Error(1)
		}
		return args.Get(0).(*executor.Result), args.Error(1)
	}
	return &executor.Result{Stdout: "", ExitCode: 0}, nil
}

func (m *mockCommandExecutor) RunStreaming(ctx context.Context, cmd string, dir string, enableLogging bool) (*executor.StreamingCmd, error) {
	args := m.MethodCalled("RunStreaming", ctx, cmd, dir, enableLogging)
	if len(args) > 0 {
		if args.Get(0) == nil {
			return nil, args.Error(1)
		}
		return args.Get(0).(*executor.StreamingCmd), args.Error(1)
	}
	return nil, nil
}

type mockPathResolver struct {
	mock.Mock
}

func (m *mockPathResolver) ValidateAbs(p string) (string, error) {
	if !filepath.IsAbs(p) {
		return "", fmt.Errorf("absolute path required, but got: %q", p)
	}
	return filepath.Clean(p), nil
}

func (m *mockPathResolver) DisplayPath(p string) string {
	return p
}

func (m *mockPathResolver) Root() string {
	return testutil.TestWorkspaceRoot
}

func setupMockResolver(m *mockPathResolver) {
	m.On("Abs", testutil.TestWorkspaceRoot).Return(testutil.TestWorkspaceRoot, nil).Maybe()
}

type toolMockFileInfo struct {
	modTime time.Time
	name    string
	size    int64
	isDir   bool
}

func (m *toolMockFileInfo) Name() string       { return m.name }
func (m *toolMockFileInfo) Size() int64        { return m.size }
func (m *toolMockFileInfo) Mode() os.FileMode  { return 0 }
func (m *toolMockFileInfo) ModTime() time.Time { return m.modTime }
func (m *toolMockFileInfo) IsDir() bool        { return m.isDir }
func (m *toolMockFileInfo) Sys() any           { return nil }

func executeFind(t *testing.T, tool *Tool, req *Request) (string, error) {
	params, err := json.Marshal(req)
	require.NoError(t, err)

	out, err := tool.InvokableRun(context.Background(), string(params))
	if err != nil {
		return "", err
	}
	return out, nil
}
