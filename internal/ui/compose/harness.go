package compose

import (
	"strings"
	"testing"

	"github.com/Cyclone1070/iav/internal/config"
	"github.com/Cyclone1070/iav/internal/domain"
	"github.com/Cyclone1070/iav/internal/ui/engine"
	"github.com/Cyclone1070/iav/internal/ui/markdown"
	teapkg "github.com/Cyclone1070/iav/internal/ui/tea"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
)

// staticCursorDetector for harness tests.
type staticCursorDetector struct {
	row int
}

func (d *staticCursorDetector) GetCursorRow() (int, error) {
	return d.row, nil
}

// --- Event-log frame harness (engine + tea + markdown) ---

// viewFrame is derived from ViewRendered events for assertions.
type viewFrame struct {
	View                string
	TotalFlushedLines   int
	MaxAbsoluteHeight   int
	QueueLen            int
	IsPrinting          bool
	ContentBeingPrinted string
}

func (f viewFrame) effectiveHistoryHeight() int {
	h := f.TotalFlushedLines
	if f.IsPrinting && f.ContentBeingPrinted != "" {
		h += strings.Count(f.ContentBeingPrinted, "\n")
		// Factor in the newline added by Println/Printf.
		// We mimic engine.currentHistoryHeight here.
		h++
	}
	return h
}

func viewFramesFromEvents(events []teapkg.FrameEvent) []viewFrame {
	var out []viewFrame
	for _, ev := range events {
		if ev.Type != teapkg.FrameEventViewRendered || ev.Snapshot == nil {
			continue
		}
		out = append(out, viewFrame{
			View:                ev.View,
			TotalFlushedLines:   ev.Snapshot.TotalFlushedLines,
			MaxAbsoluteHeight:   ev.Snapshot.MaxAbsoluteHeight,
			QueueLen:            ev.Snapshot.PrintQueueLen,
			IsPrinting:          ev.Snapshot.IsPrinting,
			ContentBeingPrinted: ev.Snapshot.ContentBeingPrinted,
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
	cd := &staticCursorDetector{row: cursorRow}
	geom, err := teapkg.ResolveGeometry(cfg, cd, height)
	if err != nil {
		t.Fatalf("ResolveGeometry: %v", err)
	}
	mdRenderer, err := markdown.NewGlamourRenderer(width)
	if err != nil {
		t.Fatalf("markdown.NewGlamourRenderer: %v", err)
	}
	sm := markdown.NewStream(mdRenderer)
	state := engine.NewInitialState(geom)
	factory := func(s *spinner.Model) engine.Deps {
		deps := NewEngineDeps(cfg, sm, width)
		deps.Spinner = nil
		return deps
	}
	sink := &teapkg.RecordingSink{}
	adapter := teapkg.NewTeaModelAdapter(state, factory, sink)
	return &harnessFrameHarness{adapter: adapter, sink: sink}
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

const maxHarnessCmdIterations = 100

func (h *harnessFrameHarness) ApplyEvent(ev domain.Event, _ string) {
	h.capture()
	_, cmd := h.adapter.Update(ev)
	h.capture()
	iters := 0
	for cmd != nil && iters < maxHarnessCmdIterations {
		msg := h.runCmdOnce(cmd)
		h.capture()
		_, cmd = h.adapter.Update(msg)
		iters++
	}
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

// --- Assertion helpers ---

func getStatusBarRow(frame string) int {
	lines := strings.Split(frame, "\n")
	for i, line := range lines {
		if strings.Contains(line, "Context:") {
			return i
		}
	}
	return -1
}

func getContentBottomRow(frame string) int {
	lines := strings.Split(frame, "\n")
	lastContent := -1
	for i, line := range lines {
		if strings.Contains(line, "Context:") {
			break
		}
		if strings.TrimSpace(line) != "" {
			lastContent = i
		}
	}
	return lastContent
}

type harnessAssertTB interface {
	Helper()
	Errorf(format string, args ...interface{})
}

func assertStatusBarNeverJumpsUp(t harnessAssertTB, frames []viewFrame) {
	t.Helper()
	lastStatusRow := -1
	for i, f := range frames {
		row := getStatusBarRow(f.View)
		if row == -1 {
			continue
		}
		if lastStatusRow != -1 && row < lastStatusRow {
			t.Errorf("frame %d: status bar jumped up from row %d to %d\nView:\n%s",
				i, lastStatusRow, row, f.View)
		}
		lastStatusRow = row
	}
}

func assertVisualBottomMonotonic(t harnessAssertTB, frames []viewFrame) {
	t.Helper()
	lastVisualBottom := -1
	for i, f := range frames {
		lines := strings.Split(f.View, "\n")
		frameHeight := len(lines)
		if len(lines) > 0 && lines[len(lines)-1] == "" {
			frameHeight--
		}
		if frameHeight == 0 {
			continue
		}
		visualBottom := f.effectiveHistoryHeight() + frameHeight
		if lastVisualBottom != -1 && visualBottom < lastVisualBottom {
			t.Errorf("frame %d: visual bottom dropped %d -> %d (effHistory=%d, maxAbs=%d)\nView:\n%s",
				i, lastVisualBottom, visualBottom, f.effectiveHistoryHeight(), f.MaxAbsoluteHeight, f.View)
		}
		if visualBottom > lastVisualBottom {
			lastVisualBottom = visualBottom
		}
	}
}

func assertMaxAbsoluteHeightMonotonic(t harnessAssertTB, frames []viewFrame) {
	t.Helper()
	last := -1
	for i, f := range frames {
		if last != -1 && f.MaxAbsoluteHeight < last {
			t.Errorf("frame %d: MaxAbsoluteHeight decreased %d -> %d", i, last, f.MaxAbsoluteHeight)
		}
		if f.MaxAbsoluteHeight > last {
			last = f.MaxAbsoluteHeight
		}
	}
}

func assertStatusBarAfterContent(t harnessAssertTB, frames []viewFrame) {
	t.Helper()
	for i, f := range frames {
		statusRow := getStatusBarRow(f.View)
		contentRow := getContentBottomRow(f.View)
		if statusRow != -1 && contentRow != -1 && statusRow < contentRow {
			t.Errorf("frame %d: status bar (row %d) above content (row %d)\nView:\n%s",
				i, statusRow, contentRow, f.View)
		}
	}
}

func assertNoFlushVisibilityGap(t harnessAssertTB, frames []viewFrame, contentMustRemain []string) {
	t.Helper()
	for i, f := range frames {
		if f.QueueLen == 0 {
			continue
		}
		for _, needle := range contentMustRemain {
			if strings.Contains(f.View, needle) {
				continue
			}
			if f.IsPrinting && strings.Contains(f.ContentBeingPrinted, needle) {
				continue
			}
			t.Errorf("frame %d (queue=%d): content %q missing from view (flush visibility gap)\nView:\n%s",
				i, f.QueueLen, needle, f.View)
		}
	}
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
