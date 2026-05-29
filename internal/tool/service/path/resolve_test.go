package path

import (
	"strings"
	"testing"

	"github.com/Cyclone1070/autocmd/internal/testutil"
)

func TestAbs(t *testing.T) {
	workspaceRoot := testutil.TestWorkspaceRoot
	resolver := NewResolver(workspaceRoot)
	resolver.homeDir = "/mock/home"

	tests := []struct {
		name      string
		input     string
		expected  string
		errorMsg  string
		wantError bool
	}{
		{
			name:      "tilde home path",
			input:     "~/src/main.go",
			expected:  "/mock/home/src/main.go",
			wantError: false,
		},
		{
			name:      "tilde standalone",
			input:     "~",
			expected:  "/mock/home",
			wantError: false,
		},
		{
			name:      "relative path rejection",
			input:     "src/main.go",
			wantError: true,
			errorMsg:  "absolute path required",
		},
		{
			name:      "absolute path within workspace",
			input:     testutil.TestWorkspaceRoot + "/src/main.go",
			expected:  testutil.TestWorkspaceRoot + "/src/main.go",
			wantError: false,
		},
		{
			name:      "dot rejection",
			input:     ".",
			wantError: true,
			errorMsg:  "absolute path required",
		},
		{
			name:      "absolute workspace root",
			input:     testutil.TestWorkspaceRoot,
			expected:  testutil.TestWorkspaceRoot,
			wantError: false,
		},
		{
			name:      "absolute path outside workspace",
			input:     "/etc/passwd",
			expected:  "/etc/passwd",
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			abs, err := resolver.ValidateAbs(tt.input)
			if tt.wantError {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.errorMsg != "" && !strings.Contains(err.Error(), tt.errorMsg) {
					t.Fatalf("expected error containing %q, got: %v", tt.errorMsg, err)
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
	workspaceRoot := "/Users/mac/work/project"
	homeDir := "/Users/mac"
	resolver := NewResolver(workspaceRoot)
	resolver.homeDir = homeDir

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "within workspace (shown as home-relative)",
			input:    "/Users/mac/work/project/src/main.go",
			expected: "~/work/project/src/main.go",
		},
		{
			name:     "workspace root (shown as home-relative)",
			input:    "/Users/mac/work/project",
			expected: "~/work/project",
		},
		{
			name:     "outside workspace, inside home",
			input:    "/Users/mac/.bashrc",
			expected: "~/.bashrc",
		},
		{
			name:     "home directory itself",
			input:    "/Users/mac",
			expected: "~",
		},
		{
			name:     "outside home and workspace",
			input:    "/etc/passwd",
			expected: "/etc/passwd",
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
