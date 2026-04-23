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
	"github.com/Cyclone1070/iav/internal/tool/helper/pagination"
	"github.com/cloudwego/eino/schema"
)

const (
	defaultReadFileLimit = 2000
)

// fileReader defines the minimal filesystem operations needed for reading files.
type fileReader interface {
	ReadFile(path string) ([]byte, error)
	Stat(path string) (os.FileInfo, error)
}

// checksumComputer defines the interface for checksum computation and updates.
type checksumComputer interface {
	Compute(data []byte) string
	Update(path string, checksum string)
}

// ReadFileTool handles file reading operations.
type ReadFileTool struct {
	fileOps         fileReader
	checksumManager checksumComputer
	pathResolver    pathResolver
}

// NewReadFileTool creates a new ReadFileTool with injected dependencies.
func NewReadFileTool(
	fileOps fileReader,
	checksumManager checksumComputer,
	pathResolver pathResolver,
) *ReadFileTool {
	if fileOps == nil {
		panic("fileOps is required")
	}
	if checksumManager == nil {
		panic("checksumManager is required")
	}
	if pathResolver == nil {
		panic("pathResolver is required")
	}
	return &ReadFileTool{
		fileOps:         fileOps,
		checksumManager: checksumManager,
		pathResolver:    pathResolver,
	}
}

func (t *ReadFileTool) Name() string {
	return "read_file"
}

func (t *ReadFileTool) IsConcurrentSafe() bool { return true }

// Definition returns the tool's schema for the LLM using eino schema.
func (t *ReadFileTool) Definition() *schema.ToolInfo {
	return &schema.ToolInfo{
		Name: "read_file",
		Desc: `Read a file from the local filesystem.

Usage:
- The file_path parameter MUST be an absolute path.
- Results are returned using "cat -n" format, with line numbers starting at 1.
- By default, it reads up to 2000 lines starting from the beginning of the file.
- Negative offsets count backward from the end of the file (-1 is the last line).
- When you already know which part of the file you need, only read that part by specifying offset and limit. This is important for larger files.
- This tool can only read files, not directories. To read a directory, use "ls" via the bash tool.
- If you read a file that exists but has empty contents you will receive a system reminder warning in place of file contents.`,
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"file_path": {
				Type:     schema.String,
				Desc:     "The absolute path to the file to read.",
				Required: true,
			},
			"offset": {
				Type: schema.Integer,
				Desc: "The line index to start reading from. 0 starts at the first line; negative values count backward from end of file (-1 is last line).",
			},
			"limit": {
				Type: schema.Integer,
				Desc: "The number of lines to read. Default and maximum is 2000.",
			},
		}),
	}
}

// ReadFileRequest is the input for ReadFileTool.
type ReadFileRequest struct {
	FilePath string `json:"file_path"`
	Offset   int    `json:"offset,omitempty"`
	Limit    int    `json:"limit,omitempty"`
}

// Prepare validates the request and returns an Invocation.
func (t *ReadFileTool) Prepare(params string) (domain.Invocation, error) {
	req := &ReadFileRequest{}
	if err := json.Unmarshal([]byte(params), req); err != nil {
		return nil, fmt.Errorf("failed to parse arguments: %w", err)
	}

	if req.FilePath == "" {
		return nil, fmt.Errorf("file_path is required")
	}
	if req.Limit <= 0 {
		req.Limit = defaultReadFileLimit
	}

	abs, err := t.pathResolver.Abs(req.FilePath)
	if err != nil {
		return nil, err
	}

	// Fail Fast: Verify file exists and is not a directory
	info, err := t.fileOps.Stat(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("file does not exist: %s", abs)
		}
		return nil, fmt.Errorf("failed to access file: %w", err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("path is a directory, not a file: %s", abs)
	}

	displayPath := t.pathResolver.DisplayPath(abs)

	return &readFileInvocation{
		fileOps:         t.fileOps,
		checksumManager: t.checksumManager,
		absPath:         abs,
		displayPath:     filepath.ToSlash(displayPath),
		offset:          req.Offset,
		limit:           req.Limit,
		display:         domain.NewStringDisplay(fmt.Sprintf("Read \"%s\"", filepath.ToSlash(displayPath)), ""),
	}, nil
}

type readFileInvocation struct {
	fileOps         fileReader
	checksumManager checksumComputer
	absPath         string
	displayPath     string
	offset          int
	limit           int
	display         domain.StringDisplay
}

func (i *readFileInvocation) Display() domain.ToolDisplay {
	return i.display
}

func (i *readFileInvocation) Execute(ctx context.Context) (string, domain.ToolDisplay) {
	d := i.display
	if ctx.Err() != nil {
		d.Error = domain.ToolErrorCancelled
		return domain.ToolErrorCancelled, d
	}

	data, err := i.fileOps.ReadFile(i.absPath)
	if err != nil {
		if ctx.Err() != nil {
			d.Error = domain.ToolErrorCancelled
			return domain.ToolErrorCancelled, d
		}
		d.Error = domain.ToolErrorFailed
		return fmt.Sprintf("Error: failed to read file: %v", err), d
	}

	normalized := strings.ReplaceAll(string(data), "\r\n", "\n")
	checksum := i.checksumManager.Compute([]byte(normalized))
	i.checksumManager.Update(i.absPath, checksum)

	lines := content.SplitLines(normalized)
	offset := i.offset
	if offset < 0 {
		offset = max(len(lines)+offset, 0)
	}
	paginatedLines, pagRes := pagination.ApplyPagination(lines, offset, i.limit)

	startLine := offset + 1
	endLine := startLine + len(paginatedLines) - 1
	if len(paginatedLines) == 0 {
		endLine = startLine - 1
	}
	d.Description = fmt.Sprintf("Read \"%s\" Lines %d-%d", i.displayPath, offset, offset+len(paginatedLines))

	return formatFileContent(paginatedLines, startLine, endLine, pagRes.TotalCount), d
}

func formatFileContent(lines []string, startLine, endLine, totalLines int) string {
	if totalLines == 0 {
		return "<system-reminder>Warning: the file exists but the contents are empty.</system-reminder>"
	}

	if startLine > totalLines {
		return fmt.Sprintf("<system-reminder>Warning: the file exists but is shorter than the provided offset (%d). The file has %d lines.</system-reminder>", startLine, totalLines)
	}

	if len(lines) == 0 {
		return fmt.Sprintf("<file>\n\n(End of file - total %d lines)\n</file>", totalLines)
	}

	var sb strings.Builder
	sb.WriteString("<file>\n")
	for i, line := range lines {
		fmt.Fprintf(&sb, "%d\t%s\n", startLine+i, line)
	}

	if endLine < totalLines {
		fmt.Fprintf(&sb, "\n(File has more lines. Use offset=%d to read more)", endLine)
	} else {
		fmt.Fprintf(&sb, "\n(End of file - total %d lines)", totalLines)
	}
	sb.WriteString("\n</file>")
	return sb.String()
}
