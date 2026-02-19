package engine

// Render produces the full View string from state.
func Render(state *State, deps Deps) string {
	// Don't render view if we're done/cancelled and not printing anything.
	if (state.RunState == StateDone || state.RunState == StateCancelled || state.RunState == StateQuitting) &&
		!state.IsPrinting && len(state.PrintQueue) == 0 {
		return ""
	}

	return renderContent(state, deps)
}

// RenderSnapshot is a test-facing snapshot (no Bubble Tea types).
type RenderSnapshot struct {
	View              string
	TotalFlushedLines int
}
