package domain

// Prompt workflow→UI events: UIUpdate marker, text/done.
// Used by the main agent prompt run (iav <args>).

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

// SummaryCompactionStartEvent is emitted when context compaction begins.
type SummaryCompactionStartEvent struct{}

func (SummaryCompactionStartEvent) isUIUpdate() {}

// SummaryCompactionEndEvent is emitted when context compaction completes.
// Error is empty on success.
type SummaryCompactionEndEvent struct {
	Error string
}

func (SummaryCompactionEndEvent) isUIUpdate() {}

// DoneEvent is emitted when the workflow loop completes.
type DoneEvent struct{}

func (DoneEvent) isUIUpdate() {}

// SystemNotificationEvent is emitted when a system notification is injected.
type SystemNotificationEvent struct {
	Content string
}

func (SystemNotificationEvent) isUIUpdate() {}
