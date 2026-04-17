package search

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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

func (m *mockCommandExecutor) Run(ctx context.Context, cmd string, dir string, enableLogging bool) (*executor.Result, error) {
	args := m.Mock.MethodCalled("Run", ctx, cmd, dir, enableLogging)
	if len(args) > 0 {
		if args.Get(0) == nil {
			return nil, args.Error(1)
		}
		return args.Get(0).(*executor.Result), args.Error(1)
	}
	// Default return for tests that just want to run something
	return &executor.Result{Stdout: "", ExitCode: 0}, nil
}

func (m *mockCommandExecutor) RunStreaming(ctx context.Context, cmd string, dir string, enableLogging bool) (*executor.StreamingCmd, error) {
	args := m.Mock.MethodCalled("RunStreaming", ctx, cmd, dir, enableLogging)
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
	return "/workspace"
}

func setupMockResolver(m *mockPathResolver) {
	m.On("Abs", "/workspace").Return("/workspace", nil).Maybe()
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
