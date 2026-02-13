package engine

import (
	"strings"
)

// Render produces the full View string from state.
func Render(state *State, deps Deps) string {
	if state.RunState == StateDone || state.RunState == StateCancelled || state.RunState == StateQuitting {
		return ""
	}

	content := renderContent(state, deps)
	currentHeight := 0
	if content != "" {
		currentHeight = strings.Count(content, "\n")
	}

	paddingLines := state.MaxAbsoluteHeight - (state.TotalFlushedLines + currentHeight)
	var padding string
	if paddingLines > 0 {
		padding = strings.Repeat("\n", paddingLines)
	}

	sb := statusBar(state, deps)
	if content == "" {
		return padding + sb
	}
	return content + padding + sb
}

// RenderSnapshot is a test-facing snapshot (no Bubble Tea types).
type RenderSnapshot struct {
	View               string
	TotalFlushedLines  int
	MaxAbsoluteHeight  int
}
