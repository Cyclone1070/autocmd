package search

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/testutil"
	"github.com/Cyclone1070/iav/internal/tool/service/executor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestGlob_Definition(t *testing.T) {
	tool := NewGlobTool(&mockFileSystem{}, &mockCommandExecutor{}, &mockPathResolver{})
	def := tool.Definition()

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

	tool := NewGlobTool(fs, exec, pathResolver)
	req := &GlobRequest{Pattern: "*.go", Path: testutil.TestWorkspaceRoot}

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

	tool := NewGlobTool(fs, exec, pathResolver)
	req := &GlobRequest{Pattern: "*.go", Path: testutil.TestWorkspaceRoot}

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

	tool := NewGlobTool(fs, exec, pathResolver)
	req := &GlobRequest{Pattern: "*.go", Path: testutil.TestWorkspaceRoot}

	result, err := executeFind(t, tool, req)
	assert.NoError(t, err)
	assert.Contains(t, result, "Output too large")
	assert.Contains(t, result, "read_file")
}

func TestGlob_ExecutionFailure(t *testing.T) {
	fs := &mockFileSystem{}
	exec := &mockCommandExecutor{}
	pathResolver := &mockPathResolver{}
	setupMockResolver(pathResolver)

	fs.On("Stat", testutil.TestWorkspaceRoot).Return(&toolMockFileInfo{name: "workspace", isDir: true}, nil)

	// Simulate fd failure
	exec.On("Run", mock.Anything, mock.Anything, testutil.TestWorkspaceRoot, true).
		Return(&executor.Result{Stdout: "fatal error", ExitCode: 2}, nil)

	tool := NewGlobTool(fs, exec, pathResolver)
	req := &GlobRequest{Pattern: "*.go", Path: testutil.TestWorkspaceRoot}

	result, err := executeFind(t, tool, req)
	assert.NoError(t, err)
	assert.Contains(t, result, "fatal error")
}

func TestGlob_ExecuteCancelled_ReturnsToolErrorCancelledDisplay(t *testing.T) {
	fs := &mockFileSystem{}
	exec := &mockCommandExecutor{}
	pathResolver := &mockPathResolver{}
	setupMockResolver(pathResolver)

	fs.On("Stat", testutil.TestWorkspaceRoot).Return(&toolMockFileInfo{name: "workspace", isDir: true}, nil)

	tool := NewGlobTool(fs, exec, pathResolver)
	req := &GlobRequest{Pattern: "*.go", Path: testutil.TestWorkspaceRoot}
	params, _ := json.Marshal(req)
	inv, err := tool.Prepare(string(params))
	assert.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, disp := inv.(domain.ExecutableInvocation).Execute(ctx)
	assert.ErrorIs(t, ctx.Err(), context.Canceled)
	assert.NotNil(t, disp)
	assert.Equal(t, domain.ToolErrorCancelled, disp.GetError())
}

func TestGlob_RejectsRelativePath(t *testing.T) {
	fs := &mockFileSystem{}
	exec := &mockCommandExecutor{}
	pathResolver := &mockPathResolver{}
	setupMockResolver(pathResolver)

	tool := NewGlobTool(fs, exec, pathResolver)
	req := &GlobRequest{Pattern: "*.go", Path: "relative/path"}

	_, err := executeFind(t, tool, req)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "absolute path required")
}
