package path

import (
	"os"
	"strings"
	"testing"
)

func TestAbs(t *testing.T) {
	workspaceRoot := "/workspace"
	resolver := NewResolver(workspaceRoot)

	tests := []struct {
		name      string
		input     string
		expected  string
		wantError bool
	}{
		{
			name:      "relative path within workspace",
			input:     "src/main.go",
			expected:  "/workspace/src/main.go",
			wantError: false,
		},
		{
			name:      "absolute path within workspace",
			input:     "/workspace/src/main.go",
			expected:  "/workspace/src/main.go",
			wantError: false,
		},
		{
			name:      "path with dots within workspace",
			input:     "src/../src/main.go",
			expected:  "/workspace/src/main.go",
			wantError: false,
		},
		{
			name:      "workspace root",
			input:     ".",
			expected:  "/workspace",
			wantError: false,
		},
		{
			name:      "absolute workspace root",
			input:     "/workspace",
			expected:  "/workspace",
			wantError: false,
		},
		{
			name:      "escape attempt via parent dots",
			input:     "../../../etc/passwd",
			expected:  "/etc/passwd",
			wantError: false,
		},
		{
			name:      "absolute path outside workspace",
			input:     "/etc/passwd",
			expected:  "/etc/passwd",
			wantError: false,
		},
		{
			name:      "prefix match but not child",
			input:     "/workspacefoo/bar",
			expected:  "/workspacefoo/bar",
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			abs, err := resolver.Abs(tt.input)
			if tt.wantError {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if !strings.Contains(err.Error(), "outside workspace") {
					t.Fatalf("expected outside workspace error, got: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if abs != tt.expected {
				t.Errorf("expected abs %q, got %q", tt.expected, abs)
			}
		})
	}
}

func TestDisplayPath(t *testing.T) {
	// Need to mock homedir for tilde expansion
	// For this test, let's assume /Users/mac is the home directory
	originalHome := os.Getenv("HOME")
	defer os.Setenv("HOME", originalHome)
	os.Setenv("HOME", "/Users/mac")

	workspaceRoot := "/Users/mac/project"
	resolver := NewResolver(workspaceRoot)

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "subdirectory within workspace",
			input:    "/Users/mac/project/src/main.go",
			expected: "src/main.go", // Relative, no ./
		},
		{
			name:     "workspace root",
			input:    "/Users/mac/project",
			expected: "~/project", // Root should be tilde expanded absolute
		},
		{
			name:     "outside workspace (home)",
			input:    "/Users/mac/other/file.txt",
			expected: "~/other/file.txt", // Tilde expanded
		},
		{
			name:     "outside workspace (not home)",
			input:    "/etc/passwd",
			expected: "/etc/passwd", // Absolute
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolver.DisplayPath(tt.input)
			if got != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, got)
			}
		})
	}
}


type mockFileInfo struct {
	os.FileInfo
	isDir bool
}

func (m mockFileInfo) IsDir() bool { return m.isDir }

type mockFS struct {
	abs          func(string) (string, error)
	evalSymlinks func(string) (string, error)
	stat         func(string) (os.FileInfo, error)
}

func (m mockFS) Abs(p string) (string, error)          { return m.abs(p) }
func (m mockFS) EvalSymlinks(p string) (string, error) { return m.evalSymlinks(p) }
func (m mockFS) Stat(p string) (os.FileInfo, error)    { return m.stat(p) }

func TestCanonicaliseRoot(t *testing.T) {
	t.Run("valid directory", func(t *testing.T) {
		fs := mockFS{
			abs:          func(p string) (string, error) { return "/abs/path", nil },
			evalSymlinks: func(p string) (string, error) { return "/resolved/path", nil },
			stat:         func(p string) (os.FileInfo, error) { return mockFileInfo{isDir: true}, nil },
		}
		got, err := CanonicaliseRoot(fs, "rel")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "/resolved/path" {
			t.Errorf("expected /resolved/path, got %q", got)
		}
	})

	t.Run("non-existent path", func(t *testing.T) {
		fs := mockFS{
			abs:  func(p string) (string, error) { return "/abs/path", nil },
			stat: func(p string) (os.FileInfo, error) { return nil, os.ErrNotExist },
		}
		// CanonicaliseRoot calls EvalSymlinks first.
		fs.evalSymlinks = func(p string) (string, error) { return "/abs/path", os.ErrNotExist }
		_, err := CanonicaliseRoot(fs, "non-existent")
		if err == nil {
			t.Fatal("expected error for non-existent path")
		}
	})

	t.Run("file instead of directory", func(t *testing.T) {
		fs := mockFS{
			abs:          func(p string) (string, error) { return "/abs/path", nil },
			evalSymlinks: func(p string) (string, error) { return "/abs/path", nil },
			stat:         func(p string) (os.FileInfo, error) { return mockFileInfo{isDir: false}, nil },
		}
		_, err := CanonicaliseRoot(fs, "file")
		if err == nil {
			t.Fatal("expected error for file instead of directory")
		}
	})
}
