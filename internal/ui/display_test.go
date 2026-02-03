package ui

import (
	"strings"
	"testing"

	"github.com/Cyclone1070/iav/internal/config"
	"github.com/Cyclone1070/iav/internal/domain"
)

func TestRenderDiff(t *testing.T) {
	// Setup independent theme
	cfg := config.DefaultConfig().UI
	theme := newTheme(cfg)
	width := 60

	tests := []struct {
		name   string
		diff   domain.DiffDisplay
		status toolStatus
		err    string
		prefix string
	}{
		{
			name: "Success_WithStats",
			diff: domain.DiffDisplay{
				Header:  "file.go",
				Added:   5,
				Removed: 2,
				Diff:    " @@ -1,2 +1,2 @@\n-old\n+new",
			},
			status: statusSuccess,
			prefix: "✓",
		},
		{
			name: "Error",
			diff: domain.DiffDisplay{
				Header: "file.go",
			},
			status: statusError,
			err:    "file not found",
			prefix: "✗",
		},
		{
			name: "DiffBody_Alignment",
			diff: domain.DiffDisplay{
				Header: "align.go",
				Diff:   "\n-line1\n+line2", // Check leading newline handling
			},
			status: statusRunning,
			prefix: "⣾",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := renderDiff(width, theme, tt.diff, tt.status, tt.err, tt.prefix)
			assertGolden(t, "RenderDiff_"+tt.name, output)
		})
	}
}

func TestRenderShell(t *testing.T) {
	cfg := config.DefaultConfig().UI
	theme := newTheme(cfg)
	width := 40 // Narrow width to test wrapping

	tests := []struct {
		name    string
		display domain.ShellDisplay
		output  string
		status  toolStatus
		err     string
		prefix  string
	}{
		{
			name: "Running_Command",
			display: domain.ShellDisplay{
				Header:  "List Files",
				Command: "ls -la",
			},
			output: "file1.txt\nfile2.txt",
			status: statusRunning,
			prefix: "⣾",
		},
		{
			name: "Error",
			display: domain.ShellDisplay{
				Header:  "List Files",
				Command: "ls -la",
			},
			output: "",
			status: statusError,
			err:    "exit status 1",
			prefix: "✗",
		},
		{
			name: "Long_Output_Truncation",
			display: domain.ShellDisplay{
				Header:  "Log",
				Command: "cat log.txt",
			},
			// Generate 15 lines to test truncation (shellOutputHeight = 12)
			output: strings.Repeat("line\n", 15),
			status: statusSuccess,
			prefix: "✓",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := renderShell(width, cfg.ShellOutputHeight, theme, tt.display, tt.output, tt.status, tt.err, tt.prefix)
			assertGolden(t, "RenderShell_"+tt.name, out)
		})
	}
}

func TestRenderString(t *testing.T) {
	cfg := config.DefaultConfig().UI
	theme := newTheme(cfg)

	tests := []struct {
		name    string
		display domain.StringDisplay
		status  toolStatus
		err     string
		prefix  string
	}{
		{
			name:    "Running",
			display: domain.StringDisplay("Reading massive_file.txt..."),
			status:  statusRunning,
			prefix:  "⣾",
		},
		{
			name:    "Error_Wrap",
			display: domain.StringDisplay("Reading file"),
			status:  statusError,
			err:     "permission denied",
			prefix:  "✗",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := renderString(theme, tt.display, tt.status, tt.err, tt.prefix)
			assertGolden(t, "RenderString_"+tt.name, out)
		})
	}
}

func TestPad(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		prefix string
	}{
		{
			name:   "With_Prefix",
			input:  "Line1\nLine2",
			prefix: "->",
		},
		{
			name:   "Without_Prefix",
			input:  "Line1\nLine2",
			prefix: "",
		},
		{
			name:   "Empty_Input",
			input:  "",
			prefix: "->",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := pad(tt.input, tt.prefix)
			assertGolden(t, "Pad_"+tt.name, out)
		})
	}
}
