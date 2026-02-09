package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func (m *model) View() string {
	// View is the main rendering function for the Bubble Tea model.
	// It handles:
	// 1. Current pending content (streaming markdown)
	// 2. Active tool displays
	// 3. Status bar with dynamic state
	view := m.renderView()

	// Capture frame for regression tests if in debug mode
	if m.debugMode {
		m.frameMu.Lock()
		m.frameLog = append(m.frameLog, view)
		m.frameMu.Unlock()
	}

	return view
}

// renderView contains the actual rendering logic
func (m *model) renderView() string {
	// When done or cancelled, we've already flushed everything via tea.Println
	// Return empty to prevent duplicate rendering and extra whitespace
	if m.runState == stateDone || m.runState == stateCancelled || m.runState == stateQuitting {
		return ""
	}

	content := m.renderContent()

	// Calculate current content height (line count)
	currentHeight := 0
	if content != "" {
		// Strictly count newlines for height
		currentHeight = strings.Count(content, "\n")
	}
	// NOTE: m.maxContentHeight is now updated in Update() via updateMaxContentHeight()
	// to keep View() pure.

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
	return content + padding + statusBar
}

// renderContent generates the main output content (markdown + tools) without padding/status bar.
// This is used by View() and by updateMaxContentHeight() in Update().
func (m *model) renderContent() string {
	var contentParts []string

	// 1. Pending markdown (last uncertain block)
	if pending := m.streamingMd.pending(); pending != "" {
		pending = truncateWithIndicator(pending, m.termHeight)
		contentParts = append(contentParts, pending)
	}

	// 2. All remaining tools (running + waiting-to-flush)
	for _, t := range m.tools {
		contentParts = append(contentParts, m.viewTool(t))
	}

	// Join content parts
	return strings.Join(contentParts, "\n")
}

const (
	// truncationBuffer is reserved lines for tools + status bar during overflow truncation.
	// This is a conservative heuristic since we don't know exact tool heights without rendering.
	truncationBuffer = 5

	// minVisibleLines ensures at least this many lines are shown even when truncating.
	minVisibleLines = 5
)

// truncateWithIndicator shows only the bottom portion if content is too tall.
// This is a pure function that takes explicit arguments for testability.
func truncateWithIndicator(content string, termHeight int) string {
	lines := strings.Split(content, "\n")
	// Calculate available space: total height - reserved buffer for tools + status
	maxLines := max(termHeight-truncationBuffer, minVisibleLines)

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
		left = fmt.Sprintf("%s %s", themeFunc("✓"), themeFunc(textDone))
	case stateCancelled:
		left = fmt.Sprintf("%s %s", themeFunc("✗"), themeFunc(textCancelled))
	default:
		status := textGenerating
		if m.thinking {
			status = textThinking
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
