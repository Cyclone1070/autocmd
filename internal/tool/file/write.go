package file

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/tool/helper/content"
	"github.com/cloudwego/eino/schema"
)

// fileWriter defines the minimal filesystem operations needed for writing files.
type fileWriter interface {
	Stat(path string) (os.FileInfo, error)
	ReadFile(path string) ([]byte, error)
	WriteFileAtomic(path string, content []byte, perm os.FileMode) error
	EnsureDirs(path string) error
}

// checksumUpdater defines the interface for checksum computation and updates.
type checksumUpdater interface {
	Compute(data []byte) string
	Update(path string, checksum string)
	Get(path string) (string, bool)
}

// WriteFileTool handles file writing operations.
type WriteFileTool struct {
	fileOps         fileWriter
	checksumManager checksumUpdater
	maxFileSize     int64
	pathResolver    pathResolver
}

// NewWriteFileTool creates a new WriteFileTool with injected dependencies.
func NewWriteFileTool(
	fileOps fileWriter,
	checksumManager checksumUpdater,
	pathResolver pathResolver,
	maxFileSize int64,
) *WriteFileTool {
	if fileOps == nil {
		panic("fileOps is required")
	}
	if checksumManager == nil {
		panic("checksumManager is required")
	}
	if pathResolver == nil {
		panic("pathResolver is required")
	}
	return &WriteFileTool{
		fileOps:         fileOps,
		checksumManager: checksumManager,
		pathResolver:    pathResolver,
		maxFileSize:     maxFileSize,
	}
}

func (t *WriteFileTool) Name() string {
	return "write_file"
}

func (t *WriteFileTool) IsConcurrentSafe() bool { return true }

// Definition returns the tool's schema for the LLM using eino schema.
func (t *WriteFileTool) Definition() *schema.ToolInfo {
	return &schema.ToolInfo{
		Name: "write_file",
		Desc: `Write a file to the local filesystem.

Usage:
- The file_path MUST be an absolute path.
- This tool will overwrite the existing file if there is one at the provided path.
- If this is an existing file, you MUST use the "read_file" tool first to read the file's contents. This tool will fail if you did not read the file first.
- Prefer the "edit_file" tool for modifying existing files — it only sends the diff. Only use this tool to create new files or for complete rewrites.
- NEVER create documentation files (*.md) or README files unless explicitly requested by the User.
- Only use emojis if the user explicitly requests it. Avoid writing emojis to files unless asked.`,
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"file_path": {
				Type:     schema.String,
				Desc:     "The absolute path to the file to write.",
				Required: true,
			},
			"content": {
				Type:     schema.String,
				Desc:     "The content to write to the file",
				Required: true,
			},
			"description": {
				Type:     schema.String,
				Desc:     "A brief explanation of why the file is being written or updated. Mandatory.",
				Required: true,
			},
		}),
	}
}

// WriteFileRequest is the input for WriteFileTool.
type WriteFileRequest struct {
	FilePath    string `json:"file_path"`
	Content     string `json:"content"`
	Description string `json:"description"`
}

func (t *WriteFileTool) Prepare(params string) (domain.Invocation, error) {
	req := &WriteFileRequest{}
	if err := json.Unmarshal([]byte(params), req); err != nil {
		return nil, fmt.Errorf("failed to parse arguments: %w", err)
	}

	if req.FilePath == "" {
		return nil, fmt.Errorf("file_path is required")
	}
	/* Empty content allowed */
	if int64(len(req.Content)) > t.maxFileSize {
		return nil, fmt.Errorf("content too large: %d bytes exceeds limit %d",
			len(req.Content), t.maxFileSize)
	}

	abs, err := t.pathResolver.Abs(req.FilePath)
	if err != nil {
		return nil, err
	}

	// Fail-fast checks: existence and binary content
	info, err := t.fileOps.Stat(abs)
	exists := err == nil
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to stat %s: %w", req.FilePath, err)
	}

	if exists {
		if info.IsDir() {
			return nil, fmt.Errorf("path is a directory, not a file: %s", req.FilePath)
		}
		// Read-before-write staleness check
		cachedChecksum, ok := t.checksumManager.Get(abs)
		if !ok {
			return nil, fmt.Errorf("File has not been read yet. Read it first before writing to it.")
		}

		data, err := t.fileOps.ReadFile(abs)
		if err == nil {
			normalized := strings.ReplaceAll(string(data), "\r\n", "\n")
			currentChecksum := t.checksumManager.Compute([]byte(normalized))
			if currentChecksum != cachedChecksum {
				return nil, fmt.Errorf("File has been modified since read, either by the user or by a linter. Read it again before attempting to write it.")
			}
		}
	}

	contentBytes := []byte(req.Content)
	if content.IsBinaryContent(contentBytes) {
		return nil, fmt.Errorf("cannot write binary content to: %s", req.FilePath)
	}

	displayPath := t.pathResolver.DisplayPath(abs)

	return &writeFileInvocation{
		fileOps:         t.fileOps,
		checksumManager: t.checksumManager,
		absPath:         abs,
		exists:          exists,
		content:         contentBytes,
		display:         domain.NewStringDisplay(req.Description, fmt.Sprintf("Write \"%s\"", filepath.ToSlash(displayPath))),
	}, nil
}

type writeFileInvocation struct {
	fileOps         fileWriter
	checksumManager checksumUpdater
	absPath         string
	exists          bool
	content         []byte
	display         domain.StringDisplay
}

func (i *writeFileInvocation) Display() domain.ToolDisplay {
	return i.display
}

func (i *writeFileInvocation) Execute(ctx context.Context) (string, domain.ToolDisplay) {
	d := i.display

	if ctx.Err() != nil {
		d.Error = domain.ToolErrorCancelled
		return domain.ToolErrorCancelled, d
	}

	// Normalise line endings to LF before write
	normalizedContent := strings.ReplaceAll(string(i.content), "\r\n", "\n")
	contentToWrite := []byte(normalizedContent)

	// Write file atomically
	perm := os.FileMode(0o644)
	if err := i.fileOps.WriteFileAtomic(i.absPath, contentToWrite, perm); err != nil {
		if ctx.Err() != nil {
			d.Error = domain.ToolErrorCancelled
			return domain.ToolErrorCancelled, d
		}
		d.Error = domain.ToolErrorFailed
		return fmt.Sprintf("Error: failed to write file: %v", err), d
	}

	// Update checksum cache
	checksum := i.checksumManager.Compute(contentToWrite)
	i.checksumManager.Update(i.absPath, checksum)

	if i.exists {
		return fmt.Sprintf("The file %s has been updated successfully.", i.absPath), d
	}
	return fmt.Sprintf("File created successfully at: %s", i.absPath), d
}
