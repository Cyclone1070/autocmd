package file

// This file contains edit file tests.
// Mocks are defined in write_test.go and shared across all test files in this package.

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Cyclone1070/iav/internal/config"
	"github.com/Cyclone1070/iav/internal/tool"
	"github.com/Cyclone1070/iav/internal/tool/service/path"
)

// executeReadForEdit is a test helper that calls Prepare on ReadFileTool.
func executeReadForEdit(t *testing.T, tool *ReadFileTool, req *ReadFileRequest) {
	t.Helper()
	params, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Failed to marshal request: %v", err)
	}
	_, err = tool.Prepare(context.Background(), params)
	if err != nil {
		t.Fatalf("Prepare returned error: %v", err)
	}
}

// executeEdit is a test helper that calls Execute on EditFileTool and expects success.
func executeEdit(t *testing.T, etool *EditFileTool, req *EditFileRequest) *EditFileResponse {
	t.Helper()
	result, err := etool.Execute(context.Background(), req)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	resp, ok := result.(*EditFileResponse)
	if !ok {
		t.Fatalf("Execute returned wrong type: %T", result)
	}
	if resp.Error != "" {
		t.Fatalf("Execute failed: %s", resp.Error)
	}
	return resp
}

// executeEditExpectError is a test helper that expects a tool error in the response.
func executeEditExpectError(t *testing.T, etool *EditFileTool, req *EditFileRequest) *EditFileResponse {
	t.Helper()
	result, err := etool.Execute(context.Background(), req)
	if err != nil {
		t.Fatalf("Execute returned infra error: %v", err)
	}
	resp, ok := result.(*EditFileResponse)
	if !ok {
		t.Fatalf("Execute returned wrong type: %T", result)
	}
	if resp.Error == "" {
		t.Fatalf("expected error but got success")
	}
	return resp
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
		executeReadForEdit(t, readTool, readReq)

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
		resp := executeEditExpectError(t, editTool, editReq)
		if !strings.Contains(resp.Error, "conflict") {
			t.Errorf("expected conflict error, got: %s", resp.Error)
		}
	})

	t.Run("no cached checksum skips revalidation", func(t *testing.T) {
		cfg := config.DefaultConfig()
		fs := newMockFileSystemForWrite(cfg)
		checksumManager := newMockChecksumManagerForWrite()
		content := []byte("some content")
		fs.createFile("/workspace/test.txt", content, 0o644)

		// Skip reading first, so no cache
		editTool := NewEditFileTool(fs, checksumManager, path.NewResolver(workspaceRoot), cfg)

		ops := []EditOperation{
			{
				Before:               "some",
				After:                "new",
				ExpectedReplacements: 1,
			},
		}

		editReq := &EditFileRequest{Path: "test.txt", Operations: ops}
		resp := executeEdit(t, editTool, editReq)
		if resp.Diff == "" {
			t.Errorf("expected non-empty diff")
		}
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
		executeReadForEdit(t, readTool, readReq)

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
		resp := executeEdit(t, editTool, editReq)
		if resp.Diff == "" {
			t.Errorf("expected non-empty diff")
		}

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
		executeReadForEdit(t, readTool, readReq)

		ops := []EditOperation{
			{
				Before:               "line1",
				After:                "modified1",
				ExpectedReplacements: 1, // But there are 2 occurrences
			},
		}

		editReq := &EditFileRequest{Path: "test.txt", Operations: ops}
		resp := executeEditExpectError(t, editTool, editReq)
		if !strings.Contains(resp.Error, "mismatch") {
			t.Errorf("expected mismatch error, got: %s", resp.Error)
		}
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
		executeReadForEdit(t, readTool, readReq)

		ops := []EditOperation{
			{
				Before:               "foo",
				After:                "baz",
				ExpectedReplacements: 2,
			},
		}

		editReq := &EditFileRequest{Path: "test.txt", Operations: ops}
		resp := executeEdit(t, editTool, editReq)
		if resp.Diff == "" {
			t.Errorf("expected non-empty diff")
		}

		data, _ := fs.ReadFile("/workspace/test.txt")
		expected := "baz\nbaz\nbar"
		if string(data) != expected {
			t.Errorf("expected %q, got %q", expected, string(data))
		}
	})

	t.Run("zero expected replacements defaults to 1", func(t *testing.T) {
		cfg := config.DefaultConfig()
		fs := newMockFileSystemForWrite(cfg)
		checksumManager := newMockChecksumManagerForWrite()
		content := []byte("foo\nfoo\nbar")
		fs.createFile("/workspace/test.txt", content, 0o644)

		readTool := NewReadFileTool(fs, checksumManager, path.NewResolver(workspaceRoot), cfg)
		editTool := NewEditFileTool(fs, checksumManager, path.NewResolver(workspaceRoot), cfg)

		// Read first to populate cache
		readReq := &ReadFileRequest{Path: "test.txt"}
		executeReadForEdit(t, readTool, readReq)

		ops := []EditOperation{
			{
				Before:               "foo",
				After:                "baz",
				ExpectedReplacements: 0, // Defaults to 1, but there are 2
			},
		}

		editReq := &EditFileRequest{Path: "test.txt", Operations: ops}
		resp := executeEditExpectError(t, editTool, editReq)
		if !strings.Contains(resp.Error, "mismatch") {
			t.Errorf("expected mismatch error, got: %s", resp.Error)
		}
	})

	t.Run("snippet not found", func(t *testing.T) {
		cfg := config.DefaultConfig()
		fs := newMockFileSystemForWrite(cfg)
		checksumManager := newMockChecksumManagerForWrite()
		fs.createFile("/workspace/test.txt", []byte("content"), 0o644)

		readTool := NewReadFileTool(fs, checksumManager, path.NewResolver(workspaceRoot), cfg)
		editTool := NewEditFileTool(fs, checksumManager, path.NewResolver(workspaceRoot), cfg)

		readReq := &ReadFileRequest{Path: "test.txt"}
		executeReadForEdit(t, readTool, readReq)

		ops := []EditOperation{
			{
				Before: "nonexistent",
				After:  "new",
			},
		}

		editReq := &EditFileRequest{Path: "test.txt", Operations: ops}
		resp := executeEditExpectError(t, editTool, editReq)
		if !strings.Contains(resp.Error, "snippet not found") {
			t.Errorf("expected snippet not found error, got: %s", resp.Error)
		}
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
		resp := executeEdit(t, editTool, editReq)
		if resp.Diff == "" {
			t.Errorf("expected non-empty diff")
		}

		data, _ := fs.ReadFile("/workspace/test.txt")
		expected := "existing\nnew line"
		if string(data) != expected {
			t.Errorf("expected content %q, got %q", expected, string(data))
		}
	})

	t.Run("append to empty file", func(t *testing.T) {
		cfg := config.DefaultConfig()
		fs := newMockFileSystemForWrite(cfg)
		checksumManager := newMockChecksumManagerForWrite()
		fs.createFile("/workspace/test.txt", []byte(""), 0o644)

		editTool := NewEditFileTool(fs, checksumManager, path.NewResolver(workspaceRoot), cfg)

		ops := []EditOperation{
			{
				Before: "",
				After:  "first content",
			},
		}

		editReq := &EditFileRequest{Path: "test.txt", Operations: ops}
		resp := executeEdit(t, editTool, editReq)
		if resp.Diff == "" {
			t.Errorf("expected non-empty diff")
		}

		data, _ := fs.ReadFile("/workspace/test.txt")
		expected := "first content"
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
		resp := executeEdit(t, editTool, editReq)
		if resp.Diff == "" {
			t.Errorf("expected non-empty diff")
		}

		data, _ := fs.ReadFile("/workspace/test.txt")
		expected := "start12"
		if string(data) != expected {
			t.Errorf("expected content %q, got %q", expected, string(data))
		}
	})

	t.Run("mixed replace and append", func(t *testing.T) {
		cfg := config.DefaultConfig()
		fs := newMockFileSystemForWrite(cfg)
		checksumManager := newMockChecksumManagerForWrite()
		fs.createFile("/workspace/test.txt", []byte("foo\nbar"), 0o644)

		editTool := NewEditFileTool(fs, checksumManager, path.NewResolver(workspaceRoot), cfg)

		ops := []EditOperation{
			{
				Before:               "foo",
				After:                "baz",
				ExpectedReplacements: 1,
			},
			{
				Before: "",
				After:  "\nend",
			},
		}

		editReq := &EditFileRequest{Path: "test.txt", Operations: ops}
		resp := executeEdit(t, editTool, editReq)
		if resp.Diff == "" {
			t.Errorf("expected non-empty diff")
		}

		data, _ := fs.ReadFile("/workspace/test.txt")
		expected := "baz\nbar\nend"
		if string(data) != expected {
			t.Errorf("expected content %q, got %q", expected, string(data))
		}
	})

	t.Run("append with count > 1 errors", func(t *testing.T) {
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
		resp := executeEditExpectError(t, editTool, editReq)
		if !strings.Contains(resp.Error, "mismatch") {
			t.Errorf("expected mismatch error, got: %s", resp.Error)
		}
	})

	t.Run("response methods", func(t *testing.T) {
		resp := &EditFileResponse{
			Path:         "/workspace/test.txt",
			Diff:         "some diff",
			AddedLines:   5,
			RemovedLines: 3,
		}

		if !resp.Success() {
			t.Errorf("expected Success() to be true")
		}

		expectedContent := "Successfully modified file: /workspace/test.txt"
		if resp.LLMContent() != expectedContent {
			t.Errorf("expected LLMContent %q, got %q", expectedContent, resp.LLMContent())
		}

		display := resp.Display()
		_, ok := display.(tool.DiffDisplay)
		if !ok {
			t.Fatalf("expected tool.DiffDisplay, got %T", display)
		}
	})

	t.Run("response error methods", func(t *testing.T) {
		resp := &EditFileResponse{
			Error: "some error",
		}

		if resp.Success() {
			t.Errorf("expected Success() to be false")
		}

		expectedContent := "Error: some error"
		if resp.LLMContent() != expectedContent {
			t.Errorf("expected LLMContent %q, got %q", expectedContent, resp.LLMContent())
		}

		display := resp.Display()
		_, ok := display.(tool.StringDisplay)
		if !ok {
			t.Fatalf("expected tool.StringDisplay, got %T", display)
		}
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
		resp := executeEdit(t, editTool, editReq)

		// Should match and modify
		if resp.Diff == "" {
			t.Errorf("expected non-empty diff")
		}

		// File should preserve CRLF
		data, _ := fs.ReadFile("/workspace/test.txt")
		expected := "line1\r\nmodified\r\nline3"
		if string(data) != expected {
			t.Errorf("expected content %q, got %q", expected, string(data))
		}
	})

	t.Run("CRLF file preserves line endings on write", func(t *testing.T) {
		cfg := config.DefaultConfig()
		fs := newMockFileSystemForWrite(cfg)
		checksumManager := newMockChecksumManagerForWrite()
		// File with CRLF line endings
		fs.createFile("/workspace/test.txt", []byte("hello\r\nworld"), 0o644)

		editTool := NewEditFileTool(fs, checksumManager, path.NewResolver(workspaceRoot), cfg)

		ops := []EditOperation{
			{
				Before:               "hello",
				After:                "goodbye",
				ExpectedReplacements: 1,
			},
		}

		editReq := &EditFileRequest{Path: "test.txt", Operations: ops}
		executeEdit(t, editTool, editReq)

		// File should preserve CRLF
		data, _ := fs.ReadFile("/workspace/test.txt")
		if !strings.Contains(string(data), "\r\n") {
			t.Errorf("expected CRLF to be preserved, got: %q", string(data))
		}
		expected := "goodbye\r\nworld"
		if string(data) != expected {
			t.Errorf("expected content %q, got %q", expected, string(data))
		}
	})

	t.Run("LF file stays LF on write", func(t *testing.T) {
		cfg := config.DefaultConfig()
		fs := newMockFileSystemForWrite(cfg)
		checksumManager := newMockChecksumManagerForWrite()
		// File with LF line endings
		fs.createFile("/workspace/test.txt", []byte("hello\nworld"), 0o644)

		editTool := NewEditFileTool(fs, checksumManager, path.NewResolver(workspaceRoot), cfg)

		ops := []EditOperation{
			{
				Before:               "hello",
				After:                "goodbye",
				ExpectedReplacements: 1,
			},
		}

		editReq := &EditFileRequest{Path: "test.txt", Operations: ops}
		executeEdit(t, editTool, editReq)

		// File should stay LF
		data, _ := fs.ReadFile("/workspace/test.txt")
		if strings.Contains(string(data), "\r\n") {
			t.Errorf("expected LF to be preserved, got CRLF: %q", string(data))
		}
		expected := "goodbye\nworld"
		if string(data) != expected {
			t.Errorf("expected content %q, got %q", expected, string(data))
		}
	})
}
