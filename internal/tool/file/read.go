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
	"github.com/Cyclone1070/iav/internal/tool/helper/content"
	"github.com/Cyclone1070/iav/internal/tool/helper/pagination"
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
	config          *config.Config
}

// NewReadFileTool creates a new ReadFileTool with injected dependencies.
func NewReadFileTool(
	fileOps fileReader,
	checksumManager checksumComputer,
	pathResolver pathResolver,
	cfg *config.Config,
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
	if cfg == nil {
		panic("config is required")
	}
	return &ReadFileTool{
		fileOps:         fileOps,
		checksumManager: checksumManager,
		pathResolver:    pathResolver,
		config:          cfg,
	}
}

// Name returns the tool's identifier.
func (t *ReadFileTool) Name() string {
	return "read_file"
}

// Declaration returns the tool's schema for the LLM.
func (t *ReadFileTool) Declaration() domain.Declaration {
	return domain.Declaration{
		Name:        "read_file",
		Description: "Read file contents with optional pagination. Use offset/limit to read large files in chunks.",
		Parameters: &domain.Schema{
			Type: domain.TypeObject,
			Properties: map[string]*domain.Schema{
				"path":   {Type: domain.TypeString, Description: "Path to file"},
				"offset": {Type: domain.TypeInteger, Description: "Start line index (0-indexed)"},
				"limit":  {Type: domain.TypeInteger, Description: "Max lines to return"},
			},
			Required: []string{"path"},
		},
	}
}

// ReadFileRequest is the input for ReadFileTool.
type ReadFileRequest struct {
	Path   string `json:"path"`
	Offset int    `json:"offset,omitempty"`
	Limit  int    `json:"limit,omitempty"`
}

// Prepare validates the request and returns an Invocation.
func (t *ReadFileTool) Prepare(ctx context.Context, params json.RawMessage) (domain.Invocation, error) {
	req := &ReadFileRequest{}
	if err := json.Unmarshal(params, req); err != nil {
		return nil, fmt.Errorf("failed to parse arguments: %w", err)
	}

	if req.Path == "" {
		return nil, fmt.Errorf("path is required")
	}
	if req.Offset < 0 {
		req.Offset = 0
	}
	if req.Limit <= 0 {
		req.Limit = t.config.Tools.DefaultReadFileLimit
	}

	abs, err := t.pathResolver.Abs(req.Path)
	if err != nil {
		return nil, err
	}

	// Fail Fast: Verify file exists and is not a directory
	info, err := t.fileOps.Stat(abs)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("file does not exist: %s", abs)
		}
		return nil, fmt.Errorf("failed to access file: %w", err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("path is a directory, not a file: %s", abs)
	}

	return &readFileInvocation{
		fileOps:         t.fileOps,
		checksumManager: t.checksumManager,
		absPath:         abs,
		offset:          req.Offset,
		limit:           req.Limit,
		display:         domain.StringDisplay(fmt.Sprintf("Read %s", filepath.Base(req.Path))),
	}, nil
}

type readFileInvocation struct {
	fileOps         fileReader
	checksumManager checksumComputer
	absPath         string
	offset          int
	limit           int
	display         domain.ToolDisplay
}

func (i *readFileInvocation) Display() domain.ToolDisplay {
	return i.display
}

func (i *readFileInvocation) Execute(ctx context.Context) (string, error) {
	if ctx.Err() != nil {
		return "", ctx.Err()
	}

	data, err := i.fileOps.ReadFile(i.absPath)
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return fmt.Sprintf("Error: %s", err.Error()), err
	}

	normalized := strings.ReplaceAll(string(data), "\r\n", "\n")
	checksum := i.checksumManager.Compute([]byte(normalized))
	i.checksumManager.Update(i.absPath, checksum)

	lines := content.SplitLines(string(data))
	paginatedLines, pagRes := pagination.ApplyPagination(lines, i.offset, i.limit)

	startLine := i.offset + 1
	endLine := startLine + len(paginatedLines) - 1
	if len(paginatedLines) == 0 {
		endLine = startLine - 1
	}

	return formatFileContent(paginatedLines, startLine, endLine, pagRes.TotalCount), nil
}

func formatFileContent(lines []string, startLine, endLine, totalLines int) string {
	if len(lines) == 0 {
		return fmt.Sprintf("<file>\n\n(End of file - total %d lines)\n</file>", totalLines)
	}

	var sb strings.Builder
	sb.WriteString("<file>\n")
	for i, line := range lines {
		fmt.Fprintf(&sb, "%05d| %s\n", startLine+i, line)
	}

	if endLine < totalLines {
		fmt.Fprintf(&sb, "\n(File has more lines. Use offset=%d to read more)", endLine)
	} else {
		fmt.Fprintf(&sb, "\n(End of file - total %d lines)", totalLines)
	}
	sb.WriteString("\n</file>")
	return sb.String()
}
