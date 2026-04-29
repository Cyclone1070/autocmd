package git

import (
	"errors"
	"os"
	"testing"

	"github.com/Cyclone1070/iav/internal/testutil"
)

const testWorkspaceRoot = testutil.TestWorkspaceRoot

// mockFileSystem is a local mock implementing fileSystem for testing.
type mockFileSystem struct {
	files   map[string][]byte
	readErr error
}

func newMockFileSystem() *mockFileSystem {
	return &mockFileSystem{
		files: make(map[string][]byte),
	}
}

func (m *mockFileSystem) createFile(_ string, content []byte) {
	m.files[testWorkspaceRoot + "/.gitignore"] = content
}

func (m *mockFileSystem) Stat(path string) (os.FileInfo, error) {
	if _, ok := m.files[path]; ok {
		return nil, nil // File exists
	}
	return nil, os.ErrNotExist
}

func (m *mockFileSystem) ReadFile(path string) ([]byte, error) {
	if m.readErr != nil {
		return nil, m.readErr
	}
	content, ok := m.files[path]
	if !ok {
		return nil, os.ErrNotExist
	}
	return content, nil
}

func TestLoadGitignore(t *testing.T) {
	workspaceRoot := testWorkspaceRoot

	t.Run("load gitignore from workspace root", func(t *testing.T) {
		fs := newMockFileSystem()
		fs.createFile(testWorkspaceRoot + "/.gitignore", []byte("*.log\n*.tmp\n"))

		matcher, err := NewIgnoreMatcher(workspaceRoot, fs)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if matcher == nil {
			t.Fatal("expected non-nil matcher")
		}

		// Test matching patterns
		if !matcher.ShouldIgnore("test.log") {
			t.Error("expected test.log to be ignored")
		}
		if !matcher.ShouldIgnore("file.tmp") {
			t.Error("expected file.tmp to be ignored")
		}
		if matcher.ShouldIgnore("test.txt") {
			t.Error("expected test.txt not to be ignored")
		}
	})

	t.Run("non-existent gitignore should not error", func(t *testing.T) {
		fs := newMockFileSystem()

		service, err := NewIgnoreMatcher(workspaceRoot, fs)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if service == nil {
			t.Fatal("expected non-nil service")
		}

		// Should not ignore anything if no .gitignore
		if service.ShouldIgnore("test.log") {
			t.Error("expected no files to be ignored without .gitignore")
		}
	})

	t.Run("dotfiles matching gitignore patterns", func(t *testing.T) {
		fs := newMockFileSystem()
		fs.createFile(testWorkspaceRoot + "/.gitignore", []byte("*.log\n"))

		matcher, err := NewIgnoreMatcher(workspaceRoot, fs)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Dotfile matching pattern should be ignored
		if !matcher.ShouldIgnore(".test.log") {
			t.Error("expected .test.log to be ignored")
		}

		// Dotfile not matching pattern should not be ignored
		if matcher.ShouldIgnore(".keep") {
			t.Error("expected .keep not to be ignored")
		}
	})
}

func TestNewIgnoreMatcherErrors(t *testing.T) {
	workspaceRoot := testWorkspaceRoot

	t.Run("ReadError", func(t *testing.T) {
		fs := newMockFileSystem()
		fs.createFile(testWorkspaceRoot + "/.gitignore", []byte("*.log"))
		fs.readErr = errors.New("disk failure")

		_, err := NewIgnoreMatcher(workspaceRoot, fs)
		if err == nil {
			t.Error("expected error for read failure")
		}
		var gitErr *GitignoreReadError
		if !errors.As(err, &gitErr) {
			t.Errorf("expected GitignoreReadError, got %T: %v", err, err)
		}
	})
}

func TestShouldIgnoreLogic(t *testing.T) {
	workspaceRoot := testWorkspaceRoot

	t.Run("WindowsLineEndings", func(t *testing.T) {
		fs := newMockFileSystem()
		// Use \r\n line endings
		fs.createFile(testWorkspaceRoot + "/.gitignore", []byte("*.log\r\nnode_modules\r\n"))

		matcher, err := NewIgnoreMatcher(workspaceRoot, fs)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !matcher.ShouldIgnore("app.log") {
			t.Error("failed to match pattern with CRLF")
		}
		if !matcher.ShouldIgnore("node_modules/foo") {
			t.Error("failed to match directory with CRLF")
		}
	})

	t.Run("PathNormalization", func(t *testing.T) {
		fs := newMockFileSystem()
		fs.createFile(testWorkspaceRoot + "/.gitignore", []byte("*.log"))

		matcher, err := NewIgnoreMatcher(workspaceRoot, fs)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Consecutive slashes
		if !matcher.ShouldIgnore("foo//bar.log") {
			t.Error("failed to ignore path with consecutive slashes")
		}

		// Dot path (current dir)
		if !matcher.ShouldIgnore("./baz.log") {
			t.Error("failed to ignore path with dot prefix")
		}
	})
}
