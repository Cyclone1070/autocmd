package edit

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEditFile(t *testing.T) {
	workspaceRoot := "/workspace"
	maxFileSize := int64(1024 * 1024)

	t.Run("Edit single match successfully", func(t *testing.T) {
		fs := newMockFileOps()
		checksumManager := newMockChecksumManagerShared()
		fs.files["/workspace/test.txt"] = []byte("hello world")
		// Simulate read
		checksumManager.Update("/workspace/test.txt", checksumManager.Compute([]byte("hello world")))

		tool := NewTool(fs, checksumManager, &mockPathResolver{workspaceRoot: workspaceRoot}, maxFileSize)

		req := &Request{
			FilePath:    "/workspace/test.txt",
			Description: "changing hello to goodbye",
			OldString:   "hello",
			NewString:   "goodbye",
			ReplaceAll:  false,
		}

		params, _ := json.Marshal(req)
		_, err := tool.validate(string(params))
		require.NoError(t, err)

		out, runErr := tool.InvokableRun(context.Background(), string(params))
		require.NoError(t, runErr)
		assert.Equal(t, "The file /workspace/test.txt has been updated successfully.", out)
		assert.Equal(t, "goodbye world", string(fs.files["/workspace/test.txt"]))
	})

	t.Run("Edit all matches successfully", func(t *testing.T) {
		fs := newMockFileOps()
		checksumManager := newMockChecksumManagerShared()
		fs.files["/workspace/test.txt"] = []byte("a a a")
		// Simulate read
		checksumManager.Update("/workspace/test.txt", checksumManager.Compute([]byte("a a a")))

		tool := NewTool(fs, checksumManager, &mockPathResolver{workspaceRoot: workspaceRoot}, maxFileSize)

		req := &Request{
			FilePath:    "/workspace/test.txt",
			Description: "changing all a to b",
			OldString:   "a",
			NewString:   "b",
			ReplaceAll:  true,
		}

		params, _ := json.Marshal(req)
		_, err := tool.validate(string(params))
		require.NoError(t, err)

		out, _ := tool.InvokableRun(context.Background(), string(params))
		assert.Equal(t, "The file /workspace/test.txt has been updated. All occurrences were successfully replaced.", out)
		assert.Equal(t, "b b b", string(fs.files["/workspace/test.txt"]))
	})

	t.Run("Fails if multiple matches and replace_all is false", func(t *testing.T) {
		fs := newMockFileOps()
		checksumManager := newMockChecksumManagerShared()
		fs.files["/workspace/test.txt"] = []byte("a a")
		checksumManager.Update("/workspace/test.txt", checksumManager.Compute([]byte("a a")))

		tool := NewTool(fs, checksumManager, &mockPathResolver{workspaceRoot: workspaceRoot}, maxFileSize)

		req := &Request{
			FilePath:    "/workspace/test.txt",
			Description: "should fail",
			OldString:   "a",
			NewString:   "b",
			ReplaceAll:  false,
		}

		params, _ := json.Marshal(req)
		_, err := tool.validate(string(params))
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "found 2 matches of the string to replace, but replace_all is false")
	})

	t.Run("Create new file via empty old_string", func(t *testing.T) {
		fs := newMockFileOps()
		checksumManager := newMockChecksumManagerShared()
		// No file at /workspace/new.txt

		tool := NewTool(fs, checksumManager, &mockPathResolver{workspaceRoot: workspaceRoot}, maxFileSize)

		req := &Request{
			FilePath:    "/workspace/new.txt",
			Description: "creating new file",
			OldString:   "",
			NewString:   "brand new content",
			ReplaceAll:  false,
		}

		params, _ := json.Marshal(req)
		_, err := tool.validate(string(params))
		require.NoError(t, err)

		out, _ := tool.InvokableRun(context.Background(), string(params))
		assert.Equal(t, "The file /workspace/new.txt has been updated successfully.", out)
		assert.Equal(t, "brand new content", string(fs.files["/workspace/new.txt"]))
	})

	t.Run("Fail to create file if already exists with content", func(t *testing.T) {
		fs := newMockFileOps()
		checksumManager := newMockChecksumManagerShared()
		fs.files["/workspace/exists.txt"] = []byte("not empty")
		checksumManager.Update("/workspace/exists.txt", checksumManager.Compute([]byte("not empty")))

		tool := NewTool(fs, checksumManager, &mockPathResolver{workspaceRoot: workspaceRoot}, maxFileSize)

		req := &Request{
			FilePath:    "/workspace/exists.txt",
			Description: "trying to overwrite",
			OldString:   "",
			NewString:   "danger",
			ReplaceAll:  false,
		}

		params, _ := json.Marshal(req)
		_, err := tool.validate(string(params))
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cannot create new file - file already exists")
	})

	t.Run("Edit with curly quotes match", func(t *testing.T) {
		fs := newMockFileOps()
		checksumManager := newMockChecksumManagerShared()
		// File has curly quotes: “hello”
		fs.files["/workspace/test.txt"] = []byte("“hello”")
		checksumManager.Update("/workspace/test.txt", checksumManager.Compute([]byte("“hello”")))

		tool := NewTool(fs, checksumManager, &mockPathResolver{workspaceRoot: workspaceRoot}, maxFileSize)

		req := &Request{
			FilePath:    "/workspace/test.txt",
			Description: "matching curly with straight",
			OldString:   "\"hello\"", // LLM sends straight quotes
			NewString:   "\"goodbye\"",
			ReplaceAll:  false,
		}

		params, _ := json.Marshal(req)
		_, err := tool.validate(string(params))
		require.NoError(t, err)

		out, _ := tool.InvokableRun(context.Background(), string(params))
		assert.Equal(t, "The file /workspace/test.txt has been updated successfully.", out)
		// Should preserve curly quotes in the replacement: “goodbye”
		assert.Equal(t, "“goodbye”", string(fs.files["/workspace/test.txt"]))
	})

	t.Run("Strips trailing whitespace from new_string", func(t *testing.T) {
		fs := newMockFileOps()
		checksumManager := newMockChecksumManagerShared()
		fs.files["/workspace/test.go"] = []byte("package main\n\nfunc main() {}\n")
		checksumManager.Update("/workspace/test.go", checksumManager.Compute([]byte("package main\n\nfunc main() {}\n")))

		tool := NewTool(fs, checksumManager, &mockPathResolver{workspaceRoot: workspaceRoot}, maxFileSize)

		req := &Request{
			FilePath:    "/workspace/test.go",
			Description: "adding comment",
			OldString:   "func main() {}",
			NewString:   "func main() {} // comment    ", // Trailing spaces
			ReplaceAll:  false,
		}

		params, _ := json.Marshal(req)
		_, err := tool.validate(string(params))
		require.NoError(t, err)

		_, _ = tool.InvokableRun(context.Background(), string(params))
		// Spaces should be stripped
		assert.Equal(t, "package main\n\nfunc main() {} // comment\n", string(fs.files["/workspace/test.go"]))
	})

	t.Run("Does not strip trailing whitespace from markdown", func(t *testing.T) {
		fs := newMockFileOps()
		checksumManager := newMockChecksumManagerShared()
		fs.files["/workspace/test.md"] = []byte("# Title\n")
		checksumManager.Update("/workspace/test.md", checksumManager.Compute([]byte("# Title\n")))

		tool := NewTool(fs, checksumManager, &mockPathResolver{workspaceRoot: workspaceRoot}, maxFileSize)

		req := &Request{
			FilePath:    "/workspace/test.md",
			Description: "adding line break",
			OldString:   "# Title",
			NewString:   "# Title  ", // Markdown hard line break (2 spaces)
			ReplaceAll:  false,
		}

		params, _ := json.Marshal(req)
		_, err := tool.validate(string(params))
		require.NoError(t, err)

		_, _ = tool.InvokableRun(context.Background(), string(params))
		// Spaces should BEAUTIFULLY remain
		assert.Equal(t, "# Title  \n", string(fs.files["/workspace/test.md"]))
	})

	t.Run("Rejects relative path", func(t *testing.T) {
		fs := newMockFileOps()
		checksumManager := newMockChecksumManagerShared()
		tool := NewTool(fs, checksumManager, &mockPathResolver{workspaceRoot: workspaceRoot}, maxFileSize)

		req := &Request{
			FilePath:    "test.txt",
			Description: "should fail",
			OldString:   "hello",
			NewString:   "goodbye",
			ReplaceAll:  false,
		}

		params, _ := json.Marshal(req)
		_, err := tool.validate(string(params))
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "absolute path required")
	})

	t.Run("Fails if existing file has not been read", func(t *testing.T) {
		fs := newMockFileOps()
		checksumManager := newMockChecksumManagerShared()
		fs.files["/workspace/test.txt"] = []byte("hello world")
		// Do NOT simulate read (checksumManager is empty)

		tool := NewTool(fs, checksumManager, &mockPathResolver{workspaceRoot: workspaceRoot}, maxFileSize)

		req := &Request{
			FilePath:    "/workspace/test.txt",
			Description: "changing hello to goodbye",
			OldString:   "hello",
			NewString:   "goodbye",
			ReplaceAll:  false,
		}

		params, _ := json.Marshal(req)
		_, err := tool.validate(string(params))
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "file has not been read yet; read it first before editing it")
	})
}

// Shared mocks used by file package tests.
type mockPathResolver struct {
	workspaceRoot string
}

func (m *mockPathResolver) ValidateAbs(p string) (string, error) {
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
