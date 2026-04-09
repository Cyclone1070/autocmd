package search

import (
	"context"
	"encoding/json"
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

func (t *GrepTool) Name() string {
	return "grep"
}

func (t *GrepTool) Desc() string {
	return "Search for patterns in files using ripgrep. Optimized for content search across multiple files."
}

func (t *GrepTool) IsConcurrentSafe() bool { return true }

// Definition returns the tool's schema for the LLM using eino schema.
func (t *GrepTool) Definition() *schema.ToolInfo {
	return &schema.ToolInfo{
		Name: "grep",
		Desc: "Search for patterns in files using ripgrep. Optimized for content search across multiple files.",
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
				Enum: []string{"files_with_matches", "content", "count"},
				Desc: "Output mode: \"content\" shows matching lines (supports -A/-B/-C context, -n line numbers, head_limit), \"files_with_matches\" shows file paths (supports head_limit), \"count\" shows match counts (supports head_limit). Defaults to \"files_with_matches\".",
			},
			"head_limit": {
				Type: schema.Integer,
				Desc: "Limit output to first N lines/entries, equivalent to \"| head -N\". Works across all output modes: content (limits output lines), files_with_matches (limits file paths), count (limits count entries). Defaults to 250 when unspecified. Pass 0 for unlimited (use sparingly — large result sets waste context).",
			},
			"offset": {
				Type: schema.Integer,
				Desc: "Skip first N lines/entries before applying head_limit, equivalent to \"| tail -n +N | head -N\". Works across all output modes. Defaults to 0.",
			},
			"case_insensitive": {
				Type: schema.Boolean,
				Desc: "Case insensitive search (rg -i)",
			},
			"-i": {
				Type: schema.Boolean,
				Desc: "Alias for case_insensitive.",
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
			"multiline": {
				Type: schema.Boolean,
				Desc: "Enable multiline mode where . matches newlines and patterns can span lines (rg -U --multiline-dotall). Default: false.",
			},
			"type": {
				Type: schema.String,
				Desc: "File type to search (rg --type). Common types: js, py, rust, go, java, etc. More efficient than include for standard file types.",
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
		req.HeadLimit = new(int)
		*req.HeadLimit = defaultHeadLimit
	}
	if req.Offset == nil {
		req.Offset = new(int)
		*req.Offset = defaultOffset
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
		req.Path = "."
		searchPath = "."
	}

	absSearchPath, err := t.pathResolver.Abs(searchPath)
	if err != nil {
		return nil, err
	}

	displayPath := t.pathResolver.DisplayPath(absSearchPath)

	// Check if path exists
	_, err = t.fs.Stat(absSearchPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("path does not exist: %s", searchPath)
		}
		return nil, fmt.Errorf("failed to stat %s: %w", searchPath, err)
	}

	return &grepInvocation{
		fs:              t.fs,
		commandExecutor: t.commandExecutor,
		pathResolver:    t.pathResolver,
		absPath:         absSearchPath,
		req:             req,
		display:         domain.NewStringDisplay("", fmt.Sprintf("GREP \"%s\" IN \"%s\"", req.Pattern, filepath.ToSlash(displayPath))),
	}, nil
}

type grepInvocation struct {
	fs              fileSystem
	commandExecutor commandExecutor
	pathResolver    pathResolver
	absPath         string
	workDir         string
	req             *GrepRequest
	display         domain.StringDisplay
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
		d.Error = domain.ToolErrorFailed
		return fmt.Sprintf("Error: Failed to access %s: %v", i.absPath, err), d, nil
	}

	mode := i.req.OutputMode
	cmd, workDir, err := i.prepareGrepCommand()
	if err != nil {
		d.Error = domain.ToolErrorFailed
		return fmt.Sprintf("Error: %v", err), d, nil
	}
	i.workDir = workDir

	res, err := i.commandExecutor.Run(ctx, cmd, workDir, os.Environ())
	if err != nil {
		if ctx.Err() != nil {
			d.Error = domain.ToolErrorCancelled
			return "execution cancelled", d, ctx.Err()
		}
		d.Error = domain.ToolErrorFailed
		return fmt.Sprintf("Error: rg failed: %v", err), d, nil
	}

	if res.ExitCode != 0 && res.ExitCode != 1 {
		d.Error = domain.ToolErrorFailed
		return fmt.Sprintf("Error: ripgrep failed with exit code %d\n%s", res.ExitCode, res.Stderr), d, nil
	}

	return i.formatResults(res.Stdout, mode), d, nil
}

func (i *grepInvocation) prepareGrepCommand() ([]string, string, error) {
	mode := i.req.OutputMode
	cmd := []string{"rg", "--hidden", "--with-filename"}
	for _, excl := range vcsExclusions {
		cmd = append(cmd, "--glob", "!"+excl)
	}
	cmd = append(cmd, "--max-columns", strconv.Itoa(defaultMaxColumns))

	switch mode {
	case "files_with_matches":
		cmd = append(cmd, "-l")
	case "count":
		cmd = append(cmd, "-c")
	case "content":
		if i.req.ShowLineNumbers == nil || *i.req.ShowLineNumbers {
			cmd = append(cmd, "-n")
		}
		contextLines := 0
		if i.req.ContextLines != nil {
			contextLines = *i.req.ContextLines
		} else if i.req.ContextC != nil {
			contextLines = *i.req.ContextC
		}
		if contextLines > 0 {
			cmd = append(cmd, "-C", strconv.Itoa(contextLines))
		} else {
			if i.req.ContextB != nil && *i.req.ContextB > 0 {
				cmd = append(cmd, "-B", strconv.Itoa(*i.req.ContextB))
			}
			if i.req.ContextA != nil && *i.req.ContextA > 0 {
				cmd = append(cmd, "-A", strconv.Itoa(*i.req.ContextA))
			}
		}
	}

	if i.req.I != nil && *i.req.I {
		cmd = append(cmd, "-i")
	}
	if i.req.Multiline != nil && *i.req.Multiline {
		cmd = append(cmd, "-U", "--multiline-dotall")
	}
	if i.req.Type != "" {
		cmd = append(cmd, "--type", i.req.Type)
	}
	if i.req.Glob != "" {
		globs := splitGlobs(i.req.Glob)
		for _, g := range globs {
			cmd = append(cmd, "--glob", g)
		}
	}

	if strings.HasPrefix(i.req.Pattern, "-") {
		cmd = append(cmd, "-e", i.req.Pattern)
	} else {
		cmd = append(cmd, "--", i.req.Pattern)
	}

	workDir := i.absPath
	stat, err := i.fs.Stat(workDir)
	if err == nil && !stat.IsDir() {
		workDir = filepath.Dir(workDir)
		cmd = append(cmd, filepath.Base(i.absPath))
	}

	return cmd, workDir, nil
}

func (i *grepInvocation) formatResults(stdout string, mode string) string {
	headLimit := defaultHeadLimit
	if i.req.HeadLimit != nil {
		headLimit = *i.req.HeadLimit
	}
	if headLimit == 0 {
		headLimit = maxUnlimitedResults
	}
	offset := 0
	if i.req.Offset != nil {
		offset = *i.req.Offset
	}

	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	if stdout == "" {
		lines = []string{}
	}

	if mode == "files_with_matches" && len(lines) > 0 {
		type fileMatch struct {
			path  string
			mtime time.Time
		}
		var matches []fileMatch
		for _, line := range lines {
			// Strip quotes if any
			line = strings.Trim(line, "\"")
			abs := filepath.Join(i.workDir, line)
			info, err := i.fs.Stat(abs)
			mtime := time.Time{}
			if err == nil {
				mtime = info.ModTime()
			}
			matches = append(matches, fileMatch{path: line, mtime: mtime})
		}
		sort.SliceStable(matches, func(i, j int) bool {
			if !matches[i].mtime.Equal(matches[j].mtime) {
				return matches[i].mtime.After(matches[j].mtime)
			}
			return matches[i].path < matches[j].path
		})
		lines = make([]string, len(matches))
		for idx, m := range matches {
			lines[idx] = m.path
		}
	}

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
		fmt.Fprintf(&sb, "Found %d %s", len(visibleLines), plural(len(visibleLines), "file"))
		if wasTruncated || offset > 0 {
			fmt.Fprintf(&sb, " limit: %d, offset: %d", headLimit, offset)
		}
		sb.WriteString("\n")
		for _, line := range visibleLines {
			rel := i.pathResolver.DisplayPath(filepath.Join(i.workDir, line))
			sb.WriteString(filepath.ToSlash(rel) + "\n")
		}
	case "count":
		totalOccurrences := 0
		distinctFiles := make(map[string]bool)
		for _, line := range visibleLines {
			idx := strings.LastIndex(line, ":")
			if idx == -1 {
				sb.WriteString(line + "\n")
				continue
			}
			file := line[:idx]
			count := line[idx+1:]
			rel := i.pathResolver.DisplayPath(filepath.Join(i.workDir, file))
			distinctFiles[rel] = true
			var c int
			fmt.Sscanf(count, "%d", &c)
			totalOccurrences += c
			fmt.Fprintf(&sb, "%s:%s\n", filepath.ToSlash(rel), count)
		}
		sb.WriteString("\nFound ")
		fmt.Fprintf(&sb, "%d total %s across %d %s.", totalOccurrences, plural(totalOccurrences, "occurrence"), len(distinctFiles), plural(len(distinctFiles), "file"))

		if wasTruncated || offset > 0 {
			fmt.Fprintf(&sb, " with pagination = limit: %d, offset: %d", headLimit, offset)
		}
	case "content":
		sb.WriteString("Matches:\n")
		for _, line := range visibleLines {
			file, lineNum, content, ok := parseGrepLine(line)
			if !ok {
				sb.WriteString(line + "\n")
				continue
			}
			rel := i.pathResolver.DisplayPath(filepath.Join(i.workDir, file))
			fmt.Fprintf(&sb, "%s:%s:%s\n", filepath.ToSlash(rel), lineNum, content)
		}
		if wasTruncated || offset > 0 {
			limitInfo := fmt.Sprintf("limit: %d, offset: %d", headLimit, offset)
			fmt.Fprintf(&sb, "\n\n[Showing results with pagination = %s]", limitInfo)
		}
	}

	return strings.TrimSpace(sb.String())
}

func splitGlobs(globStr string) []string {
	var globs []string
	var current strings.Builder
	braceLevel := 0
	for _, char := range globStr {
		switch char {
		case '{':
			braceLevel++
		case '}':
			braceLevel--
		}
		if (char == ' ' || char == ',') && braceLevel == 0 {
			if current.Len() > 0 {
				globs = append(globs, strings.TrimSpace(current.String()))
				current.Reset()
			}
		} else {
			current.WriteRune(char)
		}
	}
	if current.Len() > 0 {
		globs = append(globs, strings.TrimSpace(current.String()))
	}
	return globs
}

func parseGrepLine(line string) (file, lineNum, content string, ok bool) {
	// Ripgrep format: filename:line:content or filename-line-content (context)
	// We look for the first colon or dash that is followed by a digit.
	idx := -1
	for i := 1; i < len(line)-1; i++ {
		if (line[i] == ':' || line[i] == '-') && (line[i+1] >= '0' && line[i+1] <= '9') {
			idx = i
			break
		}
	}
	if idx == -1 {
		return "", "", "", false
	}

	sep := line[idx]
	file = strings.Trim(line[:idx], "\"")
	rest := line[idx+1:]

	secondSepIdx := strings.IndexByte(rest, sep)
	if secondSepIdx == -1 {
		return "", "", "", false
	}

	lineNum = rest[:secondSepIdx]
	content = rest[secondSepIdx+1:]
	return file, lineNum, content, true
}

func plural(n int, singular string) string {
	if n == 1 {
		return singular
	}
	return singular + "s"
}
