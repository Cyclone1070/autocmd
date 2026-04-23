// Package layout provides viewport layout utilities.
// truncateWithIndicator limits content height and shows an overflow indicator when content exceeds term height.

package ui

import (
	"fmt"
)

// TruncatingGater implements viewport-style truncation.
type TruncatingGater struct {
	maxLines int
}

func (g *TruncatingGater) Gate(lines []string) ([]string, int) {
	if g.maxLines <= 0 || len(lines) == 0 {
		return lines, 0
	}
	if len(lines) <= g.maxLines {
		return lines, 0
	}

	if g.maxLines == 1 {
		return []string{fmt.Sprintf("    ▲ [%d lines truncated]", len(lines))}, 1
	}

	// We need 2 lines for the indicator header (one empty line, one text line)
	maxContentLines := g.maxLines - 2
	overflow := len(lines) - maxContentLines
	header := fmt.Sprintf("    ▲ [%d lines truncated]", overflow)

	if maxContentLines == 0 {
		return []string{"", header}, 2
	}

	visible := lines[overflow:]
	return append([]string{"", header}, visible...), 2
}

// NewTruncatingGater creates a gater that truncates content after maxLines.
func NewTruncatingGater(maxLines int) *TruncatingGater {
	return &TruncatingGater{maxLines: maxLines}
}

type NoOpGater struct{}

func (g *NoOpGater) Gate(lines []string) ([]string, int) { return lines, 0 }

// NewNoOpGater returns a gater that performs no truncation.
func NewNoOpGater() *NoOpGater {
	return &NoOpGater{}
}

// ToolOutputGater implements tool-specific truncation logic.
type ToolOutputGater struct {
	maxLines int
}

func (g *ToolOutputGater) Gate(lines []string) ([]string, int) {
	if g.maxLines <= 0 || len(lines) == 0 {
		return lines, 0
	}
	overflow := len(lines) - g.maxLines
	if overflow <= 0 {
		return lines, 0
	}
	visible := lines[overflow:]
	indicator := fmt.Sprintf("  ▲ [%d lines truncated]", overflow)
	return append([]string{indicator}, visible...), 1
}

// NewToolOutputGater creates a gater optimized for tool output (bash.diff).
func NewToolOutputGater(maxLines int) *ToolOutputGater {
	return &ToolOutputGater{maxLines: maxLines}
}
