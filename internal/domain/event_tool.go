package domain

// Tool lifecycle workflow→UI events (start/stream/end).

// ToolStartEvent is sent by preview middleware using the ToolDisplay returned from tool Preview.
type ToolStartEvent struct {
	Display ToolDisplay
	CallID  string
}

func (ToolStartEvent) isUIUpdate() {}

// ToolStreamEvent is emitted for streaming tool output (bash.commands).
type ToolStreamEvent struct {
	CallID string
	Chunk  string
}

func (ToolStreamEvent) isUIUpdate() {}

// ToolEndEvent is emitted when tool execution completes with the final baked ToolDisplay.
type ToolEndEvent struct {
	Display ToolDisplay
	CallID  string
}

func (ToolEndEvent) isUIUpdate() {}

// ToolApprovalRequestEvent marks a running tool as awaiting explicit user approval.
type ToolApprovalRequestEvent struct {
	CallID string
}

func (ToolApprovalRequestEvent) isUIUpdate() {}
