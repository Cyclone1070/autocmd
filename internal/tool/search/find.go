package search

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/tool/helper/summary"
	"github.com/cloudwego/eino/schema"
)

const (
	maxFindResults = 100
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
	pathResolver    pathResolver
}

// NewFindFileTool creates a new FindFileTool with injected dependencies.
func NewFindFileTool(
	fs fileSystem,
	commandExecutor commandExecutor,
	pathResolver pathResolver,
) *FindFileTool {
	if fs == nil {
		panic("fs is required")
	}
	if commandExecutor == nil {
		panic("commandExecutor is required")
	}
	if pathResolver == nil {
		panic("pathResolver is required")
	}
	return &FindFileTool{
		fs:              fs,
		commandExecutor: commandExecutor,
		pathResolver:    pathResolver,
	}
}

// Name returns the name of the tool.
func (t *FindFileTool) Name() string {
	return "find_file"
}

// Definition returns the JSON schema for the tool.
func (t *FindFileTool) Definition() *schema.ToolInfo {
	return &schema.ToolInfo{
		Name: t.Name(),
		Desc: "Find files matching a glob pattern.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"pattern": {
				Type:     schema.String,
				Desc:     "Glob pattern to match files.",
				Required: true,
			},
			"path": {
				Type: schema.String,
				Desc: "Path to search within. Defaults to workspace root.",
			},
		}),
	}
}

// Prepare validates input and resolves path.
func (t *FindFileTool) Prepare(ctx context.Context, params string) (domain.Invocation, error) {
	req := &FindFileRequest{}
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

	relPath, err := t.pathResolver.Rel(absPath)
	if err != nil {
		relPath = filepath.Base(absPath)
	}

	return &findFileInvocation{
		fs:              t.fs,
		commandExecutor: t.commandExecutor,
		pathResolver:    t.pathResolver,
		absPath:         absPath,
		pattern:         req.Pattern,
		display:         domain.NewStringDisplay(fmt.Sprintf("FIND %s", summary.Summarize(req.Pattern)), fmt.Sprintf("FIND '%s' IN %s", summary.Summarize(req.Pattern), filepath.ToSlash(relPath))),
	}, nil
}

type findFileInvocation struct {
	fs              fileSystem
	commandExecutor commandExecutor
	pathResolver    pathResolver
	absPath         string
	pattern         string
	display         domain.ToolDisplay
}

func (i *findFileInvocation) Display() domain.ToolDisplay {
	return i.display
}

func (i *findFileInvocation) Execute(ctx context.Context) (string, domain.ToolDisplay, error) {
	d := i.display.(domain.StringDisplay)

	if ctx.Err() != nil {
		return "", i.display, ctx.Err()
	}

	// Re-verify State
	info, err := i.fs.Stat(i.absPath)
	if err != nil {
		if ctx.Err() != nil {
			return "", i.display, ctx.Err()
		}
		if os.IsNotExist(err) {
			d.Error = err.Error()
			return fmt.Sprintf("Error: Path %s no longer exists.", i.absPath), d, errors.New("Execution failed")
		}
		d.Error = err.Error()
		return fmt.Sprintf("Error: Failed to access %s: %v", i.absPath, err), d, errors.New("Execution failed")
	}
	if !info.IsDir() {
		d.Error = "path is not a directory"
		return fmt.Sprintf("Error: Path %s is no longer a directory.", i.absPath), d, errors.New("Execution failed")
	}

	// fd --glob "pattern" searchPath
	cmd := []string{"fd", "--glob", i.pattern, i.absPath}

	res, err := i.commandExecutor.Run(ctx, cmd, i.absPath, os.Environ())
	if err != nil {
		if ctx.Err() != nil {
			return "", i.display, ctx.Err()
		}
		d.Error = err.Error()
		return fmt.Sprintf("Error: fd failed to start: %v", err), d, errors.New("Execution failed")
	}

	if res.ExitCode != 0 && res.ExitCode != 1 {
		d.Error = fmt.Sprintf("exit code %d", res.ExitCode)
		return fmt.Sprintf("Error: fd failed with exit code %d: %s", res.ExitCode, res.Stderr), d, errors.New("Execution failed")
	}

	maxResults := maxFindResults
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
		return "No matches found.", d, nil
	}

	formattedMatches := strings.Join(matches, "\n")
	if hitMaxResults {
		formattedMatches += "\n\n(Results truncated. Consider using a more specific pattern.)"
	}

	return formattedMatches, d, nil
}
