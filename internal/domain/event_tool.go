package domain

// Tool lifecycle workflow→UI events (start/stream/end).

// ToolStartEvent is emitted after Prepare succeeds.
type ToolStartEvent struct {
	CallID   string      // Unique ID from domain.ToolCall.ID
	Display  ToolDisplay // Rich display computed during Prepare
}

func (ToolStartEvent) isUIUpdate() {}

// ToolStreamEvent is emitted for streaming tool output (shell commands).
type ToolStreamEvent struct {
	CallID string
	Chunk  string
}

func (ToolStreamEvent) isUIUpdate() {}

// ToolEndEvent is emitted when tool execution completes with the final baked ToolDisplay.
type ToolEndEvent struct {
	CallID  string
	Display ToolDisplay // Final state for UI (use GetError() for failure vs success)
}

func (ToolEndEvent) isUIUpdate() {}
