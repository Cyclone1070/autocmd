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
	"github.com/Cyclone1070/iav/internal/runtimectx"
	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

const (
	toolName           = "glob"
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


// IsConcurrentSafe indicates if the glob tool can be run concurrently.
func (t *Tool) IsConcurrentSafe() bool { return true }

func (t *Tool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: toolName,
		Desc: `Fast file pattern matching tool that works with any codebase size.

Usage:
- The path parameter MUST be an absolute path (or start with ~).
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
				Desc: fmt.Sprintf("The absolute (or ~) directory path to search in. If not specified, the workspace root (currently \"%s\") will be used.", t.pathResolver.Root()),
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
	llmContent, finalDisplay := t.executeGlob(ctx, req)
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
	searchPath := req.Path
	if searchPath == "" {
		searchPath = t.pathResolver.Root()
	}
	displayPath := t.pathResolver.DisplayPath(searchPath)
	return domain.NewStringDisplay(fmt.Sprintf("Glob \"%s\" in \"%s\"", req.Pattern, displayPath), "")
}

func (t *Tool) PreflightValidate(input *compose.ToolInput) error {
	_, err := t.validate(input.Arguments)
	return err
}

type validatedRequest struct {
	absPath string
	pattern string
}

func (t *Tool) validate(params string) (*validatedRequest, error) {
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

	absPath, err := t.pathResolver.ValidateAbs(searchPath)
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

	return &validatedRequest{
		absPath: absPath,
		pattern: req.Pattern,
	}, nil
}

func (t *Tool) executeGlob(ctx context.Context, req *validatedRequest) (string, domain.ToolDisplay) {
	displayPath := t.pathResolver.DisplayPath(req.absPath)
	d := domain.NewStringDisplay(fmt.Sprintf("Glob \"%s\" in \"%s\"", req.pattern, filepath.ToSlash(displayPath)), "")
	if ctx.Err() != nil {
		return domain.ToolErrorCancelled, d.WithError(domain.ToolErrorCancelled)
	}

	workDir := t.pathResolver.Root()

	// Use absolute path for rg to get absolute path output
	args := []string{"rg", "--files", "--glob", req.pattern, "--sort=modified", "--no-ignore", "--hidden", req.absPath}
	cmdStr := joinArgs(args)

	ctx, cancel := context.WithTimeout(ctx, defaultGlobTimeout)
	defer cancel()

	res, err := t.commandExecutor.Run(ctx, cmdStr, workDir, true)
	timedOut := false
	if err != nil {
		switch {
		case errors.Is(err, context.DeadlineExceeded):
			d.Error = domain.ToolErrorTimedOut
			timedOut = true
		case errors.Is(err, context.Canceled):
			d.Error = domain.ToolErrorCancelled
			return domain.ToolErrorCancelled, d
		default:
			d.Error = domain.ToolErrorFailed
			return fmt.Sprintf("Error: ripgrep failed to start: %v", err), d
		}
	}

	output := res.Stdout
	var count int
	if res.LogPath != "" {
		count, _ = t.countLines(res.LogPath)
		output = fmt.Sprintf("Output too large (%d files found). Full output saved to %s. Use `read_file` tool to read full output.", count, res.LogPath)
	} else {
		count = strings.Count(output, "\n")
	}

	d.Description = fmt.Sprintf("%s (%d files)", d.Description, count)
	if output == "" {
		output = "No files found"
	}

	if !timedOut {
		if res.ExitCode != 0 && res.ExitCode != 1 {
			output = fmt.Sprintf("Error: ripgrep failed\n%s", output)
		}
	}

	output = strings.TrimRight(output, "\n")
	output = fmt.Sprintf("%s\n\n<exit_code>%d</exit_code>", output, res.ExitCode)
	if timedOut {
		output = fmt.Sprintf("%s\n<timeout>true</timeout>", output)
		return output, d.WithError(domain.ToolErrorTimedOut)
	}

	return strings.TrimSpace(output), d
}

func (t *Tool) countLines(path string) (int, error) {
	f, err := t.fs.Open(path)
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

