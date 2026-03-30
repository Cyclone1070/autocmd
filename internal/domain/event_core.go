package domain

// Core workflow→UI events: UIUpdate marker, text/thinking/done.

// UIUpdate is the interface for all events flowing from Workflow to UI.
type UIUpdate interface {
	isUIUpdate()
}

// TextEvent is emitted when the LLM produces text output.
type TextEvent struct {
	Text      string
	IsThought bool
}

func (TextEvent) isUIUpdate() {}

// ThinkingEvent is emitted when the LLM is processing.
type ThinkingEvent struct{}

func (ThinkingEvent) isUIUpdate() {}

// DoneEvent is emitted when the workflow loop completes.
type DoneEvent struct{}

func (DoneEvent) isUIUpdate() {}
