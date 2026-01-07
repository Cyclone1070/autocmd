package file

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Cyclone1070/iav/internal/config"
	"github.com/Cyclone1070/iav/internal/tool/service/path"
)

// Local mocks for read tests

type mockFileSystemForRead struct {
	files  map[string][]byte
	dirs   map[string]bool
	config *config.Config
}

func newMockFileSystemForRead(cfg *config.Config) *mockFileSystemForRead {
	return &mockFileSystemForRead{
		files:  make(map[string][]byte),
		dirs:   make(map[string]bool),
		config: cfg,
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
// Execute errors: returns (llmContent, err) per contract
func executeRead(t *testing.T, rtool *ReadFileTool, req *ReadFileRequest) (string, error) {
	t.Helper()
	params, _ := json.Marshal(req)
	inv, err := rtool.Prepare(context.Background(), params)
	if err != nil {
		return "", err
	}
	return inv.Execute(context.Background())
}

func TestReadFile(t *testing.T) {
	workspaceRoot := "/workspace"
	maxFileSize := int64(1024 * 1024) // 1MB

	t.Run("full read caches checksum", func(t *testing.T) {
		cfg := config.DefaultConfig()
		cfg.Tools.MaxFileSize = maxFileSize
		fs := newMockFileSystemForRead(cfg)
		checksumManager := newMockChecksumManagerForRead()
		contentStr := "test content"
		content := []byte(contentStr)
		fs.createFile("/workspace/test.txt", content)

		readTool := NewReadFileTool(fs, checksumManager, path.NewResolver(workspaceRoot), cfg)

		readReq := &ReadFileRequest{Path: "test.txt", Offset: 0, Limit: 100}
		output, err := executeRead(t, readTool, readReq)

		if err != nil {
			t.Fatalf("Execute failed: %v", err)
		}

		assertContains(t, output, "00001| test content")
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
		cfg := config.DefaultConfig()
		cfg.Tools.MaxFileSize = maxFileSize
		fs := newMockFileSystemForRead(cfg)
		checksumManager := newMockChecksumManagerForRead()
		content := []byte("line1\nline2\nline3\nline4")
		fs.createFile("/workspace/test.txt", content)

		readTool := NewReadFileTool(fs, checksumManager, path.NewResolver(workspaceRoot), cfg)

		// Read lines 2 and 3 (Offset=1, Limit=2)
		readReq := &ReadFileRequest{Path: "test.txt", Offset: 1, Limit: 2}
		output, err := executeRead(t, readTool, readReq)

		if err != nil {
			t.Fatalf("Execute failed: %v", err)
		}

		assertContains(t, output, "00002| line2")
		assertContains(t, output, "00003| line3")
		assertContains(t, output, "(File has more lines. Use offset=3 to read more)")
	})

	t.Run("offset beyond EOF", func(t *testing.T) {
		cfg := config.DefaultConfig()
		fs := newMockFileSystemForRead(cfg)
		checksumManager := newMockChecksumManagerForRead()
		content := []byte("line1")
		fs.createFile("/workspace/test.txt", content)

		readTool := NewReadFileTool(fs, checksumManager, path.NewResolver(workspaceRoot), cfg)
		offset := 100

		readReq := &ReadFileRequest{Path: "test.txt", Offset: offset, Limit: 10}
		output, err := executeRead(t, readTool, readReq)

		if err != nil {
			t.Fatalf("Execute failed: %v", err)
		}

		assertContains(t, output, "(End of file - total 1 lines)")
	})

	t.Run("directory rejection", func(t *testing.T) {
		cfg := config.DefaultConfig()
		fs := newMockFileSystemForRead(cfg)
		checksumManager := newMockChecksumManagerForRead()
		fs.createDir("/workspace/subdir")

		readTool := NewReadFileTool(fs, checksumManager, path.NewResolver(workspaceRoot), cfg)

		readReq := &ReadFileRequest{Path: "subdir"}
		params, _ := json.Marshal(readReq)
		_, err := readTool.Prepare(context.Background(), params)
		if err == nil {
			t.Error("expected Prepare to fail for directory")
		}
		if !strings.Contains(err.Error(), "directory") {
			t.Errorf("expected directory error message, got: %v", err)
		}
	})

	t.Run("file not found", func(t *testing.T) {
		cfg := config.DefaultConfig()
		fs := newMockFileSystemForRead(cfg)
		checksumManager := newMockChecksumManagerForRead()

		readTool := NewReadFileTool(fs, checksumManager, path.NewResolver(workspaceRoot), cfg)

		readReq := &ReadFileRequest{Path: "nonexistent.txt"}
		params, _ := json.Marshal(readReq)
		_, err := readTool.Prepare(context.Background(), params)
		if err == nil {
			t.Error("expected Prepare to fail for non-existent file")
		}
		if !strings.Contains(err.Error(), "does not exist") {
			t.Errorf("expected 'does not exist' error message, got: %v", err)
		}
	})

	t.Run("empty path returns error", func(t *testing.T) {
		cfg := config.DefaultConfig()
		fs := newMockFileSystemForRead(cfg)
		checksumManager := newMockChecksumManagerForRead()

		readTool := NewReadFileTool(fs, checksumManager, path.NewResolver(workspaceRoot), cfg)

		readReq := &ReadFileRequest{Path: ""}
		_, err := executeRead(t, readTool, readReq)
		if err == nil {
			t.Error("expected error for empty path")
		}
		assertContains(t, err.Error(), "path is required")
	})

	t.Run("path outside workspace returns error", func(t *testing.T) {
		cfg := config.DefaultConfig()
		fs := newMockFileSystemForRead(cfg)
		checksumManager := newMockChecksumManagerForRead()

		readTool := NewReadFileTool(fs, checksumManager, path.NewResolver(workspaceRoot), cfg)

		readReq := &ReadFileRequest{Path: "../outside.txt"}
		_, err := executeRead(t, readTool, readReq)
		if err == nil {
			t.Error("expected error for path outside workspace")
		}
		assertContains(t, err.Error(), "outside workspace")
	})

	t.Run("negative offset clamps to zero", func(t *testing.T) {
		cfg := config.DefaultConfig()
		cfg.Tools.MaxFileSize = maxFileSize
		fs := newMockFileSystemForRead(cfg)
		checksumManager := newMockChecksumManagerForRead()
		fs.createFile("/workspace/test.txt", []byte("line1\nline2"))

		readTool := NewReadFileTool(fs, checksumManager, path.NewResolver(workspaceRoot), cfg)

		readReq := &ReadFileRequest{Path: "test.txt", Offset: -5, Limit: 10}
		output, err := executeRead(t, readTool, readReq)

		if err != nil {
			t.Fatalf("Execute failed: %v", err)
		}

		// Should start from line 1 (offset clamped to 0)
		assertContains(t, output, "00001| line1")
	})

	t.Run("zero limit defaults to config", func(t *testing.T) {
		cfg := config.DefaultConfig()
		cfg.Tools.MaxFileSize = maxFileSize
		cfg.Tools.DefaultReadFileLimit = 1
		fs := newMockFileSystemForRead(cfg)
		checksumManager := newMockChecksumManagerForRead()
		fs.createFile("/workspace/test.txt", []byte("line1\nline2\nline3"))

		readTool := NewReadFileTool(fs, checksumManager, path.NewResolver(workspaceRoot), cfg)

		readReq := &ReadFileRequest{Path: "test.txt", Limit: 0}
		output, err := executeRead(t, readTool, readReq)

		if err != nil {
			t.Fatalf("Execute failed: %v", err)
		}

		// Should only show 1 line (defaulted from config)
		assertContains(t, output, "00001| line1")
		assertContains(t, output, "(File has more lines. Use offset=1 to read more)")
	})

	t.Run("high limit is accepted", func(t *testing.T) {
		cfg := config.DefaultConfig()
		cfg.Tools.MaxFileSize = maxFileSize
		fs := newMockFileSystemForRead(cfg)
		checksumManager := newMockChecksumManagerForRead()
		fs.createFile("/workspace/test.txt", []byte("line1\nline2"))

		readTool := NewReadFileTool(fs, checksumManager, path.NewResolver(workspaceRoot), cfg)

		readReq := &ReadFileRequest{Path: "test.txt", Limit: 100000}
		output, err := executeRead(t, readTool, readReq)

		if err != nil {
			t.Fatalf("Execute failed: %v", err)
		}

		// Should show both lines
		assertContains(t, output, "00001| line1")
		assertContains(t, output, "00002| line2")
		assertContains(t, output, "(End of file - total 2 lines)")
	})
}

func assertContains(t *testing.T, s, substr string) {
	t.Helper()
	if !bytes.Contains([]byte(s), []byte(substr)) {
		t.Errorf("expected %q to contain %q", s, substr)
	}
}
