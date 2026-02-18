package engine

// Render produces the full View string from state.
func Render(state *State, deps Deps) string {
	// Don't render view if we're done/cancelled and not printing anything.
	if (state.RunState == StateDone || state.RunState == StateCancelled || state.RunState == StateQuitting) &&
		!state.IsPrinting && len(state.PrintQueue) == 0 {
		return ""
	}

	content := renderContent(state, deps)
	indicator := activityIndicator(state)

	return content + indicator
}

func activityIndicator(state *State) string {
	// Only show activity indicator while running.
	if state.RunState != StateRunning {
		return ""
	}

	// Only show if nothing is actively being typed or printed, and no tools are running.
	if state.TypingBuffer != "" || state.IsPrinting || len(state.PrintQueue) > 0 || len(state.Tools) > 0 {
		return ""
	}

	// 3-stage animated dots
	dots := "."
	switch state.IdleTicks % 3 {
	case 1:
		dots = ".."
	case 2:
		dots = "..."
	}
	return dots
}

// RenderSnapshot is a test-facing snapshot (no Bubble Tea types).
type RenderSnapshot struct {
	View              string
	TotalFlushedLines int
}
