package ui

import (
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
