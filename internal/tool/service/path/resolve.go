package path

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Resolver provides path resolution within a workspace boundary.
type Resolver struct {
	workspaceRoot string
	homeDir       string
}

// NewResolver creates a new path resolver for the given workspace.
func NewResolver(workspaceRoot string) *Resolver {
	if workspaceRoot == "" {
		panic("workspaceRoot is required")
	}
	home, _ := os.UserHomeDir()
	return &Resolver{
		workspaceRoot: workspaceRoot,
		homeDir:       home,
	}
}

// Root returns the canonical workspace root path.
func (r *Resolver) Root() string {
	return r.workspaceRoot
}

// FileSystem abstracts filesystem operations for path resolution.
type FileSystem interface {
	Abs(path string) (string, error)
	EvalSymlinks(path string) (string, error)
	Stat(path string) (os.FileInfo, error)
}

// OSFileSystem implements FileSystem using the real OS.
type OSFileSystem struct{}

func (OSFileSystem) Abs(path string) (string, error)          { return filepath.Abs(path) }
func (OSFileSystem) EvalSymlinks(path string) (string, error) { return filepath.EvalSymlinks(path) }
func (OSFileSystem) Stat(path string) (os.FileInfo, error)    { return os.Stat(path) }

// CanonicaliseRoot canonicalises a workspace root path by making it absolute and resolving symlinks.
// Returns an error if the path doesn't exist or isn't a directory.
func CanonicaliseRoot(fs FileSystem, root string) (string, error) {
	absRoot, err := fs.Abs(root)
	if err != nil {
		return "", fmt.Errorf("invalid workspace root %s: %w", root, err)
	}

	// Resolve symlinks in the workspace root to get canonical path
	resolved, err := fs.EvalSymlinks(absRoot)
	if err != nil {
		return "", fmt.Errorf("invalid workspace root %s: %w", absRoot, err)
	}

	info, err := fs.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("invalid workspace root %s: %w", resolved, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("workspace root is not a directory: %s", resolved)
	}
	return resolved, nil
}

// Abs ensures a path is absolute and returns its cleaned version.
// It returns an error if the path is relative.
func (r *Resolver) Abs(path string) (string, error) {
	if r.workspaceRoot == "" {
		return "", fmt.Errorf("workspace root not set")
	}

	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("absolute path required, but got: %q. Please provide full path from root (starts with /)", path)
	}

	return filepath.Clean(path), nil
}

// DisplayPath formats an absolute path for display purposes.
// It prioritizes collapsing the home directory to ~, matching the UI design choice.
func (r *Resolver) DisplayPath(path string) string {
	// 1. Try home-relative (collapsing home to ~)
	if r.homeDir != "" {
		if path == r.homeDir {
			return "~"
		}
		if strings.HasPrefix(path, r.homeDir+string(os.PathSeparator)) {
			return "~" + path[len(r.homeDir):]
		}
	}

	// 2. Try workspace-relative (only if outside home, or if home dir detection failed)
	rel, err := filepath.Rel(r.workspaceRoot, path)
	if err == nil && !strings.HasPrefix(rel, "..") {
		if rel == "." {
			return "."
		}
		return rel
	}

	// 3. Fallback to absolute
	return path
}
