package file

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Cyclone1070/iav/internal/config"
	"github.com/Cyclone1070/iav/internal/tool"
	"github.com/pmezard/go-difflib/difflib"
)

// fileEditor defines the minimal filesystem operations needed for editing files.
type fileEditor interface {
	Stat(path string) (os.FileInfo, error)
	ReadFile(path string) ([]byte, error)
	WriteFileAtomic(path string, content []byte, perm os.FileMode) error
}

// checksumManager defines the interface for full checksum management.
type checksumManager interface {
	Compute(data []byte) string
	Get(path string) (checksum string, ok bool)
	Update(path string, checksum string)
}

// EditFileTool handles file editing operations.
type EditFileTool struct {
	fileOps         fileEditor
	checksumManager checksumManager
	config          *config.Config
	pathResolver    pathResolver
}

// NewEditFileTool creates a new EditFileTool with injected dependencies.
func NewEditFileTool(
	fileOps fileEditor,
	checksumManager checksumManager,
	pathResolver pathResolver,
	cfg *config.Config,
) *EditFileTool {
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
	return &EditFileTool{
		fileOps:         fileOps,
		checksumManager: checksumManager,
		config:          cfg,
		pathResolver:    pathResolver,
	}
}

func (t *EditFileTool) Name() string {
	return "edit_file"
}

func (t *EditFileTool) Declaration() tool.Declaration {
	return tool.Declaration{
		Name:        "edit_file",
		Description: "Edit an existing file by replacing text. Supports multiple operations.",
		Parameters: &tool.Schema{
			Type: tool.TypeObject,
			Properties: map[string]*tool.Schema{
				"path": {Type: tool.TypeString, Description: "Path to file"},
				"operations": {
					Type:        tool.TypeArray,
					Description: "List of edit operations",
					Items: &tool.Schema{
						Type: tool.TypeObject,
						Properties: map[string]*tool.Schema{
							"before":                {Type: tool.TypeString, Description: "Text to find"},
							"after":                 {Type: tool.TypeString, Description: "Replacement text"},
							"expected_replacements": {Type: tool.TypeInteger, Description: "Expected match count"},
						},
						Required: []string{"before", "after"},
					},
				},
			},
			Required: []string{"path", "operations"},
		},
	}
}

func (t *EditFileTool) Request() any {
	return &EditFileRequest{}
}

// Execute applies edit operations to an existing file in the workspace.
// It detects concurrent modifications by comparing file checksums and validates
// operations before applying them. The file is written atomically.
//
// Note: There is a narrow race condition window between checksum validation and write.
// For guaranteed conflict-free edits, external file locking would be required.
//
// Note: ctx is accepted for API consistency but not used - file I/O is synchronous.
func (t *EditFileTool) Execute(ctx context.Context, req any) (any, error) {
	r, ok := req.(*EditFileRequest)
	if !ok {
		return nil, fmt.Errorf("invalid request type: %T", req)
	}

	if err := r.Validate(); err != nil {
		return &EditFileResponse{Error: err.Error()}, nil
	}

	abs, err := t.pathResolver.Abs(r.Path)
	if err != nil {
		return &EditFileResponse{Error: err.Error()}, nil
	}

	// Check if file exists
	info, err := t.fileOps.Stat(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return &EditFileResponse{Error: fmt.Sprintf("file does not exist: %s", abs)}, nil
		}
		return &EditFileResponse{Error: fmt.Sprintf("failed to stat %s: %v", abs, err)}, nil
	}

	// Read full file content
	data, err := t.fileOps.ReadFile(abs)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return &EditFileResponse{Error: err.Error()}, nil
	}

	rawContent := string(data)

	// Detect original line endings for restoration
	hasCRLF := strings.Contains(rawContent, "\r\n")

	// Normalize to \n for consistent matching
	oldContent := strings.ReplaceAll(rawContent, "\r\n", "\n")

	// Compute current checksum (on normalized content for consistency)
	currentChecksum := t.checksumManager.Compute([]byte(oldContent))

	// Check for conflicts with cached version
	priorChecksum, checksumOk := t.checksumManager.Get(abs)
	if checksumOk && priorChecksum != currentChecksum {
		return &EditFileResponse{Error: fmt.Sprintf("edit conflict: file changed since last read: %s", abs)}, nil
	}

	// Preserve original permissions
	originalPerm := info.Mode()

	// Apply operations sequentially (on normalized content)
	content := oldContent
	for _, op := range r.Operations {
		// Normalize operation strings for matching
		before := strings.ReplaceAll(op.Before, "\r\n", "\n")
		after := strings.ReplaceAll(op.After, "\r\n", "\n")

		// Empty Before means append to end of file
		if before == "" {
			// Append has exactly 1 logical "target" (end of file).
			// If count > 1 is specified, it's a mismatch since there's only 1 place to append.
			if op.ExpectedReplacements > 1 {
				return &EditFileResponse{Error: fmt.Sprintf("replacement count mismatch: append has 1 target, got %d", op.ExpectedReplacements)}, nil
			}
			content += after
			continue
		}

		count := strings.Count(content, before)
		if count == 0 {
			return &EditFileResponse{Error: fmt.Sprintf("snippet not found: %q in %s", op.Before, abs)}, nil
		}

		expected := op.ExpectedReplacements

		if count != expected {
			return &EditFileResponse{Error: fmt.Sprintf("replacement count mismatch in %s: expected %d, found %d", abs, expected, count)}, nil
		}

		content = strings.Replace(content, before, after, expected)
	}

	// Restore original line endings if file had CRLF
	finalContent := content
	if hasCRLF {
		finalContent = strings.ReplaceAll(content, "\n", "\r\n")
	}

	newContentBytes := []byte(finalContent)

	// Check size limit
	maxFileSize := t.config.Tools.MaxFileSize
	if int64(len(newContentBytes)) > maxFileSize {
		return &EditFileResponse{Error: fmt.Sprintf("file too large after edit: %s (size %d, limit %d)", abs, len(newContentBytes), maxFileSize)}, nil
	}

	// Write the modified content atomically
	if err := t.fileOps.WriteFileAtomic(abs, newContentBytes, originalPerm); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return &EditFileResponse{Error: fmt.Sprintf("failed to write file %s: %v", abs, err)}, nil
	}

	// Compute new checksum and update cache (on normalized content)
	newChecksum := t.checksumManager.Compute([]byte(content))
	t.checksumManager.Update(abs, newChecksum)

	diff, added, removed := computeUnifiedDiff(filepath.Base(abs), oldContent, content)

	return &EditFileResponse{
		Path:         abs,
		Diff:         diff,
		AddedLines:   added,
		RemovedLines: removed,
	}, nil
}

func computeUnifiedDiff(filename, oldContent, newContent string) (diff string, added, removed int) {
	ud := difflib.UnifiedDiff{
		A:        difflib.SplitLines(oldContent),
		B:        difflib.SplitLines(newContent),
		FromFile: "a/" + filename,
		ToFile:   "b/" + filename,
		Context:  3,
	}
	diff, _ = difflib.GetUnifiedDiffString(ud)

	// Count added/removed lines
	for _, line := range strings.Split(diff, "\n") {
		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			added++
		} else if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
			removed++
		}
	}
	return diff, added, removed
}
