// Package layout provides viewport layout utilities.
// truncateWithIndicator limits content height and shows an overflow indicator when content exceeds term height.

package ui

import (
	"fmt"
)

const indicatorHeight = 1

// TruncatingGater implements viewport-style truncation.
type TruncatingGater struct {
	maxLines int
}

// Gate implements the gater interface by truncating lines from the top.
func (g *TruncatingGater) Gate(lines []string, scrollOffset int, scrollable bool, theme *Theme) ([]string, int) {
	if g.maxLines <= 0 || len(lines) == 0 {
		return lines, 0
	}
	if len(lines) <= g.maxLines {
		return lines, 0
	}

	// We reserve 4 lines for indicators (top + bottom).
	// If the budget is too small, fallback to simple truncation.
	if g.maxLines <= indicatorHeight*2 {
		return lines[len(lines)-g.maxLines:], 0
	}

	maxContentLines := g.maxLines - indicatorHeight*2
	maxScroll := len(lines) - maxContentLines

	clampedOffset := max(min(scrollOffset, maxScroll), 0)

	// tailStart is where the window begins when scrollOffset is 0.
	tailStart := len(lines) - maxContentLines
	start := tailStart - clampedOffset
	end := start + maxContentLines

	// Greedy expansion: if we are at either end of the content, we don't need
	// one of the indicators. We can use that line for more content to fill the budget.
	if start == 0 && end < len(lines) {
		end += indicatorHeight
	} else if end == len(lines) && start > 0 {
		start -= indicatorHeight
	}

	visible := lines[start:end]
	var result []string
	if start > 0 {
		indicator := theme.Muted(fmt.Sprintf("    ▲ [%d lines above]", start))
		if scrollable {
			indicator += "  " + theme.Primary("Ctrl+u") + theme.Muted(" scroll up")
		}
		result = append(result, indicator)
	}

	result = append(result, visible...)

	if end < len(lines) {
		indicator := theme.Muted(fmt.Sprintf("    ▼ [%d lines below]", len(lines)-end))
		if scrollable {
			indicator += "  " + theme.Primary("Ctrl+d") + theme.Muted(" scroll down")
		}
		result = append(result, indicator)
	}

	return result, maxScroll
}

// NewTruncatingGater creates a gater that truncates content after maxLines.
func NewTruncatingGater(maxLines int) *TruncatingGater {
	return &TruncatingGater{maxLines: maxLines}
}

// NoOpGater implements a gater that performs no truncation.
type NoOpGater struct{}

// Gate returns the input lines as-is.
func (g *NoOpGater) Gate(lines []string, _ int, _ bool, _ *Theme) ([]string, int) { return lines, 0 }

// NewNoOpGater returns a gater that performs no truncation.
func NewNoOpGater() *NoOpGater {
	return &NoOpGater{}
}

// ToolOutputGater implements tool-specific truncation logic.
type ToolOutputGater struct {
	maxLines int
}

// Gate implements the gater interface optimized for tool output.
func (g *ToolOutputGater) Gate(lines []string, _ int, _ bool, theme *Theme) ([]string, int) {
	if len(lines) <= g.maxLines {
		return lines, 0
	}
	overflow := len(lines) - g.maxLines
	visible := lines[overflow:]
	indicator := theme.Muted(fmt.Sprintf("  ▲ [%d lines above]", overflow))
	return append([]string{indicator}, visible...), 0
}

// NewToolOutputGater creates a gater optimized for tool output (bash.diff).
func NewToolOutputGater(maxLines int) *ToolOutputGater {
	return &ToolOutputGater{maxLines: maxLines}
}
