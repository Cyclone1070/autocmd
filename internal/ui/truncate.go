package ui

import (
	"fmt"
	"strings"
)

const (
	// truncationBuffer is reserved lines for tools + status bar during overflow truncation.
	truncationBuffer = 5
	// minVisibleLines ensures at least this many lines are shown even when truncating.
	minVisibleLines = 5
)

// TruncateWithIndicator shows only the bottom portion if content is too tall.
// Exported for deps/engine layout adapter.
func TruncateWithIndicator(content string, termHeight int) string {
	return truncateWithIndicator(content, termHeight)
}

func truncateWithIndicator(content string, termHeight int) string {
	lines := strings.Split(content, "\n")
	maxLines := max(termHeight-truncationBuffer, minVisibleLines)

	if len(lines) <= maxLines {
		return content
	}

	overflow := len(lines) - maxLines
	visible := lines[overflow:]
	header := fmt.Sprintf("\n  ↑ (%d lines temporarily truncated)", overflow)
	return header + "\n" + strings.Join(visible, "\n")
}
