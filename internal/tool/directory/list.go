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

	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/cloudwego/eino/schema"
)

const (
	defaultMaxResults  = 50000
	fallbackMaxResults = 100
)

// directoryEntry is an internal helper for directory entries.
type directoryEntry struct {
	Name  string
	IsDir bool
}

// ListDirTool allows agents to list directory contents with proper validation and tree formatting.
type ListDirTool struct {
	fs           dirLister
	pathResolver pathResolver
	// ignoreMatcher is optional (can be nil)
	ignoreMatcher ignoreMatcher

	maxResults int // Internal cap, can be overridden in tests
}

// NewListDirectoryTool creates a new ListDirTool.
func NewListDirectoryTool(
	fs dirLister,
	pathResolver pathResolver,
	ignoreMatcher ignoreMatcher, // optional, can be nil
) *ListDirTool {
	if fs == nil {
		panic("fs is required")
	}
	if pathResolver == nil {
		panic("pathResolver is required")
	}
	return &ListDirTool{
		fs:            fs,
		pathResolver:  pathResolver,
		ignoreMatcher: ignoreMatcher,
		maxResults:    defaultMaxResults,
	}
}

// Name returns the name of the tool.
func (t *ListDirTool) Name() string {
	return "list_directory"
}

// Definition returns the JSON schema for the tool using eino schema.
func (t *ListDirTool) Definition() *schema.ToolInfo {
	return &schema.ToolInfo{
		Name: t.Name(),
		Desc: "Lists the contents of a directory. Returns the output as a tree structure. Truncates results if there are too many items.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"path": {
				Type:     schema.String,
				Desc:     "Path to the directory.",
				Required: true,
			},
			"ignore": {
				Type: schema.Array,
				ElemInfo: &schema.ParameterInfo{
					Type: schema.String,
				},
				Desc: "Optional glob patterns to ignore (e.g. '*.test.ts').",
			},
		}),
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
	resolvedPath   string
	ignorePatterns []string
	display        domain.ToolDisplay
	maxResults     int
}

// Prepare validates path existence and returns an Invocation.
func (t *ListDirTool) Prepare(ctx context.Context, params string) (domain.Invocation, error) {
	var req ListDirRequest
	if err := json.Unmarshal([]byte(params), &req); err != nil {
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

	rel, err := t.pathResolver.Rel(absPath)
	if err != nil {
		rel = filepath.Base(absPath)
	}

	return &listDirInvocation{
		fs:             t.fs,
		pathResolver:   t.pathResolver,
		ignoreMatcher:  t.ignoreMatcher,
		resolvedPath:   absPath,
		ignorePatterns: req.Ignore,
		display:        domain.NewStringDisplay(fmt.Sprintf("Listing %s", filepath.ToSlash(rel))),
		maxResults:     t.maxResults,
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
	maxResults := i.maxResults
	if maxResults <= 0 {
		maxResults = fallbackMaxResults
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
		fmt.Fprintf(&sb, "\n(Results truncated. %d items hidden. Use specificity or ignore patterns to see more.)", hiddenCount)
	} else if len(validEntries) == 0 {
		sb.WriteString("  (empty)")
	}

	return sb.String(), nil
}

// Display returns the user-facing description.
func (i *listDirInvocation) Display() domain.ToolDisplay {
	return i.display
}
