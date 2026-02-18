package engine

import (
	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/ui/theme"
)

// TermSize holds terminal dimensions.
type TermSize struct {
	Width  int
	Height int
}

// RunState represents the overall session state.
type RunState int

const (
	StateRunning RunState = iota
	StateQuitting
	StateDone
	StateCancelled
)

// ToolState holds display data and status for a single tool.
type ToolState struct {
	CallID      string
	Display     domain.ToolDisplay
	Status      theme.ToolStatus
	Err         string
	ShellOutput string
}

// PrintItem represents content to print (queue element).
type PrintItem struct {
	Content string
	Raw     bool
}

// Deps holds engine dependencies (injected).
type Deps struct {
	Markdown     MarkdownStream
	Theme        ThemeAdapter
	Layout       LayoutAdapter
	ToolRenderer ToolRenderer // Renders a tool for display
	Spinner      SpinnerViewProvider
}

// State is the full UI engine state.
type State struct {
	// Markdown/text (handled via MarkdownStream dependency)
	// Tools
	Tools []*ToolState

	// Layout tracking
	TotalFlushedLines      int
	ContentBeingPrinted    string
	ContentBeingPrintedRaw bool
	PrintQueue             []PrintItem
	IsPrinting             bool

	// Session state
	TypingBuffer string // Simulated typing buffer
	IdleTicks    int    // Ticks since last activity (for "..." animation)
	RunState     RunState

	// TermSize (read-only after init)
	TermSize TermSize
}

// NewInitialState creates the initial engine state.
func NewInitialState(ts TermSize) *State {
	return &State{
		Tools:      make([]*ToolState, 0),
		PrintQueue: nil,
		TermSize:   ts,
	}
}

// Msg wraps engine inputs (domain events + internal messages).
type Msg interface {
	isMsg()
}

// --- Domain event wrappers (engine-facing, exported for runtime) ---

// MsgTick represents a timer tick.
type MsgTick struct{}

func (MsgTick) isMsg() {}

// MsgText represents a text chunk.
type MsgText struct {
	Text string
}

func (MsgText) isMsg() {}

// MsgToolStart represents a tool start.
type MsgToolStart struct {
	CallID  string
	Display domain.ToolDisplay
}

func (MsgToolStart) isMsg() {}

// MsgToolStream represents streaming tool output.
type MsgToolStream struct {
	CallID string
	Chunk  string
}

func (MsgToolStream) isMsg() {}

// MsgToolEnd represents a tool completion.
type MsgToolEnd struct {
	CallID string
	Error  string
}

func (MsgToolEnd) isMsg() {}

// MsgDone represents workflow completion.
type MsgDone struct{}

func (MsgDone) isMsg() {}

// MsgPrintFinished is sent when a print completes (used by runtime).
type MsgPrintFinished struct{}

func (MsgPrintFinished) isMsg() {}

// MsgCtrlC represents Ctrl+C.
type MsgCtrlC struct{}

func (MsgCtrlC) isMsg() {}
