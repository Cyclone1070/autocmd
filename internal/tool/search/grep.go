package search

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/cloudwego/eino/schema"
)

const (
	defaultMaxLineLength = 10000

	// Default pagination limits
	defaultHeadLimit = 250
	defaultOffset    = 0

	// Prevents base64/minified files from blowing up context
	defaultMaxColumns = 500

	defaultOutputMode = "files_with_matches"

	// High limit for "unlimited" results
	maxUnlimitedResults = 10000000
)

var vcsExclusions = []string{".git", ".svn", ".hg", ".bzr", ".jj", ".sl"}

// GrepRequest matches Claude Code's input schema.
type GrepRequest struct {
	Pattern         string `json:"pattern"`
	Path            string `json:"path"`
	Glob            string `json:"glob,omitempty"`
	OutputMode      string `json:"output_mode,omitempty"`
	HeadLimit       *int   `json:"head_limit,omitempty"`
	Offset          *int   `json:"offset,omitempty"`
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

	maxLineLength int // For testing
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
		maxLineLength:   defaultMaxLineLength,
	}
}

func (t *GrepTool) Name() string {
	return "grep"
}

func (t *GrepTool) IsConcurrentSafe() bool { return true }

func (t *GrepTool) Definition() *schema.ToolInfo {
	return &schema.ToolInfo{
		Name: "grep",
		Desc: "Search for patterns within file contents across a directory.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"pattern": {
				Type:     schema.String,
				Desc:     "The regular expression pattern to search for in file contents",
				Required: true,
			},
			"path": {
				Type: schema.String,
				Desc: "File or directory to search in (rg PATH). Defaults to current working directory.",
			},
			"glob": {
				Type: schema.String,
				Desc: "Glob pattern to filter files (e.g. \"*.js\", \"*.{ts,tsx}\") - maps to rg --glob",
			},
			"output_mode": {
				Type: schema.String,
				Desc: fmt.Sprintf("Output mode: \"content\" shows matching lines (supports -A/-B/-C context, -n line numbers, head_limit), \"files_with_matches\" shows file paths (supports head_limit), \"count\" shows match counts (supports head_limit). Defaults to %q.", defaultOutputMode),
				Enum: []string{"content", "files_with_matches", "count"},
			},
			"head_limit": {
				Type: schema.Integer,
				Desc: fmt.Sprintf("Limit output to first N lines/entries, equivalent to \"| head -N\". Works across all output modes: content (limits output lines), files_with_matches (limits file paths), count (limits count entries). Defaults to %d when unspecified. Pass 0 for unlimited (use sparingly — large result sets waste context).", defaultHeadLimit),
			},
			"offset": {
				Type: schema.Integer,
				Desc: fmt.Sprintf("Skip first N lines/entries before applying head_limit, equivalent to \"| tail -n +N | head -N\". Works across all output modes. Defaults to %d.", defaultOffset),
			},
			"context": {
				Type: schema.Integer,
				Desc: "Number of lines to show before and after each match (rg -C). Requires output_mode: \"content\", ignored otherwise.",
			},
			"-C": {
				Type: schema.Integer,
				Desc: "Alias for context.",
			},
			"-B": {
				Type: schema.Integer,
				Desc: "Number of lines to show before each match (rg -B). Requires output_mode: \"content\", ignored otherwise.",
			},
			"-A": {
				Type: schema.Integer,
				Desc: "Number of lines to show after each match (rg -A). Requires output_mode: \"content\", ignored otherwise.",
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
				Desc: "File type to search (rg --type). Common types: js, py, rust, go, java, etc. More efficient than glob for standard file types.",
			},
			"multiline": {
				Type: schema.Boolean,
				Desc: "Enable multiline mode where . matches newlines and patterns can span lines (rg -U --multiline-dotall). Default: false.",
			},
		}),
	}
}

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
	if req.HeadLimit == nil {
		req.HeadLimit = new(defaultHeadLimit)
	}
	if req.Offset == nil {
		req.Offset = new(defaultOffset)
	}
	if req.ShowLineNumbers == nil {
		req.ShowLineNumbers = new(true)
	}
	if req.I == nil {
		req.I = new(false)
	}

	searchPath := req.Path
	if searchPath == "" {
		req.Path = "."
		searchPath = "."
	}

	absSearchPath, err := t.pathResolver.Abs(searchPath)
	if err != nil {
		return nil, err
	}

	// Check if path exists (file or directory is fine)
	_, err = t.fs.Stat(absSearchPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("path does not exist: %s", searchPath)
		}
		return nil, fmt.Errorf("failed to stat %s: %w", searchPath, err)
	}

	relPath, err := t.pathResolver.Rel(absSearchPath)
	if err != nil {
		relPath = filepath.Base(absSearchPath)
	}

	return &grepInvocation{
		fs:              t.fs,
		commandExecutor: t.commandExecutor,
		pathResolver:    t.pathResolver,
		absPath:         absSearchPath,
		req:             req,
		display:         domain.NewStringDisplay("", fmt.Sprintf("GREP '%s' IN %s", req.Pattern, filepath.ToSlash(relPath))),
		maxLineLength:   t.maxLineLength,
	}, nil
}

type grepInvocation struct {
	fs              fileSystem
	commandExecutor commandExecutor
	pathResolver    pathResolver
	absPath         string
	req             *GrepRequest
	display         domain.StringDisplay
	maxLineLength   int
}

func (i *grepInvocation) Display() domain.ToolDisplay {
	return i.display
}

func (i *grepInvocation) Execute(ctx context.Context) (string, domain.ToolDisplay, error) {
	d := i.display

	if ctx.Err() != nil {
		d.Error = domain.ToolErrorCancelled
		return "execution cancelled", d, ctx.Err()
	}

	// Re-verify state
	_, err := i.fs.Stat(i.absPath)
	if err != nil {
		if ctx.Err() != nil {
			d.Error = domain.ToolErrorCancelled
			return "execution cancelled", d, ctx.Err()
		}
		if os.IsNotExist(err) {
			d.Error = err.Error()
			return fmt.Sprintf("Error: Path %s no longer exists.", i.absPath), d, errors.New("Execution failed")
		}
		d.Error = err.Error()
		return fmt.Sprintf("Error: Failed to access %s: %v", i.absPath, err), d, errors.New("Execution failed")
	}

	// Determine Output Mode
	mode := i.req.OutputMode

	// Build ripgrep command
	cmd := []string{"rg", "--hidden", "--with-filename"}
	for _, excl := range vcsExclusions {
		cmd = append(cmd, fmt.Sprintf("--glob=!%s", excl))
	}
	cmd = append(cmd, "--max-columns", strconv.Itoa(defaultMaxColumns))

	// Mode flags
	switch mode {
	case "files_with_matches":
		cmd = append(cmd, "-l")
	case "count":
		cmd = append(cmd, "-c")
	case "content":
		// default behavior of rg is content
		if *i.req.ShowLineNumbers {
			cmd = append(cmd, "-n")
		}

		// Context
		contextLines := 0
		if i.req.ContextC != nil {
			contextLines = *i.req.ContextC
		} else if i.req.ContextLines != nil {
			contextLines = *i.req.ContextLines
		}

		if contextLines > 0 {
			cmd = append(cmd, "-C", fmt.Sprintf("%d", contextLines))
		} else {
			if i.req.ContextB != nil && *i.req.ContextB > 0 {
				cmd = append(cmd, "-B", fmt.Sprintf("%d", *i.req.ContextB))
			}
			if i.req.ContextA != nil && *i.req.ContextA > 0 {
				cmd = append(cmd, "-A", fmt.Sprintf("%d", *i.req.ContextA))
			}
		}
	}

	// Global Search behavior flags
	if *i.req.I {
		cmd = append(cmd, "-i")
	}
	if i.req.Multiline != nil && *i.req.Multiline {
		cmd = append(cmd, "-U", "--multiline-dotall")
	}
	if i.req.Type != "" {
		cmd = append(cmd, "--type", i.req.Type)
	}
	if i.req.Glob != "" {
		// Split by spaces or commas, but ignore them inside {} braces
		var globs []string
		var current strings.Builder
		braceLevel := 0
		for _, char := range i.req.Glob {
			switch char {
			case '{':
				braceLevel++
			case '}':
				braceLevel--
			}

			if (char == ' ' || char == ',') && braceLevel == 0 {
				if current.Len() > 0 {
					globs = append(globs, current.String())
					current.Reset()
				}
			} else {
				current.WriteRune(char)
			}
		}
		if current.Len() > 0 {
			globs = append(globs, current.String())
		}

		for _, g := range globs {
			cmd = append(cmd, fmt.Sprintf("--glob=%s", strings.TrimSpace(g)))
		}
	}

	// Pattern handling (protect patterns starting with -)
	if strings.HasPrefix(i.req.Pattern, "-") {
		cmd = append(cmd, "-e", i.req.Pattern)
	} else {
		cmd = append(cmd, "--", i.req.Pattern)
	}
	cmd = append(cmd, i.absPath)

	// Ensure working directory is a directory
	workDir := i.absPath
	stat, err := i.fs.Stat(workDir)
	if err == nil && !stat.IsDir() {
		workDir = filepath.Dir(workDir)
	}

	res, err := i.commandExecutor.Run(ctx, cmd, workDir, os.Environ())
	if err != nil {
		if ctx.Err() != nil {
			d.Error = domain.ToolErrorCancelled
			return "execution cancelled", d, ctx.Err()
		}
		d.Error = err.Error()
		return fmt.Sprintf("Error: rg failed to start: %v", err), d, errors.New("Execution failed")
	}

	// rg ExitCode 1 means no matches found, which is not a tool failure.
	if res.ExitCode != 0 && res.ExitCode != 1 {
		d.Error = fmt.Sprintf("exit code %d", res.ExitCode)
		return fmt.Sprintf("Error: rg failed with exit code %d: %s", res.ExitCode, res.Stderr), d, errors.New("Execution failed")
	}

	// Process output based on mode
	return i.formatResults(res.Stdout, mode), d, nil
}

func (i *grepInvocation) formatResults(stdout string, mode string) string {
	headLimit := *i.req.HeadLimit
	if headLimit == 0 {
		headLimit = maxUnlimitedResults
	}
	offset := *i.req.Offset

	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	if stdout == "" {
		lines = []string{}
	}

	// For files_with_matches mode, we need to stat and sort by recency
	if mode == "files_with_matches" && len(lines) > 0 {
		type fileMatch struct {
			path  string
			mtime time.Time
		}
		var matches []fileMatch
		for _, line := range lines {
			info, err := i.fs.Stat(line)
			mtime := time.Time{}
			if err == nil {
				mtime = info.ModTime()
			}
			matches = append(matches, fileMatch{path: line, mtime: mtime})
		}
		// Sort by mtime descending
		sort.SliceStable(matches, func(i, j int) bool {
			if !matches[i].mtime.Equal(matches[j].mtime) {
				return matches[i].mtime.After(matches[j].mtime)
			}
			return matches[i].path < matches[j].path // Alphabetical tie-break
		})
		// Re-populate lines
		lines = make([]string, len(matches))
		for idx, m := range matches {
			lines[idx] = m.path
		}
	}

	// If offset >= len(lines), return "No matches found" or similar
	if offset >= len(lines) && len(lines) > 0 {
		return "No results found in the specified range."
	}
	if len(lines) == 0 {
		if mode == "files_with_matches" {
			return "No files found"
		}
		return "No matches found."
	}

	var sb strings.Builder
	totalCount := len(lines)

	end := offset + headLimit
	wasTruncated := false
	if end > len(lines) {
		end = len(lines)
	} else if end < len(lines) {
		wasTruncated = true
	}
	visibleLines := lines[offset:end]

	switch mode {
	case "files_with_matches":
		fmt.Fprintf(&sb, "Found %d files", totalCount)
		// Only show limit/offset if truncation actually occurred (matching Claude Code behavior)
		if wasTruncated || offset > 0 {
			fmt.Fprintf(&sb, " limit: %d, offset: %d", headLimit, offset)
		}
		sb.WriteString("\n")
		for _, line := range visibleLines {
			rel, err := i.pathResolver.Rel(line)
			if err != nil {
				rel = filepath.Base(line)
			}
			sb.WriteString(filepath.ToSlash(rel) + "\n")
		}
	case "count":
		totalOccurrences := 0
		for _, line := range visibleLines {
			parts := strings.Split(line, ":")
			file := parts[0]
			rel, err := i.pathResolver.Rel(file)
			if err != nil {
				rel = filepath.Base(file)
			}
			count := ""
			if len(parts) > 1 {
				count = parts[1]
				var c int
				fmt.Sscanf(count, "%d", &c)
				totalOccurrences += c
			}
			fmt.Fprintf(&sb, "%s:%s\n", filepath.ToSlash(rel), count)
		}
		sb.WriteString("\nFound ")
		if totalOccurrences == 1 {
			sb.WriteString("1 total occurrence")
		} else {
			fmt.Fprintf(&sb, "%d total occurrences", totalOccurrences)
		}
		sb.WriteString(" across ")
		if totalCount == 1 {
			sb.WriteString("1 file.")
		} else {
			fmt.Fprintf(&sb, "%d files.", totalCount)
		}
		if wasTruncated || offset > 0 {
			fmt.Fprintf(&sb, " with pagination = limit: %d, offset: %d", headLimit, offset)
		}
	case "content":
		for _, line := range visibleLines {
			// Lines are typically "file:line:content"
			parts := strings.SplitN(line, ":", 3)
			if len(parts) < 3 {
				// might be context line "file-line-content"
				parts = strings.SplitN(line, "-", 3)
			}
			if len(parts) >= 3 {
				rel, err := i.pathResolver.Rel(parts[0])
				if err != nil {
					rel = filepath.Base(parts[0])
				}
				fmt.Fprintf(&sb, "%s:%s:%s\n", filepath.ToSlash(rel), parts[1], parts[2])
			} else {
				sb.WriteString(line + "\n")
			}
		}
		if wasTruncated || offset > 0 {
			fmt.Fprintf(&sb, "\n[Showing results with pagination = limit: %d, offset: %d]", headLimit, offset)
		}
	}

	return strings.TrimSpace(sb.String())
}
