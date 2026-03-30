package search

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/tool/service/executor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestFindFile_Basic(t *testing.T) {
	fs := &mockFileSystem{}
	exec := &mockCommandExecutor{}
	pathResolver := &mockPathResolver{}

	pathResolver.On("Abs", ".").Return("/workspace", nil)
	pathResolver.On("Rel", "/workspace").Return(".", nil)
	pathResolver.On("Rel", "/workspace/a.go").Return("a.go", nil)

	fs.On("Stat", "/workspace").Return(&toolMockFileInfo{name: "workspace", isDir: true}, nil)

	output := "/workspace/a.go\n"
	exec.On("Run", mock.Anything,
		[]string{"fd", "--glob", "*.go", "/workspace"},
		"/workspace",
		os.Environ(),
	).Return(&executor.Result{Stdout: output, ExitCode: 0}, nil)

	tool := NewFindFileTool(fs, exec, pathResolver)
	req := &FindFileRequest{Pattern: "*.go"}

	result, err := executeFind(t, tool, req)
	assert.NoError(t, err)
	assert.Equal(t, "a.go", result)
}

func TestFindFile_NoMatches(t *testing.T) {
	fs := &mockFileSystem{}
	exec := &mockCommandExecutor{}
	pathResolver := &mockPathResolver{}

	pathResolver.On("Abs", ".").Return("/workspace", nil)
	pathResolver.On("Rel", "/workspace").Return(".", nil)
	fs.On("Stat", "/workspace").Return(&toolMockFileInfo{name: "workspace", isDir: true}, nil)

	exec.On("Run", mock.Anything, mock.Anything, "/workspace", os.Environ()).
		Return(&executor.Result{Stdout: "", ExitCode: 0}, nil)

	tool := NewFindFileTool(fs, exec, pathResolver)
	req := &FindFileRequest{Pattern: "*.go"}

	result, err := executeFind(t, tool, req)
	assert.NoError(t, err)
	assert.Equal(t, "No matches found.", result)
}

func TestFindFile_Environment(t *testing.T) {
	fs := &mockFileSystem{}
	exec := &mockCommandExecutor{}
	pathResolver := &mockPathResolver{}

	pathResolver.On("Abs", ".").Return("/workspace", nil)
	pathResolver.On("Rel", "/workspace").Return(".", nil)
	fs.On("Stat", "/workspace").Return(&toolMockFileInfo{name: "workspace", isDir: true}, nil)

	exec.On("Run", mock.Anything, mock.Anything, "/workspace", os.Environ()).
		Return(&executor.Result{Stdout: "", ExitCode: 0}, nil)

	tool := NewFindFileTool(fs, exec, pathResolver)
	req := &FindFileRequest{Pattern: "*.go"}

	_, _ = executeFind(t, tool, req)
	exec.AssertExpectations(t)
}

func TestFindFile_ExecutionFailure(t *testing.T) {
	fs := &mockFileSystem{}
	exec := &mockCommandExecutor{}
	pathResolver := &mockPathResolver{}

	pathResolver.On("Abs", ".").Return("/workspace", nil)
	pathResolver.On("Rel", "/workspace").Return(".", nil)
	fs.On("Stat", "/workspace").Return(&toolMockFileInfo{name: "workspace", isDir: true}, nil)

	// Simulate fd failure
	exec.On("Run", mock.Anything, mock.Anything, "/workspace", os.Environ()).
		Return(&executor.Result{Stderr: "fatal error", ExitCode: 2}, nil)

	tool := NewFindFileTool(fs, exec, pathResolver)
	req := &FindFileRequest{Pattern: "*.go"}

	result, err := executeFind(t, tool, req)
	assert.Error(t, err)
	assert.Equal(t, "Execution failed", err.Error())
	assert.Contains(t, result, "fatal error")
}

func TestFindFile_ExecuteCancelled_ReturnsToolErrorCancelledDisplay(t *testing.T) {
	fs := &mockFileSystem{}
	exec := &mockCommandExecutor{}
	pathResolver := &mockPathResolver{}

	pathResolver.On("Abs", ".").Return("/workspace", nil)
	pathResolver.On("Rel", "/workspace").Return(".", nil)
	fs.On("Stat", "/workspace").Return(&toolMockFileInfo{name: "workspace", isDir: true}, nil)

	tool := NewFindFileTool(fs, exec, pathResolver)
	req := &FindFileRequest{Pattern: "*.go"}
	params, _ := json.Marshal(req)
	inv, err := tool.Prepare(string(params))
	assert.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, disp, execErr := inv.Execute(ctx)
	assert.ErrorIs(t, execErr, context.Canceled)
	assert.NotNil(t, disp)
	assert.Equal(t, domain.ToolErrorCancelled, disp.GetError())
}
