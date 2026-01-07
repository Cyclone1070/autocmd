package file

// This file contains edit file tests.
// Mocks are defined in write_test.go and shared across all test files in this package.

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Cyclone1070/iav/internal/config"
	"github.com/Cyclone1070/iav/internal/tool"
	"github.com/Cyclone1070/iav/internal/tool/service/path"
)

// executeEdit calls Prepare then Execute, returning the LLM output string.
// Prepare errors: returns ("", err)
// Execute errors: returns (llmContent, err) per contract
func executeEdit(t *testing.T, etool *EditFileTool, req *EditFileRequest) (string, error) {
	t.Helper()
	params, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Failed to marshal request: %v", err)
	}
	inv, err := etool.Prepare(context.Background(), params)
	if err != nil {
		return err.Error(), err
	}
	return inv.Execute(context.Background())
}

func TestEditFile(t *testing.T) {
	workspaceRoot := "/workspace"
	maxFileSize := int64(1024 * 1024) // 1MB

	t.Run("conflict detection when cache checksum differs", func(t *testing.T) {
		cfg := config.DefaultConfig()
		cfg.Tools.MaxFileSize = maxFileSize
		fs := newMockFileSystemForWrite(cfg)
		checksumManager := newMockChecksumManagerForWrite()
		originalContent := []byte("original content")
		fs.createFile("/workspace/test.txt", originalContent, 0o644)

		readTool := NewReadFileTool(fs, checksumManager, path.NewResolver(workspaceRoot), cfg)
		editTool := NewEditFileTool(fs, checksumManager, path.NewResolver(workspaceRoot), cfg)

		// Read file to populate cache
		readReq := &ReadFileRequest{Path: "test.txt"}
		params, _ := json.Marshal(readReq)
		inv, _ := readTool.Prepare(context.Background(), params)
		inv.Execute(context.Background())

		// Modify file externally (simulate external change)
		modifiedContent := []byte("modified externally")
		fs.createFile("/workspace/test.txt", modifiedContent, 0o644)

		// Try to edit - should fail with conflict
		ops := []EditOperation{
			{
				Before:               "original content",
				After:                "new content",
				ExpectedReplacements: 1,
			},
		}

		editReq := &EditFileRequest{Path: "test.txt", Operations: ops}
		output, err := executeEdit(t, editTool, editReq)
		if err == nil {
			t.Fatal("expected conflict error")
		}
		assertContains(t, output, "conflict")
	})

	t.Run("no cached checksum skips revalidation", func(t *testing.T) {
		cfg := config.DefaultConfig()
		fs := newMockFileSystemForWrite(cfg)
		checksumManager := newMockChecksumManagerForWrite()
		content := []byte("some content")
		fs.createFile("/workspace/test.txt", content, 0o644)

		editTool := NewEditFileTool(fs, checksumManager, path.NewResolver(workspaceRoot), cfg)

		ops := []EditOperation{
			{
				Before:               "some",
				After:                "new",
				ExpectedReplacements: 1,
			},
		}

		editReq := &EditFileRequest{Path: "test.txt", Operations: ops}
		output, err := executeEdit(t, editTool, editReq)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		assertContains(t, output, "Successfully modified file")
	})

	t.Run("multiple operations", func(t *testing.T) {
		cfg := config.DefaultConfig()
		cfg.Tools.MaxFileSize = maxFileSize
		fs := newMockFileSystemForWrite(cfg)
		checksumManager := newMockChecksumManagerForWrite()
		content := []byte("line1\nline2\nline3")
		fs.createFile("/workspace/test.txt", content, 0o644)

		readTool := NewReadFileTool(fs, checksumManager, path.NewResolver(workspaceRoot), cfg)
		editTool := NewEditFileTool(fs, checksumManager, path.NewResolver(workspaceRoot), cfg)

		// Read first to populate cache
		readReq := &ReadFileRequest{Path: "test.txt"}
		params, _ := json.Marshal(readReq)
		inv, _ := readTool.Prepare(context.Background(), params)
		inv.Execute(context.Background())

		ops := []EditOperation{
			{
				Before:               "line1",
				After:                "modified1",
				ExpectedReplacements: 1,
			},
			{
				Before:               "line2",
				After:                "modified2",
				ExpectedReplacements: 1,
			},
		}

		editReq := &EditFileRequest{Path: "test.txt", Operations: ops}
		output, err := executeEdit(t, editTool, editReq)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		assertContains(t, output, "Successfully modified file")

		// Verify final content
		data, _ := fs.ReadFile("/workspace/test.txt")
		expected := "modified1\nmodified2\nline3"
		if string(data) != expected {
			t.Errorf("expected content %q, got %q", expected, string(data))
		}
	})

	t.Run("mismatch in expected replacements", func(t *testing.T) {
		cfg := config.DefaultConfig()
		cfg.Tools.MaxFileSize = maxFileSize
		fs := newMockFileSystemForWrite(cfg)
		checksumManager := newMockChecksumManagerForWrite()
		content := []byte("line1\nline1\nline3")
		fs.createFile("/workspace/test.txt", content, 0o644)

		readTool := NewReadFileTool(fs, checksumManager, path.NewResolver(workspaceRoot), cfg)
		editTool := NewEditFileTool(fs, checksumManager, path.NewResolver(workspaceRoot), cfg)

		// Read first to populate cache
		readReq := &ReadFileRequest{Path: "test.txt"}
		params, _ := json.Marshal(readReq)
		inv, _ := readTool.Prepare(context.Background(), params)
		inv.Execute(context.Background())

		ops := []EditOperation{
			{
				Before:               "line1",
				After:                "modified1",
				ExpectedReplacements: 1, // But there are 2 occurrences
			},
		}

		editReq := &EditFileRequest{Path: "test.txt", Operations: ops}
		output, err := executeEdit(t, editTool, editReq)
		if err == nil {
			t.Fatal("expected mismatch error")
		}
		assertContains(t, output, "mismatch")
	})

	t.Run("replacement when snippet appears multiple times but ExpectedReplacements matches", func(t *testing.T) {
		cfg := config.DefaultConfig()
		fs := newMockFileSystemForWrite(cfg)
		checksumManager := newMockChecksumManagerForWrite()
		content := []byte("foo\nfoo\nbar")
		fs.createFile("/workspace/test.txt", content, 0o644)

		readTool := NewReadFileTool(fs, checksumManager, path.NewResolver(workspaceRoot), cfg)
		editTool := NewEditFileTool(fs, checksumManager, path.NewResolver(workspaceRoot), cfg)

		// Read first to populate cache
		readReq := &ReadFileRequest{Path: "test.txt"}
		params, _ := json.Marshal(readReq)
		inv, _ := readTool.Prepare(context.Background(), params)
		inv.Execute(context.Background())

		ops := []EditOperation{
			{
				Before:               "foo",
				After:                "baz",
				ExpectedReplacements: 2,
			},
		}

		editReq := &EditFileRequest{Path: "test.txt", Operations: ops}
		output, err := executeEdit(t, editTool, editReq)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		assertContains(t, output, "Successfully modified file")

		data, _ := fs.ReadFile("/workspace/test.txt")
		expected := "baz\nbaz\nbar"
		if string(data) != expected {
			t.Errorf("expected %q, got %q", expected, string(data))
		}
	})

	t.Run("snippet not found", func(t *testing.T) {
		cfg := config.DefaultConfig()
		fs := newMockFileSystemForWrite(cfg)
		checksumManager := newMockChecksumManagerForWrite()
		fs.createFile("/workspace/test.txt", []byte("content"), 0o644)

		editTool := NewEditFileTool(fs, checksumManager, path.NewResolver(workspaceRoot), cfg)

		ops := []EditOperation{
			{
				Before: "nonexistent",
				After:  "new",
			},
		}

		editReq := &EditFileRequest{Path: "test.txt", Operations: ops}
		output, err := executeEdit(t, editTool, editReq)
		if err == nil {
			t.Fatal("expected snippet not found error")
		}
		assertContains(t, output, "snippet not found")
	})

	t.Run("append to non-empty file", func(t *testing.T) {
		cfg := config.DefaultConfig()
		fs := newMockFileSystemForWrite(cfg)
		checksumManager := newMockChecksumManagerForWrite()
		fs.createFile("/workspace/test.txt", []byte("existing"), 0o644)

		editTool := NewEditFileTool(fs, checksumManager, path.NewResolver(workspaceRoot), cfg)

		ops := []EditOperation{
			{
				Before: "",
				After:  "\nnew line",
			},
		}

		editReq := &EditFileRequest{Path: "test.txt", Operations: ops}
		output, err := executeEdit(t, editTool, editReq)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		assertContains(t, output, "Successfully modified file")

		data, _ := fs.ReadFile("/workspace/test.txt")
		expected := "existing\nnew line"
		if string(data) != expected {
			t.Errorf("expected content %q, got %q", expected, string(data))
		}
	})

	t.Run("multiple appends in one request", func(t *testing.T) {
		cfg := config.DefaultConfig()
		fs := newMockFileSystemForWrite(cfg)
		checksumManager := newMockChecksumManagerForWrite()
		fs.createFile("/workspace/test.txt", []byte("start"), 0o644)

		editTool := NewEditFileTool(fs, checksumManager, path.NewResolver(workspaceRoot), cfg)

		ops := []EditOperation{
			{
				Before: "",
				After:  "1",
			},
			{
				Before: "",
				After:  "2",
			},
		}

		editReq := &EditFileRequest{Path: "test.txt", Operations: ops}
		output, err := executeEdit(t, editTool, editReq)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		assertContains(t, output, "Successfully modified file")

		data, _ := fs.ReadFile("/workspace/test.txt")
		expected := "start12"
		if string(data) != expected {
			t.Errorf("expected content %q, got %q", expected, string(data))
		}
	})

	t.Run("append with count greater than 1 errors", func(t *testing.T) {
		cfg := config.DefaultConfig()
		fs := newMockFileSystemForWrite(cfg)
		checksumManager := newMockChecksumManagerForWrite()
		fs.createFile("/workspace/test.txt", []byte("start"), 0o644)

		editTool := NewEditFileTool(fs, checksumManager, path.NewResolver(workspaceRoot), cfg)

		ops := []EditOperation{
			{
				Before:               "",
				After:                "tail",
				ExpectedReplacements: 2, // Should fail since only 1 place to append
			},
		}

		editReq := &EditFileRequest{Path: "test.txt", Operations: ops}
		output, err := executeEdit(t, editTool, editReq)
		if err == nil {
			t.Fatal("expected mismatch error for append with count > 1")
		}
		assertContains(t, output, "mismatch")
	})

	t.Run("CRLF file with LF snippet matches", func(t *testing.T) {
		cfg := config.DefaultConfig()
		fs := newMockFileSystemForWrite(cfg)
		checksumManager := newMockChecksumManagerForWrite()
		// File with CRLF line endings
		fs.createFile("/workspace/test.txt", []byte("line1\r\nline2\r\nline3"), 0o644)

		editTool := NewEditFileTool(fs, checksumManager, path.NewResolver(workspaceRoot), cfg)

		ops := []EditOperation{
			{
				Before:               "line2", // LF snippet
				After:                "modified",
				ExpectedReplacements: 1,
			},
		}

		editReq := &EditFileRequest{Path: "test.txt", Operations: ops}
		output, err := executeEdit(t, editTool, editReq)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		assertContains(t, output, "Successfully modified file")

		// File should preserve CRLF
		data, _ := fs.ReadFile("/workspace/test.txt")
		expected := "line1\r\nmodified\r\nline3"
		if string(data) != expected {
			t.Errorf("expected content %q, got %q", expected, string(data))
		}
	})

	// Validation tests moved from types_test.go
	t.Run("empty path returns error", func(t *testing.T) {
		cfg := config.DefaultConfig()
		fs := newMockFileSystemForWrite(cfg)
		checksumManager := newMockChecksumManagerForWrite()
		editTool := NewEditFileTool(fs, checksumManager, path.NewResolver(workspaceRoot), cfg)

		req := &EditFileRequest{Path: "", Operations: []EditOperation{{Before: "old", After: "new"}}}
		_, err := executeEdit(t, editTool, req)
		if err == nil {
			t.Error("expected error for empty path")
		}
		assertContains(t, err.Error(), "path is required")
	})

	t.Run("empty operations returns error", func(t *testing.T) {
		cfg := config.DefaultConfig()
		fs := newMockFileSystemForWrite(cfg)
		checksumManager := newMockChecksumManagerForWrite()
		editTool := NewEditFileTool(fs, checksumManager, path.NewResolver(workspaceRoot), cfg)

		req := &EditFileRequest{Path: "test.txt", Operations: []EditOperation{}}
		_, err := executeEdit(t, editTool, req)
		if err == nil {
			t.Error("expected error for empty operations")
		}
		assertContains(t, err.Error(), "operations are required")
	})

	t.Run("negative expected replacements defaults to 1", func(t *testing.T) {
		cfg := config.DefaultConfig()
		fs := newMockFileSystemForWrite(cfg)
		checksumManager := newMockChecksumManagerForWrite()
		fs.createFile("/workspace/test.txt", []byte("foo\nfoo\nbar"), 0o644)
		editTool := NewEditFileTool(fs, checksumManager, path.NewResolver(workspaceRoot), cfg)

		req := &EditFileRequest{
			Path:       "test.txt",
			Operations: []EditOperation{{Before: "foo", After: "baz", ExpectedReplacements: -1}},
		}
		// Defaults to 1, but there are 2 occurrences — should error
		output, err := executeEdit(t, editTool, req)
		if err == nil {
			t.Error("expected mismatch error")
		}
		assertContains(t, output, "mismatch")
	})

	t.Run("path outside workspace returns error", func(t *testing.T) {
		cfg := config.DefaultConfig()
		fs := newMockFileSystemForWrite(cfg)
		checksumManager := newMockChecksumManagerForWrite()
		editTool := NewEditFileTool(fs, checksumManager, path.NewResolver(workspaceRoot), cfg)

		req := &EditFileRequest{Path: "../outside.txt", Operations: []EditOperation{{Before: "a", After: "b"}}}
		_, err := executeEdit(t, editTool, req)
		if err == nil {
			t.Error("expected error for path outside workspace")
		}
		assertContains(t, err.Error(), "outside workspace")
	})

	t.Run("file changed between Prepare and Execute fails", func(t *testing.T) {
		cfg := config.DefaultConfig()
		fs := newMockFileSystemForWrite(cfg)
		checksumManager := newMockChecksumManagerForWrite()
		fs.createFile("/workspace/test.txt", []byte("original"), 0o644)

		editTool := NewEditFileTool(fs, checksumManager, path.NewResolver(workspaceRoot), cfg)

		// Prepare the edit
		req := &EditFileRequest{
			Path:       "test.txt",
			Operations: []EditOperation{{Before: "original", After: "modified"}},
		}
		params, _ := json.Marshal(req)
		inv, err := editTool.Prepare(context.Background(), params)
		if err != nil {
			t.Fatalf("Prepare failed: %v", err)
		}

		// Simulate external change between Prepare and Execute
		fs.createFile("/workspace/test.txt", []byte("changed externally"), 0o644)

		// Execute should return error in message, nil error
		output, err := inv.Execute(context.Background())
		if err == nil {
			t.Errorf("expected operation error for logging per tool.md contract")
		}
		assertContains(t, output, "file changed since edit was prepared")
	})

	t.Run("Display returns DiffDisplay with diff content", func(t *testing.T) {
		cfg := config.DefaultConfig()
		fs := newMockFileSystemForWrite(cfg)
		checksumManager := newMockChecksumManagerForWrite()
		fs.createFile("/workspace/test.txt", []byte("old content"), 0o644)

		editTool := NewEditFileTool(fs, checksumManager, path.NewResolver(workspaceRoot), cfg)

		req := &EditFileRequest{
			Path:       "test.txt",
			Operations: []EditOperation{{Before: "old", After: "new"}},
		}
		params, _ := json.Marshal(req)
		inv, err := editTool.Prepare(context.Background(), params)
		if err != nil {
			t.Fatalf("Prepare failed: %v", err)
		}

		display := inv.Display()
		diffDisplay, ok := display.(tool.DiffDisplay)
		if !ok {
			t.Fatalf("expected DiffDisplay, got %T", display)
		}

		if diffDisplay.Filename != "test.txt" {
			t.Errorf("expected Filename 'test.txt', got %q", diffDisplay.Filename)
		}
		if diffDisplay.Diff == "" {
			t.Error("expected non-empty Diff")
		}
		if diffDisplay.AddedLines == 0 && diffDisplay.RemovedLines == 0 {
			t.Error("expected non-zero added or removed lines")
		}
	})
}
