package search

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/tool/service/executor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestGrep_Basic(t *testing.T) {
	fs := &mockFileSystem{}
	exec := &mockCommandExecutor{}
	pathResolver := &mockPathResolver{}; setupMockResolver(pathResolver)

	pathResolver.On("DisplayPath", "/workspace").Return("file.txt")

	fs.On("Stat", "/workspace").Return(&toolMockFileInfo{name: "workspace", isDir: true}, nil).Maybe()
	fs.On("Stat", "/workspace/file.txt").Return(&toolMockFileInfo{name: "file.txt"}, nil).Maybe()

	// Since default is files_with_matches, rg will be called with -l
	output := "/workspace/file.txt\n"
	exec.On("Run", mock.Anything,
		[]string{"rg", "--hidden", "--with-filename", "--glob=!.git", "--glob=!.svn", "--glob=!.hg", "--glob=!.bzr", "--glob=!.jj", "--glob=!.sl", "--max-columns", "500", "-l", "--", "pattern", "/workspace"},
		"/workspace",
		mock.Anything,
	).Return(&executor.Result{Stdout: output, ExitCode: 0}, nil)

	tool := NewGrepTool(fs, exec, pathResolver)
	req := &GrepRequest{Pattern: "pattern"}

	result, err := executeSearch(t, tool, req)
	assert.NoError(t, err)

	expected := "Found 1 files\n\"file.txt\""
	assert.Equal(t, expected, result)
}

func TestGrep_ExecuteCancelled_ReturnsToolErrorCancelledDisplay(t *testing.T) {
	fs := &mockFileSystem{}
	exec := &mockCommandExecutor{}
	pathResolver := &mockPathResolver{}; setupMockResolver(pathResolver)

	fs.On("Stat", "/workspace").Return(&toolMockFileInfo{name: "workspace", isDir: true}, nil).Maybe()

	tool := NewGrepTool(fs, exec, pathResolver)
	req := &GrepRequest{Pattern: "pattern"}
	params, _ := json.Marshal(req)
	inv, err := tool.Prepare(string(params))
	assert.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, disp, execErr := inv.(domain.ExecutableInvocation).Execute(ctx)
	assert.ErrorIs(t, execErr, context.Canceled)
	assert.NotNil(t, disp)
	assert.Equal(t, domain.ToolErrorCancelled, disp.GetError())
}

func TestGrep_Environment(t *testing.T) {
	fs := &mockFileSystem{}
	exec := &mockCommandExecutor{}
	pathResolver := &mockPathResolver{}; setupMockResolver(pathResolver)

	fs.On("Stat", "/workspace").Return(&toolMockFileInfo{name: "workspace", isDir: true}, nil).Maybe()

	// Verify mock.Anything is passed
	exec.On("Run", mock.Anything, mock.Anything, "/workspace", mock.Anything).
		Return(&executor.Result{Stdout: "", ExitCode: 0}, nil)

	tool := NewGrepTool(fs, exec, pathResolver)
	req := &GrepRequest{Pattern: "pattern"}

	_, _ = executeSearch(t, tool, req)
	exec.AssertExpectations(t)
}

func TestGrep_NoMatches(t *testing.T) {
	fs := &mockFileSystem{}
	exec := &mockCommandExecutor{}
	pathResolver := &mockPathResolver{}; setupMockResolver(pathResolver)

	fs.On("Stat", "/workspace").Return(&toolMockFileInfo{name: "workspace", isDir: true}, nil).Maybe()

	exec.On("Run", mock.Anything, mock.Anything, "/workspace", mock.Anything).
		Return(&executor.Result{Stdout: "", ExitCode: 1}, nil)

	tool := NewGrepTool(fs, exec, pathResolver)
	req := &GrepRequest{Pattern: "pattern"}

	result, err := executeSearch(t, tool, req)
	assert.NoError(t, err)
	assert.Equal(t, "No files found", result)
}

func TestGrep_FileTarget_Regression(t *testing.T) {
	fs := &mockFileSystem{}
	exec := &mockCommandExecutor{}
	pathResolver := &mockPathResolver{}; setupMockResolver(pathResolver)

	// Target is a specific file
	absFilePath := "/workspace/temp.md"
	absDirPath := "/workspace"

	pathResolver.On("Abs", "temp.md").Return(absFilePath, nil)
	pathResolver.On("DisplayPath", absFilePath).Return("temp.md")

	// File exists and is NOT a directory
	fs.On("Stat", absFilePath).Return(&toolMockFileInfo{name: "temp.md", isDir: false}, nil).Maybe()

	// We expect the working directory to be the PARENT directory, not the file itself
	exec.On("Run", mock.Anything,
		[]string{"rg", "--hidden", "--with-filename", "--glob=!.git", "--glob=!.svn", "--glob=!.hg", "--glob=!.bzr", "--glob=!.jj", "--glob=!.sl", "--max-columns", "500", "-l", "--", "pattern", absFilePath},
		absDirPath, // THIS IS THE FIX: Should be dir, not file
		mock.Anything,
	).Return(&executor.Result{Stdout: "", ExitCode: 0}, nil)

	tool := NewGrepTool(fs, exec, pathResolver)

	req := &GrepRequest{Pattern: "pattern", Path: "temp.md"}

	_, err := executeSearch(t, tool, req)
	assert.NoError(t, err)

	exec.AssertExpectations(t)
}

func TestGrep_ExecutionFailure(t *testing.T) {
	fs := &mockFileSystem{}
	exec := &mockCommandExecutor{}
	pathResolver := &mockPathResolver{}; setupMockResolver(pathResolver)


	fs.On("Stat", "/workspace").Return(&toolMockFileInfo{name: "workspace", isDir: true}, nil).Maybe()

	// Simulate rg failure
	exec.On("Run", mock.Anything, mock.Anything, "/workspace", mock.Anything).
		Return(&executor.Result{Stderr: "fatal error", ExitCode: 2}, nil)

	tool := NewGrepTool(fs, exec, pathResolver)
	req := &GrepRequest{Pattern: "pattern"}

	result, err := executeSearch(t, tool, req)
	assert.Error(t, err)
	assert.Equal(t, "Execution failed", err.Error())
	assert.Contains(t, result, "fatal error")
}

func TestGrep_Parity_Formatting_FilesWithMatches(t *testing.T) {
	fs := &mockFileSystem{}
	exec := &mockCommandExecutor{}
	pathResolver := &mockPathResolver{}; setupMockResolver(pathResolver)

	pathResolver.On("DisplayPath", "/workspace").Return("file1.txt")
	pathResolver.On("DisplayPath", "/workspace").Return("file2.txt")

	fs.On("Stat", "/workspace").Return(&toolMockFileInfo{name: "workspace", isDir: true}, nil).Maybe()
	fs.On("Stat", "/workspace/file1.txt").Return(&toolMockFileInfo{name: "file1.txt"}, nil).Maybe()
	fs.On("Stat", "/workspace/file2.txt").Return(&toolMockFileInfo{name: "file2.txt"}, nil).Maybe()

	// rg output for -l (files_with_matches)
	output := "/workspace/file1.txt\n/workspace/file2.txt\n"
	// We expect rg -l ...
	exec.On("Run", mock.Anything,
		[]string{"rg", "--hidden", "--with-filename", "--glob=!.git", "--glob=!.svn", "--glob=!.hg", "--glob=!.bzr", "--glob=!.jj", "--glob=!.sl", "--max-columns", "500", "-l", "--", "pattern", "/workspace"},
		"/workspace",
		mock.Anything,
	).Return(&executor.Result{Stdout: output, ExitCode: 0}, nil)

	tool := NewGrepTool(fs, exec, pathResolver)
	req := &GrepRequest{
		Pattern:    "pattern",
		OutputMode: "files_with_matches",
	}

	result, err := executeSearch(t, tool, req)
	assert.NoError(t, err)

	expected := "Found 2 files\n\"file1.txt\"\n\"file2.txt\""
	assert.Equal(t, expected, result)
}

func TestGrep_Parity_Formatting_Count(t *testing.T) {
	fs := &mockFileSystem{}
	exec := &mockCommandExecutor{}
	pathResolver := &mockPathResolver{}; setupMockResolver(pathResolver)

	pathResolver.On("DisplayPath", "/workspace").Return("file1.txt")

	fs.On("Stat", "/workspace").Return(&toolMockFileInfo{name: "workspace", isDir: true}, nil).Maybe()

	// rg output for -c (count) usually is file:count
	output := "/workspace/file1.txt:5\n"
	exec.On("Run", mock.Anything,
		[]string{"rg", "--hidden", "--with-filename", "--glob=!.git", "--glob=!.svn", "--glob=!.hg", "--glob=!.bzr", "--glob=!.jj", "--glob=!.sl", "--max-columns", "500", "-c", "--", "pattern", "/workspace"},
		"/workspace",
		mock.Anything,
	).Return(&executor.Result{Stdout: output, ExitCode: 0}, nil)

	tool := NewGrepTool(fs, exec, pathResolver)
	req := &GrepRequest{
		Pattern:    "pattern",
		OutputMode: "count",
	}

	result, err := executeSearch(t, tool, req)
	assert.NoError(t, err)

	expected := "\"file1.txt\":5\n\nFound 5 total occurrences across 1 file."
	assert.Equal(t, expected, result)
}

func TestGrep_Parity_Pagination(t *testing.T) {
	fs := &mockFileSystem{}
	exec := &mockCommandExecutor{}
	pathResolver := &mockPathResolver{}; setupMockResolver(pathResolver)

	pathResolver.On("DisplayPath", "/workspace").Return("file1.txt")
	pathResolver.On("DisplayPath", "/workspace").Return("file2.txt")

	fs.On("Stat", "/workspace").Return(&toolMockFileInfo{name: "workspace", isDir: true}, nil).Maybe()
	fs.On("Stat", "/workspace/file1.txt").Return(&toolMockFileInfo{name: "file1.txt"}, nil).Maybe()
	fs.On("Stat", "/workspace/file2.txt").Return(&toolMockFileInfo{name: "file2.txt"}, nil).Maybe()

	// rg output - we return many files, but head_limit=1
	output := "/workspace/file1.txt\n/workspace/file2.txt\n"
	exec.On("Run", mock.Anything, mock.Anything, "/workspace", mock.Anything).
		Return(&executor.Result{Stdout: output, ExitCode: 0}, nil)

	tool := NewGrepTool(fs, exec, pathResolver)
	limit := 1
	offset := 0
	req := &GrepRequest{
		Pattern:   "pattern",
		HeadLimit: new(limit),
		Offset:    new(offset),
	}

	result, err := executeSearch(t, tool, req)
	assert.NoError(t, err)

	// Format: "Found 2 files limit: 1, offset: 0\nfile1.txt"?? 
	// Let's re-read the plan.
	// B. Output Modes Processing (Exact Claude Code Formatting)
	// Mode files_with_matches (Default):
	// Return header: Found X files[ limit: A, offset: B].
	// List of relative paths.

	expected := "Found 2 files limit: 1, offset: 0\n\"file1.txt\""
	assert.Equal(t, expected, result)
}

func TestGrep_Parity_Flags(t *testing.T) {
	fs := &mockFileSystem{}
	exec := &mockCommandExecutor{}
	pathResolver := &mockPathResolver{}; setupMockResolver(pathResolver)

	fs.On("Stat", "/workspace").Return(&toolMockFileInfo{name: "workspace", isDir: true}, nil).Maybe()

	tool := NewGrepTool(fs, exec, pathResolver)

	// Test case-insensitive and multiline
	caseInsensitive := true
	multiline := true
	req := &GrepRequest{
		Pattern:         "pattern",
		CaseInsensitive: &caseInsensitive,
		Multiline:       &multiline,
	}

	exec.On("Run", mock.Anything,
		[]string{"rg", "--hidden", "--with-filename", "--glob=!.git", "--glob=!.svn", "--glob=!.hg", "--glob=!.bzr", "--glob=!.jj", "--glob=!.sl", "--max-columns", "500", "-l", "-i", "-U", "--multiline-dotall", "--", "pattern", "/workspace"},
		"/workspace",
		mock.Anything,
	).Return(&executor.Result{Stdout: "", ExitCode: 1}, nil)

	_, _ = executeSearch(t, tool, req)
	exec.AssertExpectations(t)
}

func TestGrep_Parity_Flags_PrecedenceAndIgnore(t *testing.T) {
	fs := &mockFileSystem{}
	exec := &mockCommandExecutor{}
	pathResolver := &mockPathResolver{}; setupMockResolver(pathResolver)

	fs.On("Stat", "/workspace").Return(&toolMockFileInfo{name: "workspace", isDir: true}, nil).Maybe()

	tool := NewGrepTool(fs, exec, pathResolver)

	// Case 1: Context flags should be IGNORED in files_with_matches mode
	// Even if -A, -B, -C are passed, they SHOULD NOT appear in rg command
	req1 := &GrepRequest{
		Pattern:    "pattern",
		OutputMode: "files_with_matches",
		ContextA:   new(5),
		ContextB:   new(5),
	}

	exec.On("Run", mock.Anything,
		[]string{"rg", "--hidden", "--with-filename", "--glob=!.git", "--glob=!.svn", "--glob=!.hg", "--glob=!.bzr", "--glob=!.jj", "--glob=!.sl", "--max-columns", "500", "-l", "--", "pattern", "/workspace"},
		"/workspace",
		mock.Anything,
	).Return(&executor.Result{Stdout: "", ExitCode: 1}, nil)

	_, _ = executeSearch(t, tool, req1)

	// Case 2: -C takes precedence over -A and -B in content mode
	// rg command should have -C 10 but NOT -A or -B
	req2 := &GrepRequest{
		Pattern:         "pattern",
		OutputMode:      "content",
		ContextC:        new(10),
		ContextA:        new(5),
		ContextB:        new(5),
		ShowLineNumbers: new(false),
	}

	exec.On("Run", mock.Anything,
		[]string{"rg", "--hidden", "--with-filename", "--glob=!.git", "--glob=!.svn", "--glob=!.hg", "--glob=!.bzr", "--glob=!.jj", "--glob=!.sl", "--max-columns", "500", "-C", "10", "--", "pattern", "/workspace"},
		"/workspace",
		mock.Anything,
	).Return(&executor.Result{Stdout: "", ExitCode: 0}, nil)

	_, _ = executeSearch(t, tool, req2)

	exec.AssertExpectations(t)
}

func TestGrep_Parity_RecencySorting(t *testing.T) {
	fs := &mockFileSystem{}
	exec := &mockCommandExecutor{}
	pathResolver := &mockPathResolver{}; setupMockResolver(pathResolver)

	pathResolver.On("DisplayPath", "/workspace").Return("old.txt")
	pathResolver.On("DisplayPath", "/workspace").Return("new.txt")

	fs.On("Stat", "/workspace").Return(&toolMockFileInfo{name: "workspace", isDir: true}, nil).Maybe()
	
	// Mock Stats for the files
	now := time.Now()
	fs.On("Stat", "/workspace/old.txt").Return(&toolMockFileInfo{name: "old.txt", modTime: now.Add(-1 * time.Hour)}, nil).Maybe()
	fs.On("Stat", "/workspace/new.txt").Return(&toolMockFileInfo{name: "new.txt", modTime: now}, nil).Maybe()

	// rg returns old first, then new
	output := "/workspace/old.txt\n/workspace/new.txt\n"
	exec.On("Run", mock.Anything, mock.Anything, "/workspace", mock.Anything).
		Return(&executor.Result{Stdout: output, ExitCode: 0}, nil)

	tool := NewGrepTool(fs, exec, pathResolver)
	req := &GrepRequest{Pattern: "pattern", OutputMode: "files_with_matches"}

	result, err := executeSearch(t, tool, req)
	assert.NoError(t, err)

	// Expected order: NEW first then OLD (due to recency sorting)
	expected := "Found 2 files\n\"new.txt\"\n\"old.txt\""
	assert.Equal(t, expected, result)
}

func TestGrep_Parity_SmartGlobParsing(t *testing.T) {
	fs := &mockFileSystem{}
	exec := &mockCommandExecutor{}
	pathResolver := &mockPathResolver{}; setupMockResolver(pathResolver)

	fs.On("Stat", "/workspace").Return(&toolMockFileInfo{name: "workspace", isDir: true}, nil).Maybe()

	tool := NewGrepTool(fs, exec, pathResolver)
	
	// Glob with braces containing space, and comma/space separated others
	req := &GrepRequest{
		Pattern: "pattern",
		Glob:    "{a, b} *.js, *.ts",
	}

	exec.On("Run", mock.Anything,
		[]string{
			"rg", "--hidden", "--with-filename",
			"--glob=!.git", "--glob=!.svn", "--glob=!.hg", "--glob=!.bzr", "--glob=!.jj", "--glob=!.sl",
			"--max-columns", "500", "-l",
			"--glob={a, b}", "--glob=*.js", "--glob=*.ts",
			"--", "pattern", "/workspace",
		},
		"/workspace",
		mock.Anything,
	).Return(&executor.Result{Stdout: "", ExitCode: 1}, nil)

	_, _ = executeSearch(t, tool, req)
	exec.AssertExpectations(t)
}

func TestGrep_Parity_PatternSafety(t *testing.T) {
	fs := &mockFileSystem{}
	exec := &mockCommandExecutor{}
	pathResolver := &mockPathResolver{}; setupMockResolver(pathResolver)

	fs.On("Stat", "/workspace").Return(&toolMockFileInfo{name: "workspace", isDir: true}, nil).Maybe()

	tool := NewGrepTool(fs, exec, pathResolver)
	
	// Pattern starting with dash
	req := &GrepRequest{Pattern: "-flag-pattern"}

	exec.On("Run", mock.Anything,
		[]string{
			"rg", "--hidden", "--with-filename",
			"--glob=!.git", "--glob=!.svn", "--glob=!.hg", "--glob=!.bzr", "--glob=!.jj", "--glob=!.sl",
			"--max-columns", "500", "-l",
			"-e", "-flag-pattern", "/workspace",
		},
		"/workspace",
		mock.Anything,
	).Return(&executor.Result{Stdout: "", ExitCode: 1}, nil)

	_, _ = executeSearch(t, tool, req)
	exec.AssertExpectations(t)
}

func TestGrep_Parity_ConditionalTruncationReporting(t *testing.T) {
	fs := &mockFileSystem{}
	exec := &mockCommandExecutor{}
	pathResolver := &mockPathResolver{}; setupMockResolver(pathResolver)

	pathResolver.On("DisplayPath", "/workspace").Return("file1.txt")
	pathResolver.On("DisplayPath", "/workspace").Return("file2.txt")

	fs.On("Stat", "/workspace").Return(&toolMockFileInfo{name: "workspace", isDir: true}, nil).Maybe()
	fs.On("Stat", "/workspace/file1.txt").Return(&toolMockFileInfo{name: "file1.txt"}, nil).Maybe()
	fs.On("Stat", "/workspace/file2.txt").Return(&toolMockFileInfo{name: "file2.txt"}, nil).Maybe()

	// Case 1: Under limit, NO FOOTER should appear
	output1 := "/workspace/file1.txt\n"
	exec.On("Run", mock.Anything, mock.Anything, "/workspace", mock.Anything).
		Return(&executor.Result{Stdout: output1, ExitCode: 0}, nil).Once()

	tool := NewGrepTool(fs, exec, pathResolver)
	limit := 10
	req1 := &GrepRequest{Pattern: "pattern", HeadLimit: new(limit)}

	result1, _ := executeSearch(t, tool, req1)
	assert.NotContains(t, result1, "limit: 10")

	// Case 2: At/Over limit, FOOTER SHOULD appear
	output2 := "/workspace/file1.txt\n/workspace/file2.txt\n"
	exec.On("Run", mock.Anything, mock.Anything, "/workspace", mock.Anything).
		Return(&executor.Result{Stdout: output2, ExitCode: 0}, nil).Once()

	limit2 := 1
	req2 := &GrepRequest{Pattern: "pattern", HeadLimit: new(limit2)}

	result2, _ := executeSearch(t, tool, req2)
	assert.Contains(t, result2, "limit: 1")
}

func TestGrep_Parity_TypeFilter(t *testing.T) {
	fs := &mockFileSystem{}
	exec := &mockCommandExecutor{}
	pathResolver := &mockPathResolver{}; setupMockResolver(pathResolver)

	fs.On("Stat", "/workspace").Return(&toolMockFileInfo{name: "workspace", isDir: true}, nil).Maybe()

	tool := NewGrepTool(fs, exec, pathResolver)
	req := &GrepRequest{Pattern: "pattern", Type: "go"}

	exec.On("Run", mock.Anything,
		mock.MatchedBy(func(args []string) bool {
			found := false
			for i, arg := range args {
				if arg == "--type" && i+1 < len(args) && args[i+1] == "go" {
					found = true
				}
			}
			return found
		}),
		"/workspace",
		mock.Anything,
	).Return(&executor.Result{Stdout: "", ExitCode: 1}, nil)

	_, _ = executeSearch(t, tool, req)
	exec.AssertExpectations(t)
}

func TestGrep_Parity_RecencySorting_TieBreak(t *testing.T) {
	fs := &mockFileSystem{}
	exec := &mockCommandExecutor{}
	pathResolver := &mockPathResolver{}; setupMockResolver(pathResolver)

	pathResolver.On("DisplayPath", "/workspace").Return("a.txt")
	pathResolver.On("DisplayPath", "/workspace").Return("b.txt")

	fs.On("Stat", "/workspace").Return(&toolMockFileInfo{name: "workspace", isDir: true}, nil).Maybe()
	
	// Same mtime for both
	now := time.Now()
	fs.On("Stat", "/workspace/a.txt").Return(&toolMockFileInfo{name: "a.txt", modTime: now}, nil).Maybe()
	fs.On("Stat", "/workspace/b.txt").Return(&toolMockFileInfo{name: "b.txt", modTime: now}, nil).Maybe()

	// rg returns b first
	output := "/workspace/b.txt\n/workspace/a.txt\n"
	exec.On("Run", mock.Anything, mock.Anything, "/workspace", mock.Anything).
		Return(&executor.Result{Stdout: output, ExitCode: 0}, nil)

	tool := NewGrepTool(fs, exec, pathResolver)
	req := &GrepRequest{Pattern: "pattern", OutputMode: "files_with_matches"}

	result, _ := executeSearch(t, tool, req)

	// Expected order: "a.txt" first (alphabetical tie-break)
	expected := "Found 2 files\n\"a.txt\"\n\"b.txt\""
	assert.Equal(t, expected, result)
}

func TestGrep_Parity_OffsetOverflow(t *testing.T) {
	fs := &mockFileSystem{}
	exec := &mockCommandExecutor{}
	pathResolver := &mockPathResolver{}; setupMockResolver(pathResolver)

	fs.On("Stat", "/workspace").Return(&toolMockFileInfo{name: "workspace", isDir: true}, nil).Maybe()

	output := "/workspace/file1.txt\n"
	fs.On("Stat", "/workspace/file1.txt").Return(&toolMockFileInfo{name: "file1.txt"}, nil).Maybe()

	exec.On("Run", mock.Anything, mock.Anything, "/workspace", mock.Anything).
		Return(&executor.Result{Stdout: output, ExitCode: 0}, nil)

	tool := NewGrepTool(fs, exec, pathResolver)
	offset := 5
	req := &GrepRequest{Pattern: "pattern", Offset: new(offset)}

	result, _ := executeSearch(t, tool, req)
	assert.Equal(t, "No results found in the specified range.", result)
}

func TestGrep_Parity_ContentMode_ContextLines(t *testing.T) {
	fs := &mockFileSystem{}
	exec := &mockCommandExecutor{}
	pathResolver := &mockPathResolver{}; setupMockResolver(pathResolver)

	pathResolver.On("DisplayPath", "/workspace").Return("main.go")

	fs.On("Stat", "/workspace").Return(&toolMockFileInfo{name: "workspace", isDir: true}, nil).Maybe()

	// Mixed match (:) and context (-) lines
	output := "/workspace/main.go-10-before\n/workspace/\"main.go\":11:match\n/workspace/main.go-12-after\n"
	exec.On("Run", mock.Anything, mock.Anything, "/workspace", mock.Anything).
		Return(&executor.Result{Stdout: output, ExitCode: 0}, nil)

	tool := NewGrepTool(fs, exec, pathResolver)
	req := &GrepRequest{Pattern: "pattern", OutputMode: "content"}

	result, _ := executeSearch(t, tool, req)
	
	// Paths should be relativized even for context lines (line 10 and 12 use -)
	assert.Contains(t, result, "\"main.go\":11:match")
	assert.Contains(t, result, "\"main.go\":10:before") // We format it as : even for context in our relay
	assert.Contains(t, result, "\"main.go\":12:after")
}

func TestGrep_Parity_Count_SingleFile(t *testing.T) {
	fs := &mockFileSystem{}
	exec := &mockCommandExecutor{}
	pathResolver := &mockPathResolver{}; setupMockResolver(pathResolver)

	pathResolver.On("Abs", "file.txt").Return("/workspace/file.txt", nil)
	pathResolver.On("DisplayPath", "/workspace").Return("file.txt")
	fs.On("Stat", "/workspace/file.txt").Return(&toolMockFileInfo{name: "file.txt", isDir: false}, nil).Maybe()
	fs.On("Stat", "/workspace").Return(&toolMockFileInfo{name: "workspace", isDir: true}, nil).Maybe()

	// rg MUST return filename:count now because we force --with-filename
	output := "/workspace/file.txt:3\n"
	exec.On("Run", mock.Anything,
		[]string{"rg", "--hidden", "--with-filename", "--glob=!.git", "--glob=!.svn", "--glob=!.hg", "--glob=!.bzr", "--glob=!.jj", "--glob=!.sl", "--max-columns", "500", "-c", "--", "pattern", "/workspace/file.txt"},
		"/workspace",
		mock.Anything,
	).Return(&executor.Result{Stdout: output, ExitCode: 0}, nil)

	tool := NewGrepTool(fs, exec, pathResolver)
	req := &GrepRequest{Pattern: "pattern", Path: "file.txt", OutputMode: "count"}

	result, _ := executeSearch(t, tool, req)
	
	// RED: This will currently output "3:\n\nFound 0 total occurrences..."
	assert.Contains(t, result, "\"file.txt\":3")
	assert.Contains(t, result, "Found 3 total occurrences")
}

func TestGrep_ContextLine_DashCorrupt(t *testing.T) {
	fs := &mockFileSystem{}
	exec := &mockCommandExecutor{}
	pathResolver := &mockPathResolver{}; setupMockResolver(pathResolver)

	// Path with a dash in it
	pathResolver.On("Abs", "my-folder/file.txt").Return("/workspace/my-folder/file.txt", nil)
	pathResolver.On("DisplayPath", "/workspace").Return("my-folder/file.txt")
	fs.On("Stat", "/workspace/my-folder/file.txt").Return(&toolMockFileInfo{name: "file.txt", isDir: false}, nil).Maybe()
	fs.On("Stat", "/workspace").Return(&toolMockFileInfo{name: "workspace", isDir: true}, nil).Maybe()

	// Context output using dashes: filename-line-content
	output := "/workspace/\"my-folder/file.txt\":10:context line here\n/workspace/\"my-folder/file.txt\":11:match line\n"
	exec.On("Run", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(&executor.Result{Stdout: output, ExitCode: 0}, nil)

	tool := NewGrepTool(fs, exec, pathResolver)
	req := &GrepRequest{Pattern: "match", Path: "my-folder/file.txt", OutputMode: "content"}

	result, _ := executeSearch(t, tool, req)
	
	// RED: Current brittle parser will split on the FIRST dash in the filename.
	// It would think the path is "my" and the content is "folder/file.txt-10-context..."
	assert.Contains(t, result, "\"my-folder/file.txt\":10:context line here")
	assert.Contains(t, result, "\"my-folder/file.txt\":11:match line")
}


func TestGrep_Parity_Flags_Aliases(t *testing.T) {
	fs := &mockFileSystem{}
	exec := &mockCommandExecutor{}
	pathResolver := &mockPathResolver{}; setupMockResolver(pathResolver)

	fs.On("Stat", "/workspace").Return(&toolMockFileInfo{name: "workspace", isDir: true}, nil).Maybe()

	tool := NewGrepTool(fs, exec, pathResolver)
	
	// Case 1: Short flag 'i'
	req1 := &GrepRequest{Pattern: "apple", I: new(true)}
	exec.On("Run", mock.Anything, mock.MatchedBy(func(args []string) bool {
		foundI := false
		for _, a := range args { if a == "-i" { foundI = true } }
		return foundI
	}), "/workspace", mock.Anything).Return(&executor.Result{Stdout: "", ExitCode: 0}, nil).Once()

	_, _ = executeSearch(t, tool, req1)

	// Case 2: Alias 'show_line_numbers'
	req2 := &GrepRequest{Pattern: "apple", ShowLineNumbers: new(false), OutputMode: "content"}
	exec.On("Run", mock.Anything, mock.MatchedBy(func(args []string) bool {
		foundN := false
		for _, a := range args { if a == "-n" { foundN = true } }
		return !foundN // Expect NO -n
	}), "/workspace", mock.Anything).Return(&executor.Result{Stdout: "", ExitCode: 0}, nil).Once()

	_, _ = executeSearch(t, tool, req2)
	exec.AssertExpectations(t)
}

