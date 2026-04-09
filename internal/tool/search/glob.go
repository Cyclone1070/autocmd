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
	maxFindResults = 100
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
		Desc: "Find files matching a glob pattern.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"pattern": {
				Type:     schema.String,
				Desc:     "Glob pattern to match files.",
				Required: true,
			},
			"path": {
				Type: schema.String,
				Desc: "Path to search within. Defaults to the current directory.",
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
		fs:              t.fs,
		commandExecutor: t.commandExecutor,
		pathResolver:    t.pathResolver,
		absPath:         absPath,
		pattern:         req.Pattern,
		display:         domain.NewStringDisplay("", fmt.Sprintf("GLOB \"%s\" IN \"%s\"", req.Pattern, filepath.ToSlash(displayPath))),
	}, nil
}

type globInvocation struct {
	fs              fileSystem
	commandExecutor commandExecutor
	pathResolver    pathResolver
	absPath         string
	pattern         string
	display         domain.StringDisplay
}

func (i *globInvocation) Display() domain.ToolDisplay {
	return i.display
}

func (i *globInvocation) Execute(ctx context.Context) (string, domain.ToolDisplay, error) {
	d := i.display

	if ctx.Err() != nil {
		d.Error = domain.ToolErrorCancelled
		return "execution cancelled", d, ctx.Err()
	}

	// Re-verify State
	info, err := i.fs.Stat(i.absPath)
	if err != nil {
		if ctx.Err() != nil {
			d.Error = domain.ToolErrorCancelled
			return "execution cancelled", d, ctx.Err()
		}
		if os.IsNotExist(err) {
			d.Error = domain.ToolErrorFailed
			return fmt.Sprintf("Error: Path %s no longer exists.", i.absPath), d, nil
		}
		d.Error = domain.ToolErrorFailed
		return fmt.Sprintf("Error: Failed to access %s: %v", i.absPath, err), d, nil
	}
	if !info.IsDir() {
		d.Error = domain.ToolErrorFailed
		return fmt.Sprintf("Error: Path %s is no longer a directory.", i.absPath), d, nil
	}

	// fd --glob "pattern" searchPath
	cmd := []string{"fd", "--glob", i.pattern, i.absPath}

	res, err := i.commandExecutor.Run(ctx, cmd, i.absPath, os.Environ())
	if err != nil {
		if ctx.Err() != nil {
			d.Error = domain.ToolErrorCancelled
			return "execution cancelled", d, ctx.Err()
		}
		d.Error = domain.ToolErrorFailed
		return fmt.Sprintf("Error: fd failed to start: %v", err), d, nil
	}

	if res.ExitCode != 0 && res.ExitCode != 1 {
		d.Error = domain.ToolErrorFailed
		return fmt.Sprintf("Error: fd failed with exit code %d: %s", res.ExitCode, res.Stderr), d, nil
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

		relPath := i.pathResolver.DisplayPath(line)
		matches = append(matches, fmt.Sprintf("\"%s\"", filepath.ToSlash(relPath)))

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
