package file

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Cyclone1070/iav/internal/domain"
	udiff "github.com/aymanbagabas/go-udiff"
	"github.com/cloudwego/eino/schema"
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
	maxFileSize     int64
	pathResolver    pathResolver
}

// NewEditFileTool creates a new EditFileTool with injected dependencies.
func NewEditFileTool(
	fileOps fileEditor,
	checksumManager checksumManager,
	pathResolver pathResolver,
	maxFileSize int64,
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
	return &EditFileTool{
		fileOps:         fileOps,
		checksumManager: checksumManager,
		maxFileSize:     maxFileSize,
		pathResolver:    pathResolver,
	}
}

func (t *EditFileTool) Name() string {
	return "edit_file"
}

func (t *EditFileTool) IsConcurrentSafe() bool { return true }

// Definition returns the tool's schema for the LLM using eino schema.
func (t *EditFileTool) Definition() *schema.ToolInfo {
	return &schema.ToolInfo{
		Name: "edit_file",
		Desc: "Edit an existing file by replacing text. Supports multiple operations.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"path": {
				Type:     schema.String,
				Desc:     "Path to file",
				Required: true,
			},
			"comment": {
				Type:     schema.String,
				Desc:     "A brief comment (under 80 characters) describing what this edit accomplishes for display in the UI. Mandatory.",
				Required: true,
			},
			"operations": {
				Type: schema.Array,
				Desc: "List of edit operations",
				ElemInfo: &schema.ParameterInfo{
					Type: schema.Object,
					SubParams: map[string]*schema.ParameterInfo{
						"before": {
							Type: schema.String,
							Desc: "Text to find",
						},
						"after": {
							Type: schema.String,
							Desc: "Replacement text",
						},
						"expected_replacements": {
							Type: schema.Integer,
							Desc: "Expected match count",
						},
					},
					Required: true,
				},
				Required: true,
			},
		}),
	}
}

// Prepare validates the request, reads the file, applies edits in memory, and returns an Invocation.
func (t *EditFileTool) Prepare(params string) (domain.Invocation, error) {
	req := &EditFileRequest{}
	if err := json.Unmarshal([]byte(params), req); err != nil {
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

	// Read file and apply edits in memory to compute diff
	info, err := t.fileOps.Stat(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("file does not exist: %s", req.Path)
		}
		return nil, fmt.Errorf("failed to stat %s: %w", req.Path, err)
	}

	data, err := t.fileOps.ReadFile(abs)
	if err != nil {
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
	if int64(len(newContentBytes)) > t.maxFileSize {
		return nil, fmt.Errorf("file too large after edit: %s (size %d, limit %d)", req.Path, len(newContentBytes), t.maxFileSize)
	}

	diff, added, removed := computeUnifiedDiff(oldContent, content)

	displayPath := t.pathResolver.DisplayPath(abs)

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
			fmt.Sprintf("EDIT \"%s\"", filepath.ToSlash(displayPath)),
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
	display          domain.DiffDisplay
}

func (i *editFileInvocation) Display() domain.ToolDisplay {
	return i.display
}

func (i *editFileInvocation) Execute(ctx context.Context) (string, domain.ToolDisplay) {
	d := i.display
	if ctx.Err() != nil {
		d.Error = domain.ToolErrorCancelled
		return domain.ToolErrorCancelled, d
	}

	// Re-read file and verify checksum to prevent TOCTOU race
	data, err := i.fileOps.ReadFile(i.absPath)
	if err != nil {
		if ctx.Err() != nil {
			d.Error = domain.ToolErrorCancelled
			return domain.ToolErrorCancelled, d
		}
		d.Error = domain.ToolErrorFailed
		return fmt.Sprintf("Error: failed to re-read file for verification: %v", err), d
	}
	normalized := strings.ReplaceAll(string(data), "\r\n", "\n")
	currentChecksum := i.checksumManager.Compute([]byte(normalized))
	if currentChecksum != i.expectedChecksum {
		d.Error = domain.ToolErrorFailed
		return fmt.Sprintf("Error: edit conflict: file changed since edit was prepared: %s", i.relPath), d
	}

	// Write the modified content atomically using pre-computed content
	if err := i.fileOps.WriteFileAtomic(i.absPath, i.newContent, i.originalPerm); err != nil {
		if ctx.Err() != nil {
			d.Error = domain.ToolErrorCancelled
			return domain.ToolErrorCancelled, d
		}
		d.Error = domain.ToolErrorFailed
		return fmt.Sprintf("Error: failed to write modified content: %v", err), d
	}

	// Update checksum cache with normalized content
	newNormalized := strings.ReplaceAll(string(i.newContent), "\r\n", "\n")
	newChecksum := i.checksumManager.Compute([]byte(newNormalized))
	i.checksumManager.Update(i.absPath, newChecksum)

	return fmt.Sprintf("Successfully modified file: %s", i.relPath), d
}

func computeUnifiedDiff(oldContent, newContent string) (diff string, added, removed int) {
	// Use empty labels since we strip the headers anyway
	rawDiff := udiff.Unified("", "", oldContent, newContent)

	// Strip the --- and +++ header lines, keep only hunks
	var lines []string
	for line := range strings.SplitSeq(rawDiff, "\n") {
		if strings.HasPrefix(line, "---") || strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "@@") || strings.HasPrefix(line, "\\") {
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
