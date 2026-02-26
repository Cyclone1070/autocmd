package search

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Cyclone1070/iav/internal/config"
	"github.com/Cyclone1070/iav/internal/tool/service/executor"
	"github.com/Cyclone1070/iav/internal/tool/service/path"
)

// Local mocks for find tests

type mockFileInfoForFind struct {
	name  string
	isDir bool
}

func (m *mockFileInfoForFind) Name() string       { return m.name }
func (m *mockFileInfoForFind) Size() int64        { return 0 }
func (m *mockFileInfoForFind) Mode() os.FileMode  { return 0o644 }
func (m *mockFileInfoForFind) ModTime() time.Time { return time.Time{} }
func (m *mockFileInfoForFind) IsDir() bool        { return m.isDir }
func (m *mockFileInfoForFind) Sys() any           { return nil }

type mockFileSystemForFind struct {
	dirs map[string]bool
}

func newMockFileSystemForFind() *mockFileSystemForFind {
	return &mockFileSystemForFind{
		dirs: make(map[string]bool),
	}
}

func (m *mockFileSystemForFind) createDir(path string) {
	m.dirs[path] = true
}

func (m *mockFileSystemForFind) Stat(path string) (os.FileInfo, error) {
	if m.dirs[path] {
		return &mockFileInfoForFind{name: path, isDir: true}, nil
	}
	return nil, os.ErrNotExist
}

type mockCommandExecutorForFind struct {
	runFunc func(ctx context.Context, cmd []string, dir string, env []string) (*executor.Result, error)
}

func (m *mockCommandExecutorForFind) Run(ctx context.Context, cmd []string, dir string, env []string) (*executor.Result, error) {
	if m.runFunc != nil {
		return m.runFunc(ctx, cmd, dir, env)
	}
	return nil, fmt.Errorf("not implemented")
}

func executeFind(ctx context.Context, t *FindFileTool, req *FindFileRequest) (string, error) {
	params, err := json.Marshal(req)
	if err != nil {
		return "", err
	}

	invocation, err := t.Prepare(ctx, params)
	if err != nil {
		return "", err
	}

	return invocation.Execute(ctx)
}

// Test functions

func TestFindFile_BasicGlob(t *testing.T) {
	fs := newMockFileSystemForFind()
	fs.createDir("/workspace")
	workspaceRoot := "/workspace"
	cfg := config.DefaultConfig()

	mockRunner := &mockCommandExecutorForFind{}
	mockRunner.runFunc = func(ctx context.Context, cmd []string, dir string, env []string) (*executor.Result, error) {
		output := "/workspace/a/b/file.go\n/workspace/a/file.go\n"
		return &executor.Result{Stdout: output, ExitCode: 0}, nil
	}

	findTool := NewFindFileTool(fs, mockRunner, cfg, path.NewResolver(workspaceRoot))

	req := &FindFileRequest{Pattern: "*.go", Path: ""}
	resp, err := executeFind(context.Background(), findTool, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	matches := strings.Split(strings.TrimSpace(resp), "\n")
	if len(matches) != 2 {
		t.Fatalf("expected 2 matches, got %d. Output: %q", len(matches), resp)
	}

	// Note: We removed alphabetical sorting, so order depends on mock output (which is preserved)
	expectedMatches := []string{"a/b/file.go", "a/file.go"}
	for i, expected := range expectedMatches {
		if matches[i] != expected {
			t.Errorf("match %d: expected %q, got %q", i, expected, matches[i])
		}
	}
}

func TestFindFile_InvalidGlob(t *testing.T) {
	fs := newMockFileSystemForFind()
	fs.createDir("/workspace")
	workspaceRoot := "/workspace"
	cfg := config.DefaultConfig()

	findTool := NewFindFileTool(fs, &mockCommandExecutorForFind{}, cfg, path.NewResolver(workspaceRoot))

	req := &FindFileRequest{Pattern: "[", Path: ""}
	_, err := executeFind(context.Background(), findTool, req)
	if err == nil {
		t.Fatalf("expected error for invalid glob, got nil")
	}
	if !strings.Contains(err.Error(), "invalid pattern") {
		t.Errorf("expected invalid pattern error, got: %v", err)
	}
}

func TestFindFile_NonExistentPath(t *testing.T) {
	fs := newMockFileSystemForFind()
	fs.createDir("/workspace")
	workspaceRoot := "/workspace"
	cfg := config.DefaultConfig()

	findTool := NewFindFileTool(fs, &mockCommandExecutorForFind{}, cfg, path.NewResolver(workspaceRoot))

	req := &FindFileRequest{Pattern: "*.go", Path: "nonexistent/dir"}
	_, err := executeFind(context.Background(), findTool, req)
	if err == nil {
		t.Fatalf("expected error for non-existent path, got nil")
	}
	if !strings.Contains(err.Error(), "path does not exist") {
		t.Errorf("expected path does not exist error, got: %v", err)
	}
}

func TestFindFile_CommandFailure(t *testing.T) {
	fs := newMockFileSystemForFind()
	fs.createDir("/workspace")
	workspaceRoot := "/workspace"
	cfg := config.DefaultConfig()

	mockRunner := &mockCommandExecutorForFind{}
	mockRunner.runFunc = func(ctx context.Context, cmd []string, dir string, env []string) (*executor.Result, error) {
		return &executor.Result{Stdout: "", Stderr: "fd error", ExitCode: 2}, nil
	}

	findTool := NewFindFileTool(fs, mockRunner, cfg, path.NewResolver(workspaceRoot))

	req := &FindFileRequest{Pattern: "*.go", Path: ""}
	resp, err := executeFind(context.Background(), findTool, req)
	// Operation error: err returned for logging, loop continues
	if err == nil {
		t.Fatal("expected operation error to be returned for logging")
	}
	if !strings.Contains(resp, "Error: fd failed with exit code 2: fd error") {
		t.Errorf("expected error message in output, got: %q", resp)
	}
}

func TestFindFile_NoMatches(t *testing.T) {
	fs := newMockFileSystemForFind()
	fs.createDir("/workspace")
	workspaceRoot := "/workspace"
	cfg := config.DefaultConfig()

	mockRunner := &mockCommandExecutorForFind{}
	mockRunner.runFunc = func(ctx context.Context, cmd []string, dir string, env []string) (*executor.Result, error) {
		return &executor.Result{Stdout: "", ExitCode: 0}, nil
	}

	findTool := NewFindFileTool(fs, mockRunner, cfg, path.NewResolver(workspaceRoot))

	req := &FindFileRequest{Pattern: "*.nonexistent", Path: ""}
	resp, err := executeFind(context.Background(), findTool, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp != "No matches found." {
		t.Errorf("expected 'No matches found.', got output: %q", resp)
	}
}

func TestFindFile_HitMaxResults(t *testing.T) {
	fs := newMockFileSystemForFind()
	fs.createDir("/workspace")
	workspaceRoot := "/workspace"
	cfg := config.DefaultConfig()

	mockRunner := &mockCommandExecutorForFind{}
	mockRunner.runFunc = func(ctx context.Context, cmd []string, dir string, env []string) (*executor.Result, error) {
		var output strings.Builder
		for i := range 150 {
			fmt.Fprintf(&output, "/workspace/file%d.go\n", i)
		}
		return &executor.Result{Stdout: output.String(), ExitCode: 0}, nil
	}

	findTool := NewFindFileTool(fs, mockRunner, cfg, path.NewResolver(workspaceRoot))

	req := &FindFileRequest{Pattern: "*.go", Path: ""}
	resp, err := executeFind(context.Background(), findTool, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(resp, "(Results truncated. Consider using a more specific pattern.)") {
		t.Error("expected truncation message in output")
	}

	lines := strings.Split(strings.TrimSpace(resp), "\n")
	// 100 results + 1 empty line (from trim space) + truncation message? No, Split on TrimSpace(resp)
	// resp format: "file0.go\n...\nfile99.go\n\n(Results truncated...)"
	// TrimSpace(resp) format: "file0.go\n...\nfile99.go\n\n(Results truncated...)"
	// Split(\n) will give matching lines and then the truncation message.

	count := 0
	for _, line := range lines {
		if strings.HasSuffix(line, ".go") {
			count++
		}
	}
	if count != 100 {
		t.Errorf("expected 100 matches, got %d", count)
	}
}

func TestFindFile_PrepareValidation(t *testing.T) {
	fs := newMockFileSystemForFind()
	fs.createDir("/workspace")
	workspaceRoot := "/workspace"
	cfg := config.DefaultConfig()
	findTool := NewFindFileTool(fs, &mockCommandExecutorForFind{}, cfg, path.NewResolver(workspaceRoot))

	tests := []struct {
		name    string
		pattern string
		path    string
		wantErr bool
	}{
		{"Valid", "*.txt", "", false},
		{"EmptyPattern", "", "", true},
		{"InvalidPattern", "[", "", true},
		{"NonExistentPath", "*.txt", "missing", true},
		{"PathOutside", "*.txt", "../../outside", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &FindFileRequest{Pattern: tt.pattern, Path: tt.path}
			params, _ := json.Marshal(req)
			_, err := findTool.Prepare(context.Background(), params)
			if (err != nil) != tt.wantErr {
				t.Errorf("Prepare() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
