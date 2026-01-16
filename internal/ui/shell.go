package ui

import (
	"fmt"
	"strings"

	"github.com/Cyclone1070/iav/internal/domain"
)

// Output scrollback height
const shellOutputHeight = 8

func (m *model) viewShellDisplay(d domain.ShellDisplay, t *toolState, prefix string) string {
	sep := m.theme.separator.Render(strings.Repeat("─", m.width-2)) // -2 for borders
	header := d.Header
	if t.status == statusError {
		header = fmt.Sprintf("%s — %s", header, t.err)
	}

	cmdLine := fmt.Sprintf("$ %s", d.Command)

	if t.status == statusError {
		return fmt.Sprintf(" %s %s \n   %s", prefix, header, cmdLine)
	}

	// 2. Output
	output := t.shellOutput.String()
	lines := strings.Split(strings.TrimRight(output, "\n"), "\n")

	// Calculate visible window
	var visibleLines []string
	if len(lines) > shellOutputHeight {
		visibleLines = lines[len(lines)-shellOutputHeight:]
	} else {
		visibleLines = lines
	}

	content := strings.Join(visibleLines, "\n")
	paddedContent := m.pad(content, "") // Indent lines, no prefix

	return fmt.Sprintf(" %s %s \n   %s\n%s\n%s",
		prefix,
		header,
		cmdLine,
		sep,
		paddedContent)
}
