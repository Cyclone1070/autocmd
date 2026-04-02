package file

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/stretchr/testify/assert"
)

// Local mocks for write tests

// mockFileInfoForWrite implements os.FileInfo for testing
type mockFileInfoForWrite struct {
	name  string
	size  int64
	mode  os.FileMode
	isDir bool
}

func (m *mockFileInfoForWrite) Name() string       { return m.name }
func (m *mockFileInfoForWrite) Size() int64        { return m.size }
func (m *mockFileInfoForWrite) Mode() os.FileMode  { return m.mode }
func (m *mockFileInfoForWrite) ModTime() time.Time { return time.Time{} }
func (m *mockFileInfoForWrite) IsDir() bool        { return m.isDir }
func (m *mockFileInfoForWrite) Sys() any           { return nil }

type fileEntry struct {
	content []byte
	mode    os.FileMode
}

type symlinkEntry struct {
	target string
}

// mockFileSystemForWrite provides comprehensive filesystem mocking for write tests
type mockFileSystemForWrite struct {
	files           map[string]fileEntry
	dirs            map[string]bool
	symlinks        map[string]symlinkEntry
	operationErrors map[string]error
	maxFileSize     int64
}

func newMockFileSystemForWrite(maxFileSize int64) *mockFileSystemForWrite {
	return &mockFileSystemForWrite{
		files:           make(map[string]fileEntry),
		dirs:            make(map[string]bool),
		symlinks:        make(map[string]symlinkEntry),
		operationErrors: make(map[string]error),
		maxFileSize:     maxFileSize,
	}
}

func TestWriteFile_ExecuteCancelled_ReturnsToolErrorCancelledDisplay(t *testing.T) {
	workspaceRoot := "/workspace"
	fs := newMockFileSystemForWrite(1024 * 1024)
	fs.dirs["/workspace"] = true
	checksumManager := newMockChecksumManagerForWrite()
	wtool := NewWriteFileTool(fs, checksumManager, &mockPathResolver{workspaceRoot: workspaceRoot}, 1024*1024)

	req := &WriteFileRequest{Path: "a.txt", Content: "hello", Comment: "write"}
	params, _ := json.Marshal(req)
	inv, err := wtool.Prepare(string(params))
	assert.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, disp, execErr := inv.(domain.ExecutableInvocation).Execute(ctx)

	assert.ErrorIs(t, execErr, context.Canceled)
	assert.NotNil(t, disp)
	assert.Equal(t, domain.ToolErrorCancelled, disp.GetError())
}

type mockPathResolver struct {
	workspaceRoot string
}

func (m *mockPathResolver) Abs(p string) (string, error) {
	if strings.Contains(p, "..") {
		return "", fmt.Errorf("path %s is outside workspace", p)
	}
	if strings.HasPrefix(p, "/") {
		return p, nil
	}
	return filepath.Join(m.workspaceRoot, p), nil
}

func (m *mockPathResolver) Rel(path string) (string, error) {
	return filepath.Rel(m.workspaceRoot, path)
}

func (m *mockFileSystemForWrite) createFile(path string, content []byte, mode os.FileMode) {
	m.files[path] = fileEntry{content: content, mode: mode}
}

func (m *mockFileSystemForWrite) setOperationError(operation string, err error) {
	m.operationErrors[operation] = err
}

func (m *mockFileSystemForWrite) Stat(path string) (os.FileInfo, error) {
	// Check symlinks first
	if link, ok := m.symlinks[path]; ok {
		// Follow symlink
		return m.Stat(link.target)
	}

	if m.dirs[path] {
		return &mockFileInfoForWrite{name: filepath.Base(path), isDir: true, mode: 0755}, nil
	}
	if entry, ok := m.files[path]; ok {
		return &mockFileInfoForWrite{name: filepath.Base(path), size: int64(len(entry.content)), mode: entry.mode}, nil
	}
	return nil, os.ErrNotExist
}

func (m *mockFileSystemForWrite) ReadFile(path string) ([]byte, error) {
	entry, ok := m.files[path]
	if !ok {
		return nil, os.ErrNotExist
	}

	if m.maxFileSize > 0 && int64(len(entry.content)) > m.maxFileSize {
		return nil, fmt.Errorf("file too large: %d bytes exceeds limit %d", len(entry.content), m.maxFileSize)
	}

	return entry.content, nil
}

func (m *mockFileSystemForWrite) EnsureDirs(path string) error {
	if m.operationErrors["EnsureDirs"] != nil {
		return m.operationErrors["EnsureDirs"]
	}
	// Create all parent directories
	dir := filepath.Dir(path)
	parts := strings.Split(dir, "/")
	current := ""
	for _, part := range parts {
		if part == "" {
			continue
		}
		current = current + "/" + part
		m.dirs[current] = true
	}
	return nil
}

func (m *mockFileSystemForWrite) WriteFileAtomic(path string, content []byte, perm os.FileMode) error {
	// Check for injected error at the WriteFileAtomic level
	if m.operationErrors["WriteFileAtomic"] != nil {
		return m.operationErrors["WriteFileAtomic"]
	}

	// Simulate atomic write
	m.files[path] = fileEntry{
		content: content,
		mode:    perm,
	}
	return nil
}

type mockChecksumManagerForWrite struct {
	checksums map[string]string
}

func newMockChecksumManagerForWrite() *mockChecksumManagerForWrite {
	return &mockChecksumManagerForWrite{
		checksums: make(map[string]string),
	}
}

func (m *mockChecksumManagerForWrite) Compute(content []byte) string {
	return fmt.Sprintf("checksum-%d", len(content))
}

func (m *mockChecksumManagerForWrite) Get(path string) (string, bool) {
	checksum, ok := m.checksums[path]
	return checksum, ok
}

func (m *mockChecksumManagerForWrite) Update(path, checksum string) {
	m.checksums[path] = checksum
}

func (m *mockChecksumManagerForWrite) Clear() {
	m.checksums = make(map[string]string)
}

func executeWrite(t *testing.T, wtool *WriteFileTool, req *WriteFileRequest) (string, error) {
	t.Helper()
	params, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Failed to marshal request: %v", err)
	}
	inv, err := wtool.Prepare(string(params))
	if err != nil {
		return "", err
	}
	out, _, err := inv.(domain.ExecutableInvocation).Execute(context.Background())
	return out, err
}

// Test functions

func TestWriteFile(t *testing.T) {
	workspaceRoot := "/workspace"
	maxFileSize := int64(1024 * 1024) // 1MB

	t.Run("create new file succeeds and updates cache", func(t *testing.T) {
		fs := newMockFileSystemForWrite(maxFileSize)
		checksumManager := newMockChecksumManagerForWrite()

		writeTool := NewWriteFileTool(fs, checksumManager, &mockPathResolver{workspaceRoot: workspaceRoot}, maxFileSize)
		content := "test content"

		req := &WriteFileRequest{Path: "new.txt", Content: content, Comment: "test comment"}
		_, err := executeWrite(t, writeTool, req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Verify file was created
		data, err := fs.ReadFile("/workspace/new.txt")
		if err != nil {
			t.Fatalf("failed to read created file: %v", err)
		}
		if string(data) != content {
			t.Errorf("expected content %q, got %q", content, string(data))
		}

		// Verify cache was updated
		checksum, ok := checksumManager.Get("/workspace/new.txt")
		if !ok {
			t.Error("expected cache to be updated after write")
		}
		if checksum == "" {
			t.Error("expected non-empty checksum in cache")
		}
	})

	t.Run("existing file rejection", func(t *testing.T) {
		fs := newMockFileSystemForWrite(maxFileSize)
		checksumManager := newMockChecksumManagerForWrite()
		fs.createFile("/workspace/existing.txt", []byte("existing"), 0o644)

		writeTool := NewWriteFileTool(fs, checksumManager, &mockPathResolver{workspaceRoot: workspaceRoot}, maxFileSize)

		req := &WriteFileRequest{Path: "existing.txt", Content: "new content", Comment: "test comment"}
		_, err := executeWrite(t, writeTool, req)
		if err == nil {
			t.Errorf("expected error for existing file, got nil")
		}
	})

	t.Run("large content rejection", func(t *testing.T) {
		fs := newMockFileSystemForWrite(maxFileSize)
		checksumManager := newMockChecksumManagerForWrite()

		// Create content larger than limit
		largeContent := make([]byte, maxFileSize+1)
		for i := range largeContent {
			largeContent[i] = 'A'
		}

		writeTool := NewWriteFileTool(fs, checksumManager, &mockPathResolver{workspaceRoot: workspaceRoot}, maxFileSize)

		req := &WriteFileRequest{Path: "large.txt", Content: string(largeContent), Comment: "test comment"}
		_, err := executeWrite(t, writeTool, req)
		if err == nil {
			t.Errorf("expected error for large content, got nil")
		}
	})

	t.Run("binary content rejection", func(t *testing.T) {
		fs := newMockFileSystemForWrite(maxFileSize)
		checksumManager := newMockChecksumManagerForWrite()

		writeTool := NewWriteFileTool(fs, checksumManager, &mockPathResolver{workspaceRoot: workspaceRoot}, maxFileSize)
		// Content with NUL byte
		binaryContent := []byte{0x48, 0x65, 0x6C, 0x00, 0x6C, 0x6F}

		req := &WriteFileRequest{Path: "binary.bin", Content: string(binaryContent), Comment: "test comment"}
		_, err := executeWrite(t, writeTool, req)
		if err == nil {
			t.Errorf("expected error for binary content, got nil")
		}
	})

	t.Run("verify default permissions 0o644", func(t *testing.T) {
		fs := newMockFileSystemForWrite(maxFileSize)
		checksumManager := newMockChecksumManagerForWrite()
		writeTool := NewWriteFileTool(fs, checksumManager, &mockPathResolver{workspaceRoot: workspaceRoot}, maxFileSize)

		expectedPerm := os.FileMode(0o644)

		req := &WriteFileRequest{Path: "default_perm.txt", Content: "content", Comment: "test comment"}
		_, err := executeWrite(t, writeTool, req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		info, err := fs.Stat("/workspace/default_perm.txt")
		if err != nil {
			t.Fatalf("failed to stat file: %v", err)
		}

		if info.Mode().Perm() != expectedPerm {
			t.Errorf("expected permissions %o, got %o", expectedPerm, info.Mode().Perm())
		}
	})

	t.Run("nested directory creation", func(t *testing.T) {
		fs := newMockFileSystemForWrite(maxFileSize)
		checksumManager := newMockChecksumManagerForWrite()
		writeTool := NewWriteFileTool(fs, checksumManager, &mockPathResolver{workspaceRoot: workspaceRoot}, maxFileSize)

		req := &WriteFileRequest{Path: "nested/deep/file.txt", Content: "content", Comment: "test comment"}
		_, err := executeWrite(t, writeTool, req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Verify file was created
		data, err := fs.ReadFile("/workspace/nested/deep/file.txt")
		if err != nil {
			t.Fatalf("failed to read created file: %v", err)
		}
		if string(data) != "content" {
			t.Errorf("expected content %q, got %q", "content", string(data))
		}
	})

	t.Run("ensure dirs failure", func(t *testing.T) {
		fs := newMockFileSystemForWrite(maxFileSize)
		checksumManager := newMockChecksumManagerForWrite()
		fs.setOperationError("EnsureDirs", errors.New("failed to mkdir"))

		writeTool := NewWriteFileTool(fs, checksumManager, &mockPathResolver{workspaceRoot: workspaceRoot}, maxFileSize)

		req := &WriteFileRequest{Path: "nested/deep/file.txt", Content: "content", Comment: "test comment"}
		result, err := executeWrite(t, writeTool, req)
		if err == nil {
			t.Errorf("expected operation error for logging per tool.md contract")
		}
		if err.Error() != "Execution failed" {
			t.Errorf("expected 'Execution failed' error, got: %v", err)
		}
		if !strings.Contains(result, "failed to create directories") {
			t.Errorf("expected error message about directories, got: %s", result)
		}
	})

	t.Run("empty content allowed", func(t *testing.T) {
		fs := newMockFileSystemForWrite(maxFileSize)
		checksumManager := newMockChecksumManagerForWrite()

		writeTool := NewWriteFileTool(fs, checksumManager, &mockPathResolver{workspaceRoot: workspaceRoot}, maxFileSize)

		req := &WriteFileRequest{Path: "empty.txt", Content: "", Comment: "test comment"}
		_, err := executeWrite(t, writeTool, req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		data, err := fs.ReadFile("/workspace/empty.txt")
		if err != nil {
			t.Fatalf("failed to read created empty file: %v", err)
		}
		if len(data) != 0 {
			t.Errorf("expected empty content, got %q", string(data))
		}
	})

	t.Run("absolute path in request is normalized for display", func(t *testing.T) {
		fs := newMockFileSystemForWrite(maxFileSize)
		checksumManager := newMockChecksumManagerForWrite()
		workspaceRoot := "/workspace"
		absFile := "/workspace/subdir/new.txt"
		
		writeTool := NewWriteFileTool(fs, checksumManager, &mockPathResolver{workspaceRoot: workspaceRoot}, maxFileSize)
		
		// Agent sends absolute path
		params, _ := json.Marshal(&WriteFileRequest{Path: absFile, Content: "test", Comment: "test comment"})
		inv, err := writeTool.Prepare(string(params))
		if err != nil {
			t.Fatalf("Prepare failed: %v", err)
		}
		
		// Display should show "WRITE subdir/new.txt" normalized from subdir/new.txt
		display := inv.Display().(domain.StringDisplay)
		assert.Equal(t, "WRITE subdir/new.txt", display.Content)
	})
}
