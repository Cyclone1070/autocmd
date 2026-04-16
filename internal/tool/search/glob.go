package search

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/cloudwego/eino/schema"
)

const (
	DefaultGlobTimeout = 20 * time.Second
)

// GlobRequest represents the parameters for a glob operation
type GlobRequest struct {
	Pattern string `json:"pattern"`
	Path    string `json:"path"`
}

// GlobTool handles file finding operations via glob patterns.
type GlobTool struct {
	fs              fileSystem
	commandExecutor commandExecutor
	pathResolver    pathResolver
}

// NewGlobTool creates a new GlobTool with injected dependencies.
func NewGlobTool(
	fs fileSystem,
	commandExecutor commandExecutor,
	pathResolver pathResolver,
) *GlobTool {
	if fs == nil {
		panic("fs is required")
	}
	if commandExecutor == nil {
		panic("commandExecutor is required")
	}
	if pathResolver == nil {
		panic("pathResolver is required")
	}
	return &GlobTool{
		fs:              fs,
		commandExecutor: commandExecutor,
		pathResolver:    pathResolver,
	}
}

// Name returns the name of the tool.
func (t *GlobTool) Name() string {
	return "glob"
}

func (t *GlobTool) IsConcurrentSafe() bool { return true }

// Definition returns the JSON schema for the tool.
func (t *GlobTool) Definition() *schema.ToolInfo {
	return &schema.ToolInfo{
		Name: t.Name(),
		Desc: `- Fast file pattern matching tool that works with any codebase size
- Supports glob patterns like "**/*.js" or "src/**/*.ts"
- Returns matching file paths sorted by modification time
- Use this tool when you need to find files by name patterns`,
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"pattern": {
				Type:     schema.String,
				Desc:     "The glob pattern to match files against",
				Required: true,
			},
			"path": {
				Type: schema.String,
				Desc: "The directory to search in. If not specified, the current working directory will be used. IMPORTANT: Omit this field to use the default directory. DO NOT enter \"undefined\" or \"null\" - simply omit it for the default behavior. Must be a valid directory path if provided.",
			},
		}),
	}
}

// Prepare validates input and resolves path.
func (t *GlobTool) Prepare(params string) (domain.Invocation, error) {
	req := &GlobRequest{}
	if err := json.Unmarshal([]byte(params), req); err != nil {
		return nil, fmt.Errorf("failed to parse arguments: %w", err)
	}

	if req.Pattern == "" {
		return nil, fmt.Errorf("pattern is required")
	}

	// Validate pattern syntax
	if _, err := filepath.Match(req.Pattern, ""); err != nil {
		return nil, fmt.Errorf("invalid pattern %s: %v", req.Pattern, err)
	}

	searchPath := req.Path
	if searchPath == "" {
		searchPath = "."
	}

	absPath, err := t.pathResolver.Abs(searchPath)
	if err != nil {
		return nil, err
	}

	displayPath := t.pathResolver.DisplayPath(absPath)

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

	// DisplayPath is used for the TUI summary below

	return &globInvocation{
		tool:    t,
		absPath: absPath,
		pattern: req.Pattern,
		display: domain.NewStringDisplay("", fmt.Sprintf("GLOB \"%s\" IN \"%s\"", req.Pattern, filepath.ToSlash(displayPath))),
	}, nil
}

type globInvocation struct {
	tool    *GlobTool
	pattern string
	absPath string
	display domain.StringDisplay
}

func (i *globInvocation) Display() domain.ToolDisplay {
	return i.display
}

func (i *globInvocation) Execute(ctx context.Context) (string, domain.ToolDisplay) {
	d := i.display

	if ctx.Err() != nil {
		d.Error = domain.ToolErrorCancelled
		return domain.ToolErrorCancelled, d
	}

	// Search target should be relative to workspace root for clean relative output
	relPath, err := filepath.Rel(i.tool.pathResolver.Root(), i.absPath)
	if err != nil || strings.HasPrefix(relPath, "..") {
		relPath = i.absPath
	}
	workDir := i.tool.pathResolver.Root()

	// rg --files --glob "pattern" --sort=modified --no-ignore --hidden <target>
	args := []string{"rg", "--files", "--glob", i.pattern, "--sort=modified", "--no-ignore", "--hidden", relPath}
	cmdStr := joinArgs(args)

	ctx, cancel := context.WithTimeout(ctx, DefaultGlobTimeout)
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

	if res.ExitCode != 0 && res.ExitCode != 1 && !timedOut {
		d.Error = domain.ToolErrorFailed
		output = fmt.Sprintf("Error: ripgrep failed with exit code %d\n%s", res.ExitCode, output)
	} else if output == "" && (res.ExitCode == 0 || res.ExitCode == 1) && !timedOut {
		return "No files found", d
	}

	if timedOut {
		d.Error = domain.ToolErrorTimedOut
		output = fmt.Sprintf("%s\n\n<execution_status>\n  <exit_code>%d</exit_code>\n  <timedout>true</timedout>\n</execution_status>", output, res.ExitCode)
	}

	return strings.TrimSpace(output), d
}

func (i *globInvocation) countLines(path string) (int, error) {
	f, err := i.tool.fs.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	count := 0
	buf := make([]byte, 32*1024)
	for {
		n, err := f.Read(buf)
		if n > 0 {
			for idx := range n {
				if buf[idx] == '\n' {
					count++
				}
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return count, err
		}
	}
	return count, nil
}
