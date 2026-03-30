package search

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/Cyclone1070/iav/internal/tool/service/executor"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// Shared Mocks

type mockFileSystem struct {
	mock.Mock
}

func (m *mockFileSystem) Stat(path string) (os.FileInfo, error) {
	args := m.Called(path)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(os.FileInfo), args.Error(1)
}

type mockCommandExecutor struct {
	mock.Mock
}

func (m *mockCommandExecutor) Run(ctx context.Context, cmd []string, dir string, env []string) (*executor.Result, error) {
	args := m.Called(ctx, cmd, dir, env)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*executor.Result), args.Error(1)
}

type mockPathResolver struct {
	mock.Mock
}

func (m *mockPathResolver) Abs(p string) (string, error) {
	args := m.Called(p)
	return args.String(0), args.Error(1)
}

func (m *mockPathResolver) Rel(p string) (string, error) {
	args := m.Called(p)
	return args.String(0), args.Error(1)
}

type toolMockFileInfo struct {
	name  string
	isDir bool
}

func (m *toolMockFileInfo) Name() string       { return m.name }
func (m *toolMockFileInfo) Size() int64        { return 0 }
func (m *toolMockFileInfo) Mode() os.FileMode  { return 0 }
func (m *toolMockFileInfo) ModTime() time.Time { return time.Time{} }
func (m *toolMockFileInfo) IsDir() bool        { return m.isDir }
func (m *toolMockFileInfo) Sys() any           { return nil }

func executeSearch(t *testing.T, tool *SearchContentTool, req *SearchContentRequest) (string, error) {
	params, err := json.Marshal(req)
	require.NoError(t, err)

	invocation, err := tool.Prepare(string(params))
	if err != nil {
		return "", err
	}

	out, _, err := invocation.Execute(context.Background())
	return out, err
}

func executeFind(t *testing.T, tool *FindFileTool, req *FindFileRequest) (string, error) {
	params, err := json.Marshal(req)
	require.NoError(t, err)

	invocation, err := tool.Prepare(string(params))
	if err != nil {
		return "", err
	}

	out, _, err := invocation.Execute(context.Background())
	return out, err
}
