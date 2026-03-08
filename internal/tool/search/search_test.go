package search

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Cyclone1070/iav/internal/config"
	"github.com/Cyclone1070/iav/internal/tool/service/executor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// --- Local Mocks ---

// MockFileInfo implements os.FileInfo
type MockFileInfo struct {
	NameVal    string
	SizeVal    int64
	ModeVal    os.FileMode
	ModTimeVal time.Time
	IsDirVal   bool
	SysVal     any
}

func (m MockFileInfo) Name() string       { return m.NameVal }
func (m MockFileInfo) Size() int64        { return m.SizeVal }
func (m MockFileInfo) Mode() os.FileMode  { return m.ModeVal }
func (m MockFileInfo) ModTime() time.Time { return m.ModTimeVal }
func (m MockFileInfo) IsDir() bool        { return m.IsDirVal }
func (m MockFileInfo) Sys() any           { return m.SysVal }

// MockFileSystem implements fileSystem
type MockFileSystem struct {
	mock.Mock
}

func (m *MockFileSystem) Stat(path string) (os.FileInfo, error) {
	args := m.Called(path)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(os.FileInfo), args.Error(1)
}

// MockCommandExecutor implements commandExecutor
type MockCommandExecutor struct {
	mock.Mock
}

func (m *MockCommandExecutor) Run(ctx context.Context, command []string, cwd string, env []string) (*executor.Result, error) {
	args := m.Called(ctx, command, cwd, env)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	res := args.Get(0).(executor.Result)
	return &res, args.Error(1)
}

// MockPathResolver implements pathResolver
type MockPathResolver struct {
	mock.Mock
}

func (m *MockPathResolver) Abs(path string) (string, error) {
	args := m.Called(path)
	return args.String(0), args.Error(1)
}

func (m *MockPathResolver) Rel(path string) (string, error) {
	args := m.Called(path)
	return args.String(0), args.Error(1)
}

// --- Test Helper ---

func executeSearch(t *testing.T, stool *SearchContentTool, req *SearchContentRequest) (string, error) {
	t.Helper()
	params, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Failed to marshal request: %v", err)
	}

	inv, err := stool.Prepare(context.Background(), params)
	if err != nil {
		return "", err
	}

	return inv.Execute(context.Background())
}

// --- Tests ---

func TestSearchContent_BasicSearch(t *testing.T) {
	fs := new(MockFileSystem)
	exec := new(MockCommandExecutor)
	pathResolver := new(MockPathResolver)
	cfg := config.DefaultConfig()

	// Mock path resolution
	pathResolver.On("Abs", ".").Return("/root", nil)
	pathResolver.On("Rel", "/root/file.txt").Return("file.txt", nil)

	// Mock FS stat
	fs.On("Stat", "/root").Return(MockFileInfo{IsDirVal: true}, nil)

	// Mock execution with OpenCode-style rg output
	rgOutput := `{"type":"match","data":{"path":{"text":"/root/file.txt"},"lines":{"text":"match content"},"line_number":1,"absolute_offset":0,"submatches":[]}}`
	exec.On("Run",
		context.Background(),
		[]string{"rg", "--json", "--glob=!.git/*", "--", "pattern", "/root"},
		"/root",
		([]string)(nil),
	).Return(executor.Result{Stdout: rgOutput, ExitCode: 0}, nil)

	tool := NewSearchContentTool(fs, exec, cfg, pathResolver)

	req := &SearchContentRequest{
		Pattern: "pattern",
	}

	result, err := executeSearch(t, tool, req)
	require.NoError(t, err)

	expected := `Found 1 matches

file.txt:
  Line 1: match content
`
	assert.Equal(t, expected, result)
}

func TestSearchContent_Includes(t *testing.T) {
	fs := new(MockFileSystem)
	exec := new(MockCommandExecutor)
	pathResolver := new(MockPathResolver)
	cfg := config.DefaultConfig()

	pathResolver.On("Abs", ".").Return("/root", nil)
	fs.On("Stat", "/root").Return(MockFileInfo{IsDirVal: true}, nil)

	// Check that includes are passed as globals
	exec.On("Run",
		context.Background(),
		[]string{"rg", "--json", "--glob=!.git/*", "--glob=*.go", "--", "pattern", "/root"},
		"/root",
		([]string)(nil),
	).Return(executor.Result{Stdout: "", ExitCode: 0}, nil)

	tool := NewSearchContentTool(fs, exec, cfg, pathResolver)

	req := &SearchContentRequest{
		Pattern: "pattern",
		Include: "*.go",
	}

	_, err := executeSearch(t, tool, req)
	require.NoError(t, err)
}

func TestSearchContent_PathValidation(t *testing.T) {
	fs := new(MockFileSystem)
	exec := new(MockCommandExecutor)
	pathResolver := new(MockPathResolver)
	cfg := config.DefaultConfig()

	// 1. Path does not exist
	pathResolver.On("Abs", "nonexistent").Return("/root/nonexistent", nil)
	fs.On("Stat", "/root/nonexistent").Return(nil, os.ErrNotExist)

	tool := NewSearchContentTool(fs, exec, cfg, pathResolver)

	req := &SearchContentRequest{Pattern: "foo", Path: "nonexistent"}
	_, err := executeSearch(t, tool, req)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "path does not exist")

	// 2. Path is a file (Should be allowed now)
	pathResolver.On("Abs", "file.txt").Return("/root/file.txt", nil)
	fs.On("Stat", "/root/file.txt").Return(MockFileInfo{IsDirVal: false}, nil)

	// Expect it to proceed to execution
	exec.On("Run",
		context.Background(),
		[]string{"rg", "--json", "--glob=!.git/*", "--", "foo", "/root/file.txt"},
		"/root/file.txt",
		([]string)(nil),
	).Return(executor.Result{Stdout: "", ExitCode: 0}, nil)

	reqFile := &SearchContentRequest{Pattern: "foo", Path: "file.txt"}
	_, err = executeSearch(t, tool, reqFile)
	assert.NoError(t, err)
}

func TestSearchContent_Truncation(t *testing.T) {
	fs := new(MockFileSystem)
	exec := new(MockCommandExecutor)
	pathResolver := new(MockPathResolver)
	cfg := config.DefaultConfig()

	pathResolver.On("Abs", ".").Return("/root", nil)
	fs.On("Stat", "/root").Return(MockFileInfo{IsDirVal: true}, nil)
	pathResolver.On("Rel", "/root/file.txt").Return("file.txt", nil)

	// Generate 101 matches
	var rgOutputBuilder strings.Builder
	for range 101 {
		rgOutputBuilder.WriteString(`{"type":"match","data":{"path":{"text":"/root/file.txt"},"lines":{"text":"match"},"line_number":1}}` + "\n")
	}

	exec.On("Run", context.Background(),
		[]string{"rg", "--json", "--glob=!.git/*", "--", "pattern", "/root"},
		"/root",
		([]string)(nil),
	).Return(executor.Result{Stdout: rgOutputBuilder.String(), ExitCode: 0}, nil)

	tool := NewSearchContentTool(fs, exec, cfg, pathResolver)

	req := &SearchContentRequest{Pattern: "pattern"}
	result, err := executeSearch(t, tool, req)
	require.NoError(t, err)

	assert.Contains(t, result, "Found 100 matches") // Should be capped at 100
	assert.Contains(t, result, "(Results are truncated")
}

func TestSearchContent_NoMatches(t *testing.T) {
	fs := new(MockFileSystem)
	exec := new(MockCommandExecutor)
	pathResolver := new(MockPathResolver)
	cfg := config.DefaultConfig()

	pathResolver.On("Abs", ".").Return("/root", nil)
	fs.On("Stat", "/root").Return(MockFileInfo{IsDirVal: true}, nil)

	// rg returns exit code 1 when no matches found
	exec.On("Run", context.Background(),
		[]string{"rg", "--json", "--glob=!.git/*", "--", "pattern", "/root"},
		"/root",
		([]string)(nil),
	).Return(executor.Result{Stdout: "", ExitCode: 1}, nil)

	tool := NewSearchContentTool(fs, exec, cfg, pathResolver)

	req := &SearchContentRequest{Pattern: "pattern"}
	result, err := executeSearch(t, tool, req)
	require.NoError(t, err)
	assert.Equal(t, "No matches found.", result)
}

func TestSearchContent_CommandFailure(t *testing.T) {
	fs := new(MockFileSystem)
	exec := new(MockCommandExecutor)
	pathResolver := new(MockPathResolver)
	cfg := config.DefaultConfig()

	pathResolver.On("Abs", ".").Return("/root", nil)
	fs.On("Stat", "/root").Return(MockFileInfo{IsDirVal: true}, nil)

	// rg fails with exit code 2
	exec.On("Run", context.Background(),
		[]string{"rg", "--json", "--glob=!.git/*", "--", "pattern", "/root"},
		"/root",
		([]string)(nil),
	).Return(executor.Result{Stderr: "Fatal error", ExitCode: 2}, nil)

	tool := NewSearchContentTool(fs, exec, cfg, pathResolver)

	req := &SearchContentRequest{Pattern: "pattern"}
	result, err := executeSearch(t, tool, req)
	assert.Error(t, err) // Operation errors return err for logging per tool.md contract
	assert.Contains(t, result, "rg failed")
}

func TestSearchContent_InvalidJSON(t *testing.T) {
	fs := new(MockFileSystem)
	exec := new(MockCommandExecutor)
	pathResolver := new(MockPathResolver)
	cfg := config.DefaultConfig()

	pathResolver.On("Abs", ".").Return("/root", nil)
	fs.On("Stat", "/root").Return(MockFileInfo{IsDirVal: true}, nil)

	// rg returns partial/invalid JSON (should be skipped)
	exec.On("Run", context.Background(),
		[]string{"rg", "--json", "--glob=!.git/*", "--", "pattern", "/root"},
		"/root",
		([]string)(nil),
	).Return(executor.Result{Stdout: "invalid json\n", ExitCode: 0}, nil)

	tool := NewSearchContentTool(fs, exec, cfg, pathResolver)

	req := &SearchContentRequest{Pattern: "pattern"}
	result, err := executeSearch(t, tool, req)
	require.NoError(t, err)
	assert.Equal(t, "No matches found.", result) // Should treat as no matches
}

func TestSearchContent_LineLengthLimit(t *testing.T) {
	fs := new(MockFileSystem)
	exec := new(MockCommandExecutor)
	pathResolver := new(MockPathResolver)
	cfg := config.DefaultConfig()
	tool := NewSearchContentTool(fs, exec, cfg, pathResolver)
	tool.maxLineLength = 10

	pathResolver.On("Abs", ".").Return("/root", nil)
	pathResolver.On("Rel", "/root/file.txt").Return("file.txt", nil)
	fs.On("Stat", "/root").Return(MockFileInfo{IsDirVal: true}, nil)

	longLine := "very long line that exceeds limit"
	rgOutput := `{"type":"match","data":{"path":{"text":"/root/file.txt"},"lines":{"text":"` + longLine + `"},"line_number":1}}`
	exec.On("Run", context.Background(),
		[]string{"rg", "--json", "--glob=!.git/*", "--", "pattern", "/root"},
		"/root",
		([]string)(nil),
	).Return(executor.Result{Stdout: rgOutput, ExitCode: 0}, nil)

	req := &SearchContentRequest{Pattern: "pattern"}
	result, err := executeSearch(t, tool, req)
	require.NoError(t, err)

	assert.Contains(t, result, "very long ...[truncated]")
}

func TestNewSearchContentTool_Panics(t *testing.T) {
	fs := new(MockFileSystem)
	exec := new(MockCommandExecutor)
	pathResolver := new(MockPathResolver)
	cfg := config.DefaultConfig()

	assert.PanicsWithValue(t, "fs is required", func() {
		NewSearchContentTool(nil, exec, cfg, pathResolver)
	})
	assert.PanicsWithValue(t, "commandExecutor is required", func() {
		NewSearchContentTool(fs, nil, cfg, pathResolver)
	})
	assert.PanicsWithValue(t, "cfg is required", func() {
		NewSearchContentTool(fs, exec, nil, pathResolver)
	})
	assert.PanicsWithValue(t, "pathResolver is required", func() {
		NewSearchContentTool(fs, exec, cfg, nil)
	})
}
