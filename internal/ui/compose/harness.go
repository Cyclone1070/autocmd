package compose

import (
	"strings"
	"testing"

	"github.com/Cyclone1070/iav/internal/config"
	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/ui/engine"
	"github.com/Cyclone1070/iav/internal/ui/markdown"
	teapkg "github.com/Cyclone1070/iav/internal/ui/tea"
	tea "github.com/charmbracelet/bubbletea"
)

const maxHarnessCmdIterations = 100

// --- Event-log frame harness (engine + tea + markdown) ---

// viewFrame is derived from ViewRendered events for assertions.
type viewFrame struct {
	View              string
	TotalFlushedLines int
}

func viewFramesFromEvents(events []teapkg.FrameEvent) []viewFrame {
	var out []viewFrame
	for _, ev := range events {
		if ev.Type != teapkg.FrameEventViewRendered || ev.Snapshot == nil {
			continue
		}
		out = append(out, viewFrame{
			View:              ev.View,
			TotalFlushedLines: ev.Snapshot.TotalFlushedLines,
		})
	}
	return out
}

type harnessFrameHarness struct {
	adapter *teapkg.TeaModelAdapter
	sink    *teapkg.RecordingSink
}

func newHarnessFrameHarness(t *testing.T, width, height, cursorRow int) *harnessFrameHarness {
	t.Helper()
	cfg := config.DefaultConfig()
	cfg.UI.ChatWindowWidth = width
	geom := engine.TermSize{Width: width, Height: height}
	mdRenderer, err := markdown.NewGlamourRenderer(width)
	if err != nil {
		t.Fatalf("markdown.NewGlamourRenderer: %v", err)
	}
	sm := markdown.NewStream(mdRenderer)
	state := engine.NewInitialState(geom)
	factory := func() engine.Deps {
		return NewEngineDeps(cfg, sm, width)
	}
	sink := &teapkg.RecordingSink{}
	adapter := teapkg.NewTeaModelAdapter(state, factory, sink)
	h := &harnessFrameHarness{adapter: adapter, sink: sink}

	// Run Init
	h.runLoop(adapter.Init())

	return h
}

func (h *harnessFrameHarness) runLoop(cmd tea.Cmd) {
	iters := 0
	for cmd != nil && iters < maxHarnessCmdIterations {
		// Stop if we are "idle" (no typing, no printing, no tools)
		// and the next message would just be a tick.
		// This prevents infinite loops from the activity indicator.
		s := h.adapter.State
		if s.TypingBuffer == "" && !s.IsPrinting && len(s.PrintQueue) == 0 && len(s.Tools) == 0 {
			// Check if the command is a tick
			// We can't easily check the command content, so we use a heuristic:
			// if we've reached a stable state, stop.
			// But we need at least one VR frame.
			if iters > 10 {
				break
			}
		}

		cmd = h.ProcessCmd(cmd)
		iters++
	}
}

func (h *harnessFrameHarness) ApplyEvent(ev domain.Event, _ string) {
	cmd := h.ApplyEventOnly(ev)
	h.runLoop(cmd)
}

// capture triggers a render so ViewRendered is emitted to the sink.
func (h *harnessFrameHarness) capture() {
	_ = h.adapter.View()
}

func (h *harnessFrameHarness) runCmdOnce(cmd tea.Cmd) tea.Msg {
	if cmd == nil {
		return nil
	}
	return cmd()
}

// ApplyEventOnly applies an event and returns the resulting command without running it.
// This allows tests to inspect state before side effects (like printing) complete.
func (h *harnessFrameHarness) ApplyEventOnly(ev domain.Event) tea.Cmd {
	h.capture()
	_, cmd := h.adapter.Update(ev)
	h.capture()
	return cmd
}

// ProcessCmd runs a single command, feeds the result back to Update, and returns the next command.
// It captures frames before and after the update.
func (h *harnessFrameHarness) ProcessCmd(cmd tea.Cmd) tea.Cmd {
	if cmd == nil {
		return nil
	}
	msg := h.runCmdOnce(cmd)
	h.capture()
	_, nextCmd := h.adapter.Update(msg)
	h.capture()
	return nextCmd
}

// Events returns the canonical FrameEvent log for assertions.
func (h *harnessFrameHarness) Events() []teapkg.FrameEvent {
	return h.sink.Events
}

func (h *harnessFrameHarness) FullTranscript() string {
	var b strings.Builder
	for _, ev := range h.sink.Events {
		if ev.Type == teapkg.FrameEventHistoryFlushed {
			b.WriteString(ev.Content)
			if !ev.Raw {
				b.WriteByte('\n')
			}
		}
	}
	frames := h.ViewFrames()
	if len(frames) > 0 {
		b.WriteString(frames[len(frames)-1].View)
	}
	return b.String()
}

// ViewFrames returns view frames derived from ViewRendered events.
func (h *harnessFrameHarness) ViewFrames() []viewFrame {
	return viewFramesFromEvents(h.sink.Events)
}

// eventSequenceString produces a compact representation of the event log for golden tests.
func eventSequenceString(events []teapkg.FrameEvent) string {
	var b strings.Builder
	for i, ev := range events {
		if i > 0 {
			b.WriteByte(' ')
		}
		switch ev.Type {
		case teapkg.FrameEventViewRendered:
			b.WriteString("VR")
		case teapkg.FrameEventHistoryFlushed:
			b.WriteString("HF")
			if ev.Content != "" {
				// Truncate for readability; content may have newlines
				s := strings.ReplaceAll(ev.Content, "\n", "\\n")
				if len(s) > 20 {
					s = s[:20] + "..."
				}
				b.WriteString("(")
				b.WriteString(s)
				b.WriteString(")")
			}
		case teapkg.FrameEventQuitRequested:
			b.WriteString("QR")
		default:
			b.WriteString("?")
		}
	}
	return b.String()
}
