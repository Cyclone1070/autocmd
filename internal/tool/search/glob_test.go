package search

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/Cyclone1070/iav/internal/domain"
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

	// params := def.ParamsOneOf.Params // can't access unexported field
}

func TestGlob_Basic(t *testing.T) {
	fs := &mockFileSystem{}
	exec := &mockCommandExecutor{}
	pathResolver := &mockPathResolver{}
	setupMockResolver(pathResolver)

	pathResolver.On("Abs", "/workspace").Return("/workspace", nil).Maybe()
	pathResolver.On("DisplayPath", "/workspace/a.go").Return("a.go")

	fs.On("Stat", "/workspace").Return(&toolMockFileInfo{name: "workspace", isDir: true}, nil)

	output := "a.go\n"
	exec.On("Run", mock.Anything,
		[]string{"rg", "--files", "--glob", "*.go", "--sort=modified", "--no-ignore", "--hidden"},
		"/workspace",
		os.Environ(),
	).Return(&executor.Result{Stdout: output, ExitCode: 0}, nil)

	tool := NewGlobTool(fs, exec, pathResolver)
	req := &GlobRequest{Pattern: "*.go"}

	result, err := executeFind(t, tool, req)
	assert.NoError(t, err)
	assert.Equal(t, "a.go", result)
}

func TestGlob_NoMatches(t *testing.T) {
	fs := &mockFileSystem{}
	exec := &mockCommandExecutor{}
	pathResolver := &mockPathResolver{}
	setupMockResolver(pathResolver)

	fs.On("Stat", "/workspace").Return(&toolMockFileInfo{name: "workspace", isDir: true}, nil)

	exec.On("Run", mock.Anything, mock.Anything, "/workspace", os.Environ()).
		Return(&executor.Result{Stdout: "", ExitCode: 0}, nil)

	tool := NewGlobTool(fs, exec, pathResolver)
	req := &GlobRequest{Pattern: "*.go"}

	result, err := executeFind(t, tool, req)
	assert.NoError(t, err)
	assert.Equal(t, "No files found", result)
}

func TestGlob_Truncation(t *testing.T) {
	fs := &mockFileSystem{}
	exec := &mockCommandExecutor{}
	pathResolver := &mockPathResolver{}
	setupMockResolver(pathResolver)

	fs.On("Stat", "/workspace").Return(&toolMockFileInfo{name: "workspace", isDir: true}, nil)

	var output strings.Builder
	for i := 0; i < 110; i++ {
		output.WriteString(fmt.Sprintf("file%d.go\n", i))
	}
	exec.On("Run", mock.Anything, mock.Anything, "/workspace", os.Environ()).
		Return(&executor.Result{Stdout: output.String(), ExitCode: 0}, nil)

	tool := NewGlobTool(fs, exec, pathResolver)
	req := &GlobRequest{Pattern: "*.go"}

	result, err := executeFind(t, tool, req)
	assert.NoError(t, err)

	lines := strings.Split(result, "\n")
	assert.Len(t, lines, 101) // 100 files + 1 truncation message
	assert.Equal(t, "(Results are truncated. Consider using a more specific path or pattern.)", lines[100])
}

func TestGlob_ExecutionFailure(t *testing.T) {
	fs := &mockFileSystem{}
	exec := &mockCommandExecutor{}
	pathResolver := &mockPathResolver{}; setupMockResolver(pathResolver)

	fs.On("Stat", "/workspace").Return(&toolMockFileInfo{name: "workspace", isDir: true}, nil)

	// Simulate fd failure
	exec.On("Run", mock.Anything, mock.Anything, "/workspace", os.Environ()).
		Return(&executor.Result{Stderr: "fatal error", ExitCode: 2}, nil)

	tool := NewGlobTool(fs, exec, pathResolver)
	req := &GlobRequest{Pattern: "*.go"}

	result, err := executeFind(t, tool, req)
	assert.NoError(t, err)
	assert.Contains(t, result, "fatal error")
}

func TestGlob_ExecuteCancelled_ReturnsToolErrorCancelledDisplay(t *testing.T) {
	fs := &mockFileSystem{}
	exec := &mockCommandExecutor{}
	pathResolver := &mockPathResolver{}; setupMockResolver(pathResolver)

	fs.On("Stat", "/workspace").Return(&toolMockFileInfo{name: "workspace", isDir: true}, nil)

	tool := NewGlobTool(fs, exec, pathResolver)
	req := &GlobRequest{Pattern: "*.go"}
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
