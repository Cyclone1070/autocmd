package search

import (
	"io"
	"testing"

	"github.com/Cyclone1070/iav/internal/tool/service/executor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestGrep_RawRelative(t *testing.T) {
	fs := &mockFileSystem{}
	exec := &mockCommandExecutor{}
	pathResolver := &mockPathResolver{}; setupMockResolver(pathResolver)

	// Mock existence of the search target
	fs.On("Stat", "/workspace/internal").Return(&toolMockFileInfo{name: "internal", isDir: true}, nil).Maybe()

	// Ripgrep is run with absolute path and returns absolute paths.
	output := "/workspace/internal/file.txt:1:match\n"
	exec.On("Run", mock.Anything, mock.Anything, "/workspace", true).
		Return(&executor.Result{Stdout: output, ExitCode: 0}, nil)

	tool := NewGrepTool(fs, exec, pathResolver)
	req := &GrepRequest{Pattern: "pattern", Path: "/workspace/internal"}

	result, err := executeSearch(t, tool, req)
	assert.NoError(t, err)

	// Result should preserve ripgrep output and append metadata at the end.
	assert.Contains(t, result, output)
	assert.Contains(t, result, "<exit_code>0</exit_code>")
}

func TestGrep_OffloadedRaw(t *testing.T) {
	fs := &mockFileSystem{}
	exec := &mockCommandExecutor{}
	pathResolver := &mockPathResolver{}; setupMockResolver(pathResolver)

	fs.On("Stat", "/workspace").Return(&toolMockFileInfo{name: "workspace", isDir: true}, nil).Maybe()

	exec.On("Run", mock.Anything, mock.Anything, "/workspace", true).
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
	req := &GrepRequest{Pattern: "pattern", Path: "/workspace"}

	result, err := executeSearch(t, tool, req)
	assert.NoError(t, err)

	assert.Contains(t, result, "Output too large")
	assert.Contains(t, result, "read_file")
}

func TestGrep_NoMatchesRaw(t *testing.T) {
	fs := &mockFileSystem{}
	exec := &mockCommandExecutor{}
	pathResolver := &mockPathResolver{}; setupMockResolver(pathResolver)

	fs.On("Stat", "/workspace").Return(&toolMockFileInfo{name: "workspace", isDir: true}, nil).Maybe()

	exec.On("Run", mock.Anything, mock.Anything, "/workspace", true).
		Return(&executor.Result{Stdout: "", ExitCode: 1}, nil)

	tool := NewGrepTool(fs, exec, pathResolver)
	req := &GrepRequest{Pattern: "pattern", Path: "/workspace"}

	result, err := executeSearch(t, tool, req)
	assert.NoError(t, err)
	assert.Equal(t, "No matches found\n\n<exit_code>1</exit_code>", result)
}

func TestGrep_MalformedStats(t *testing.T) {
	fs := &mockFileSystem{}
	exec := &mockCommandExecutor{}
	pathResolver := &mockPathResolver{}; setupMockResolver(pathResolver)

	fs.On("Stat", "/workspace").Return(&toolMockFileInfo{name: "workspace", isDir: true}, nil).Maybe()

	exec.On("Run", mock.Anything, mock.Anything, "/workspace", true).
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
	req := &GrepRequest{Pattern: "pattern", Path: "/workspace"}

	result, err := executeSearch(t, tool, req)
	assert.NoError(t, err)

	// Expected behavior when Sscanf/Atoi fails: default to 0 rather than crashing
	assert.Contains(t, result, "Output too large (0 matches across 0 files)")
}

func TestGrep_RejectsRelativePath(t *testing.T) {
	fs := &mockFileSystem{}
	exec := &mockCommandExecutor{}
	pathResolver := &mockPathResolver{}; setupMockResolver(pathResolver)

	tool := NewGrepTool(fs, exec, pathResolver)
	req := &GrepRequest{Pattern: "pattern", Path: "relative/path"}

	_, err := executeSearch(t, tool, req)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "absolute path required")
}
