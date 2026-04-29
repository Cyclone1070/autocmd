package read

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testWorkspaceRoot = "/workspace"

// Local mocks for read tests

type mockFileSystemForRead struct {
	files map[string][]byte
	dirs  map[string]bool
}

func newMockFileSystemForRead() *mockFileSystemForRead {
	return &mockFileSystemForRead{
		files: make(map[string][]byte),
		dirs:  make(map[string]bool),
	}
}

func (m *mockFileSystemForRead) createFile(path string, content []byte) {
	m.files[path] = content
}

func (m *mockFileSystemForRead) createDir(path string) {
	m.dirs[path] = true
}

func (m *mockFileSystemForRead) ReadFile(path string) ([]byte, error) {
	// Check if it's a directory
	if m.dirs[path] {
		return nil, fmt.Errorf("read %s: is a directory", path)
	}

	content, ok := m.files[path]
	if !ok {
		return nil, os.ErrNotExist
	}

	return content, nil
}

func (m *mockFileSystemForRead) Stat(path string) (os.FileInfo, error) {
	if m.dirs[path] {
		return mockFileInfoForRead{name: path, isDir: true}, nil
	}
	if content, ok := m.files[path]; ok {
		return mockFileInfoForRead{name: path, size: int64(len(content)), isDir: false}, nil
	}
	return nil, os.ErrNotExist
}

type mockFileInfoForRead struct {
	name  string
	size  int64
	isDir bool
}

func (m mockFileInfoForRead) Name() string       { return m.name }
func (m mockFileInfoForRead) Size() int64        { return m.size }
func (m mockFileInfoForRead) Mode() os.FileMode  { return 0o644 }
func (m mockFileInfoForRead) ModTime() time.Time { return time.Time{} }
func (m mockFileInfoForRead) IsDir() bool        { return m.isDir }
func (m mockFileInfoForRead) Sys() any           { return nil }

type mockChecksumManagerForRead struct {
	checksums map[string]string
}

func newMockChecksumManagerForRead() *mockChecksumManagerForRead {
	return &mockChecksumManagerForRead{
		checksums: make(map[string]string),
	}
}

func (m *mockChecksumManagerForRead) Compute(content []byte) string {
	return fmt.Sprintf("mock-checksum-%x", content)
}

func (m *mockChecksumManagerForRead) Update(path, checksum string) {
	m.checksums[path] = checksum
}

func (m *mockChecksumManagerForRead) Get(path string) (string, bool) {
	checksum, ok := m.checksums[path]
	return checksum, ok
}

// Test functions

// executeRead calls Prepare then Execute, returning the LLM output string.
// Prepare errors: returns ("", err)
// Execute errors: returns (llmContent, err) per contract.
func executeRead(t *testing.T, rtool *Tool, req *Request) (string, error) {
	t.Helper()
	params, _ := json.Marshal(req)
	inv, err := rtool.Prepare(string(params))
	if err != nil {
		return "", err
	}
	out, _ := inv.(domain.ExecutableInvocation).Execute(context.Background())
	return out, err
}

func TestReadFile(t *testing.T) {
	workspaceRoot := testWorkspaceRoot

	t.Run("Execute cancelled returns ToolErrorCancelled display", func(t *testing.T) {
		fs := newMockFileSystemForRead()
		fs.createDir(testWorkspaceRoot)
		fs.createFile("/workspace/a.txt", []byte("hello"))

		checksumManager := newMockChecksumManagerForRead()
		rtool := NewTool(fs, checksumManager, &mockPathResolver{workspaceRoot: workspaceRoot})

		req := &Request{FilePath: "/workspace/a.txt"}
		params, _ := json.Marshal(req)
		inv, err := rtool.Prepare(string(params))
		require.NoError(t, err)

		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, disp := inv.(domain.ExecutableInvocation).Execute(ctx)

		require.ErrorIs(t, ctx.Err(), context.Canceled)
		require.NotNil(t, disp)
		assert.Equal(t, domain.ToolErrorCancelled, disp.GetError())
	})

	t.Run("full read caches checksum", func(t *testing.T) {
		fs := newMockFileSystemForRead()
		checksumManager := newMockChecksumManagerForRead()
		contentStr := "test content"
		content := []byte(contentStr)
		fs.createFile("/workspace/test.txt", content)

		readTool := NewTool(fs, checksumManager, &mockPathResolver{workspaceRoot: workspaceRoot})

		readReq := &Request{FilePath: "/workspace/test.txt", Offset: 0, Limit: 100}
		output, err := executeRead(t, readTool, readReq)

		if err != nil {
			t.Fatalf("Execute failed: %v", err)
		}

		assertContains(t, output, "1\ttest content")
		assertContains(t, output, "(End of file - total 1 lines)")

		// Verify cache was updated
		checksum, ok := checksumManager.Get("/workspace/test.txt")
		if !ok {
			t.Error("expected cache to be updated after full read")
		}
		if checksum == "" {
			t.Error("expected non-empty checksum in cache")
		}
	})

	t.Run("partial read using offset and limit", func(t *testing.T) {
		fs := newMockFileSystemForRead()
		checksumManager := newMockChecksumManagerForRead()
		content := []byte("line1\nline2\nline3\nline4")
		fs.createFile("/workspace/test.txt", content)

		readTool := NewTool(fs, checksumManager, &mockPathResolver{workspaceRoot: workspaceRoot})

		// Read lines 2 and 3 (Offset=1, Limit=2)
		readReq := &Request{FilePath: "/workspace/test.txt", Offset: 1, Limit: 2}
		output, err := executeRead(t, readTool, readReq)

		if err != nil {
			t.Fatalf("Execute failed: %v", err)
		}

		assertContains(t, output, "2\tline2")
		assertContains(t, output, "3\tline3")
		assertContains(t, output, "(File has more lines. Use offset=3 to read more)")
	})

	t.Run("offset beyond EOF", func(t *testing.T) {
		fs := newMockFileSystemForRead()
		checksumManager := newMockChecksumManagerForRead()
		content := []byte("line1")
		fs.createFile("/workspace/test.txt", content)

		readTool := NewTool(fs, checksumManager, &mockPathResolver{workspaceRoot: workspaceRoot})
		offset := 100

		readReq := &Request{FilePath: "/workspace/test.txt", Offset: offset, Limit: 10}
		output, err := executeRead(t, readTool, readReq)

		if err != nil {
			t.Fatalf("Execute failed: %v", err)
		}

		assert.Contains(t, output, "<system-reminder>Warning: the file exists but is shorter than the provided offset (101). The file has 1 lines.</system-reminder>")
	})

	t.Run("directory rejection", func(t *testing.T) {
		fs := newMockFileSystemForRead()
		checksumManager := newMockChecksumManagerForRead()
		fs.createDir("/workspace/subdir")

		readTool := NewTool(fs, checksumManager, &mockPathResolver{workspaceRoot: workspaceRoot})

		readReq := &Request{FilePath: "/workspace/subdir"}
		params, _ := json.Marshal(readReq)
		_, err := readTool.Prepare(string(params))
		if err == nil {
			t.Error("expected Prepare to fail for directory")
		}
		if !strings.Contains(err.Error(), "directory") {
			t.Errorf("expected directory error message, got: %v", err)
		}
	})

	t.Run("file not found", func(t *testing.T) {
		fs := newMockFileSystemForRead()
		checksumManager := newMockChecksumManagerForRead()

		readTool := NewTool(fs, checksumManager, &mockPathResolver{workspaceRoot: workspaceRoot})

		readReq := &Request{FilePath: "/workspace/nonexistent.txt"}
		params, _ := json.Marshal(readReq)
		_, err := readTool.Prepare(string(params))
		if err == nil {
			t.Error("expected Prepare to fail for non-existent file")
		}
		if !strings.Contains(err.Error(), "does not exist") {
			t.Errorf("expected 'does not exist' error message, got: %v", err)
		}
	})

	t.Run("empty path returns error", func(t *testing.T) {
		fs := newMockFileSystemForRead()
		checksumManager := newMockChecksumManagerForRead()

		readTool := NewTool(fs, checksumManager, &mockPathResolver{workspaceRoot: workspaceRoot})

		readReq := &Request{FilePath: ""}
		_, err := executeRead(t, readTool, readReq)
		if err == nil {
			t.Error("expected error for empty path")
		}
		assertContains(t, err.Error(), "path is required")
	})

	t.Run("relative path returns error", func(t *testing.T) {
		fs := newMockFileSystemForRead()
		checksumManager := newMockChecksumManagerForRead()

		readTool := NewTool(fs, checksumManager, &mockPathResolver{workspaceRoot: workspaceRoot})

		readReq := &Request{FilePath: "test.txt"}
		_, err := executeRead(t, readTool, readReq)
		if err == nil {
			t.Error("expected error for relative path")
		}
		assertContains(t, err.Error(), "absolute path required")
	})

	t.Run("negative offset reads from end of file", func(t *testing.T) {
		fs := newMockFileSystemForRead()
		checksumManager := newMockChecksumManagerForRead()
		fs.createFile("/workspace/test.txt", []byte("line1\nline2"))

		readTool := NewTool(fs, checksumManager, &mockPathResolver{workspaceRoot: workspaceRoot})

		readReq := &Request{FilePath: "/workspace/test.txt", Offset: -1, Limit: 1}
		output, err := executeRead(t, readTool, readReq)

		if err != nil {
			t.Fatalf("Execute failed: %v", err)
		}

		assertContains(t, output, "2\tline2")
		assertContains(t, output, "(End of file - total 2 lines)")
	})

	t.Run("zero limit defaults to constant", func(t *testing.T) {
		fs := newMockFileSystemForRead()
		checksumManager := newMockChecksumManagerForRead()
		fs.createFile("/workspace/test.txt", []byte("line1\nline2\nline3"))

		readTool := NewTool(fs, checksumManager, &mockPathResolver{workspaceRoot: workspaceRoot})

		readReq := &Request{FilePath: "/workspace/test.txt", Limit: 0}
		output, err := executeRead(t, readTool, readReq)

		if err != nil {
			t.Fatalf("Execute failed: %v", err)
		}

		// Should show all lines as they are within the default limit (2000)
		assertContains(t, output, "1\tline1")
		assertContains(t, output, "2\tline2")
		assertContains(t, output, "3\tline3")
		assertContains(t, output, "(End of file - total 3 lines)")
	})

	t.Run("high limit is accepted", func(t *testing.T) {
		fs := newMockFileSystemForRead()
		checksumManager := newMockChecksumManagerForRead()
		fs.createFile("/workspace/test.txt", []byte("line1\nline2"))

		readTool := NewTool(fs, checksumManager, &mockPathResolver{workspaceRoot: workspaceRoot})

		readReq := &Request{FilePath: "/workspace/test.txt", Limit: 100000}
		output, err := executeRead(t, readTool, readReq)

		if err != nil {
			t.Fatalf("Execute failed: %v", err)
		}

		// Should show both lines
		assertContains(t, output, "1\tline1")
		assertContains(t, output, "2\tline2")
		assertContains(t, output, "(End of file - total 2 lines)")
	})

	t.Run("windows line endings normalization", func(t *testing.T) {
		fs := newMockFileSystemForRead()
		checksumManager := newMockChecksumManagerForRead()
		// Use Windows line endings
		content := []byte("line1\r\nline2")
		fs.createFile("/workspace/test.txt", content)

		readTool := NewTool(fs, checksumManager, &mockPathResolver{workspaceRoot: workspaceRoot})
		output, err := executeRead(t, readTool, &Request{FilePath: "/workspace/test.txt"})
		if err != nil {
			t.Fatalf("Execute failed: %v", err)
		}

		// Verify output doesn't contain \r (it should be stripped if normalized)
		if strings.Contains(output, "\r") {
			t.Errorf("output still contains \\r: %q", output)
		}
		assertContains(t, output, "1\tline1")
		assertContains(t, output, "2\tline2")
	})

	t.Run("read failure after prepare", func(t *testing.T) {
		fs := newMockFileSystemForRead()
		checksumManager := newMockChecksumManagerForRead()
		fs.createFile("/workspace/test.txt", []byte("content"))

		readTool := NewTool(fs, checksumManager, &mockPathResolver{workspaceRoot: workspaceRoot})

		readReq := &Request{FilePath: "/workspace/test.txt"}
		params, _ := json.Marshal(readReq)
		inv, err := readTool.Prepare(string(params))
		require.NoError(t, err)

		// Delete file after prepare to cause Execute failure
		delete(fs.files, "/workspace/test.txt")

		output, _ := inv.(domain.ExecutableInvocation).Execute(context.Background())
		require.NoError(t, err)
		assertContains(t, output, "Error:")
	})

	t.Run("absolute path in request is normalized for display", func(t *testing.T) {
		fs := newMockFileSystemForRead()
		checksumManager := newMockChecksumManagerForRead()
		workspaceRoot := testWorkspaceRoot
		absFile := "/workspace/subdir/test.txt"
		fs.createFile(absFile, []byte("content"))

		readTool := NewTool(fs, checksumManager, &mockPathResolver{workspaceRoot: workspaceRoot})

		// Agent sends absolute path
		params, _ := json.Marshal(&Request{FilePath: absFile})
		inv, err := readTool.Prepare(string(params))
		if err != nil {
			t.Fatalf("Prepare failed: %v", err)
		}

		// Display should show full absolute path
		display := inv.Display().(domain.StringDisplay)
		assert.Equal(t, "Read \"/workspace/subdir/test.txt\"", display.Description)
		assert.Equal(t, "", display.Content)
	})

	t.Run("execute includes read range in display", func(t *testing.T) {
		fs := newMockFileSystemForRead()
		checksumManager := newMockChecksumManagerForRead()
		workspaceRoot := testWorkspaceRoot
		absFile := "/workspace/subdir/test.txt"
		fs.createFile(absFile, []byte("line1\nline2\nline3"))

		readTool := NewTool(fs, checksumManager, &mockPathResolver{workspaceRoot: workspaceRoot})
		params, _ := json.Marshal(&Request{FilePath: absFile, Offset: 0, Limit: 2})
		inv, err := readTool.Prepare(string(params))
		require.NoError(t, err)

		_, finalDisplay := inv.(domain.ExecutableInvocation).Execute(context.Background())
		typed, ok := finalDisplay.(domain.StringDisplay)
		require.True(t, ok)
		assert.Equal(t, "Read \"/workspace/subdir/test.txt\" Lines 0-2", typed.Description)
	})

	t.Run("empty file returns system reminder", func(t *testing.T) {
		fs := newMockFileSystemForRead()
		fs.createFile("/workspace/empty.txt", []byte(""))
		checksumManager := newMockChecksumManagerForRead()
		rtool := NewTool(fs, checksumManager, &mockPathResolver{workspaceRoot: workspaceRoot})

		output, err := executeRead(t, rtool, &Request{FilePath: "/workspace/empty.txt"})
		require.NoError(t, err)
		assert.Contains(t, output, "<system-reminder>Warning: the file exists but the contents are empty.</system-reminder>")
	})

	t.Run("offset too large returns system reminder", func(t *testing.T) {
		fs := newMockFileSystemForRead()
		fs.createFile("/workspace/short.txt", []byte("line1\nline2"))
		checksumManager := newMockChecksumManagerForRead()
		rtool := NewTool(fs, checksumManager, &mockPathResolver{workspaceRoot: workspaceRoot})

		// File has 2 lines, offset is 10 (line 11)
		output, err := executeRead(t, rtool, &Request{FilePath: "/workspace/short.txt", Offset: 10})
		require.NoError(t, err)
		assert.Contains(t, output, "<system-reminder>Warning: the file exists but is shorter than the provided offset (11). The file has 2 lines.</system-reminder>")
	})
}

func assertContains(t *testing.T, s, substr string) {
	t.Helper()
	if !bytes.Contains([]byte(s), []byte(substr)) {
		t.Errorf("expected %q to contain %q", s, substr)
	}
}




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
