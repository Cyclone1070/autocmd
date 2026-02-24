// Package layout provides viewport layout utilities.
// TruncateWithIndicator limits content height and shows an overflow indicator when content exceeds term height.

package ui

import (
	"strings"
)

// TruncateWithIndicator shows only the bottom portion if content is too tall.
func TruncateWithIndicator(content string, termHeight int) string {
	if termHeight <= 0 {
		return ""
	}

	lines := strings.Split(content, "\n")
	if len(lines) <= termHeight {
		return content
	}

	header := "  ▲ [Truncated]"
	if termHeight == 1 {
		return header
	}

	// We need 1 line for the "▲ [Truncated]" header
	maxContentLines := termHeight - 1
	overflow := len(lines) - maxContentLines
	visible := lines[overflow:]
	return header + "\n" + strings.Join(visible, "\n")
}
