package search

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/testutil"
	"github.com/Cyclone1070/iav/internal/tool/service/executor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestGrep_RawRelative(t *testing.T) {
	fs := &mockFileSystem{}
	exec := &mockCommandExecutor{}
	pathResolver := &mockPathResolver{}
	setupMockResolver(pathResolver)

	// Mock existence of the search target
	fs.On("Stat", testutil.TestWorkspaceRoot + "/internal").Return(&toolMockFileInfo{name: "internal", isDir: true}, nil).Maybe()

	// Ripgrep is run with absolute path and returns absolute paths.
	output := testutil.TestWorkspaceRoot + "/internal/file.txt:1:match\n"
	exec.On("Run", mock.Anything, mock.Anything, testutil.TestWorkspaceRoot, true).
		Return(&executor.Result{Stdout: output, ExitCode: 0}, nil)

	tool := NewGrepTool(fs, exec, pathResolver)
	req := &GrepRequest{Pattern: "pattern", Path: testutil.TestWorkspaceRoot + "/internal"}

	result, err := executeSearch(t, tool, req)
	assert.NoError(t, err)

	// Result should preserve ripgrep output and append metadata at the end.
	assert.Contains(t, result, output)
	assert.Contains(t, result, "<exit_code>0</exit_code>")
}

func TestGrep_OffloadedRaw(t *testing.T) {
	fs := &mockFileSystem{}
	exec := &mockCommandExecutor{}
	pathResolver := &mockPathResolver{}
	setupMockResolver(pathResolver)

	fs.On("Stat", testutil.TestWorkspaceRoot).Return(&toolMockFileInfo{name: "workspace", isDir: true}, nil).Maybe()

	exec.On("Run", mock.Anything, mock.Anything, testutil.TestWorkspaceRoot, true).
		Return(&executor.Result{
			Stdout:   "",
			ExitCode: 0,
			LogPath:  "/tmp/offloaded.log",
		}, nil)

	// Mock file for analyzer
	mf := &mockFile{}
	mf.On("Stat").Return(&toolMockFileInfo{name: "offloaded.log", size: 500}, nil)
	mf.On("Seek", mock.Anything, mock.Anything).Return(int64(0), nil)
	mf.On("Read", mock.Anything).Return(0, io.EOF)
	mf.On("Close").Return(nil)
	fs.On("Open", "/tmp/offloaded.log").Return(mf, nil)

	tool := NewGrepTool(fs, exec, pathResolver)
	req := &GrepRequest{Pattern: "pattern", Path: testutil.TestWorkspaceRoot}

	result, err := executeSearch(t, tool, req)
	assert.NoError(t, err)

	assert.Contains(t, result, "Output too large")
	assert.Contains(t, result, "read_file")
}

func TestGrep_NoMatchesRaw(t *testing.T) {
	fs := &mockFileSystem{}
	exec := &mockCommandExecutor{}
	pathResolver := &mockPathResolver{}
	setupMockResolver(pathResolver)

	fs.On("Stat", testutil.TestWorkspaceRoot).Return(&toolMockFileInfo{name: "workspace", isDir: true}, nil).Maybe()

	exec.On("Run", mock.Anything, mock.Anything, testutil.TestWorkspaceRoot, true).
		Return(&executor.Result{Stdout: "", ExitCode: 1}, nil)

	tool := NewGrepTool(fs, exec, pathResolver)
	req := &GrepRequest{Pattern: "pattern", Path: testutil.TestWorkspaceRoot}

	result, err := executeSearch(t, tool, req)
	assert.NoError(t, err)
	assert.Equal(t, "No matches found\n\n<exit_code>1</exit_code>", result)
}

func TestGrep_MalformedStats(t *testing.T) {
	fs := &mockFileSystem{}
	exec := &mockCommandExecutor{}
	pathResolver := &mockPathResolver{}
	setupMockResolver(pathResolver)

	fs.On("Stat", testutil.TestWorkspaceRoot).Return(&toolMockFileInfo{name: "workspace", isDir: true}, nil).Maybe()

	exec.On("Run", mock.Anything, mock.Anything, testutil.TestWorkspaceRoot, true).
		Return(&executor.Result{
			Stdout:   "",
			ExitCode: 0,
			LogPath:  "/tmp/malformed.log",
		}, nil)

	// Mock file with counts that will overflow a standard int
	mf := &mockFile{}
	mf.On("Stat").Return(&toolMockFileInfo{name: "malformed.log", size: 500}, nil)
	mf.On("Seek", mock.Anything, mock.Anything).Return(int64(0), nil)
	mf.On("Read", mock.Anything).Run(func(args mock.Arguments) {
		p := args.Get(0).([]byte)
		content := "999999999999999999999 matches\n999999999999999999999 files contained matches"
		copy(p, content)
	}).Return(80, io.EOF)
	mf.On("Close").Return(nil)
	fs.On("Open", "/tmp/malformed.log").Return(mf, nil)

	tool := NewGrepTool(fs, exec, pathResolver)
	req := &GrepRequest{Pattern: "pattern", Path: testutil.TestWorkspaceRoot}

	result, err := executeSearch(t, tool, req)
	assert.NoError(t, err)

	// Expected behavior when Sscanf/Atoi fails: default to 0 rather than crashing
	assert.Contains(t, result, "Output too large (0 matches across 0 files)")
}

func TestGrep_RejectsRelativePath(t *testing.T) {
	fs := &mockFileSystem{}
	exec := &mockCommandExecutor{}
	pathResolver := &mockPathResolver{}
	setupMockResolver(pathResolver)

	tool := NewGrepTool(fs, exec, pathResolver)
	req := &GrepRequest{Pattern: "pattern", Path: "relative/path"}

	_, err := executeSearch(t, tool, req)
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
	// Default for tests that don't care about specific path existence.
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
	// Default return for tests that just want to run something.
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

func (m *mockPathResolver) Abs(p string) (string, error) {
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
	name    string
	isDir   bool
	size    int64
	modTime time.Time
}

func (m *toolMockFileInfo) Name() string       { return m.name }
func (m *toolMockFileInfo) Size() int64        { return m.size }
func (m *toolMockFileInfo) Mode() os.FileMode  { return 0 }
func (m *toolMockFileInfo) ModTime() time.Time { return m.modTime }
func (m *toolMockFileInfo) IsDir() bool        { return m.isDir }
func (m *toolMockFileInfo) Sys() any           { return nil }

func executeSearch(t *testing.T, tool *GrepTool, req *GrepRequest) (string, error) {
	params, err := json.Marshal(req)
	require.NoError(t, err)

	invocation, err := tool.Prepare(string(params))
	if err != nil {
		return "", err
	}

	out, _ := invocation.(domain.ExecutableInvocation).Execute(context.Background())
	return out, nil
}

func executeFind(t *testing.T, tool *GlobTool, req *GlobRequest) (string, error) {
	params, err := json.Marshal(req)
	require.NoError(t, err)

	invocation, err := tool.Prepare(string(params))
	if err != nil {
		return "", err
	}

	out, _ := invocation.(domain.ExecutableInvocation).Execute(context.Background())
	return out, nil
}
