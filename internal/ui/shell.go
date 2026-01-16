package ui

import (
	"fmt"
	"strings"

	"github.com/Cyclone1070/iav/internal/domain"
)

// Output scrollback height
const shellOutputHeight = 8

func renderShell(width int, theme *theme, d domain.ShellDisplay, output string, status toolStatus, err string, prefix string) string {
	sep := theme.Separator(width)
	header := d.Header
	if status == statusError {
		header = fmt.Sprintf("%s — %s", header, theme.Error(err))
	}

	cmdLine := fmt.Sprintf("$ %s", d.Command)

	if status == statusError {
		return fmt.Sprintf(" %s %s \n   %s", prefix, header, cmdLine)
	}

	// 2. Output
	lines := strings.Split(strings.TrimRight(output, "\n"), "\n")

	// Calculate visible window
	var visibleLines []string
	if len(lines) > shellOutputHeight {
		visibleLines = lines[len(lines)-shellOutputHeight:]
	} else {
		visibleLines = lines
	}

	content := strings.Join(visibleLines, "\n")
	paddedContent := pad(content, "") // Indent lines, no prefix

	return fmt.Sprintf(" %s %s \n   %s\n%s\n%s",
		prefix,
		header,
		cmdLine,
		sep,
		paddedContent)
}
