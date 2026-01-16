package ui

import (
	"strings"
	"testing"

	"github.com/Cyclone1070/iav/internal/config"
	"github.com/Cyclone1070/iav/internal/domain"
)

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
			// Generate 10 lines, should only show last 8
			output: strings.Repeat("line\n", 10),
			status: statusSuccess,
			prefix: "✓",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := renderShell(width, theme, tt.display, tt.output, tt.status, tt.err, tt.prefix)
			assertGolden(t, "RenderShell_"+tt.name, out)
		})
	}
}
