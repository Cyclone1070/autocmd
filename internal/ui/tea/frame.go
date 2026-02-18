package tea

import (
	bubbletea "github.com/charmbracelet/bubbletea"
)

// FrameEvent represents a single user-visible UI change.
type FrameEvent struct {
	Type FrameEventType

	// ViewRendered
	View     string
	Snapshot *RenderSnapshot

	// HistoryFlushed
	Content string
	Raw     bool
}

// FrameEventType identifies the kind of frame event.
type FrameEventType int

const (
	FrameEventViewRendered FrameEventType = iota
	FrameEventHistoryFlushed
	FrameEventQuitRequested
)

func (t FrameEventType) String() string {
	switch t {
	case FrameEventViewRendered:
		return "ViewRendered"
	case FrameEventHistoryFlushed:
		return "HistoryFlushed"
	case FrameEventQuitRequested:
		return "QuitRequested"
	default:
		return "Unknown"
	}
}

// RenderSnapshot captures engine state at render time.
type RenderSnapshot struct {
	TotalFlushedLines int
}

// FrameSink receives all frame events in order.
// Production sink writes to terminal; test sink records for assertions.
type FrameSink interface {
	OnFrameEvent(ev FrameEvent)
	// PrintCmd returns the tea.Cmd for a history flush. Production: actual print. Test: no-op that sends msgPrintFinished.
	PrintCmd(content string, raw bool) bubbletea.Cmd
}

// ProductionSink writes frame events to the terminal (via Bubble Tea's default output).
type ProductionSink struct{}

func (ProductionSink) OnFrameEvent(ev FrameEvent) {
	// no-op for recording; production only needs to perform the print via PrintCmd
}

func (ProductionSink) PrintCmd(content string, raw bool) bubbletea.Cmd {
	if raw {
		return bubbletea.Sequence(
			bubbletea.Printf("%s", content),
			func() bubbletea.Msg { return MsgPrintFinished{} },
		)
	}
	return bubbletea.Sequence(
		bubbletea.Println(content),
		func() bubbletea.Msg { return MsgPrintFinished{} },
	)
}

// RecordingSink records all frame events for test assertions.
type RecordingSink struct {
	Events []FrameEvent
}

func (r *RecordingSink) OnFrameEvent(ev FrameEvent) {
	r.Events = append(r.Events, ev)
}

func (r *RecordingSink) PrintCmd(content string, raw bool) bubbletea.Cmd {
	return func() bubbletea.Msg { return MsgPrintFinished{} }
}

// NoopSink does nothing; used when observability is not needed.
type NoopSink struct{}

func (NoopSink) OnFrameEvent(ev FrameEvent) {}

func (NoopSink) PrintCmd(content string, raw bool) bubbletea.Cmd {
	// Still perform the print for backward compatibility when no sink is configured.
	if raw {
		return bubbletea.Sequence(
			bubbletea.Printf("%s", content),
			func() bubbletea.Msg { return MsgPrintFinished{} },
		)
	}
	return bubbletea.Sequence(
		bubbletea.Println(content),
		func() bubbletea.Msg { return MsgPrintFinished{} },
	)
}
