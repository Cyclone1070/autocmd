// Package edit provides tools for reading, writing, and editing files.
package edit

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/runtimectx"
	udiff "github.com/aymanbagabas/go-udiff"
	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
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

// Request is the input for Tool.
type Request struct {
	FilePath    string `json:"file_path"`
	Description string `json:"description"`
	OldString   string `json:"old_string"`
	NewString   string `json:"new_string"`
	ReplaceAll  bool   `json:"replace_all"`
}

// Tool handles file editing operations.
type Tool struct {
	fileOps         fileEditor
	checksumManager checksumManager
	pathResolver    pathResolver
	maxFileSize     int64
}

// NewTool creates a new Tool with injected dependencies.
func NewTool(
	fileOps fileEditor,
	checksumManager checksumManager,
	pathResolver pathResolver,
	maxFileSize int64,
) *Tool {
	if fileOps == nil {
		panic("fileOps is required")
	}
	if checksumManager == nil {
		panic("checksumManager is required")
	}
	if pathResolver == nil {
		panic("pathResolver is required")
	}
	return &Tool{
		fileOps:         fileOps,
		checksumManager: checksumManager,
		maxFileSize:     maxFileSize,
		pathResolver:    pathResolver,
	}
}

// Name returns the unique identifier for the edit file tool.
func (t *Tool) Name() string {
	return "edit_file"
}

// IsConcurrentSafe indicates if the edit file tool can be run concurrently.
func (t *Tool) IsConcurrentSafe() bool { return true }

func (t *Tool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "edit_file",
		Desc: `Performs exact string replacements in files.

Usage:
- The file_path MUST be an absolute path (or start with ~).
- You must use the "read_file" tool at least once in the conversation before editing. This tool will error if you attempt an edit without reading the file.
- When editing text from Read tool output, ensure you preserve the exact indentation (tabs/spaces) as it appears AFTER the line number prefix. The line number prefix format is: line number + tab. Everything after that is the actual file content to match. Never include any part of the line number prefix in the old_string or new_string.
- ALWAYS prefer editing existing files in the codebase. NEVER write new files unless explicitly required.
- Only use emojis if the user explicitly requests it. Avoid adding emojis to files unless asked.
- The edit will FAIL if "old_string" is not unique in the file. Either provide a larger string with more surrounding context to make it unique or use "replace_all" to change every instance of "old_string".
- Use "replace_all" for replacing and renaming strings across the file. This parameter is useful if you want to rename a variable for instance.`,
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"file_path": {
				Type:     schema.String,
				Desc:     "The absolute path (or ~ path) to the file to edit.",
				Required: true,
			},
			"description": {
				Type:     schema.String,
				Desc:     "A brief explanation of why the file is being edited. Mandatory.",
				Required: true,
			},
			"old_string": {
				Type:     schema.String,
				Desc:     "The text to replace",
				Required: true,
			},
			"new_string": {
				Type:     schema.String,
				Desc:     "The text to replace it with (must be different from old_string)",
				Required: true,
			},
			"replace_all": {
				Type: schema.Boolean,
				Desc: "Replace all occurrences of old_string (default false)",
			},
		}),
	}, nil
}

func (t *Tool) InvokableRun(ctx context.Context, argumentsInJSON string, _ ...einotool.Option) (string, error) {
	req, err := t.validate(argumentsInJSON)
	if err != nil {
		return "", err
	}
	callID := compose.GetToolCallID(ctx)
	llmContent, finalDisplay := t.executeEdit(ctx, req)
	if events, ok := runtimectx.EventSenderFrom(ctx); ok && events != nil {
		events.SendUIUpdate(domain.ToolEndEvent{CallID: callID, Display: finalDisplay})
	}
	if sink, ok := runtimectx.ToolDisplaySinkFrom(ctx); ok && sink != nil {
		sink(callID, finalDisplay)
	}
	return llmContent, nil
}

func (t *Tool) Preview(input *compose.ToolInput) domain.ToolDisplay {
	req := &Request{}
	if err := json.Unmarshal([]byte(input.Arguments), req); err != nil {
		return domain.NewStringDisplay(fmt.Sprintf("Run \"%s\"", t.Name()), "")
	}
	displayPath := t.pathResolver.DisplayPath(req.FilePath)
	return domain.NewDiffDisplay(req.Description, fmt.Sprintf("Edit \"%s\"", displayPath), 0, 0, "")
}

func (t *Tool) PreflightValidate(input *compose.ToolInput) error {
	_, err := t.validate(input.Arguments)
	return err
}

type validatedRequest struct {
	newContent       []byte
	absPath          string
	expectedChecksum string
	originalPerm     os.FileMode
	replaceAll       bool
	description      string
	target           string
	diff             string
	added            int
	removed          int
}

func (t *Tool) validate(params string) (*validatedRequest, error) {
	req := &Request{}
	if err := json.Unmarshal([]byte(params), req); err != nil {
		return nil, fmt.Errorf("failed to parse arguments: %w", err)
	}

	if req.FilePath == "" {
		return nil, fmt.Errorf("file_path is required")
	}
	if req.Description == "" {
		return nil, fmt.Errorf("description is required")
	}

	abs, err := t.pathResolver.ValidateAbs(req.FilePath)
	if err != nil {
		return nil, err
	}

	var rawContent string
	var currentChecksum string
	var info os.FileInfo

	// Read file and apply edits in memory to compute diff
	info, err = t.fileOps.Stat(abs)
	if err != nil {
		if os.IsNotExist(err) {
			if req.OldString != "" {
				return nil, fmt.Errorf("file does not exist: %s", abs)
			}
			// File doesn't exist and OldString is empty: new file creation
			rawContent = ""
		} else {
			return nil, fmt.Errorf("failed to stat %s: %w", abs, err)
		}
	} else {
		data, err := t.fileOps.ReadFile(abs)
		if err != nil {
			return nil, fmt.Errorf("failed to read file %s: %w", abs, err)
		}
		rawContent = string(data)

		if req.OldString == "" {
			if strings.TrimSpace(rawContent) != "" {
				return nil, fmt.Errorf("cannot create new file - file already exists")
			}
		}
	}

	hasCRLF := strings.Contains(rawContent, "\r\n")
	oldContent := strings.ReplaceAll(rawContent, "\r\n", "\n")

	// Check for conflicts with cached version
	currentChecksum = t.checksumManager.Compute([]byte(oldContent))
	priorChecksum, checksumOk := t.checksumManager.Get(abs)
	if checksumOk && priorChecksum != currentChecksum {
		return nil, fmt.Errorf("edit conflict: file changed since last read: %s", abs)
	}

	// Apply replacement
	before := strings.ReplaceAll(req.OldString, "\r\n", "\n")
	after := strings.ReplaceAll(req.NewString, "\r\n", "\n")

	// Strip trailing whitespace from new text unless it is a markdown file
	// to avoid accidental dirty edits from the LLM.
	isMarkdown := strings.HasSuffix(abs, ".md") || strings.HasSuffix(abs, ".mdx")
	if !isMarkdown {
		after = stripTrailingWhitespace(after)
	}

	var content string
	if before == "" {
		// If OldString is empty, we replace the entire content (which must be empty per check above)
		content = after
	} else {
		var matches int
		var actualOldString string
		// Try exact match first
		if strings.Contains(oldContent, before) {
			actualOldString = before
		} else {
			// Try normalized match
			normedOld := normalizeQuotes(oldContent)
			normedBefore := normalizeQuotes(before)
			found := strings.Contains(normedOld, normedBefore)
			if found {
				// We found a match in normalized space, now extract the actual string from the original content
				runes := []rune(oldContent)
				normedRunes := []rune(normedOld)
				normedBeforeRunes := []rune(normedBefore)

				// Re-find in normedRunes to be safe with rune indices
				start := -1
				for i := 0; i <= len(normedRunes)-len(normedBeforeRunes); i++ {
					match := true
					for j := range normedBeforeRunes {
						if normedRunes[i+j] != normedBeforeRunes[j] {
							match = false
							break
						}
					}
					if match {
						start = i
						break
					}
				}

				if start != -1 {
					actualOldString = string(runes[start : start+len(normedBeforeRunes)])
				} else {
					return nil, fmt.Errorf("string to replace not found in file (normalization failed).\nString: %s", req.OldString)
				}
			} else {
				return nil, fmt.Errorf("string to replace not found in file.\nString: %s", req.OldString)
			}
		}

		matches = strings.Count(oldContent, actualOldString)
		if matches == 0 {
			// Should not happen if we found it above, but safe guard
			return nil, fmt.Errorf("string to replace not found in file.\nString: %s", req.OldString)
		}

		if matches > 1 && !req.ReplaceAll {
			return nil, fmt.Errorf("found %d matches of the string to replace, but replace_all is false. To replace all occurrences, set replace_all to true. To replace only one occurrence, please provide more context to uniquely identify the instance.\nString: %s", matches, req.OldString)
		}

		// Preserve curly quote style in the replacement if the original match had them
		after = preserveQuoteStyle(before, actualOldString, after)

		if req.ReplaceAll {
			content = strings.ReplaceAll(oldContent, actualOldString, after)
		} else {
			content = strings.Replace(oldContent, actualOldString, after, 1)
		}
	}

	// Restore original line endings if file had CRLF
	finalContent := content
	if hasCRLF {
		finalContent = strings.ReplaceAll(content, "\n", "\r\n")
	}

	newContentBytes := []byte(finalContent)

	// Check size limit
	if int64(len(newContentBytes)) > t.maxFileSize {
		return nil, fmt.Errorf("file too large after edit: %s (size %d, limit %d)", abs, len(newContentBytes), t.maxFileSize)
	}

	diff, added, removed := computeDiff(oldContent, content)

	perm := os.FileMode(domain.DefaultFilePerm)
	if info != nil {
		perm = info.Mode()
	}

	displayPath := t.pathResolver.DisplayPath(abs)

	return &validatedRequest{
		absPath:          abs,
		newContent:       newContentBytes,
		originalPerm:     perm,
		expectedChecksum: currentChecksum,
		replaceAll:       req.ReplaceAll,
		description:      req.Description,
		target:           fmt.Sprintf("Edit \"%s\"", filepath.ToSlash(displayPath)),
		diff:             diff,
		added:            added,
		removed:          removed,
	}, nil
}

func (t *Tool) executeEdit(ctx context.Context, req *validatedRequest) (string, domain.ToolDisplay) {
	d := domain.NewDiffDisplay(req.description, req.target, req.added, req.removed, req.diff)
	if ctx.Err() != nil {
		d.Error = domain.ToolErrorCancelled
		return domain.ToolErrorCancelled, d
	}

	// Re-read file and verify checksum to prevent TOCTOU race
	data, err := t.fileOps.ReadFile(req.absPath)
	var currentRaw string
	if err != nil {
		if os.IsNotExist(err) {
			currentRaw = ""
		} else {
			if ctx.Err() != nil {
				d.Error = domain.ToolErrorCancelled
				return domain.ToolErrorCancelled, d
			}
			d.Error = domain.ToolErrorFailed
			return fmt.Sprintf("Error: failed to re-read file for verification: %v", err), d
		}
	} else {
		currentRaw = string(data)
	}

	normalized := strings.ReplaceAll(currentRaw, "\r\n", "\n")
	currentChecksum := t.checksumManager.Compute([]byte(normalized))
	if currentChecksum != req.expectedChecksum {
		d.Error = domain.ToolErrorFailed
		return fmt.Sprintf("Error: edit conflict: file changed since edit was prepared: %s", req.absPath), d
	}

	// Write the modified content atomically using pre-computed content
	if err := t.fileOps.WriteFileAtomic(req.absPath, req.newContent, req.originalPerm); err != nil {
		if ctx.Err() != nil {
			d.Error = domain.ToolErrorCancelled
			return domain.ToolErrorCancelled, d
		}
		d.Error = domain.ToolErrorFailed
		return fmt.Sprintf("Error: failed to write modified content: %v", err), d
	}

	// Update checksum cache with normalized content
	newNormalized := strings.ReplaceAll(string(req.newContent), "\r\n", "\n")
	newChecksum := t.checksumManager.Compute([]byte(newNormalized))
	t.checksumManager.Update(req.absPath, newChecksum)

	if req.replaceAll {
		return fmt.Sprintf("The file %s has been updated. All occurrences were successfully replaced.", req.absPath), d
	}
	return fmt.Sprintf("The file %s has been updated successfully.", req.absPath), d
}

func normalizeQuotes(s string) string {
	s = strings.ReplaceAll(s, "‘", "'")
	s = strings.ReplaceAll(s, "’", "'")
	s = strings.ReplaceAll(s, "“", "\"")
	s = strings.ReplaceAll(s, "”", "\"")
	return s
}

func stripTrailingWhitespace(s string) string {
	var lines []string
	for line := range strings.SplitSeq(s, "\n") {
		trimmed := strings.TrimRight(line, " \t\r")
		lines = append(lines, trimmed)
	}
	return strings.Join(lines, "\n")
}

func preserveQuoteStyle(oldStr, actualOldStr, newStr string) string {
	if oldStr == actualOldStr {
		return newStr
	}

	hasDouble := strings.Contains(actualOldStr, "“") || strings.Contains(actualOldStr, "”")
	hasSingle := strings.Contains(actualOldStr, "‘") || strings.Contains(actualOldStr, "’")

	if !hasDouble && !hasSingle {
		return newStr
	}

	result := newStr
	if hasDouble {
		result = applyCurlyDoubleQuotes(result)
	}
	if hasSingle {
		result = applyCurlySingleQuotes(result)
	}

	return result
}

func isOpeningContext(runes []rune, index int) bool {
	if index == 0 {
		return true
	}
	prev := runes[index-1]
	return prev == ' ' || prev == '\t' || prev == '\n' || prev == '\r' || prev == '(' || prev == '[' || prev == '{' || prev == '—' || prev == '–'
}

func applyCurlyDoubleQuotes(s string) string {
	runes := []rune(s)
	var result []rune
	for i, r := range runes {
		if r == '"' {
			if isOpeningContext(runes, i) {
				result = append(result, '“')
			} else {
				result = append(result, '”')
			}
		} else {
			result = append(result, r)
		}
	}
	return string(result)
}

func applyCurlySingleQuotes(s string) string {
	runes := []rune(s)
	var result []rune
	for i, r := range runes {
		if r == '\'' {
			if i > 0 && i < len(runes)-1 {
				prev := runes[i-1]
				next := runes[i+1]
				if (prev >= 'a' && prev <= 'z') || (prev >= 'A' && prev <= 'Z') {
					if (next >= 'a' && next <= 'z') || (next >= 'A' && next <= 'Z') {
						result = append(result, '’')
						continue
					}
				}
			}

			if isOpeningContext(runes, i) {
				result = append(result, '‘')
			} else {
				result = append(result, '’')
			}
		} else {
			result = append(result, r)
		}
	}
	return string(result)
}

func computeDiff(oldContent, newContent string) (diff string, added, removed int) {
	rawDiff := udiff.Unified("", "", oldContent, newContent)

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
