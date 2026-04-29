// Package glob provides tools for finding files and searching their content.
package glob

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/cloudwego/eino/schema"
)

const (
	defaultGlobTimeout = 20 * time.Second
	readBufferSize     = 32 * 1024
)

// Request represents the parameters for a glob operation.
type Request struct {
	Pattern string `json:"pattern"`
	Path    string `json:"path"`
}

// Tool handles file finding operations via glob patterns.
type Tool struct {
	fs              fileSystem
	commandExecutor commandExecutor
	pathResolver    pathResolver
}

// NewTool creates a new Tool with injected dependencies.
func NewTool(
	fs fileSystem,
	commandExecutor commandExecutor,
	pathResolver pathResolver,
) *Tool {
	if fs == nil {
		panic("fs is required")
	}
	if commandExecutor == nil {
		panic("commandExecutor is required")
	}
	if pathResolver == nil {
		panic("pathResolver is required")
	}
	return &Tool{
		fs:              fs,
		commandExecutor: commandExecutor,
		pathResolver:    pathResolver,
	}
}

// Name returns the name of the tool.
func (t *Tool) Name() string {
	return "glob"
}

// IsConcurrentSafe indicates if the glob tool can be run concurrently.
func (t *Tool) IsConcurrentSafe() bool { return true }

// Definition returns the JSON schema for the tool.
func (t *Tool) Definition() *schema.ToolInfo {
	return &schema.ToolInfo{
		Name: t.Name(),
		Desc: `Fast file pattern matching tool that works with any codebase size.

Usage:
- The path parameter MUST be an absolute path.
- Supports glob patterns like "**/*.js" or "src/**/*.ts".
- Returns matching absolute file paths sorted by modification time.
- Use this tool when you need to find files by name patterns.`,
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"pattern": {
				Type:     schema.String,
				Desc:     "The glob pattern to match files against",
				Required: true,
			},
			"path": {
				Type: schema.String,
				Desc: fmt.Sprintf("The absolute directory path to search in. If not specified, the workspace root (currently \"%s\") will be used.", t.pathResolver.Root()),
			},
		}),
	}
}

// Prepare validates input and resolves path.
func (t *Tool) Prepare(params string) (domain.Invocation, error) {
	req := &Request{}
	if err := json.Unmarshal([]byte(params), req); err != nil {
		return nil, fmt.Errorf("failed to parse arguments: %w", err)
	}

	if req.Pattern == "" {
		return nil, fmt.Errorf("pattern is required")
	}

	// Validate pattern syntax
	if _, err := filepath.Match(req.Pattern, ""); err != nil {
		return nil, fmt.Errorf("invalid pattern %s: %w", req.Pattern, err)
	}

	searchPath := req.Path
	if searchPath == "" {
		searchPath = t.pathResolver.Root()
	}

	absPath, err := t.pathResolver.Abs(searchPath)
	if err != nil {
		return nil, err
	}

	// Fail Fast: Verify path exists and is a directory
	info, err := t.fs.Stat(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("path does not exist: %s", absPath)
		}
		return nil, fmt.Errorf("failed to access path: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("not a directory: %s", absPath)
	}

	displayPath := t.pathResolver.DisplayPath(absPath)

	return &invocation{
		tool:    t,
		absPath: absPath,
		pattern: req.Pattern,
		display: domain.NewStringDisplay(fmt.Sprintf("Glob \"%s\" in \"%s\"", req.Pattern, filepath.ToSlash(displayPath)), ""),
	}, nil
}

type invocation struct {
	tool    *Tool
	pattern string
	absPath string
	display domain.StringDisplay
}

func (i *invocation) Display() domain.ToolDisplay {
	return i.display
}

func (i *invocation) Execute(ctx context.Context) (string, domain.ToolDisplay) {
	d := i.display

	if ctx.Err() != nil {
		d.Error = domain.ToolErrorCancelled
		return domain.ToolErrorCancelled, d
	}

	workDir := i.tool.pathResolver.Root()

	// Use absolute path for rg to get absolute path output
	args := []string{"rg", "--files", "--glob", i.pattern, "--sort=modified", "--no-ignore", "--hidden", i.absPath}
	cmdStr := joinArgs(args)

	ctx, cancel := context.WithTimeout(ctx, defaultGlobTimeout)
	defer cancel()

	res, err := i.tool.commandExecutor.Run(ctx, cmdStr, workDir, true)
	timedOut := false
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			timedOut = true
		} else if ctx.Err() != nil {
			d.Error = domain.ToolErrorCancelled
			return domain.ToolErrorCancelled, d
		} else {
			d.Error = domain.ToolErrorFailed
			return fmt.Sprintf("Error: ripgrep failed to start: %v", err), d
		}
	}

	output := res.Stdout
	if res.LogPath != "" {
		count, _ := i.countLines(res.LogPath)
		output = fmt.Sprintf("Output too large (%d files found). Full output saved to %s. Use `read_file` tool to read full output.", count, res.LogPath)
	}
	if output == "" {
		output = "No files found"
	}

	if !timedOut {
		if res.ExitCode != 0 && res.ExitCode != 1 {
			d.Error = domain.ToolErrorFailed
			output = fmt.Sprintf("Error: ripgrep failed\n%s", output)
		}
	}

	output = strings.TrimRight(output, "\n")
	output = fmt.Sprintf("%s\n\n<exit_code>%d</exit_code>", output, res.ExitCode)
	if timedOut {
		d.Error = domain.ToolErrorTimedOut
		output = fmt.Sprintf("%s\n<timeout>true</timeout>", output)
	}

	return strings.TrimSpace(output), d
}

func (i *invocation) countLines(path string) (int, error) {
	f, err := i.tool.fs.Open(path)
	if err != nil {
		return 0, err
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil {
			slog.Error("failed to close file", "error", closeErr)
		}
	}()

	count := 0
	buf := make([]byte, readBufferSize)
	for {
		n, err := f.Read(buf)
		if n > 0 {
			for idx := range n {
				if buf[idx] == '\n' {
					count++
				}
			}
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return count, err
		}
	}
	return count, nil
}
