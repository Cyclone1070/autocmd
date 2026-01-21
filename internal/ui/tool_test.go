package ui

import (
	"testing"

	"github.com/Cyclone1070/iav/internal/config"
	"github.com/Cyclone1070/iav/internal/domain"
)

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
