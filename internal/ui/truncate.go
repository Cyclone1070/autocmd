// Package layout provides viewport layout utilities.
// TruncateWithIndicator limits content height and shows an overflow indicator when content exceeds term height.

package ui

import (
	"fmt"
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

	if termHeight == 1 {
		return fmt.Sprintf("  ▲ [%d lines truncated]", len(lines))
	}

	// We need 2 lines for the indicator header (one empty line, one text line)
	maxContentLines := termHeight - 2
	overflow := len(lines) - maxContentLines
	header := fmt.Sprintf("\n  ▲ [%d lines truncated]", overflow)

	if maxContentLines == 0 {
		return header
	}

	visible := lines[overflow:]
	return header + "\n" + strings.Join(visible, "\n")
}
