package search

import (
	"os"
	"testing"

	"github.com/Cyclone1070/iav/internal/tool/service/executor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestSearchContent_Basic(t *testing.T) {
	fs := &mockFileSystem{}
	exec := &mockCommandExecutor{}
	pathResolver := &mockPathResolver{}

	pathResolver.On("Abs", ".").Return("/workspace", nil)
	pathResolver.On("Rel", "/workspace").Return(".", nil)
	pathResolver.On("Rel", "/workspace/file.txt").Return("file.txt", nil)

	fs.On("Stat", "/workspace").Return(&toolMockFileInfo{name: "workspace", isDir: true}, nil)

	output := `{"type":"match","data":{"path":{"text":"/workspace/file.txt"},"lines":{"text":"match content"},"line_number":1}}`
	exec.On("Run", mock.Anything,
		[]string{"rg", "--json", "--glob=!.git/*", "--", "pattern", "/workspace"},
		"/workspace",
		os.Environ(),
	).Return(&executor.Result{Stdout: output, ExitCode: 0}, nil)

	tool := NewSearchContentTool(fs, exec, pathResolver)
	req := &SearchContentRequest{Pattern: "pattern"}

	result, err := executeSearch(t, tool, req)
	assert.NoError(t, err)

	expected := "Found 1 matches\n\nfile.txt:\n  Line 1: match content\n"
	assert.Equal(t, expected, result)
}

func TestSearchContent_Environment(t *testing.T) {
	fs := &mockFileSystem{}
	exec := &mockCommandExecutor{}
	pathResolver := &mockPathResolver{}

	pathResolver.On("Abs", ".").Return("/workspace", nil)
	pathResolver.On("Rel", "/workspace").Return(".", nil)
	fs.On("Stat", "/workspace").Return(&toolMockFileInfo{name: "workspace", isDir: true}, nil)

	// Verify os.Environ() is passed
	exec.On("Run", mock.Anything, mock.Anything, "/workspace", os.Environ()).
		Return(&executor.Result{Stdout: "", ExitCode: 0}, nil)

	tool := NewSearchContentTool(fs, exec, pathResolver)
	req := &SearchContentRequest{Pattern: "pattern"}

	_, _ = executeSearch(t, tool, req)
	exec.AssertExpectations(t)
}

func TestSearchContent_NoMatches(t *testing.T) {
	fs := &mockFileSystem{}
	exec := &mockCommandExecutor{}
	pathResolver := &mockPathResolver{}

	pathResolver.On("Abs", ".").Return("/workspace", nil)
	pathResolver.On("Rel", "/workspace").Return(".", nil)
	fs.On("Stat", "/workspace").Return(&toolMockFileInfo{name: "workspace", isDir: true}, nil)

	exec.On("Run", mock.Anything, mock.Anything, "/workspace", os.Environ()).
		Return(&executor.Result{Stdout: "", ExitCode: 1}, nil)

	tool := NewSearchContentTool(fs, exec, pathResolver)
	req := &SearchContentRequest{Pattern: "pattern"}

	result, err := executeSearch(t, tool, req)
	assert.NoError(t, err)
	assert.Equal(t, "No matches found.", result)
}
func TestSearchContent_FileTarget_Regression(t *testing.T) {
	fs := &mockFileSystem{}
	exec := &mockCommandExecutor{}
	pathResolver := &mockPathResolver{}

	// Target is a specific file
	absFilePath := "/workspace/temp.md"
	absDirPath := "/workspace"

	pathResolver.On("Abs", "temp.md").Return(absFilePath, nil)
	pathResolver.On("Rel", absFilePath).Return("temp.md", nil)

	// File exists and is NOT a directory
	fs.On("Stat", absFilePath).Return(&toolMockFileInfo{name: "temp.md", isDir: false}, nil)

	// We expect the working directory to be the PARENT directory, not the file itself
	exec.On("Run", mock.Anything,
		[]string{"rg", "--json", "--glob=!.git/*", "--", "pattern", absFilePath},
		absDirPath, // THIS IS THE FIX: Should be dir, not file
		os.Environ(),
	).Return(&executor.Result{Stdout: "", ExitCode: 0}, nil)

	tool := NewSearchContentTool(fs, exec, pathResolver)

	req := &SearchContentRequest{Pattern: "pattern", Path: "temp.md"}

	_, err := executeSearch(t, tool, req)
	assert.NoError(t, err)

	exec.AssertExpectations(t)
}

func TestSearchContent_ExecutionFailure(t *testing.T) {
	fs := &mockFileSystem{}
	exec := &mockCommandExecutor{}
	pathResolver := &mockPathResolver{}

	pathResolver.On("Abs", ".").Return("/workspace", nil)
	pathResolver.On("Rel", "/workspace").Return(".", nil)

	fs.On("Stat", "/workspace").Return(&toolMockFileInfo{name: "workspace", isDir: true}, nil)

	// Simulate rg failure
	exec.On("Run", mock.Anything, mock.Anything, "/workspace", os.Environ()).
		Return(&executor.Result{Stderr: "fatal error", ExitCode: 2}, nil)

	tool := NewSearchContentTool(fs, exec, pathResolver)
	req := &SearchContentRequest{Pattern: "pattern"}

	result, err := executeSearch(t, tool, req)
	assert.Error(t, err)
	assert.Equal(t, "Execution failed", err.Error())
	assert.Contains(t, result, "fatal error")
}
