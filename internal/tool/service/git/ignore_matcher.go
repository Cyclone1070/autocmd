// Package git provides git-related services, such as gitignore matching.
package git

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Cyclone1070/iav/internal/tool/helper/content"
	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
)

// GitignoreReadError is returned when .gitignore cannot be read.
type GitignoreReadError struct {
	Path  string
	Cause error
}

func (e *GitignoreReadError) Error() string {
	return fmt.Sprintf("failed to read .gitignore at %s: %v", e.Path, e.Cause)
}
func (e *GitignoreReadError) Unwrap() error { return e.Cause }

// fileSystem defines the minimal filesystem interface needed for gitignore service.
type fileSystem interface {
	Stat(path string) (os.FileInfo, error)
	ReadFile(path string) ([]byte, error)
}

// IgnoreMatcher implements gitignore pattern matching using go-git's gitignore matcher.
type IgnoreMatcher struct {
	matcher gitignore.Matcher
}

// NewIgnoreMatcher creates a new gitignore matcher by loading .gitignore from workspace root.
// Returns a matcher that never ignores if .gitignore doesn't exist (no error).
func NewIgnoreMatcher(workspaceRoot string, fs fileSystem) (*IgnoreMatcher, error) {
	if workspaceRoot == "" {
		panic("workspaceRoot is required")
	}
	if fs == nil {
		panic("fs is required")
	}
	gitignorePath := filepath.Join(workspaceRoot, ".gitignore")

	// Check if .gitignore exists
	if info, _ := fs.Stat(gitignorePath); info == nil {
		// .gitignore doesn't exist - return a matcher that never ignores
		return &IgnoreMatcher{matcher: nil}, nil
	}

	// Read .gitignore file
	data, err := fs.ReadFile(gitignorePath)
	if err != nil {
		return nil, &GitignoreReadError{Path: gitignorePath, Cause: err}
	}

	// Parse gitignore patterns line by line
	var patterns []gitignore.Pattern
	lines := content.SplitLines(string(data))
	for _, line := range lines {
		if line == "" {
			continue // Skip blank lines
		}
		pattern := gitignore.ParsePattern(line, nil)
		if pattern != nil {
			patterns = append(patterns, pattern)
		}
	}
	matcher := gitignore.NewMatcher(patterns)

	return &IgnoreMatcher{matcher: matcher}, nil
}

// ShouldIgnore checks if a relative path matches any gitignore patterns.
// Returns false if no .gitignore was loaded.
func (m *IgnoreMatcher) ShouldIgnore(relativePath string) bool {
	if m.matcher == nil {
		return false
	}

	// Convert to gitignore format (split path into segments)
	segments := splitPath(relativePath)
	return m.matcher.Match(segments, false)
}

// splitPath splits a path into segments for gitignore matching.
// It normalizes path separators and filters out empty and "." segments.
func splitPath(path string) []string {
	if path == "" {
		return []string{}
	}

	// Normalize path separators
	normalized := filepath.ToSlash(path)

	// Split by forward slash
	parts := strings.Split(normalized, "/")
	var segments []string
	for _, part := range parts {
		if part != "" && part != "." {
			segments = append(segments, part)
		}
	}

	return segments
}

// NoOpMatcher is a gitignore matcher that never ignores any files.
// It is used when gitignore functionality is disabled or fails to initialize.
type NoOpMatcher struct{}

// ShouldIgnore always returns false for NoOpMatcher.
func (m *NoOpMatcher) ShouldIgnore(_ string) bool {
	return false
}
