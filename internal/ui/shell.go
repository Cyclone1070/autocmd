package ui

import (
	"fmt"
	"strings"

	"github.com/Cyclone1070/iav/internal/domain"
)

const (
	shellBoxHeight = 10
	shellBoxWidth  = 60 // Should be responsive? Lipgloss handles max width if we don't set it hard.
)

func (m *model) viewShellDisplay(d domain.ShellDisplay, t *toolState) string {
	sep := m.theme.separator.Render(strings.Repeat("─", boxWidth))
	header := d.Header
	if t.status == statusError {
		header = fmt.Sprintf("%s — %s", header, t.err)
	}

	cmdLine := fmt.Sprintf("$ %s", d.Command)

	if t.status == statusError {
		// On error, clear output and just show header + command
		return fmt.Sprintf("%s\n%s", header, cmdLine)
	}

	// 2. Output
	output := t.shellOutput.String()

	// Handle auto-scrolling
	lines := strings.Split(strings.TrimRight(output, "\n"), "\n")

	// Calculate visible window
	const outputHeight = 8
	var visibleLines []string
	if len(lines) > outputHeight {
		visibleLines = lines[len(lines)-outputHeight:]
	} else {
		visibleLines = lines
	}

	content := strings.Join(visibleLines, "\n")

	return fmt.Sprintf("%s\n%s\n%s\n%s",
		header,
		cmdLine,
		sep,
		content)
}
