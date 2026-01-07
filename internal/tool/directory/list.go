package directory

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Cyclone1070/iav/internal/config"
	"github.com/Cyclone1070/iav/internal/tool"
)

// directoryEntry is an internal helper for directory entries.
type directoryEntry struct {
	Name  string
	IsDir bool
}

// ListDirTool allows agents to list directory contents with proper validation and tree formatting.
type ListDirTool struct {
	fs           dirLister
	config       *config.Config
	pathResolver pathResolver
	// ignoreMatcher is optional (can be nil)
	ignoreMatcher ignoreMatcher
}

// NewListDirectoryTool creates a new ListDirTool.
func NewListDirectoryTool(
	fs dirLister,
	cfg *config.Config,
	pathResolver pathResolver,
	ignoreMatcher ignoreMatcher, // optional, can be nil
) *ListDirTool {
	if fs == nil {
		panic("fs is required")
	}
	if cfg == nil {
		panic("cfg is required")
	}
	if pathResolver == nil {
		panic("pathResolver is required")
	}
	return &ListDirTool{
		fs:            fs,
		config:        cfg,
		pathResolver:  pathResolver,
		ignoreMatcher: ignoreMatcher,
	}
}

// Name returns the name of the tool.
func (t *ListDirTool) Name() string {
	return "list_directory"
}

// Declaration returns the JSON schema for the tool.
func (t *ListDirTool) Declaration() tool.Declaration {
	return tool.Declaration{
		Name:        t.Name(),
		Description: "Lists the contents of a directory. Returns the output as a tree structure. Truncates results if there are too many items.",
		Parameters: &tool.Schema{
			Type: tool.TypeObject,
			Properties: map[string]*tool.Schema{
				"path": {
					Type:        tool.TypeString,
					Description: "Path to the directory.",
				},
				"ignore": {
					Type:        tool.TypeArray,
					Items:       &tool.Schema{Type: tool.TypeString},
					Description: "Optional glob patterns to ignore (e.g. '*.test.ts').",
				},
			},
			Required: []string{"path"},
		},
	}
}

// ListDirRequest represents the input parameters.
type ListDirRequest struct {
	Path   string   `json:"path"`
	Ignore []string `json:"ignore,omitempty"`
}

// listDirInvocation represents a validated request.
type listDirInvocation struct {
	fs             dirLister
	pathResolver   pathResolver
	ignoreMatcher  ignoreMatcher
	config         *config.Config
	resolvedPath   string
	ignorePatterns []string
	display        tool.ToolDisplay
}

// Prepare validates path existence and returns an Invocation.
func (t *ListDirTool) Prepare(ctx context.Context, params json.RawMessage) (tool.Invocation, error) {
	var req ListDirRequest
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, fmt.Errorf("failed to parse arguments: %w", err)
	}

	if req.Path == "" {
		return nil, errors.New("path is required")
	}

	// 1. Resolve Path
	absPath, err := t.pathResolver.Abs(req.Path)
	if err != nil {
		return nil, fmt.Errorf("invalid path: %w", err)
	}

	// 2. Validate Existence (Fail Fast)
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

	return &listDirInvocation{
		fs:             t.fs,
		pathResolver:   t.pathResolver,
		ignoreMatcher:  t.ignoreMatcher,
		config:         t.config,
		resolvedPath:   absPath,
		ignorePatterns: req.Ignore,
		display:        tool.StringDisplay(fmt.Sprintf("Listing %s", filepath.Base(absPath))),
	}, nil
}

// Execute performs the safe directory listing (re-verifying existence).
func (i *listDirInvocation) Execute(ctx context.Context) (string, error) {
	if ctx.Err() != nil {
		return "", ctx.Err()
	}

	// 1. Re-verify State (TOCTOU Safety)
	info, err := i.fs.Stat(i.resolvedPath)
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		if os.IsNotExist(err) {
			return fmt.Sprintf("Error: Directory %s no longer exists.", i.resolvedPath), err
		}
		return fmt.Sprintf("Error: Failed to access %s: %v", i.resolvedPath, err), err
	}
	if !info.IsDir() {
		return fmt.Sprintf("Error: Path %s is no longer a directory.", i.resolvedPath), fmt.Errorf("path is no longer a directory: %s", i.resolvedPath)
	}

	// 2. List Directory (Shallow / Depth 1)
	entries, err := i.fs.ListDir(i.resolvedPath)
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return fmt.Sprintf("Error: Failed to list directory contents: %v", err), err
	}

	// 3. Filter & Convert
	var validEntries []directoryEntry
	for _, e := range entries {
		name := e.Name()

		// Check gitignore patterns (relative to workspace root)
		fullPath := filepath.Join(i.resolvedPath, name)
		relPath, err := i.pathResolver.Rel(fullPath)
		if err != nil {
			relPath = name
		}

		if i.ignoreMatcher != nil && i.ignoreMatcher.ShouldIgnore(relPath) {
			continue
		}

		// Check explicit ignore patterns from request
		isIgnored := false
		for _, pattern := range i.ignorePatterns {
			if matched, _ := filepath.Match(pattern, name); matched {
				isIgnored = true
				break
			}
		}
		if isIgnored {
			continue
		}

		validEntries = append(validEntries, directoryEntry{
			Name:  name,
			IsDir: e.IsDir(),
		})
	}

	// 4. Sort (Dirs first, then files)
	sort.Slice(validEntries, func(x, y int) bool {
		if validEntries[x].IsDir != validEntries[y].IsDir {
			return validEntries[x].IsDir // true (dir) comes before false (file)
		}
		return validEntries[x].Name < validEntries[y].Name
	})

	// 5. Truncate
	maxResults := i.config.Tools.MaxListDirectoryResults
	if maxResults <= 0 {
		maxResults = 100
	}

	truncated := false
	hiddenCount := 0
	if len(validEntries) > maxResults {
		hiddenCount = len(validEntries) - maxResults
		validEntries = validEntries[:maxResults]
		truncated = true
	}

	// 6. Format Tree
	var sb strings.Builder
	sb.WriteString(i.resolvedPath)
	if !strings.HasSuffix(i.resolvedPath, "/") {
		sb.WriteString("/")
	}
	sb.WriteString("\n")

	for _, entry := range validEntries {
		sb.WriteString("  ") // indent
		sb.WriteString(entry.Name)
		if entry.IsDir {
			sb.WriteString("/")
		}
		sb.WriteString("\n")
	}

	if truncated {
		sb.WriteString(fmt.Sprintf("\n(Results truncated. %d items hidden. Use specificity or ignore patterns to see more.)", hiddenCount))
	} else if len(validEntries) == 0 {
		sb.WriteString("  (empty)")
	}

	return sb.String(), nil
}

// Display returns the user-facing description.
func (i *listDirInvocation) Display() tool.ToolDisplay {
	return i.display
}
