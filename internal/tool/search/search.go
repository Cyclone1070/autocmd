package search

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/cloudwego/eino/schema"
)

const (
	maxSearchResults     = 100
	defaultMaxLineLength = 10000
)

// SearchContentRequest matches OpenCode's input schema.
type SearchContentRequest struct {
	Pattern string `json:"pattern"`
	Path    string `json:"path"`
	Include string `json:"include,omitempty"`
}

// SearchContentTool handles content searching operations.
type SearchContentTool struct {
	fs              fileSystem
	commandExecutor commandExecutor
	pathResolver    pathResolver

	maxLineLength int // For testing
}

// NewSearchContentTool creates a new SearchContentTool with injected dependencies.
func NewSearchContentTool(
	fs fileSystem,
	commandExecutor commandExecutor,
	pathResolver pathResolver,
) *SearchContentTool {
	if fs == nil {
		panic("fs is required")
	}
	if commandExecutor == nil {
		panic("commandExecutor is required")
	}
	if pathResolver == nil {
		panic("pathResolver is required")
	}
	return &SearchContentTool{
		fs:              fs,
		commandExecutor: commandExecutor,
		pathResolver:    pathResolver,
		maxLineLength:   defaultMaxLineLength,
	}
}

func (t *SearchContentTool) Name() string {
	return "search_content"
}

func (t *SearchContentTool) Definition() *schema.ToolInfo {
	return &schema.ToolInfo{
		Name: "search_content",
		Desc: "Search for content matching a regex pattern in files.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"pattern": {
				Type:     schema.String,
				Desc:     "The regex pattern to search for in file contents",
				Required: true,
			},
			"path": {
				Type: schema.String,
				Desc: "The file or directory to search in. Defaults to the current working directory.",
			},
			"include": {
				Type: schema.String,
				Desc: "File pattern to include in the search (e.g. \"*.js\", \"*.{ts,tsx}\")",
			},
		}),
	}
}

func (t *SearchContentTool) Prepare(ctx context.Context, params string) (domain.Invocation, error) {
	req := &SearchContentRequest{}
	if err := json.Unmarshal([]byte(params), req); err != nil {
		return nil, fmt.Errorf("failed to parse arguments: %w", err)
	}

	if req.Pattern == "" {
		return nil, fmt.Errorf("pattern is required")
	}

	searchPath := req.Path
	if searchPath == "" {
		searchPath = "."
	}

	absSearchPath, err := t.pathResolver.Abs(searchPath)
	if err != nil {
		return nil, err
	}

	// Check if path exists (file or directory is fine)
	_, err = t.fs.Stat(absSearchPath)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("path does not exist: %s", searchPath)
		}
		return nil, fmt.Errorf("failed to stat %s: %w", searchPath, err)
	}

	return &searchContentInvocation{
		fs:              t.fs,
		commandExecutor: t.commandExecutor,
		pathResolver:    t.pathResolver,
		absPath:         absSearchPath,
		pattern:         req.Pattern,
		include:         req.Include,
		display:         domain.NewStringDisplay("", fmt.Sprintf("Searching for '%s' in %s", req.Pattern, filepath.Base(absSearchPath))),
		maxLineLength:   t.maxLineLength,
	}, nil
}

type searchContentInvocation struct {
	fs              fileSystem
	commandExecutor commandExecutor
	pathResolver    pathResolver
	absPath         string
	pattern         string
	include         string
	display         domain.ToolDisplay
	maxLineLength   int
}

func (i *searchContentInvocation) Display() domain.ToolDisplay {
	return i.display
}

func (i *searchContentInvocation) Execute(ctx context.Context) (string, error) {
	if ctx.Err() != nil {
		return "", ctx.Err()
	}

	// Re-verify state
	_, err := i.fs.Stat(i.absPath)
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		if os.IsNotExist(err) {
			return fmt.Sprintf("Error: Path %s no longer exists.", i.absPath), err
		}
		return fmt.Sprintf("Error: Failed to access %s: %v", i.absPath, err), err
	}

	maxResults := maxSearchResults
	maxLineLength := i.maxLineLength

	// Build ripgrep command
	// rg --json [--glob=<include>] [--] <pattern> <path>
	cmd := []string{"rg", "--json", "--glob=!.git/*"}

	if i.include != "" {
		cmd = append(cmd, fmt.Sprintf("--glob=%s", i.include))
	}

	// Double check pattern/path separation
	cmd = append(cmd, "--", i.pattern, i.absPath)

	res, err := i.commandExecutor.Run(ctx, cmd, i.absPath, nil)
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return fmt.Sprintf("Error: rg failed to start: %v", err), err
	}

	if res.ExitCode != 0 && res.ExitCode != 1 {
		return fmt.Sprintf("Error: rg failed with exit code %d: %s", res.ExitCode, res.Stderr), fmt.Errorf("rg exit code %d", res.ExitCode)
	}

	// Process output
	var matches []searchContentMatch
	hitMaxResults := false
	lines := strings.SplitSeq(res.Stdout, "\n")

	for line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var rgMatch struct {
			Type string `json:"type"`
			Data struct {
				Path struct {
					Text string `json:"text"`
				} `json:"path"`
				Lines struct {
					Text string `json:"text"`
				} `json:"lines"`
				LineNumber int `json:"line_number"`
			} `json:"data"`
		}

		if err := json.Unmarshal([]byte(line), &rgMatch); err != nil {
			continue
		}

		if rgMatch.Type == "match" {
			relPath, err := i.pathResolver.Rel(rgMatch.Data.Path.Text)
			if err != nil {
				relPath = rgMatch.Data.Path.Text
			}

			lineContent := strings.TrimSpace(rgMatch.Data.Lines.Text)
			if len(lineContent) > maxLineLength {
				lineContent = lineContent[:maxLineLength] + "...[truncated]"
			}

			matches = append(matches, searchContentMatch{
				File:        filepath.ToSlash(relPath),
				LineNumber:  rgMatch.Data.LineNumber,
				LineContent: lineContent,
			})

			if len(matches) >= maxResults {
				hitMaxResults = true
				break
			}
		}
	}

	return formatSearchMatches(matches, hitMaxResults), nil
}

// searchContentMatch represents a single match in a file
// Internal usage only, no longer part of public API
type searchContentMatch struct {
	File        string
	LineNumber  int
	LineContent string
}

// formatSearchMatches formats matches OpenCode style
func formatSearchMatches(matches []searchContentMatch, truncated bool) string {
	if len(matches) == 0 {
		return "No matches found."
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Found %d matches\n", len(matches))

	currentFile := ""
	for _, m := range matches {
		if m.File != currentFile {
			if currentFile != "" {
				sb.WriteString("\n")
			}
			sb.WriteString("\n" + m.File + ":\n")
			currentFile = m.File
		}
		fmt.Fprintf(&sb, "  Line %d: %s\n", m.LineNumber, m.LineContent)
	}

	if truncated {
		sb.WriteString("\n(Results are truncated. Consider using a more specific path or pattern.)\n")
	}

	return sb.String()
}
