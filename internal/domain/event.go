package domain

// Event is the interface for all workflow events.
// UI handles events via type switch.
type Event interface {
	isEvent()
}

// TextEvent is emitted when the LLM produces text output.
type TextEvent struct {
	Text string
}

func (TextEvent) isEvent() {}

// ThinkingEvent is emitted when the LLM is processing.
type ThinkingEvent struct{}

func (ThinkingEvent) isEvent() {}

// DoneEvent is emitted when the workflow loop completes.
type DoneEvent struct{}

func (DoneEvent) isEvent() {}

// ToolStartEvent is emitted after Prepare succeeds.
// Display contains rich display data (DiffDisplay, StringDisplay, etc.) for UI.
type ToolStartEvent struct {
	CallID   string      // Unique ID from domain.ToolCall.ID
	ToolName string      // Tool identifier
	Display  ToolDisplay // Rich display computed during Prepare
}

func (ToolStartEvent) isEvent() {}

// ToolStreamEvent is emitted for streaming tool output (shell commands).
type ToolStreamEvent struct {
	CallID string
	Chunk  string
}

func (ToolStreamEvent) isEvent() {}

// ToolEndEvent is emitted when tool execution completes.
type ToolEndEvent struct {
	CallID string
	Error  string // Empty = success, non-empty = failure message for UI
}

func (ToolEndEvent) isEvent() {}
