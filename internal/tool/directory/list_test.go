package directory

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/tool/service/path"
)

// -- Local Mocks (Preserved from original test) --

type mockFileInfoForList struct {
	name  string
	size  int64
	mode  os.FileMode
	isDir bool
}

func (m *mockFileInfoForList) Name() string       { return m.name }
func (m *mockFileInfoForList) Size() int64        { return m.size }
func (m *mockFileInfoForList) Mode() os.FileMode  { return m.mode }
func (m *mockFileInfoForList) ModTime() time.Time { return time.Time{} }
func (m *mockFileInfoForList) IsDir() bool        { return m.isDir }
func (m *mockFileInfoForList) Sys() any           { return nil }

// mockDirEntry wraps mockFileInfoForList to implement os.DirEntry
type mockDirEntry struct {
	*mockFileInfoForList
}

func (m *mockDirEntry) Type() os.FileMode {
	return m.mode
}

func (m *mockDirEntry) Info() (os.FileInfo, error) {
	return m.mockFileInfoForList, nil
}

type mockFileSystemForList struct {
	files    map[string][]byte
	dirs     map[string]bool
	symlinks map[string]string
	errors   map[string]error
}

func newMockFileSystemForList() *mockFileSystemForList {
	return &mockFileSystemForList{
		files:    make(map[string][]byte),
		dirs:     make(map[string]bool),
		symlinks: make(map[string]string),
		errors:   make(map[string]error),
	}
}

func (m *mockFileSystemForList) createFile(path string, content []byte, mode os.FileMode) {
	m.files[path] = content
	m.dirs[path] = false
}

func (m *mockFileSystemForList) createDir(path string) {
	m.dirs[path] = true
}

func (m *mockFileSystemForList) remove(path string) {
	delete(m.files, path)
	delete(m.dirs, path)
	delete(m.symlinks, path)
}

func (m *mockFileSystemForList) Stat(path string) (os.FileInfo, error) {
	if err, ok := m.errors[path]; ok {
		return nil, err
	}

	// Follow symlinks
	finalPath := path
	for {
		if target, ok := m.symlinks[finalPath]; ok {
			finalPath = target
		} else {
			break
		}
	}

	if isDir, ok := m.dirs[finalPath]; ok {
		mode := os.FileMode(0755)
		if !isDir {
			mode = 0o644
		}
		if isDir {
			mode |= os.ModeDir
		}
		return &mockFileInfoForList{
			name:  filepath.Base(finalPath),
			size:  0,
			mode:  mode,
			isDir: isDir,
		}, nil
	}

	if content, ok := m.files[finalPath]; ok {
		return &mockFileInfoForList{
			name:  filepath.Base(finalPath),
			size:  int64(len(content)),
			mode:  0o644,
			isDir: false,
		}, nil
	}

	return nil, os.ErrNotExist
}

func (m *mockFileSystemForList) ListDir(path string) ([]os.DirEntry, error) {
	if err, ok := m.errors[path]; ok {
		return nil, err
	}

	// Follow symlinks for the directory check
	finalPath := path
	for {
		if target, ok := m.symlinks[finalPath]; ok {
			finalPath = target
		} else {
			break
		}
	}

	if isDir, ok := m.dirs[finalPath]; !ok || !isDir {
		return nil, fmt.Errorf("not a directory")
	}

	var entries []os.DirEntry
	prefix := finalPath
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}

	// Find all direct children
	seen := make(map[string]bool)

	// Helper to collect direct children
	collectChildren := func(pathStr string) {
		if after, ok := strings.CutPrefix(pathStr, prefix); ok {
			rel := after
			parts := strings.Split(rel, "/")
			if len(parts) > 0 && parts[0] != "" && !seen[parts[0]] {
				seen[parts[0]] = true
				childPath := filepath.Join(finalPath, parts[0])
				info, err := m.Stat(childPath)
				if err != nil {
					// panic(fmt.Sprintf("mock setup error: failed to stat child %s: %v", childPath, err))
					return // Ignore if stat fails during list
				}
				if info != nil {
					// Wrap FileInfo in DirEntry
					if mockInfo, ok := info.(*mockFileInfoForList); ok {
						entries = append(entries, &mockDirEntry{mockInfo})
					}
				}
			}
		}
	}

	// Check files
	for p := range m.files {
		collectChildren(p)
	}
	// Also check dirs
	for p, isDir := range m.dirs {
		if isDir {
			collectChildren(p)
		}
	}
	// Also check symlinks
	for p := range m.symlinks {
		collectChildren(p)
	}

	return entries, nil
}

// -- TESTS --

func TestListDirTool_Validation(t *testing.T) {
	workspaceRoot := "/workspace"
	fs := newMockFileSystemForList()
	fs.createDir("/workspace")
	fs.createDir("/workspace/src")
	fs.createFile("/workspace/file.txt", []byte("content"), 0o644)

	toolInstance := NewListDirectoryTool(fs, path.NewResolver(workspaceRoot), nil)

	tests := []struct {
		name    string
		params  ListDirRequest
		wantErr string
	}{
		{
			name:    "Missing Path",
			params:  ListDirRequest{Path: ""},
			wantErr: "path is required",
		},
		{
			name:    "Path Does Not Exist",
			params:  ListDirRequest{Path: "ghost"},
			wantErr: "path does not exist",
		},
		{
			name:    "Path Is Not Directory",
			params:  ListDirRequest{Path: "file.txt"},
			wantErr: "not a directory",
		},
		{
			name:    "Path Outside Workspace",
			params:  ListDirRequest{Path: "../outside"},
			wantErr: "outside workspace root",
		},
		{
			name:   "Valid Request",
			params: ListDirRequest{Path: "src"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jsonParams, _ := json.Marshal(tt.params)
			_, err := toolInstance.Prepare(context.Background(), string(jsonParams))

			if tt.wantErr != "" {
				if err == nil {
					t.Errorf("Prepare() error = nil, wantErr %q", tt.wantErr)
				} else if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("Prepare() error = %v, wantErr %q", err, tt.wantErr)
				}
			} else {
				if err != nil {
					t.Errorf("Prepare() unexpected error: %v", err)
				}
			}
		})
	}
}

func TestListDirTool_Execute_TreeOutput(t *testing.T) {
	workspaceRoot := "/workspace"
	fs := newMockFileSystemForList()
	fs.createDir("/workspace")
	fs.createDir("/workspace/src")
	fs.createDir("/workspace/src/components")
	fs.createFile("/workspace/src/main.go", []byte{}, 0644)
	fs.createFile("/workspace/src/utils.go", []byte{}, 0644)
	fs.createFile("/workspace/README.md", []byte{}, 0644)

	toolInstance := NewListDirectoryTool(fs, path.NewResolver(workspaceRoot), nil)

	// Prepare
	req := ListDirRequest{Path: "src"}
	jsonParams, _ := json.Marshal(req)
	invocation, err := toolInstance.Prepare(context.Background(), string(jsonParams))
	if err != nil {
		t.Fatalf("Prepare failed: %v", err)
	}

	// Execute
	output, _, err := invocation.Execute(context.Background())
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// Verify Display
	display := invocation.Display()
	content, ok := display.(domain.StringDisplay)
	if !ok {
		t.Errorf("Display() returned wrong type")
	} else if !strings.Contains(content.Content, "LIST src") {
		t.Errorf("Display() content mismatch: %s", content.Content)
	}

	// Verify Tree Output
	expectedLines := []string{
		"/workspace/src/",
		"  components/",
		"  main.go",
		"  utils.go",
	}
	for _, exp := range expectedLines {
		if !strings.Contains(output, exp) {
			t.Errorf("Output missing expected line: %q\nGot:\n%s", exp, output)
		}
	}

	// Verify Sorting (dirs first)
	utilsIdx := strings.Index(output, "utils.go")
	compIdx := strings.Index(output, "components/")
	if compIdx == -1 || utilsIdx == -1 {
		t.Errorf("Missing entries in sorting check")
	} else if compIdx > utilsIdx {
		t.Errorf("Sorting error: directories should come before files. components/ at %d, utils.go at %d", compIdx, utilsIdx)
	}

	t.Run("absolute path in request is normalized for display", func(t *testing.T) {
		absDir := "/workspace/src/components"
		req := ListDirRequest{Path: absDir}
		jsonParams, _ := json.Marshal(req)
		
		invocation, err := toolInstance.Prepare(context.Background(), string(jsonParams))
		if err != nil {
			t.Fatalf("Prepare failed: %v", err)
		}
		
		display := invocation.Display().(domain.StringDisplay)
		// Current behavior uses filepath.Base(absPath) -> "components"
		// Desired behavior: "src/components" (relative to /workspace)
		if strings.Contains(display.Content, "LIST /workspace/src/components") {
			t.Errorf("Display should not contain absolute path: %s", display.Content)
		}
		if !strings.Contains(display.Content, "LIST src/components") {
			t.Errorf("Display should contain relative path 'src/components', got: %s", display.Content)
		}
	})
}

func TestListDirTool_Execute_Truncation(t *testing.T) {
	workspaceRoot := "/workspace"
	fs := newMockFileSystemForList()
	fs.createDir("/workspace")
	fs.createDir("/workspace/big")

	// Create 105 files
	for i := range 105 {
		fs.createFile(fmt.Sprintf("/workspace/big/file_%d.txt", i), []byte{}, 0644)
	}

	toolInstance := NewListDirectoryTool(fs, path.NewResolver(workspaceRoot), nil)
	toolInstance.maxResults = 100 // Override for testing truncation

	req := ListDirRequest{Path: "big"}
	jsonParams, _ := json.Marshal(req)
	invocation, err := toolInstance.Prepare(context.Background(), string(jsonParams))
	if err != nil {
		t.Fatalf("Prepare failed: %v", err)
	}

	output, _, err := invocation.Execute(context.Background())
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if !strings.Contains(output, "(Results truncated.") {
		t.Error("Output missing truncation warning")
	}
	if !strings.Contains(output, "5 items hidden") {
		t.Errorf("Output missing correct count in warning. Got:\n%s", output)
	}
}

func TestListDirTool_Execute_Ignore(t *testing.T) {
	workspaceRoot := "/workspace"
	fs := newMockFileSystemForList()
	fs.createDir("/workspace")
	fs.createFile("/workspace/file.txt", []byte{}, 0644)
	fs.createFile("/workspace/test.log", []byte{}, 0644)

	toolInstance := NewListDirectoryTool(fs, path.NewResolver(workspaceRoot), nil)

	req := ListDirRequest{
		Path:   ".",
		Ignore: []string{"*.log"},
	}
	jsonParams, _ := json.Marshal(req)
	invocation, err := toolInstance.Prepare(context.Background(), string(jsonParams))
	if err != nil {
		t.Fatalf("Prepare failed: %v", err)
	}

	output, _, err := invocation.Execute(context.Background())
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if strings.Contains(output, "test.log") {
		t.Error("Output should have ignored test.log")
	}
	if !strings.Contains(output, "file.txt") {
		t.Error("Output should contain file.txt")
	}
}

func TestListDirTool_Execute_ReverificationSafety(t *testing.T) {
	workspaceRoot := "/workspace"
	fs := newMockFileSystemForList()
	fs.createDir("/workspace")
	fs.createDir("/workspace/temp")

	toolInstance := NewListDirectoryTool(fs, path.NewResolver(workspaceRoot), nil)

	// 1. Prepare (Success)
	req := ListDirRequest{Path: "temp"}
	jsonParams, _ := json.Marshal(req)
	invocation, err := toolInstance.Prepare(context.Background(), string(jsonParams))
	if err != nil {
		t.Fatalf("Prepare failed: %v", err)
	}

	// 2. Modify State (Delete Directory)
	fs.remove("/workspace/temp")

	// 3. Execute (Should return error for logging per tool.md, loop continues)
	output, _, err := invocation.Execute(context.Background())
	if err == nil {
		// Per new tool.md contract, operation errors return err for logging
		t.Fatalf("Expected operation error for logging per new tool.md contract")
	}
	if err.Error() != "Execution failed" {
		t.Errorf("Expected 'Execution failed' error, got: %v", err)
	}

	if !strings.Contains(output, "no longer exists") {
		t.Errorf("Expected TOCTOU error message, got: %s", output)
	}
}
