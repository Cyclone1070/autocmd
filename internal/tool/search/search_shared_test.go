package search

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/tool/service/executor"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// Shared Mocks

type mockFileSystem struct {
	mock.Mock
}

func (m *mockFileSystem) Stat(path string) (os.FileInfo, error) {
	args := m.Mock.Called(path)
	if len(args) > 0 {
		if args.Get(0) == nil {
			return nil, args.Error(1)
		}
		return args.Get(0).(os.FileInfo), args.Error(1)
	}
	// Default for tests that don't care about specific path existence
	return &toolMockFileInfo{name: filepath.Base(path)}, nil
}

func (m *mockFileSystem) ReadFile(path string) ([]byte, error) {
	args := m.Mock.Called(path)
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

func (m *mockCommandExecutor) Run(ctx context.Context, cmd string, dir string, env []string, enableLogging bool) (*executor.Result, error) {
	args := m.Mock.MethodCalled("Run", ctx, cmd, dir, env, enableLogging)
	if len(args) > 0 {
		if args.Get(0) == nil {
			return nil, args.Error(1)
		}
		return args.Get(0).(*executor.Result), args.Error(1)
	}
	// Default return for tests that just want to run something
	return &executor.Result{Stdout: "", ExitCode: 0}, nil
}

func (m *mockCommandExecutor) RunStreaming(ctx context.Context, cmd string, dir string, env []string, enableLogging bool) (*executor.StreamingCmd, error) {
	args := m.Mock.MethodCalled("RunStreaming", ctx, cmd, dir, env, enableLogging)
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
	if p == "." || p == "" {
		return "/workspace", nil
	}
	if !strings.HasPrefix(p, "/") {
		return "/workspace/" + p, nil
	}
	return p, nil
}

func (m *mockPathResolver) DisplayPath(p string) string {
	// We'll skip Called() here to avoid panics on unmocked paths, 
	// unless we specifically want to test the resolver call.
	// For search tests, we usually don't care about the exact call as long as it works.
	if p == "/workspace" {
		return "."
	}
	if strings.HasPrefix(p, "/workspace/") {
		return p[len("/workspace/"):]
	}
	return p
}

func (m *mockPathResolver) Root() string {
	return "/workspace"
}

func setupMockResolver(m *mockPathResolver) {
	m.On("Abs", ".").Return("/workspace", nil).Maybe()
	m.On("Abs", "").Return("/workspace", nil).Maybe()
	m.On("Abs", "/workspace").Return("/workspace", nil).Maybe()
	m.On("DisplayPath", "/workspace").Return(".").Maybe()
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
