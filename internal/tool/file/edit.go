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

// EditFileRequest is the input for EditFileTool.
type EditFileRequest struct {
	FilePath   string `json:"file_path"`
	Comment    string `json:"comment"`
	OldString  string `json:"old_string"`
	NewString  string `json:"new_string"`
	ReplaceAll bool   `json:"replace_all"`
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
		Desc: `Performs exact string replacements in files.

Usage:
- You must use your ` + "`read_file`" + ` tool at least once in the conversation before editing. This tool will error if you attempt an edit without reading the file. 
- When editing text from Read tool output, ensure you preserve the exact indentation (tabs/spaces) as it appears AFTER the line number prefix. The line number prefix format is: line number + tab. Everything after that is the actual file content to match. Never include any part of the line number prefix in the old_string or new_string.
- ALWAYS prefer editing existing files in the codebase. NEVER write new files unless explicitly required.
- Only use emojis if the user explicitly requests it. Avoid adding emojis to files unless asked.
- The edit will FAIL if ` + "`old_string`" + ` is not unique in the file. Either provide a larger string with more surrounding context to make it unique or use ` + "`replace_all`" + ` to change every instance of ` + "`old_string`" + `.
- Use ` + "`replace_all`" + ` for replacing and renaming strings across the file. This parameter is useful if you want to rename a variable for instance.`,
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"file_path": {
				Type:     schema.String,
				Desc:     "The path to the file to edit (absolute or relative to the workspace root).",
				Required: true,
			},
			"comment": {
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
	}
}

// Prepare validates the request, reads the file, applies edits in memory, and returns an Invocation.
func (t *EditFileTool) Prepare(params string) (domain.Invocation, error) {
	req := &EditFileRequest{}
	if err := json.Unmarshal([]byte(params), req); err != nil {
		return nil, fmt.Errorf("failed to parse arguments: %w", err)
	}

	if req.FilePath == "" {
		return nil, fmt.Errorf("file_path is required")
	}
	if req.Comment == "" {
		return nil, fmt.Errorf("comment is required")
	}

	abs, err := t.pathResolver.Abs(req.FilePath)
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
				return nil, fmt.Errorf("file does not exist: %s", req.FilePath)
			}
			// File doesn't exist and OldString is empty: new file creation
			rawContent = ""
		} else {
			return nil, fmt.Errorf("failed to stat %s: %w", req.FilePath, err)
		}
	} else {
		data, err := t.fileOps.ReadFile(abs)
		if err != nil {
			return nil, fmt.Errorf("failed to read file %s: %w", req.FilePath, err)
		}
		rawContent = string(data)

		if req.OldString == "" {
			if strings.TrimSpace(rawContent) != "" {
				return nil, fmt.Errorf("Cannot create new file - file already exists.")
			}
		}
	}

	hasCRLF := strings.Contains(rawContent, "\r\n")
	oldContent := strings.ReplaceAll(rawContent, "\r\n", "\n")

	// Check for conflicts with cached version
	currentChecksum = t.checksumManager.Compute([]byte(oldContent))
	priorChecksum, checksumOk := t.checksumManager.Get(abs)
	if checksumOk && priorChecksum != currentChecksum {
		return nil, fmt.Errorf("edit conflict: file changed since last read: %s", req.FilePath)
	}

	// Apply replacement
	before := strings.ReplaceAll(req.OldString, "\r\n", "\n")
	after := strings.ReplaceAll(req.NewString, "\r\n", "\n")

	// Strip trailing whitespace from new text unless it is a markdown file
	// to avoid accidental dirty edits from the LLM.
	isMarkdown := strings.HasSuffix(req.FilePath, ".md") || strings.HasSuffix(req.FilePath, ".mdx")
	if !isMarkdown {
		after = stripTrailingWhitespace(after)
	}

	var content string
	var matches int
	var actualOldString string

	if before == "" {
		// If OldString is empty, we replace the entire content (which must be empty per check above)
		content = after
		matches = 1
		actualOldString = ""
	} else {
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
				// Since curly quotes and straight quotes are all 1 character in Go's string indeces?
				// No, Go strings are UTF-8. index returns byte index.
				// We need to work with runes.
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
					return nil, fmt.Errorf("String to replace not found in file (normalization failed).\nString: %s", req.OldString)
				}
			} else {
				return nil, fmt.Errorf("String to replace not found in file.\nString: %s", req.OldString)
			}
		}

		matches = strings.Count(oldContent, actualOldString)
		if matches == 0 {
			// Should not happen if we found it above, but safe guard
			return nil, fmt.Errorf("String to replace not found in file.\nString: %s", req.OldString)
		}

		if matches > 1 && !req.ReplaceAll {
			return nil, fmt.Errorf("Found %d matches of the string to replace, but replace_all is false. To replace all occurrences, set replace_all to true. To replace only one occurrence, please provide more context to uniquely identify the instance.\nString: %s", matches, req.OldString)
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
		return nil, fmt.Errorf("file too large after edit: %s (size %d, limit %d)", req.FilePath, len(newContentBytes), t.maxFileSize)
	}

	diff, added, removed := computeUnifiedDiff(oldContent, content)

	displayPath := t.pathResolver.DisplayPath(abs)

	perm := os.FileMode(0o644)
	if info != nil {
		perm = info.Mode()
	}

	return &editFileInvocation{
		fileOps:          t.fileOps,
		checksumManager:  t.checksumManager,
		absPath:          abs,
		relPath:          req.FilePath,
		newContent:       newContentBytes,
		originalPerm:     perm,
		expectedChecksum: currentChecksum,
		replaceAll:       req.ReplaceAll,
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
	replaceAll       bool
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

	if i.replaceAll {
		return fmt.Sprintf("The file %s has been updated. All occurrences were successfully replaced.", i.relPath), d
	}
	return fmt.Sprintf("The file %s has been updated successfully.", i.relPath), d
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
	// Handle both CRLF and LF by working with \n and ignoring \r
	for line := range strings.SplitSeq(s, "\n") {
		trimmed := strings.TrimRight(line, " \t\r")
		lines = append(lines, trimmed)
	}
	return strings.Join(lines, "\n")
}

func preserveQuoteStyle(oldStr, actualOldStr, newStr string) string {
	// If they're identical, no normalization occurred
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
	// Standard opening contexts: space, tab, newline, or typical openers
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
			// Don't convert apostrophes in contractions (e.g., "don't")
			// Heuristic: letter on both sides -> closing curly quote (apostrophe)
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
