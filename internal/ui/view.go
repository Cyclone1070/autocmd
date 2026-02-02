package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func (m *model) View() string {
	// When done or cancelled, we've already flushed everything via tea.Println
	// Return empty to prevent duplicate rendering and extra whitespace
	if m.runState == stateDone || m.runState == stateCancelled || m.runState == stateQuitting {
		return ""
	}

	var contentParts []string

	// 1. Pending markdown (last uncertain block)
	if pending := m.streamingMd.Pending(); pending != "" {
		pending = truncateWithIndicator(pending, m.termHeight)
		contentParts = append(contentParts, pending)
	}

	// 2. All remaining tools (running + waiting-to-flush)
	for _, t := range m.tools {
		contentParts = append(contentParts, m.viewTool(t))
	}

	// Join content parts
	content := strings.Join(contentParts, "\n")

	// Calculate current content height (line count)
	currentHeight := 0
	if content != "" {
		// Fix: Use strictly newline count.
		// "hello" (0 newlines) occupies 1 visual line but 0 vertical lines relative to start.
		// "hello\n" (1 newline) occupies 1 vertical line.
		currentHeight = strings.Count(content, "\n")
	}

	// Update max content height (only grows, never shrinks during session)
	if currentHeight > m.maxContentHeight {
		m.maxContentHeight = currentHeight
	}

	// Add padding to maintain consistent height (prevents status bar jiggling)
	paddingLines := m.maxContentHeight - currentHeight
	var padding string
	if paddingLines > 0 {
		padding = strings.Repeat("\n", paddingLines)
	}

	// Build final view: content + padding + status bar
	// Status bar now has builtin \n\n prefix
	statusBar := m.statusBar()

	if content == "" {
		// No content, just padding + status bar
		return padding + statusBar
	}

	// Content + Padding + StatusBar
	// Note: content usually does not end with \n\n unless multiple tools
	return content + padding + statusBar
}

// truncateWithIndicator shows only the bottom portion if content is too tall.
// This is a pure function that takes explicit arguments for testability.
func truncateWithIndicator(content string, termHeight int) string {
	lines := strings.Split(content, "\n")
	// Calculate available space: total height - tool buffer - status
	// But we don't know tool buffer height easily without rendering.
	// We'll use a conservative heuristic: leave 5 lines.
	maxLines := max(termHeight-5,
		// Minimum visibility
		5)

	if len(lines) <= maxLines {
		return content
	}

	overflow := len(lines) - maxLines
	visible := lines[overflow:]
	header := fmt.Sprintf("\n  ↑ (%d lines temporarily truncated)", overflow)
	return header + "\n" + strings.Join(visible, "\n")
}

func (m *model) statusBar() string {
	// Determine theme function based on state
	var themeFunc func(string) string
	switch m.runState {
	case stateDone:
		themeFunc = m.theme.Success
	case stateCancelled:
		themeFunc = m.theme.Error
	default:
		themeFunc = m.theme.Primary
	}

	// Hardcoded context window info for now
	contextInfo := themeFunc("Context: 42%")

	var left string
	switch m.runState {
	case stateDone:
		left = fmt.Sprintf("%s %s", themeFunc("✓"), themeFunc("Done"))
	case stateCancelled:
		left = fmt.Sprintf("%s %s", themeFunc("✗"), themeFunc("Cancelled"))
	default:
		status := "Generating"
		if m.thinking {
			status = "Thinking"
		}
		left = fmt.Sprintf("%s%s", m.spinner.View(), themeFunc(status))
	}

	// Calculate visual widths (ignores ANSI codes)
	leftWidth := lipgloss.Width(left)
	rightWidth := lipgloss.Width(contextInfo)
	neededWidth := leftWidth + 1 + rightWidth // +1 for minimum gap

	// If terminal too narrow, use two-line layout
	if m.width < neededWidth {
		return "\n\n" + left + "\n" + contextInfo
	}

	// Right-align: fill gap between left and right to push contextInfo to the edge
	gap := m.width - leftWidth - rightWidth
	return "\n\n" + left + strings.Repeat(" ", gap) + contextInfo
}
