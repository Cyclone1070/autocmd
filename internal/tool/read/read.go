// Package read provides tools for reading, writing, and editing files.
package read

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/runtimectx"
	"github.com/Cyclone1070/iav/internal/tool/helper/content"
	"github.com/Cyclone1070/iav/internal/tool/helper/pagination"
	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

const (
	toolName             = "read_file"
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

// Tool handles file reading operations.
type Tool struct {
	fileOps         fileReader
	checksumManager checksumComputer
	pathResolver    pathResolver
}

// NewTool creates a new Tool with injected dependencies.
func NewTool(
	fileOps fileReader,
	checksumManager checksumComputer,
	pathResolver pathResolver,
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
		pathResolver:    pathResolver,
	}
}


// IsConcurrentSafe indicates if the read file tool can be run concurrently.
func (t *Tool) IsConcurrentSafe() bool { return true }

func (t *Tool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: toolName,
		Desc: `Read a file from the local filesystem.

Usage:
- The file_path parameter MUST be an absolute path (or start with ~).
- Results are returned using "cat -n" format, with line numbers starting at 1.
- By default, it reads up to 2000 lines starting from the beginning of the file.
- Negative offsets count backward from the end of the file (-1 is the last line).
- When you already know which part of the file you need, only read that part by specifying offset and limit. This is important for larger files.
- This tool can only read files, not directories. To read a directory, use "ls" via the bash tool.
- If you read a file that exists but has empty contents you will receive a system reminder warning in place of file contents.`,
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"file_path": {
				Type:     schema.String,
				Desc:     "The absolute path (or ~ path) to the file to read.",
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
	}, nil
}

func (t *Tool) InvokableRun(ctx context.Context, argumentsInJSON string, _ ...einotool.Option) (string, error) {
	validated, err := t.validate(argumentsInJSON)
	if err != nil {
		return "", err
	}
	callID := compose.GetToolCallID(ctx)
	llmContent, finalDisplay := t.executeRead(ctx, validated)
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
		return domain.NewStringDisplay(fmt.Sprintf("Run \"%s\"", toolName), "")
	}
	displayPath := t.pathResolver.DisplayPath(req.FilePath)
	return domain.NewStringDisplay(fmt.Sprintf("Read \"%s\"", displayPath), "")
}

func (t *Tool) PreflightValidate(input *compose.ToolInput) error {
	_, err := t.validate(input.Arguments)
	return err
}

// Request is the input for Tool.
type Request struct {
	FilePath string `json:"file_path"`
	Offset   int    `json:"offset,omitempty"`
	Limit    int    `json:"limit,omitempty"`
}

type validatedRequest struct {
	absPath     string
	displayPath string
	offset      int
	limit       int
}

func (t *Tool) validate(argumentsInJSON string) (*validatedRequest, error) {
	req := &Request{}
	if err := json.Unmarshal([]byte(argumentsInJSON), req); err != nil {
		return nil, fmt.Errorf("failed to parse arguments: %w", err)
	}
	if req.FilePath == "" {
		return nil, fmt.Errorf("file_path is required")
	}
	if req.Limit <= 0 {
		req.Limit = defaultReadFileLimit
	}
	abs, err := t.pathResolver.ValidateAbs(req.FilePath)
	if err != nil {
		return nil, err
	}
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
	return &validatedRequest{
		absPath:     abs,
		displayPath: displayPath,
		offset:      req.Offset,
		limit:       req.Limit,
	}, nil
}

func (t *Tool) executeRead(ctx context.Context, validated *validatedRequest) (string, domain.ToolDisplay) {
	d := domain.NewStringDisplay(fmt.Sprintf("Read \"%s\"", validated.displayPath), "")
	if ctx.Err() != nil {
		d.Error = domain.ToolErrorCancelled
		return domain.ToolErrorCancelled, d
	}

	data, err := t.fileOps.ReadFile(validated.absPath)
	if err != nil {
		if ctx.Err() != nil {
			d.Error = domain.ToolErrorCancelled
			return domain.ToolErrorCancelled, d
		}
		d.Error = domain.ToolErrorFailed
		return fmt.Sprintf("Error: failed to read file: %v", err), d
	}

	normalized := strings.ReplaceAll(string(data), "\r\n", "\n")
	checksum := t.checksumManager.Compute([]byte(normalized))
	t.checksumManager.Update(validated.absPath, checksum)

	lines := content.SplitLines(normalized)
	offset := validated.offset
	if offset < 0 {
		offset = max(len(lines)+offset, 0)
	}
	paginatedLines, pagRes := pagination.ApplyPagination(lines, offset, validated.limit)

	startLine := offset + 1
	endLine := startLine + len(paginatedLines) - 1
	if len(paginatedLines) == 0 {
		endLine = startLine - 1
	}
	d.Description = fmt.Sprintf("Read \"%s\" Lines %d-%d", validated.displayPath, offset, offset+len(paginatedLines))

	return formatContent(paginatedLines, startLine, endLine, pagRes.TotalCount), d
}

func formatContent(lines []string, startLine, endLine, totalLines int) string {
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
