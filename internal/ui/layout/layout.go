// Package layout provides viewport layout utilities.
// TruncateWithIndicator limits content height and shows an overflow indicator when content exceeds term height.

package layout

import (
	"fmt"
	"strings"
)

const (
	truncationBuffer = 5
	minVisibleLines  = 5
)

// TruncateWithIndicator shows only the bottom portion if content is too tall.
func TruncateWithIndicator(content string, termHeight int) string {
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
