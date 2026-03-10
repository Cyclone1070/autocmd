package domain

// UIUpdate is the interface for all events flowing from Workflow to UI.
type UIUpdate interface {
	isUIUpdate()
}


// Action is the interface for all intents flowing from UI to Workflow.
type Action interface {
	isAction()
}


// StopAction is a user intent to cancel the current workflow.
type StopAction struct{}

func (StopAction) isAction() {}

// TextEvent is emitted when the LLM produces text output.
type TextEvent struct {
	Text string
}

func (TextEvent) isUIUpdate() {}

// ThinkingEvent is emitted when the LLM is processing.
type ThinkingEvent struct{}

func (ThinkingEvent) isUIUpdate() {}

// DoneEvent is emitted when the workflow loop completes.
type DoneEvent struct{}

func (DoneEvent) isUIUpdate() {}

// ToolStartEvent is emitted after Prepare succeeds.
type ToolStartEvent struct {
	CallID   string      // Unique ID from domain.ToolCall.ID
	ToolName string      // Tool identifier
	Display  ToolDisplay // Rich display computed during Prepare
}

func (ToolStartEvent) isUIUpdate() {}

// ToolStreamEvent is emitted for streaming tool output (shell commands).
type ToolStreamEvent struct {
	CallID string
	Chunk  string
}

func (ToolStreamEvent) isUIUpdate() {}

// ToolEndEvent is emitted when tool execution completes.
type ToolEndEvent struct {
	CallID string
	Error  string // Empty = success, non-empty = failure message for UI
}

func (ToolEndEvent) isUIUpdate() {}
