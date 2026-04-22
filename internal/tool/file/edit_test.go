package file

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Cyclone1070/iav/internal/domain"
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

		tool := NewEditFileTool(fs, checksumManager, &mockPathResolver{workspaceRoot: workspaceRoot}, maxFileSize)

		req := &EditFileRequest{
			FilePath:   "/workspace/test.txt",
			Description:    "changing hello to goodbye",
			OldString:  "hello",
			NewString:  "goodbye",
			ReplaceAll: false,
		}

		params, _ := json.Marshal(req)
		inv, err := tool.Prepare(string(params))
		require.NoError(t, err)

		out, _ := inv.(domain.ExecutableInvocation).Execute(context.Background())
		assert.Equal(t, "The file /workspace/test.txt has been updated successfully.", out)
		assert.Equal(t, "goodbye world", string(fs.files["/workspace/test.txt"]))

		display := inv.Display().(domain.DiffDisplay)
		assert.Equal(t, "changing hello to goodbye", display.Description)
	})

	t.Run("Edit all matches successfully", func(t *testing.T) {
		fs := newMockFileOps()
		checksumManager := newMockChecksumManagerShared()
		fs.files["/workspace/test.txt"] = []byte("a a a")
		// Simulate read
		checksumManager.Update("/workspace/test.txt", checksumManager.Compute([]byte("a a a")))

		tool := NewEditFileTool(fs, checksumManager, &mockPathResolver{workspaceRoot: workspaceRoot}, maxFileSize)

		req := &EditFileRequest{
			FilePath:   "/workspace/test.txt",
			Description:    "changing all a to b",
			OldString:  "a",
			NewString:  "b",
			ReplaceAll: true,
		}

		params, _ := json.Marshal(req)
		inv, err := tool.Prepare(string(params))
		require.NoError(t, err)

		out, _ := inv.(domain.ExecutableInvocation).Execute(context.Background())
		assert.Equal(t, "The file /workspace/test.txt has been updated. All occurrences were successfully replaced.", out)
		assert.Equal(t, "b b b", string(fs.files["/workspace/test.txt"]))
	})

	t.Run("Fails if multiple matches and replace_all is false", func(t *testing.T) {
		fs := newMockFileOps()
		checksumManager := newMockChecksumManagerShared()
		fs.files["/workspace/test.txt"] = []byte("a a")
		checksumManager.Update("/workspace/test.txt", checksumManager.Compute([]byte("a a")))

		tool := NewEditFileTool(fs, checksumManager, &mockPathResolver{workspaceRoot: workspaceRoot}, maxFileSize)

		req := &EditFileRequest{
			FilePath:   "/workspace/test.txt",
			Description:    "should fail",
			OldString:  "a",
			NewString:  "b",
			ReplaceAll: false,
		}

		params, _ := json.Marshal(req)
		_, err := tool.Prepare(string(params))
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "Found 2 matches of the string to replace, but replace_all is false")
	})

	t.Run("Create new file via empty old_string", func(t *testing.T) {
		fs := newMockFileOps()
		checksumManager := newMockChecksumManagerShared()
		// No file at /workspace/new.txt

		tool := NewEditFileTool(fs, checksumManager, &mockPathResolver{workspaceRoot: workspaceRoot}, maxFileSize)

		req := &EditFileRequest{
			FilePath:   "/workspace/new.txt",
			Description:    "creating new file",
			OldString:  "",
			NewString:  "brand new content",
			ReplaceAll: false,
		}

		params, _ := json.Marshal(req)
		inv, err := tool.Prepare(string(params))
		require.NoError(t, err)

		out, _ := inv.(domain.ExecutableInvocation).Execute(context.Background())
		assert.Equal(t, "The file /workspace/new.txt has been updated successfully.", out)
		assert.Equal(t, "brand new content", string(fs.files["/workspace/new.txt"]))
	})

	t.Run("Fail to create file if already exists with content", func(t *testing.T) {
		fs := newMockFileOps()
		checksumManager := newMockChecksumManagerShared()
		fs.files["/workspace/exists.txt"] = []byte("not empty")
		checksumManager.Update("/workspace/exists.txt", checksumManager.Compute([]byte("not empty")))

		tool := NewEditFileTool(fs, checksumManager, &mockPathResolver{workspaceRoot: workspaceRoot}, maxFileSize)

		req := &EditFileRequest{
			FilePath:   "/workspace/exists.txt",
			Description:    "trying to overwrite",
			OldString:  "",
			NewString:  "danger",
			ReplaceAll: false,
		}

		params, _ := json.Marshal(req)
		_, err := tool.Prepare(string(params))
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "Cannot create new file - file already exists")
	})

	t.Run("Edit with curly quotes match", func(t *testing.T) {
		fs := newMockFileOps()
		checksumManager := newMockChecksumManagerShared()
		// File has curly quotes: “hello”
		fs.files["/workspace/test.txt"] = []byte("“hello”")
		checksumManager.Update("/workspace/test.txt", checksumManager.Compute([]byte("“hello”")))

		tool := NewEditFileTool(fs, checksumManager, &mockPathResolver{workspaceRoot: workspaceRoot}, maxFileSize)

		req := &EditFileRequest{
			FilePath:   "/workspace/test.txt",
			Description:    "matching curly with straight",
			OldString:  "\"hello\"", // LLM sends straight quotes
			NewString:  "\"goodbye\"",
			ReplaceAll: false,
		}

		params, _ := json.Marshal(req)
		inv, err := tool.Prepare(string(params))
		require.NoError(t, err)

		out, _ := inv.(domain.ExecutableInvocation).Execute(context.Background())
		assert.Equal(t, "The file /workspace/test.txt has been updated successfully.", out)
		// Should preserve curly quotes in the replacement: “goodbye”
		assert.Equal(t, "“goodbye”", string(fs.files["/workspace/test.txt"]))
	})

	t.Run("Strips trailing whitespace from new_string", func(t *testing.T) {
		fs := newMockFileOps()
		checksumManager := newMockChecksumManagerShared()
		fs.files["/workspace/test.go"] = []byte("package main\n\nfunc main() {}\n")
		checksumManager.Update("/workspace/test.go", checksumManager.Compute([]byte("package main\n\nfunc main() {}\n")))

		tool := NewEditFileTool(fs, checksumManager, &mockPathResolver{workspaceRoot: workspaceRoot}, maxFileSize)

		req := &EditFileRequest{
			FilePath:   "/workspace/test.go",
			Description:    "adding comment",
			OldString:  "func main() {}",
			NewString:  "func main() {} // comment    ", // Trailing spaces
			ReplaceAll: false,
		}

		params, _ := json.Marshal(req)
		inv, err := tool.Prepare(string(params))
		require.NoError(t, err)

		inv.(domain.ExecutableInvocation).Execute(context.Background())
		// Spaces should be stripped
		assert.Equal(t, "package main\n\nfunc main() {} // comment\n", string(fs.files["/workspace/test.go"]))
	})

	t.Run("Does not strip trailing whitespace from markdown", func(t *testing.T) {
		fs := newMockFileOps()
		checksumManager := newMockChecksumManagerShared()
		fs.files["/workspace/test.md"] = []byte("# Title\n")
		checksumManager.Update("/workspace/test.md", checksumManager.Compute([]byte("# Title\n")))

		tool := NewEditFileTool(fs, checksumManager, &mockPathResolver{workspaceRoot: workspaceRoot}, maxFileSize)

		req := &EditFileRequest{
			FilePath:   "/workspace/test.md",
			Description:    "adding line break",
			OldString:  "# Title",
			NewString:  "# Title  ", // Markdown hard line break (2 spaces)
			ReplaceAll: false,
		}

		params, _ := json.Marshal(req)
		inv, err := tool.Prepare(string(params))
		require.NoError(t, err)

		inv.(domain.ExecutableInvocation).Execute(context.Background())
		// Spaces should BEAUTIFULLY remain
		assert.Equal(t, "# Title  \n", string(fs.files["/workspace/test.md"]))
	})

	t.Run("Rejects relative path", func(t *testing.T) {
		fs := newMockFileOps()
		checksumManager := newMockChecksumManagerShared()
		tool := NewEditFileTool(fs, checksumManager, &mockPathResolver{workspaceRoot: workspaceRoot}, maxFileSize)

		req := &EditFileRequest{
			FilePath:   "test.txt",
			Description:    "should fail",
			OldString:  "hello",
			NewString:  "goodbye",
			ReplaceAll: false,
		}

		params, _ := json.Marshal(req)
		_, err := tool.Prepare(string(params))
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "absolute path required")
	})
}
