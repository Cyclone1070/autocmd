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
	currentHeight := currentContentHeight(state, deps)
	historyHeight := currentHistoryHeight(state)

	paddingLines := state.MaxAbsoluteHeight - (historyHeight + currentHeight)
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

func currentHistoryHeight(state *State) int {
	height := state.TotalFlushedLines
	if state.ContentBeingPrinted != "" {
		// ContentBeingPrinted is already in the terminal buffer (EffectPrint enqueued)
		// but TotalFlushedLines hasn't updated yet. Count it as history to avoid duplication.
		height += strings.Count(state.ContentBeingPrinted, "\n")
		// Factor in the newline added by Println or the final newline in Printf
		if !state.ContentBeingPrintedRaw || !strings.HasSuffix(state.ContentBeingPrinted, "\n") {
			height++
		}
	}
	return height
}

func currentContentHeight(state *State, deps Deps) int {
	content := renderContent(state, deps)
	if content == "" {
		return 0
	}
	return strings.Count(content, "\n")
}

// RenderSnapshot is a test-facing snapshot (no Bubble Tea types).
type RenderSnapshot struct {
	View              string
	TotalFlushedLines int
	MaxAbsoluteHeight int
}
