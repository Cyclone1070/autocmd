// Package layout provides viewport layout utilities.
// TruncateWithIndicator limits content height and shows an overflow indicator when content exceeds term height.

package ui

import (
	"strings"
)

const (
	minVisibleLines = 3
)

// TruncateWithIndicator shows only the bottom portion if content is too tall.
func TruncateWithIndicator(content string, termHeight int) string {
	lines := strings.Split(content, "\n")
	if len(lines) <= termHeight {
		return content
	}

	// We need 1 line for the "▲ [Truncated]" header
	maxContentLines := termHeight - 1
	if maxContentLines < minVisibleLines {
		maxContentLines = minVisibleLines
	}

	overflow := len(lines) - maxContentLines
	visible := lines[overflow:]
	header := "  ▲ [Truncated]"
	return header + "\n" + strings.Join(visible, "\n")
}
