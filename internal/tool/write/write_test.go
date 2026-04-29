package write

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testWorkspaceRoot = "/workspace"

func TestWriteFile(t *testing.T) {
	workspaceRoot := testWorkspaceRoot
	maxFileSize := int64(1024 * 1024)

	t.Run("Create new file successfully", func(t *testing.T) {
		fs := newMockFileOps()
		checksumManager := newMockChecksumManagerShared()
		tool := NewTool(fs, checksumManager, &mockPathResolver{workspaceRoot: workspaceRoot}, maxFileSize)

		req := &Request{
			FilePath:    testWorkspaceRoot + "/new.txt",
			Content:     "hello",
			Description: "creating new file",
		}

		params, _ := json.Marshal(req)
		inv, err := tool.Prepare(string(params))
		require.NoError(t, err)

		out, _ := inv.(domain.ExecutableInvocation).Execute(context.Background())
		assert.Contains(t, out, "file " + testWorkspaceRoot + "/new.txt created successfully")
		assert.Equal(t, "hello", string(fs.files[testWorkspaceRoot + "/new.txt"]))

		display := inv.Display().(domain.StringDisplay)
		assert.Equal(t, "creating new file", display.Description)
	})

	t.Run("Overwrite existing file after read", func(t *testing.T) {
		fs := newMockFileOps()
		checksumManager := newMockChecksumManagerShared()
		fs.files[testWorkspaceRoot + "/exists.txt"] = []byte("old")
		// Simulate read
		checksumManager.Update(testWorkspaceRoot + "/exists.txt", checksumManager.Compute([]byte("old")))

		tool := NewTool(fs, checksumManager, &mockPathResolver{workspaceRoot: workspaceRoot}, maxFileSize)

		req := &Request{
			FilePath:    testWorkspaceRoot + "/exists.txt",
			Content:     "new",
			Description: "overwriting file",
		}

		params, _ := json.Marshal(req)
		inv, err := tool.Prepare(string(params))
		require.NoError(t, err)

		out, _ := inv.(domain.ExecutableInvocation).Execute(context.Background())
		assert.Contains(t, out, "The file " + testWorkspaceRoot + "/exists.txt has been updated successfully.")
		assert.Equal(t, "new", string(fs.files[testWorkspaceRoot + "/exists.txt"]))

		display := inv.Display().(domain.StringDisplay)
		assert.Equal(t, "overwriting file", display.Description)
	})

	t.Run("Rejects write if never read", func(t *testing.T) {
		fs := newMockFileOps()
		checksumManager := newMockChecksumManagerShared()
		fs.files[testWorkspaceRoot + "/exists.txt"] = []byte("old")
		// NO checksumManager.Update here

		tool := NewTool(fs, checksumManager, &mockPathResolver{workspaceRoot: workspaceRoot}, maxFileSize)

		req := &Request{
			FilePath: testWorkspaceRoot + "/exists.txt",
			Content:  "new",
		}

		params, _ := json.Marshal(req)
		_, err := tool.Prepare(string(params))
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "file has not been read yet")
	})

	t.Run("Rejects write if stale (mismatch checksum)", func(t *testing.T) {
		fs := newMockFileOps()
		checksumManager := newMockChecksumManagerShared()
		fs.files[testWorkspaceRoot + "/exists.txt"] = []byte("modified-externally")
		// Cache has "old"
		checksumManager.Update(testWorkspaceRoot + "/exists.txt", checksumManager.Compute([]byte("old")))

		tool := NewTool(fs, checksumManager, &mockPathResolver{workspaceRoot: workspaceRoot}, maxFileSize)

		req := &Request{
			FilePath: testWorkspaceRoot + "/exists.txt",
			Content:  "new",
		}

		params, _ := json.Marshal(req)
		_, err := tool.Prepare(string(params))
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "file has been modified since read")
	})

	t.Run("Normalizes line endings to LF", func(t *testing.T) {
		fs := newMockFileOps()
		checksumManager := newMockChecksumManagerShared()
		tool := NewTool(fs, checksumManager, &mockPathResolver{workspaceRoot: workspaceRoot}, maxFileSize)

		req := &Request{
			FilePath:    testWorkspaceRoot + "/crlf.txt",
			Content:     "line1\r\nline2",
			Description: "testing normalization",
		}

		params, _ := json.Marshal(req)
		inv, err := tool.Prepare(string(params))
		require.NoError(t, err)
		inv.(domain.ExecutableInvocation).Execute(context.Background())

		assert.Equal(t, "line1\nline2", string(fs.files[testWorkspaceRoot + "/crlf.txt"]))
	})

	t.Run("Rejects relative path", func(t *testing.T) {
		fs := newMockFileOps()
		checksumManager := newMockChecksumManagerShared()
		tool := NewTool(fs, checksumManager, &mockPathResolver{workspaceRoot: workspaceRoot}, maxFileSize)

		req := &Request{
			FilePath: "relative.txt",
			Content:  "content",
		}

		params, _ := json.Marshal(req)
		_, err := tool.Prepare(string(params))
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "absolute path required")
	})
}

// Shared mocks used by file package tests.
type mockPathResolver struct {
	workspaceRoot string
}

func (m *mockPathResolver) Abs(p string) (string, error) {
	if !filepath.IsAbs(p) {
		return "", fmt.Errorf("absolute path required, but got: %q", p)
	}
	return filepath.Clean(p), nil
}

func (m *mockPathResolver) DisplayPath(p string) string {
	return p
}

func (m *mockPathResolver) Root() string {
	return m.workspaceRoot
}

type toolMockFileInfo struct {
	name  string
	size  int64
	isDir bool
}

func (m toolMockFileInfo) Name() string       { return m.name }
func (m toolMockFileInfo) Size() int64        { return m.size }
func (m toolMockFileInfo) Mode() os.FileMode  { return 0o644 }
func (m toolMockFileInfo) ModTime() time.Time { return time.Time{} }
func (m toolMockFileInfo) IsDir() bool        { return m.isDir }
func (m toolMockFileInfo) Sys() any           { return nil }

type mockFileOps struct {
	files map[string][]byte
	dirs  map[string]bool
}

func newMockFileOps() *mockFileOps {
	return &mockFileOps{
		files: make(map[string][]byte),
		dirs:  make(map[string]bool),
	}
}

func (m *mockFileOps) ReadFile(path string) ([]byte, error) {
	if m.dirs[path] {
		return nil, fmt.Errorf("is a directory")
	}
	if c, ok := m.files[path]; ok {
		return c, nil
	}
	return nil, os.ErrNotExist
}

func (m *mockFileOps) Stat(path string) (os.FileInfo, error) {
	if m.dirs[path] {
		return toolMockFileInfo{name: path, isDir: true}, nil
	}
	if c, ok := m.files[path]; ok {
		return toolMockFileInfo{name: path, size: int64(len(c)), isDir: false}, nil
	}
	return nil, os.ErrNotExist
}

func (m *mockFileOps) WriteFileAtomic(path string, content []byte, _ os.FileMode) error {
	m.files[path] = content
	return nil
}

func (m *mockFileOps) EnsureDirs(path string) error {
	dir := filepath.Dir(path)
	m.dirs[dir] = true
	return nil
}

type mockChecksumManagerShared struct {
	checksums map[string]string
}

func newMockChecksumManagerShared() *mockChecksumManagerShared {
	return &mockChecksumManagerShared{
		checksums: make(map[string]string),
	}
}

func (m *mockChecksumManagerShared) Compute(data []byte) string {
	return fmt.Sprintf("checksum-%d", len(data))
}

func (m *mockChecksumManagerShared) Get(path string) (string, bool) {
	c, ok := m.checksums[path]
	return c, ok
}

func (m *mockChecksumManagerShared) Update(path string, checksum string) {
	m.checksums[path] = checksum
}
