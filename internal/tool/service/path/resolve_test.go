package path

import (
	"os"
	"path/filepath"
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
			expected:  "",
			wantError: true,
		},
		{
			name:      "absolute path outside workspace",
			input:     "/etc/passwd",
			expected:  "",
			wantError: true,
		},
		{
			name:      "prefix match but not child",
			input:     "/workspacefoo/bar",
			expected:  "",
			wantError: true,
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

func TestRel(t *testing.T) {
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
			expected:  "src/main.go",
			wantError: false,
		},
		{
			name:      "absolute path within workspace",
			input:     "/workspace/src/main.go",
			expected:  "src/main.go",
			wantError: false,
		},
		{
			name:      "workspace root",
			input:     "/workspace",
			expected:  "",
			wantError: false,
		},
		{
			name:      "escape attempt",
			input:     "/etc/passwd",
			expected:  "",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rel, err := resolver.Rel(tt.input)
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
			if rel != tt.expected {
				t.Errorf("expected rel %q, got %q", tt.expected, rel)
			}
		})
	}
}

func TestCanonicaliseRoot(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "pathutil-test")
	if err != nil {
		t.Fatalf("failed to create tmp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	resolvedTmpDir, err := filepath.EvalSymlinks(tmpDir)
	if err != nil {
		t.Fatalf("failed to resolve tmp dir: %v", err)
	}

	t.Run("valid directory", func(t *testing.T) {
		got, err := CanonicaliseRoot(resolvedTmpDir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != resolvedTmpDir {
			t.Errorf("expected %q, got %q", resolvedTmpDir, got)
		}
	})

	t.Run("non-existent path", func(t *testing.T) {
		_, err := CanonicaliseRoot(filepath.Join(resolvedTmpDir, "non-existent"))
		if err == nil {
			t.Fatal("expected error for non-existent path")
		}
	})

	t.Run("file instead of directory", func(t *testing.T) {
		tmpFile := filepath.Join(resolvedTmpDir, "file.txt")
		if err := os.WriteFile(tmpFile, []byte("test"), 0o644); err != nil {
			t.Fatalf("failed to create tmp file: %v", err)
		}
		_, err := CanonicaliseRoot(tmpFile)
		if err == nil {
			t.Fatal("expected error for file instead of directory")
		}
	})
}
