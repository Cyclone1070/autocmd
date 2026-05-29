// Package grep provides tools for finding files and searching their content.
package grep

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Cyclone1070/autocmd/internal/domain"
	"github.com/Cyclone1070/autocmd/internal/runtimectx"
	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

const (
	toolName = "grep"

	// Prevents base64/minified files from blowing up context.
	defaultMaxColumns = 500

	outputModeContent          = "content"
	outputModeFilesWithMatches = "files_with_matches"
	outputModeCount            = "count"

	defaultOutputMode = outputModeFilesWithMatches

	defaultGrepTimeout = 20 * time.Second
	logTailSize        = 1024
)

var vcsExclusions = []string{".git", ".svn", ".hg", ".bzr", ".jj", ".sl"}

// Request matches Claude Code's input schema.
type Request struct {
	Pattern         string `json:"pattern"`
	Path            string `json:"path"`
	Glob            string `json:"glob,omitempty"`
	OutputMode      string `json:"output_mode,omitempty"`
	ContextLines    *int   `json:"context,omitempty"`
	ContextC        *int   `json:"-C,omitempty"`
	ContextB        *int   `json:"-B,omitempty"`
	ContextA        *int   `json:"-A,omitempty"`
	ShowLineNumbers *bool  `json:"-n,omitempty"`
	I               *bool  `json:"-i,omitempty"`
	CaseInsensitive *bool  `json:"case_insensitive,omitempty"`
	Multiline       *bool  `json:"multiline,omitempty"`
	Type            string `json:"type,omitempty"`
}

// Tool handles content searching operations.
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

// IsConcurrentSafe indicates if the grep tool can be run concurrently.
func (t *Tool) IsConcurrentSafe() bool { return true }

func (t *Tool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: toolName,
		Desc: `A powerful search tool built on ripgrep.

Usage:
- The path parameter MUST be an absolute path (or start with ~).
- ALWAYS use this tool for search tasks. NEVER invoke "grep" or "rg" as a bash command. This tool has been optimized for correct permissions and access.
- Supports full regex syntax (e.g., "log.*Error", "function\s+\w+").
- Filter files with the "glob" parameter (e.g., "*.js", "**/*.tsx") or "type" parameter (e.g., "js", "py", "rust").
- Output modes:
  * "content": shows matching lines.
  * "files_with_matches": shows only file paths (default).
  * "count": shows match counts.
- Pattern syntax: Uses ripgrep (not grep) - literal braces need escaping (use "interface\{\}" to find "interface{}" in Go code).
- Multiline matching: By default, patterns match within single lines only. For cross-line patterns like "struct \{[\s\S]*?field", use "multiline: true".`,
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"pattern": {
				Type:     schema.String,
				Desc:     "The regular expression pattern to search for in file contents",
				Required: true,
			},
			"path": {
				Type: schema.String,
				Desc: fmt.Sprintf("Absolute path (or ~ path) to file or directory to search in (rg PATH). Defaults to workspace root (currently \"%s\").", t.pathResolver.Root()),
			},
			"glob": {
				Type: schema.String,
				Desc: "Glob pattern to filter files (e.g. \"*.js\", \"*.{ts,tsx}\") - maps to rg --glob",
			},
			"output_mode": {
				Type: schema.String,
				Enum: []string{outputModeContent, outputModeFilesWithMatches, outputModeCount},
				Desc: fmt.Sprintf("Output mode: \"%s\" shows matching lines (supports -A/-B/-C context, -n line numbers, head_limit), \"%s\" shows file paths (supports head_limit), \"%s\" shows match counts (supports head_limit). Defaults to \"%s\".", outputModeContent, outputModeFilesWithMatches, outputModeCount, outputModeFilesWithMatches),
			},
			"-B": {
				Type: schema.Integer,
				Desc: "Number of lines to show before each match (rg -B). Requires output_mode: \"content\", ignored otherwise.",
			},
			"-A": {
				Type: schema.Integer,
				Desc: "Number of lines to show after each match (rg -A). Requires output_mode: \"content\", ignored otherwise.",
			},
			"-C": {
				Type: schema.Integer,
				Desc: "Alias for context.",
			},
			"context": {
				Type: schema.Integer,
				Desc: "Number of lines to show before and after each match (rg -C). Requires output_mode: \"content\", ignored otherwise.",
			},
			"-n": {
				Type: schema.Boolean,
				Desc: "Show line numbers in output (rg -n). Requires output_mode: \"content\", ignored otherwise. Defaults to true.",
			},
			"-i": {
				Type: schema.Boolean,
				Desc: "Case insensitive search (rg -i)",
			},
			"type": {
				Type: schema.String,
				Desc: "File type to search (rg --type). Common types: js, py, rust, go, java, etc. More efficient than include for standard file types.",
			},
			"multiline": {
				Type: schema.Boolean,
				Desc: "Enable multiline mode where . matches newlines and patterns can span lines (rg -U --multiline-dotall). Default: false.",
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
	llmContent, finalDisplay := t.executeGrep(ctx, req)
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
	return domain.NewStringDisplay(fmt.Sprintf("Grep \"%s\" in \"%s\"", req.Pattern, displayPath), "")
}

func (t *Tool) PreflightValidate(input *compose.ToolInput) error {
	_, err := t.validate(input.Arguments)
	return err
}

type validatedRequest struct {
	req     *Request
	absPath string
}

func (t *Tool) validate(params string) (*validatedRequest, error) {
	req := &Request{}
	if err := json.Unmarshal([]byte(params), req); err != nil {
		return nil, fmt.Errorf("failed to parse arguments: %w", err)
	}

	if req.Pattern == "" {
		return nil, fmt.Errorf("pattern is required")
	}

	// Consolidate aliases
	if req.CaseInsensitive != nil && req.I == nil {
		req.I = req.CaseInsensitive
	}

	// Populate defaults
	if req.OutputMode == "" {
		req.OutputMode = defaultOutputMode
	}
	if req.ShowLineNumbers == nil {
		req.ShowLineNumbers = new(bool)
		*req.ShowLineNumbers = true
	}
	if req.I == nil {
		req.I = new(bool)
		*req.I = false
	}

	searchPath := req.Path
	if searchPath == "" {
		searchPath = t.pathResolver.Root()
	}

	absSearchPath, err := t.pathResolver.ValidateAbs(searchPath)
	if err != nil {
		return nil, err
	}

	// Check if path exists
	_, err = t.fs.Stat(absSearchPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("path does not exist: %s", absSearchPath)
		}
		return nil, fmt.Errorf("failed to stat %s: %w", absSearchPath, err)
	}

	return &validatedRequest{
		req:     req,
		absPath: absSearchPath,
	}, nil
}

func (t *Tool) executeGrep(ctx context.Context, req *validatedRequest) (string, domain.ToolDisplay) {
	displayPath := t.pathResolver.DisplayPath(req.absPath)
	d := domain.NewStringDisplay(fmt.Sprintf("Grep \"%s\" in \"%s\"", req.req.Pattern, filepath.ToSlash(displayPath)), "")
	if ctx.Err() != nil {
		return domain.ToolErrorCancelled, d.WithError(domain.ToolErrorCancelled)
	}

	cmdStr := t.prepareCommand(req)
	workDir := t.pathResolver.Root()

	ctx, cancel := context.WithTimeout(ctx, defaultGrepTimeout)
	defer cancel()

	res, err := t.commandExecutor.Run(ctx, cmdStr, workDir, true)
	timedOut := false
	if err != nil {
		switch {
		case errors.Is(err, context.DeadlineExceeded):
			timedOut = true
		case errors.Is(err, context.Canceled):
			d.Error = domain.ToolErrorCancelled
			return domain.ToolErrorCancelled, d
		default:
			d.Error = domain.ToolErrorFailed
			return fmt.Sprintf("Error: rg failed: %v", err), d
		}
	}

	output := res.Stdout
	var matches, files int
	if res.LogPath != "" {
		matches, files, _ = t.analyzeLog(res.LogPath)
		output = fmt.Sprintf("Output too large (%d matches across %d files). Full output saved to %s. Use `read_file` tool to read full output.", matches, files, res.LogPath)
	} else {
		matches, files = t.parseStats(output)
	}

	d.Description = fmt.Sprintf("%s (%d matches in %d files)", d.Description, matches, files)
	if output == "" {
		output = "No matches found"
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

	return output, d
}

func (t *Tool) prepareCommand(req *validatedRequest) string {
	mode := req.req.OutputMode
	args := []string{"rg", "--hidden", "--stats"}
	for _, excl := range vcsExclusions {
		args = append(args, "--glob", "!"+excl)
	}
	args = append(args, "--max-columns", strconv.Itoa(defaultMaxColumns))

	switch mode {
	case outputModeFilesWithMatches:
		args = append(args, "-l")
	case outputModeCount:
		args = append(args, "-c", "--with-filename")
	case outputModeContent:
		args = append(args, "--with-filename")
		if req.req.ShowLineNumbers == nil || *req.req.ShowLineNumbers {
			args = append(args, "-n")
		}
		contextLines := 0
		if req.req.ContextLines != nil {
			contextLines = *req.req.ContextLines
		} else if req.req.ContextC != nil {
			contextLines = *req.req.ContextC
		}
		if contextLines > 0 {
			args = append(args, "-C", strconv.Itoa(contextLines))
		} else {
			if req.req.ContextB != nil && *req.req.ContextB > 0 {
				args = append(args, "-B", strconv.Itoa(*req.req.ContextB))
			}
			if req.req.ContextA != nil && *req.req.ContextA > 0 {
				args = append(args, "-A", strconv.Itoa(*req.req.ContextA))
			}
		}
	}

	if req.req.I != nil && *req.req.I {
		args = append(args, "-i")
	}
	if req.req.Multiline != nil && *req.req.Multiline {
		args = append(args, "-U", "--multiline-dotall")
	}
	if req.req.Type != "" {
		args = append(args, "--type", req.req.Type)
	}
	if req.req.Glob != "" {
		globs := splitGlobs(req.req.Glob)
		for _, g := range globs {
			args = append(args, "--glob", g)
		}
	}

	if strings.HasPrefix(req.req.Pattern, "-") {
		args = append(args, "-e", req.req.Pattern)
	} else {
		args = append(args, req.req.Pattern)
	}

	// Always use absolute path for rg to get absolute path output
	args = append(args, req.absPath)

	return joinArgs(args)
}

func (t *Tool) analyzeLog(path string) (matches, files int, err error) {
	f, err := t.fs.Open(path)
	if err != nil {
		return 0, 0, err
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil {
			slog.Error("failed to close file", "error", closeErr)
		}
	}()

	info, err := f.Stat()
	if err != nil {
		return 0, 0, err
	}

	offset := max(info.Size()-int64(logTailSize), 0)

	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return 0, 0, err
	}

	content, err := io.ReadAll(f)
	if err != nil {
		return 0, 0, err
	}

	matches, files = t.parseStats(string(content))
	return matches, files, nil
}

func (t *Tool) parseStats(content string) (matches, files int) {
	reMatches := regexp.MustCompile(`(\d+)\s+matches`)
	reFiles := regexp.MustCompile(`(\d+)\s+files\s+contained\s+matches`)

	if m := reMatches.FindStringSubmatch(content); len(m) > 1 {
		if val, err := strconv.Atoi(m[1]); err == nil {
			matches = val
		}
	}
	if m := reFiles.FindStringSubmatch(content); len(m) > 1 {
		if val, err := strconv.Atoi(m[1]); err == nil {
			files = val
		}
	}
	return matches, files
}
