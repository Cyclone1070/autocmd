package engine

import (
	"github.com/Cyclone1070/iav/internal/domain"
)

// Geometry holds terminal/viewport dimensions for layout.
type Geometry struct {
	Width       int
	TermHeight  int
	SpaceBelow  int // Initial space below cursor (height - row - statusBarOverhead)
}

// ToolStatus represents tool lifecycle state.
type ToolStatus int

const (
	StatusRunning ToolStatus = iota
	StatusSuccess
	StatusError
)

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
	Status      ToolStatus
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
	Markdown  MarkdownStream
	Theme     ThemeAdapter
	Layout    LayoutAdapter
	ViewTool  func(*ToolState) string // Renders a tool for display
	Spinner   SpinnerViewProvider     // Current spinner frame (runtime provides)
}

// State is the full UI engine state.
type State struct {
	// Markdown/text (handled via MarkdownStream dependency)
	// Tools
	Tools []*ToolState

	// Layout tracking
	MaxAbsoluteHeight  int
	TotalFlushedLines  int
	ContentBeingPrinted string
	PrintQueue         []PrintItem
	IsPrinting         bool

	// Session state
	Thinking bool
	RunState RunState

	// Geometry (read-only after init)
	Geometry Geometry

	// Config-derived (width, etc. from Geometry)
}

// NewInitialState creates the initial engine state.
func NewInitialState(geom Geometry) *State {
	return &State{
		Tools:             make([]*ToolState, 0),
		PrintQueue:        nil,
		Geometry:          geom,
		MaxAbsoluteHeight: geom.SpaceBelow,
	}
}

// Msg wraps engine inputs (domain events + internal messages).
type Msg interface {
	isMsg()
}

// --- Domain event wrappers (engine-facing, exported for runtime) ---

// MsgThinking represents a thinking event.
type MsgThinking struct{}

func (MsgThinking) isMsg() {}

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
