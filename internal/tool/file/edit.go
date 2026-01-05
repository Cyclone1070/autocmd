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

// EditOperation defines a single find-replace operation.
type EditOperation struct {
	Before               string `json:"before"`
	After                string `json:"after"`
	ExpectedReplacements int    `json:"expected_replacements,omitempty"`
}

// EditFileRequest is the input for EditFileTool.
type EditFileRequest struct {
	Path       string          `json:"path"`
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

// Prepare validates the request and returns an Invocation.
func (t *EditFileTool) Prepare(ctx context.Context, params json.RawMessage) (tool.Invocation, error) {
	req := &EditFileRequest{}
	if err := json.Unmarshal(params, req); err != nil {
		return nil, fmt.Errorf("failed to unmarshal request: %w", err)
	}

	if req.Path == "" {
		return nil, fmt.Errorf("path is required")
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

	return &editFileInvocation{
		fileOps:         t.fileOps,
		checksumManager: t.checksumManager,
		config:          t.config,
		absPath:         abs,
		operations:      req.Operations,
	}, nil
}

type editFileInvocation struct {
	fileOps         fileEditor
	checksumManager checksumManager
	config          *config.Config
	absPath         string
	operations      []EditOperation

	// Results for Display()
	diff         string
	addedLines   int
	removedLines int
}

func (i *editFileInvocation) Display() tool.ToolDisplay {
	if i.diff == "" {
		return tool.StringDisplay(fmt.Sprintf("Edit %s", filepath.Base(i.absPath)))
	}
	return tool.DiffDisplay{
		Diff:         i.diff,
		AddedLines:   i.addedLines,
		RemovedLines: i.removedLines,
	}
}

func (i *editFileInvocation) Execute(ctx context.Context) (string, error) {
	// Check context before Stat
	if ctx.Err() != nil {
		return "", ctx.Err()
	}

	// Check if file exists
	info, err := i.fileOps.Stat(i.absPath)
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		if os.IsNotExist(err) {
			return fmt.Sprintf("Error: file does not exist: %s", i.absPath), err
		}
		return fmt.Sprintf("Error: failed to stat %s: %v", i.absPath, err), err
	}

	// Read full file content
	data, err := i.fileOps.ReadFile(i.absPath)
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return fmt.Sprintf("Error: %v", err), err
	}

	rawContent := string(data)
	hasCRLF := strings.Contains(rawContent, "\r\n")
	oldContent := strings.ReplaceAll(rawContent, "\r\n", "\n")

	// Check for conflicts with cached version
	currentChecksum := i.checksumManager.Compute([]byte(oldContent))
	priorChecksum, checksumOk := i.checksumManager.Get(i.absPath)
	if checksumOk && priorChecksum != currentChecksum {
		return fmt.Sprintf("Error: edit conflict: file changed since last read: %s", i.absPath), fmt.Errorf("edit conflict")
	}

	originalPerm := info.Mode()

	// Apply operations sequentially
	content := oldContent
	for _, op := range i.operations {
		before := strings.ReplaceAll(op.Before, "\r\n", "\n")
		after := strings.ReplaceAll(op.After, "\r\n", "\n")

		if before == "" {
			if op.ExpectedReplacements > 1 {
				return fmt.Sprintf("Error: replacement count mismatch: append has 1 target, got %d", op.ExpectedReplacements), fmt.Errorf("replacement count mismatch")
			}
			content += after
			continue
		}

		count := strings.Count(content, before)
		if count == 0 {
			return fmt.Sprintf("Error: snippet not found: %q in %s", op.Before, i.absPath), fmt.Errorf("snippet not found")
		}

		if count != op.ExpectedReplacements {
			return fmt.Sprintf("Error: replacement count mismatch in %s: expected %d, found %d", i.absPath, op.ExpectedReplacements, count), fmt.Errorf("replacement count mismatch")
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
	if int64(len(newContentBytes)) > i.config.Tools.MaxFileSize {
		return fmt.Sprintf("Error: file too large after edit: %s (size %d, limit %d)", i.absPath, len(newContentBytes), i.config.Tools.MaxFileSize), fmt.Errorf("file too large")
	}

	// Write the modified content atomically
	if err := i.fileOps.WriteFileAtomic(i.absPath, newContentBytes, originalPerm); err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return fmt.Sprintf("Error: failed to write file %s: %v", i.absPath, err), err
	}

	// Update checksum cache
	newChecksum := i.checksumManager.Compute([]byte(content))
	i.checksumManager.Update(i.absPath, newChecksum)

	// Compute diff for Display()
	i.diff, i.addedLines, i.removedLines = computeUnifiedDiff(filepath.Base(i.absPath), oldContent, content)

	return fmt.Sprintf("Successfully modified file: %s", i.absPath), nil
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
