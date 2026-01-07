package search

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Cyclone1070/iav/internal/config"
	"github.com/Cyclone1070/iav/internal/tool"
)

// FindFileRequest represents the parameters for a FindFile operation
type FindFileRequest struct {
	Pattern string `json:"pattern"`
	Path    string `json:"path"`
}

// FindFileTool handles file finding operations.
type FindFileTool struct {
	fs              fileSystem
	commandExecutor commandExecutor
	config          *config.Config
	pathResolver    pathResolver
}

// NewFindFileTool creates a new FindFileTool with injected dependencies.
func NewFindFileTool(
	fs fileSystem,
	commandExecutor commandExecutor,
	cfg *config.Config,
	pathResolver pathResolver,
) *FindFileTool {
	if fs == nil {
		panic("fs is required")
	}
	if commandExecutor == nil {
		panic("commandExecutor is required")
	}
	if cfg == nil {
		panic("cfg is required")
	}
	if pathResolver == nil {
		panic("pathResolver is required")
	}
	return &FindFileTool{
		fs:              fs,
		commandExecutor: commandExecutor,
		config:          cfg,
		pathResolver:    pathResolver,
	}
}

// Name returns the name of the tool.
func (t *FindFileTool) Name() string {
	return "find_file"
}

// Declaration returns the JSON schema for the tool.
func (t *FindFileTool) Declaration() tool.Declaration {
	return tool.Declaration{
		Name:        t.Name(),
		Description: "Find files matching a glob pattern.",
		Parameters: &tool.Schema{
			Type: tool.TypeObject,
			Properties: map[string]*tool.Schema{
				"pattern": {
					Type:        tool.TypeString,
					Description: "Glob pattern to match files.",
				},
				"path": {
					Type:        tool.TypeString,
					Description: "Path to search within. Defaults to workspace root.",
				},
			},
			Required: []string{"pattern"},
		},
	}
}

// Prepare validates input and resolves path.
func (t *FindFileTool) Prepare(ctx context.Context, params json.RawMessage) (tool.Invocation, error) {
	req := &FindFileRequest{}
	if err := json.Unmarshal(params, req); err != nil {
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
		return nil, fmt.Errorf("invalid path: %w", err)
	}

	// Fail Fast: Verify path exists and is a directory
	info, err := t.fs.Stat(absPath)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("path does not exist: %s", absPath)
		}
		return nil, fmt.Errorf("failed to access path: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("not a directory: %s", absPath)
	}

	return &findFileInvocation{
		fs:              t.fs,
		commandExecutor: t.commandExecutor,
		pathResolver:    t.pathResolver,
		absPath:         absPath,
		pattern:         req.Pattern,
		display:         tool.StringDisplay(fmt.Sprintf("Finding '%s' in %s", req.Pattern, filepath.Base(absPath))),
	}, nil
}

type findFileInvocation struct {
	fs              fileSystem
	commandExecutor commandExecutor
	pathResolver    pathResolver
	absPath         string
	pattern         string
	display         tool.ToolDisplay
}

func (i *findFileInvocation) Display() tool.ToolDisplay {
	return i.display
}

func (i *findFileInvocation) Execute(ctx context.Context) (string, error) {
	if ctx.Err() != nil {
		return "", ctx.Err()
	}

	// Re-verify State
	info, err := i.fs.Stat(i.absPath)
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		if os.IsNotExist(err) {
			return fmt.Sprintf("Error: Path %s no longer exists.", i.absPath), err
		}
		return fmt.Sprintf("Error: Failed to access %s: %v", i.absPath, err), err
	}
	if !info.IsDir() {
		return fmt.Sprintf("Error: Path %s is no longer a directory.", i.absPath), fmt.Errorf("path is no longer a directory: %s", i.absPath)
	}

	// fd --glob "pattern" searchPath
	cmd := []string{"fd", "--glob", i.pattern, i.absPath}

	res, err := i.commandExecutor.Run(ctx, cmd, i.absPath, nil)
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return fmt.Sprintf("Error: fd failed to start: %v", err), err
	}

	if res.ExitCode != 0 && res.ExitCode != 1 {
		return fmt.Sprintf("Error: fd failed with exit code %d: %s", res.ExitCode, res.Stderr), fmt.Errorf("fd exit code %d", res.ExitCode)
	}

	maxResults := 100
	var matches []string
	hitMaxResults := false
	lines := strings.SplitSeq(res.Stdout, "\n")
	for line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		relPath, err := i.pathResolver.Rel(line)
		if err != nil {
			relPath = line
		}
		matches = append(matches, filepath.ToSlash(relPath))

		if len(matches) >= maxResults {
			hitMaxResults = true
			break
		}
	}

	if len(matches) == 0 {
		return "No matches found.", nil
	}

	formattedMatches := strings.Join(matches, "\n")
	if hitMaxResults {
		formattedMatches += "\n\n(Results truncated. Consider using a more specific pattern.)"
	}

	return formattedMatches, nil
}
