// Package search provides tools for finding files and searching their content.
package search

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

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/cloudwego/eino/schema"
)

const (
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

// GrepRequest matches Claude Code's input schema.
type GrepRequest struct {
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

// GrepTool handles content searching operations.
type GrepTool struct {
	fs              fileSystem
	commandExecutor commandExecutor
	pathResolver    pathResolver
}

// NewGrepTool creates a new GrepTool with injected dependencies.
func NewGrepTool(
	fs fileSystem,
	commandExecutor commandExecutor,
	pathResolver pathResolver,
) *GrepTool {
	if fs == nil {
		panic("fs is required")
	}
	if commandExecutor == nil {
		panic("commandExecutor is required")
	}
	if pathResolver == nil {
		panic("pathResolver is required")
	}
	return &GrepTool{
		fs:              fs,
		commandExecutor: commandExecutor,
		pathResolver:    pathResolver,
	}
}

// Name returns the unique identifier for the grep tool.
func (t *GrepTool) Name() string {
	return "grep"
}

// IsConcurrentSafe indicates if the grep tool can be run concurrently.
func (t *GrepTool) IsConcurrentSafe() bool { return true }

// Definition returns the tool's schema for the LLM using eino schema.
func (t *GrepTool) Definition() *schema.ToolInfo {
	return &schema.ToolInfo{
		Name: "grep",
		Desc: `A powerful search tool built on ripgrep.

Usage:
- The path parameter MUST be an absolute path.
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
				Desc: fmt.Sprintf("Absolute path to file or directory to search in (rg PATH). Defaults to workspace root (currently \"%s\").", t.pathResolver.Root()),
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
	}
}

// Prepare parses the grep parameters and returns an invocation.
func (t *GrepTool) Prepare(params string) (domain.Invocation, error) {
	req := &GrepRequest{}
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

	absSearchPath, err := t.pathResolver.Abs(searchPath)
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

	displayPath := t.pathResolver.DisplayPath(absSearchPath)

	return &grepInvocation{
		fs:              t.fs,
		commandExecutor: t.commandExecutor,
		pathResolver:    t.pathResolver,
		absPath:         absSearchPath,
		req:             req,
		display:         domain.NewStringDisplay(fmt.Sprintf("Grep \"%s\" in \"%s\"", req.Pattern, filepath.ToSlash(displayPath)), ""),
	}, nil
}

type grepInvocation struct {
	fs              fileSystem
	commandExecutor commandExecutor
	pathResolver    pathResolver
	absPath         string
	req             *GrepRequest
	display         domain.StringDisplay
}

func (i *grepInvocation) Display() domain.ToolDisplay {
	return i.display
}

func (i *grepInvocation) Execute(ctx context.Context) (string, domain.ToolDisplay) {
	d := i.display

	if ctx.Err() != nil {
		d.Error = domain.ToolErrorCancelled
		return domain.ToolErrorCancelled, d
	}

	cmdStr := i.prepareGrepCommand()
	workDir := i.pathResolver.Root()

	ctx, cancel := context.WithTimeout(ctx, defaultGrepTimeout)
	defer cancel()

	res, err := i.commandExecutor.Run(ctx, cmdStr, workDir, true)
	timedOut := false
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			timedOut = true
		} else if ctx.Err() != nil {
			d.Error = domain.ToolErrorCancelled
			return domain.ToolErrorCancelled, d
		} else {
			d.Error = domain.ToolErrorFailed
			return fmt.Sprintf("Error: rg failed: %v", err), d
		}
	}

	output := res.Stdout
	if res.LogPath != "" {
		matches, files, _ := i.analyzeLog(res.LogPath)
		output = fmt.Sprintf("Output too large (%d matches across %d files). Full output saved to %s. Use `read_file` tool to read full output.", matches, files, res.LogPath)
	}
	if output == "" {
		output = "No matches found"
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

	return output, d
}

func (i *grepInvocation) prepareGrepCommand() string {
	mode := i.req.OutputMode
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
		if i.req.ShowLineNumbers == nil || *i.req.ShowLineNumbers {
			args = append(args, "-n")
		}
		contextLines := 0
		if i.req.ContextLines != nil {
			contextLines = *i.req.ContextLines
		} else if i.req.ContextC != nil {
			contextLines = *i.req.ContextC
		}
		if contextLines > 0 {
			args = append(args, "-C", strconv.Itoa(contextLines))
		} else {
			if i.req.ContextB != nil && *i.req.ContextB > 0 {
				args = append(args, "-B", strconv.Itoa(*i.req.ContextB))
			}
			if i.req.ContextA != nil && *i.req.ContextA > 0 {
				args = append(args, "-A", strconv.Itoa(*i.req.ContextA))
			}
		}
	}

	if i.req.I != nil && *i.req.I {
		args = append(args, "-i")
	}
	if i.req.Multiline != nil && *i.req.Multiline {
		args = append(args, "-U", "--multiline-dotall")
	}
	if i.req.Type != "" {
		args = append(args, "--type", i.req.Type)
	}
	if i.req.Glob != "" {
		globs := splitGlobs(i.req.Glob)
		for _, g := range globs {
			args = append(args, "--glob", g)
		}
	}

	if strings.HasPrefix(i.req.Pattern, "-") {
		args = append(args, "-e", i.req.Pattern)
	} else {
		args = append(args, i.req.Pattern)
	}

	// Always use absolute path for rg to get absolute path output
	args = append(args, i.absPath)

	return joinArgs(args)
}

func (i *grepInvocation) analyzeLog(path string) (matches, files int, err error) {
	f, err := i.fs.Open(path)
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

	reMatches := regexp.MustCompile(`(\d+)\s+matches`)
	reFiles := regexp.MustCompile(`(\d+)\s+files\s+contained\s+matches`)

	if m := reMatches.FindStringSubmatch(string(content)); len(m) > 1 {
		if val, err := strconv.Atoi(m[1]); err == nil {
			matches = val
		}
	}
	if m := reFiles.FindStringSubmatch(string(content)); len(m) > 1 {
		if val, err := strconv.Atoi(m[1]); err == nil {
			files = val
		}
	}

	return matches, files, nil
}
