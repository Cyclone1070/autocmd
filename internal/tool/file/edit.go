package file

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Cyclone1070/iav/internal/config"
	"github.com/Cyclone1070/iav/internal/domain"
	udiff "github.com/aymanbagabas/go-udiff"
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

// EditOperation defines a single find-replace operation.
type EditOperation struct {
	Before               string `json:"before"`
	After                string `json:"after"`
	ExpectedReplacements int    `json:"expected_replacements,omitempty"`
}

// EditFileRequest is the input for EditFileTool.
type EditFileRequest struct {
	Path       string          `json:"path"`
	Comment    string          `json:"comment"`
	Operations []EditOperation `json:"operations"`
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

func (t *EditFileTool) Declaration() domain.Declaration {
	return domain.Declaration{
		Name:        "edit_file",
		Description: "Edit an existing file by replacing text. Supports multiple operations.",
		Parameters: &domain.Schema{
			Type: domain.TypeObject,
			Properties: map[string]*domain.Schema{
				"path":    {Type: domain.TypeString, Description: "Path to file"},
				"comment": {Type: domain.TypeString, Description: "A brief comment describing what this edit accomplishes (e.g. 'Adding auth middleware')"},
				"operations": {
					Type:        domain.TypeArray,
					Description: "List of edit operations",
					Items: &domain.Schema{
						Type: domain.TypeObject,
						Properties: map[string]*domain.Schema{
							"before":                {Type: domain.TypeString, Description: "Text to find"},
							"after":                 {Type: domain.TypeString, Description: "Replacement text"},
							"expected_replacements": {Type: domain.TypeInteger, Description: "Expected match count"},
						},
						Required: []string{"before", "after"},
					},
				},
			},
			Required: []string{"path", "comment", "operations"},
		},
	}
}

// Prepare validates the request, reads the file, applies edits in memory, and returns an Invocation.
func (t *EditFileTool) Prepare(ctx context.Context, params json.RawMessage) (domain.Invocation, error) {
	req := &EditFileRequest{}
	if err := json.Unmarshal(params, req); err != nil {
		return nil, fmt.Errorf("failed to parse arguments: %w", err)
	}

	if req.Path == "" {
		return nil, fmt.Errorf("path is required")
	}
	if req.Comment == "" {
		return nil, fmt.Errorf("comment is required")
	}
	if len(req.Operations) == 0 {
		return nil, fmt.Errorf("operations are required")
	}
	for i := range req.Operations {
		if req.Operations[i].ExpectedReplacements <= 0 {
			req.Operations[i].ExpectedReplacements = 1
		}
	}

	abs, err := t.pathResolver.Abs(req.Path)
	if err != nil {
		return nil, err
	}

	// Check context before I/O
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	// Read file and apply edits in memory to compute diff
	info, err := t.fileOps.Stat(abs)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("file does not exist: %s", req.Path)
		}
		return nil, fmt.Errorf("failed to stat %s: %w", req.Path, err)
	}

	data, err := t.fileOps.ReadFile(abs)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("failed to read file %s: %w", req.Path, err)
	}

	rawContent := string(data)
	hasCRLF := strings.Contains(rawContent, "\r\n")
	oldContent := strings.ReplaceAll(rawContent, "\r\n", "\n")

	// Check for conflicts with cached version
	currentChecksum := t.checksumManager.Compute([]byte(oldContent))
	priorChecksum, checksumOk := t.checksumManager.Get(abs)
	if checksumOk && priorChecksum != currentChecksum {
		return nil, fmt.Errorf("edit conflict: file changed since last read: %s", req.Path)
	}

	// Apply operations sequentially in memory
	content := oldContent
	for _, op := range req.Operations {
		before := strings.ReplaceAll(op.Before, "\r\n", "\n")
		after := strings.ReplaceAll(op.After, "\r\n", "\n")

		if before == "" {
			if op.ExpectedReplacements > 1 {
				return nil, fmt.Errorf("replacement count mismatch: append has 1 target, got %d", op.ExpectedReplacements)
			}
			content += after
			continue
		}

		count := strings.Count(content, before)
		if count == 0 {
			return nil, fmt.Errorf("snippet not found: %q in %s", op.Before, req.Path)
		}

		if count != op.ExpectedReplacements {
			return nil, fmt.Errorf("replacement count mismatch in %s: expected %d, found %d", req.Path, op.ExpectedReplacements, count)
		}

		content = strings.Replace(content, before, after, op.ExpectedReplacements)
	}

	// Restore original line endings if file had CRLF
	finalContent := content
	if hasCRLF {
		finalContent = strings.ReplaceAll(content, "\n", "\r\n")
	}

	newContentBytes := []byte(finalContent)

	// Check size limit
	if int64(len(newContentBytes)) > t.config.Tools.MaxFileSize {
		return nil, fmt.Errorf("file too large after edit: %s (size %d, limit %d)", req.Path, len(newContentBytes), t.config.Tools.MaxFileSize)
	}

	diff, added, removed := computeUnifiedDiff(oldContent, content)

	return &editFileInvocation{
		fileOps:          t.fileOps,
		checksumManager:  t.checksumManager,
		absPath:          abs,
		relPath:          req.Path,
		newContent:       newContentBytes,
		originalPerm:     info.Mode(),
		expectedChecksum: currentChecksum,
		display: domain.NewDiffDisplay(
			req.Comment,
			fmt.Sprintf("Edit %s", filepath.Base(abs)),
			added,
			removed,
			diff,
		),
	}, nil
}

type editFileInvocation struct {
	fileOps          fileEditor
	checksumManager  checksumManager
	absPath          string
	relPath          string
	newContent       []byte
	originalPerm     os.FileMode
	expectedChecksum string
	display          domain.ToolDisplay
}

func (i *editFileInvocation) Display() domain.ToolDisplay {
	return i.display
}

func (i *editFileInvocation) Execute(ctx context.Context) (string, error) {
	// Check context
	if ctx.Err() != nil {
		return "", ctx.Err()
	}

	// Re-read file and verify checksum to prevent TOCTOU race
	data, err := i.fileOps.ReadFile(i.absPath)
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return fmt.Sprintf("Error: failed to read file %s: %v", i.relPath, err), err
	}
	normalized := strings.ReplaceAll(string(data), "\r\n", "\n")
	currentChecksum := i.checksumManager.Compute([]byte(normalized))
	if currentChecksum != i.expectedChecksum {
		return fmt.Sprintf("Error: file changed since edit was prepared: %s", i.relPath), fmt.Errorf("checksum mismatch")
	}

	// Write the modified content atomically using pre-computed content
	if err := i.fileOps.WriteFileAtomic(i.absPath, i.newContent, i.originalPerm); err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return fmt.Sprintf("Error: failed to write file %s: %v", i.relPath, err), err
	}

	// Update checksum cache with normalized content
	newNormalized := strings.ReplaceAll(string(i.newContent), "\r\n", "\n")
	newChecksum := i.checksumManager.Compute([]byte(newNormalized))
	i.checksumManager.Update(i.absPath, newChecksum)

	return fmt.Sprintf("Successfully modified file: %s", i.relPath), nil
}

func computeUnifiedDiff(oldContent, newContent string) (diff string, added, removed int) {
	// Use empty labels since we strip the headers anyway
	rawDiff := udiff.Unified("", "", oldContent, newContent)

	// Strip the --- and +++ header lines, keep only hunks
	var lines []string
	for line := range strings.SplitSeq(rawDiff, "\n") {
		if strings.HasPrefix(line, "---") || strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "@@") {
			continue
		}
		if strings.HasPrefix(line, "+") {
			added++
		} else if strings.HasPrefix(line, "-") {
			removed++
		}
		lines = append(lines, line)
	}
	diff = strings.Join(lines, "\n")
	return diff, added, removed
}
