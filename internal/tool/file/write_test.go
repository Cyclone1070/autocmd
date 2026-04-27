package file

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteFile(t *testing.T) {
	workspaceRoot := "/workspace"
	maxFileSize := int64(1024 * 1024)

	t.Run("Create new file successfully", func(t *testing.T) {
		fs := newMockFileOps()
		checksumManager := newMockChecksumManagerShared()
		tool := NewWriteFileTool(fs, checksumManager, &mockPathResolver{workspaceRoot: workspaceRoot}, maxFileSize)

		req := &WriteFileRequest{
			FilePath:    "/workspace/new.txt",
			Content:     "hello",
			Description: "creating new file",
		}

		params, _ := json.Marshal(req)
		inv, err := tool.Prepare(string(params))
		require.NoError(t, err)

		out, _ := inv.(domain.ExecutableInvocation).Execute(context.Background())
		assert.Contains(t, out, "File created successfully at: /workspace/new.txt")
		assert.Equal(t, "hello", string(fs.files["/workspace/new.txt"]))

		display := inv.Display().(domain.StringDisplay)
		assert.Equal(t, "creating new file", display.Description)
	})

	t.Run("Overwrite existing file after read", func(t *testing.T) {
		fs := newMockFileOps()
		checksumManager := newMockChecksumManagerShared()
		fs.files["/workspace/exists.txt"] = []byte("old")
		// Simulate read
		checksumManager.Update("/workspace/exists.txt", checksumManager.Compute([]byte("old")))

		tool := NewWriteFileTool(fs, checksumManager, &mockPathResolver{workspaceRoot: workspaceRoot}, maxFileSize)

		req := &WriteFileRequest{
			FilePath:    "/workspace/exists.txt",
			Content:     "new",
			Description: "overwriting file",
		}

		params, _ := json.Marshal(req)
		inv, err := tool.Prepare(string(params))
		require.NoError(t, err)

		out, _ := inv.(domain.ExecutableInvocation).Execute(context.Background())
		assert.Contains(t, out, "The file /workspace/exists.txt has been updated successfully.")
		assert.Equal(t, "new", string(fs.files["/workspace/exists.txt"]))

		display := inv.Display().(domain.StringDisplay)
		assert.Equal(t, "overwriting file", display.Description)
	})

	t.Run("Rejects write if never read", func(t *testing.T) {
		fs := newMockFileOps()
		checksumManager := newMockChecksumManagerShared()
		fs.files["/workspace/exists.txt"] = []byte("old")
		// NO checksumManager.Update here

		tool := NewWriteFileTool(fs, checksumManager, &mockPathResolver{workspaceRoot: workspaceRoot}, maxFileSize)

		req := &WriteFileRequest{
			FilePath: "/workspace/exists.txt",
			Content:  "new",
		}

		params, _ := json.Marshal(req)
		_, err := tool.Prepare(string(params))
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "File has not been read yet")
	})

	t.Run("Rejects write if stale (mismatch checksum)", func(t *testing.T) {
		fs := newMockFileOps()
		checksumManager := newMockChecksumManagerShared()
		fs.files["/workspace/exists.txt"] = []byte("modified-externally")
		// Cache has "old"
		checksumManager.Update("/workspace/exists.txt", checksumManager.Compute([]byte("old")))

		tool := NewWriteFileTool(fs, checksumManager, &mockPathResolver{workspaceRoot: workspaceRoot}, maxFileSize)

		req := &WriteFileRequest{
			FilePath: "/workspace/exists.txt",
			Content:  "new",
		}

		params, _ := json.Marshal(req)
		_, err := tool.Prepare(string(params))
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "File has been modified since read")
	})

	t.Run("Normalizes line endings to LF", func(t *testing.T) {
		fs := newMockFileOps()
		checksumManager := newMockChecksumManagerShared()
		tool := NewWriteFileTool(fs, checksumManager, &mockPathResolver{workspaceRoot: workspaceRoot}, maxFileSize)

		req := &WriteFileRequest{
			FilePath:    "/workspace/crlf.txt",
			Content:     "line1\r\nline2",
			Description: "testing normalization",
		}

		params, _ := json.Marshal(req)
		inv, err := tool.Prepare(string(params))
		require.NoError(t, err)
		inv.(domain.ExecutableInvocation).Execute(context.Background())

		assert.Equal(t, "line1\nline2", string(fs.files["/workspace/crlf.txt"]))
	})

	t.Run("Rejects relative path", func(t *testing.T) {
		fs := newMockFileOps()
		checksumManager := newMockChecksumManagerShared()
		tool := NewWriteFileTool(fs, checksumManager, &mockPathResolver{workspaceRoot: workspaceRoot}, maxFileSize)

		req := &WriteFileRequest{
			FilePath: "relative.txt",
			Content:  "content",
		}

		params, _ := json.Marshal(req)
		_, err := tool.Prepare(string(params))
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "absolute path required")
	})
}
