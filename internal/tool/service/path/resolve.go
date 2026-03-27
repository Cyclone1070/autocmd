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
}

// NewResolver creates a new path resolver for the given workspace.
func NewResolver(workspaceRoot string) *Resolver {
	if workspaceRoot == "" {
		panic("workspaceRoot is required")
	}
	return &Resolver{
		workspaceRoot: workspaceRoot,
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

// Abs resolves any path to absolute and validates it is within the workspace boundary.
// It cleans the path and ensures it does not escape the workspace root.
func (r *Resolver) Abs(path string) (string, error) {
	if r.workspaceRoot == "" {
		return "", fmt.Errorf("workspace root not set")
	}

	var abs string
	if filepath.IsAbs(path) {
		abs = filepath.Clean(path)
	} else {
		abs = filepath.Clean(filepath.Join(r.workspaceRoot, path))
	}

	// Boundary check: must be the root itself or a child of the root
	if !strings.HasPrefix(abs, r.workspaceRoot+"/") && abs != r.workspaceRoot {
		return "", fmt.Errorf("path is outside workspace root: %s", path)
	}

	return abs, nil
}

// Rel resolves any path to relative to the workspace root and validates it is within the boundary.
func (r *Resolver) Rel(path string) (string, error) {
	abs, err := r.Abs(path)
	if err != nil {
		return "", err
	}

	rel, err := filepath.Rel(r.workspaceRoot, abs)
	if err != nil {
		// This should theoretically not happen if Abs passed
		return "", fmt.Errorf("path is outside workspace root: %s", path)
	}


	return filepath.ToSlash(rel), nil
}
