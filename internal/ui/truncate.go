// Package layout provides viewport layout utilities.
// TruncateWithIndicator limits content height and shows an overflow indicator when content exceeds term height.

package ui

import (
	"fmt"
	"strings"
)

// TruncatingGater implements viewport-style truncation.
type TruncatingGater struct {
	maxLines int
}

func (g *TruncatingGater) Gate(content string) string {
	if g.maxLines <= 0 || content == "" {
		return content
	}
	return TruncateWithIndicator(content, g.maxLines)
}

// NewTruncatingGater creates a gater that truncates content after maxLines.
func NewTruncatingGater(maxLines int) *TruncatingGater {
	return &TruncatingGater{maxLines: maxLines}
}

type NoOpGater struct{}

func (g *NoOpGater) Gate(content string) string { return content }

// NewNoOpGater returns a gater that performs no truncation.
func NewNoOpGater() *NoOpGater {
	return &NoOpGater{}
}

// ToolOutputGater implements tool-specific truncation logic.
type ToolOutputGater struct {
	maxLines int
}

func (g *ToolOutputGater) Gate(content string) string {
	if g.maxLines <= 0 || content == "" {
		return content
	}
	lines := strings.Split(content, "\n")
	if len(lines) <= g.maxLines {
		return content
	}
	overflow := len(lines) - g.maxLines
	visible := lines[overflow:]
	indicator := fmt.Sprintf("  ▲ [%d lines truncated]", overflow)
	return indicator + "\n" + strings.Join(visible, "\n")
}

// NewToolOutputGater creates a gater optimized for tool output (shell/diff).
func NewToolOutputGater(maxLines int) *ToolOutputGater {
	return &ToolOutputGater{maxLines: maxLines}
}

// TruncateWithIndicator shows only the bottom portion if content is too tall.
func TruncateWithIndicator(content string, termHeight int) string {
	if termHeight <= 0 {
		return content
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
