package file

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Cyclone1070/iav/internal/config"
	"github.com/Cyclone1070/iav/internal/tool"
	"github.com/Cyclone1070/iav/internal/tool/helper/content"
)

// fileWriter defines the minimal filesystem operations needed for writing files.
type fileWriter interface {
	Stat(path string) (os.FileInfo, error)
	WriteFileAtomic(path string, content []byte, perm os.FileMode) error
	EnsureDirs(path string) error
}

// checksumUpdater defines the interface for checksum computation and updates.
type checksumUpdater interface {
	Compute(data []byte) string
	Update(path string, checksum string)
}

// WriteFileTool handles file writing operations.
type WriteFileTool struct {
	fileOps         fileWriter
	checksumManager checksumUpdater
	config          *config.Config
	pathResolver    pathResolver
}

// NewWriteFileTool creates a new WriteFileTool with injected dependencies.
func NewWriteFileTool(
	fileOps fileWriter,
	checksumManager checksumUpdater,
	pathResolver pathResolver,
	cfg *config.Config,
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
	if cfg == nil {
		panic("config is required")
	}
	return &WriteFileTool{
		fileOps:         fileOps,
		checksumManager: checksumManager,
		pathResolver:    pathResolver,
		config:          cfg,
	}
}

func (t *WriteFileTool) Name() string {
	return "write_file"
}

func (t *WriteFileTool) Declaration() tool.Declaration {
	return tool.Declaration{
		Name:        "write_file",
		Description: "Create a new file with the specified content. File must not already exist.",
		Parameters: &tool.Schema{
			Type: tool.TypeObject,
			Properties: map[string]*tool.Schema{
				"path":    {Type: tool.TypeString, Description: "Path to file"},
				"content": {Type: tool.TypeString, Description: "File content"},
			},
			Required: []string{"path", "content"},
		},
	}
}

func (t *WriteFileTool) Prepare(ctx context.Context, params json.RawMessage) (tool.Invocation, error) {
	req := &WriteFileRequest{}
	if err := json.Unmarshal(params, req); err != nil {
		return nil, fmt.Errorf("failed to unmarshal request: %w", err)
	}

	if req.Path == "" {
		return nil, fmt.Errorf("path is required")
	}
	if req.Content == "" {
		return nil, fmt.Errorf("content is required")
	}
	if int64(len(req.Content)) > t.config.Tools.MaxFileSize {
		return nil, fmt.Errorf("content too large: %d bytes exceeds limit %d",
			len(req.Content), t.config.Tools.MaxFileSize)
	}

	abs, err := t.pathResolver.Abs(req.Path)
	if err != nil {
		return nil, err
	}

	// Fail-fast checks: existence and binary content
	_, err = t.fileOps.Stat(abs)
	if err == nil {
		return nil, fmt.Errorf("file already exists: %s", req.Path)
	}
	if !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to stat %s: %w", req.Path, err)
	}

	contentBytes := []byte(req.Content)
	if content.IsBinaryContent(contentBytes) {
		return nil, fmt.Errorf("cannot write binary content to: %s", req.Path)
	}

	return &writeFileInvocation{
		fileOps:         t.fileOps,
		checksumManager: t.checksumManager,
		absPath:         abs,
		relPath:         req.Path,
		content:         []byte(req.Content),
		display:         tool.StringDisplay(fmt.Sprintf("Write %s", req.Path)),
	}, nil
}

type writeFileInvocation struct {
	fileOps         fileWriter
	checksumManager checksumUpdater
	absPath         string
	relPath         string
	content         []byte
	display         tool.ToolDisplay
}

func (i *writeFileInvocation) Display() tool.ToolDisplay {
	return i.display
}

func (i *writeFileInvocation) Execute(ctx context.Context) (string, error) {
	if ctx.Err() != nil {
		return "", ctx.Err()
	}

	// Check if file already exists (TOCTOU protection)
	_, err := i.fileOps.Stat(i.absPath)
	if err == nil {
		return fmt.Sprintf("Error: file already exists: %s", i.relPath),
			fmt.Errorf("file already exists")
	}
	if !os.IsNotExist(err) {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return fmt.Sprintf("Error: failed to stat %s: %v", i.relPath, err), err
	}

	// Ensure parent directories exist
	parentDir := filepath.Dir(i.absPath)
	if err := i.fileOps.EnsureDirs(parentDir); err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return fmt.Sprintf("Error: failed to create directories: %v", err), err
	}

	// Note: Binary check already done in Prepare

	// Write file atomically
	perm := os.FileMode(0o644)
	if err := i.fileOps.WriteFileAtomic(i.absPath, i.content, perm); err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return fmt.Sprintf("Error: failed to write file %s: %v", i.relPath, err), err
	}

	// Update checksum cache
	normalized := strings.ReplaceAll(string(i.content), "\r\n", "\n")
	checksum := i.checksumManager.Compute([]byte(normalized))
	i.checksumManager.Update(i.absPath, checksum)

	return fmt.Sprintf("Successfully created file: %s (%d bytes)",
		i.relPath, len(i.content)), nil
}
