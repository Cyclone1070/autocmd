package ui

import (
	"fmt"
	"strings"

	"github.com/Cyclone1070/iav/internal/domain"
)

func renderShell(width int, shellOutputHeight int, theme *theme, d domain.ShellDisplay, output string, status toolStatus, err string, prefix string) string {
	sep := theme.Separator(width, status)
	// 1. Header (Command)
	header := d.Header
	cmdLine := pad(fmt.Sprintf("$ %s", d.Command), "")
	if status == statusError {
		header = formatError(header, err, theme)
		// Early return on error - show header and command, hide output
		return fmt.Sprintf(" %s %s \n%s\n%s", prefix, header, sep, cmdLine)
	}

	// 2. Output
	lines := strings.Split(strings.TrimRight(output, "\n"), "\n")

	// Calculate visible window using configurable height
	var visibleLines []string
	if len(lines) > shellOutputHeight {
		visibleLines = lines[len(lines)-shellOutputHeight:]
	} else {
		visibleLines = lines
	}

	content := strings.Join(visibleLines, "\n")
	paddedContent := pad(content, "") // Indent lines, no prefix

	return fmt.Sprintf(" %s %s \n%s\n%s\n%s\n%s",
		prefix,
		header,
		sep,
		cmdLine,
		sep,
		paddedContent)
}
