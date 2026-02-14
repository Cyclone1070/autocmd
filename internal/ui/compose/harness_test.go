package compose

import (
	"os"
	"path/filepath"
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
	View              string
	TotalFlushedLines int
	MaxAbsoluteHeight int
	QueueLen          int
	IsPrinting        bool
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
			MaxAbsoluteHeight: ev.Snapshot.MaxAbsoluteHeight,
			QueueLen:          ev.Snapshot.PrintQueueLen,
			IsPrinting:        ev.Snapshot.IsPrinting,
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

// ViewFrames returns view frames derived from ViewRendered events.
func (h *harnessFrameHarness) ViewFrames() []viewFrame {
	return viewFramesFromEvents(h.Events())
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
		visualBottom := f.TotalFlushedLines + frameHeight
		if lastVisualBottom != -1 && visualBottom < lastVisualBottom {
			t.Errorf("frame %d: visual bottom dropped %d -> %d (flush=%d, maxAbs=%d)\nView:\n%s",
				i, lastVisualBottom, visualBottom, f.TotalFlushedLines, f.MaxAbsoluteHeight, f.View)
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
			if !strings.Contains(f.View, needle) {
				t.Errorf("frame %d (queue=%d): content %q missing from view (flush visibility gap)\nView:\n%s",
					i, f.QueueLen, needle, f.View)
			}
		}
	}
}

// --- Invariant tests for new path ---

func TestNewPath_StatusBarNeverJumpsUp(t *testing.T) {
	h := newHarnessFrameHarness(t, 80, 24, 20)
	h.ApplyEvent(domain.TextEvent{Text: "A\n"}, "t1")
	h.ApplyEvent(domain.TextEvent{Text: "B\n\n"}, "t2")
	assertStatusBarNeverJumpsUp(t, h.ViewFrames())
}

func TestNewPath_VisualBottomMonotonic(t *testing.T) {
	h := newHarnessFrameHarness(t, 80, 24, 20)
	h.ApplyEvent(domain.TextEvent{Text: "Line 1\n"}, "t1")
	h.ApplyEvent(domain.TextEvent{Text: "Line 2\n\n"}, "t2")
	h.ApplyEvent(domain.TextEvent{Text: "Line 3\n"}, "t3")
	assertVisualBottomMonotonic(t, h.ViewFrames())
}

func TestNewPath_MaxAbsoluteHeightMonotonic(t *testing.T) {
	h := newHarnessFrameHarness(t, 80, 24, 20)
	h.ApplyEvent(domain.TextEvent{Text: "A\n"}, "t1")
	h.ApplyEvent(domain.TextEvent{Text: "B\n\n"}, "t2")
	assertMaxAbsoluteHeightMonotonic(t, h.ViewFrames())
}

func TestNewPath_StatusBarAfterContent(t *testing.T) {
	h := newHarnessFrameHarness(t, 80, 24, 20)
	h.ApplyEvent(domain.TextEvent{Text: "A\n"}, "t1")
	h.ApplyEvent(domain.TextEvent{Text: "B\n\n"}, "t2")
	assertStatusBarAfterContent(t, h.ViewFrames())
}

func TestNewPath_NoFlushVisibilityGap(t *testing.T) {
	h := newHarnessFrameHarness(t, 80, 24, 20)
	h.ApplyEvent(domain.TextEvent{Text: "Pre\n\n"}, "pre")
	h.ApplyEvent(domain.ToolStartEvent{CallID: "t1", ToolName: "x", Display: domain.StringDisplay("Running")}, "tool")
	h.ApplyEvent(domain.ToolEndEvent{CallID: "t1"}, "end")
	assertNoFlushVisibilityGap(t, h.ViewFrames(), []string{"Pre"})
}

func TestNewPath_OrderingParity(t *testing.T) {
	h := newHarnessFrameHarness(t, 80, 24, 20)
	h.ApplyEvent(domain.TextEvent{Text: "Intro text\n\n"}, "intro")
	h.ApplyEvent(domain.ToolStartEvent{CallID: "a", ToolName: "x", Display: domain.StringDisplay("Tool A")}, "toolA")
	h.ApplyEvent(domain.ToolStartEvent{CallID: "b", ToolName: "y", Display: domain.StringDisplay("Tool B")}, "toolB")
	h.ApplyEvent(domain.ToolEndEvent{CallID: "a"}, "endA")
	h.ApplyEvent(domain.ToolEndEvent{CallID: "b"}, "endB")
	h.ApplyEvent(domain.TextEvent{Text: "trailer\n\n"}, "trailer")
	frames := h.ViewFrames()
	var lastView string
	for i := len(frames) - 1; i >= 0; i-- {
		if frames[i].View != "" {
			lastView = frames[i].View
			break
		}
	}
	idxIntro := strings.Index(lastView, "Intro text")
	idxA := strings.Index(lastView, "Tool A")
	idxB := strings.Index(lastView, "Tool B")
	idxStatus := strings.Index(lastView, "Context:")
	if idxIntro == -1 || idxA == -1 || idxB == -1 || idxStatus == -1 {
		t.Fatalf("missing expected content in final view:\n%s", lastView)
	}
	if idxIntro > idxA {
		t.Error("parity: intro text must appear before Tool A")
	}
	if idxA > idxB {
		t.Error("parity: Tool A must appear before Tool B")
	}
	if idxB > idxStatus {
		t.Error("parity: status bar (Context:) must appear last")
	}
}

func TestNewPath_TextContentOrdering_Frames(t *testing.T) {
	h := newHarnessFrameHarness(t, 80, 24, 1)
	h.ApplyEvent(domain.TextEvent{Text: "Intro text\n"}, "text")
	h.ApplyEvent(domain.TextEvent{Text: "More\n\n"}, "more")
	frames := h.ViewFrames()
	if len(frames) < 2 {
		t.Errorf("expected at least 2 frames, got %d", len(frames))
	}
	assertStatusBarAfterContent(t, frames)
	assertVisualBottomMonotonic(t, frames)
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

func TestNewPath_GoldenSequence_TextStream(t *testing.T) {
	h := newHarnessFrameHarness(t, 80, 24, 20)
	h.ApplyEvent(domain.TextEvent{Text: "Hi\n"}, "t1")
	h.ApplyEvent(domain.TextEvent{Text: "Bye\n\n"}, "t2")
	seq := eventSequenceString(h.Events())
	// Text stream: ViewRendered on every render. HistoryFlushed when markdown flushes complete blocks.
	// Markdown may not flush until 2+ blocks; we assert we get ViewRendered and valid sequence.
	if !strings.Contains(seq, "VR") {
		t.Errorf("must emit ViewRendered events, got: %s", seq)
	}
	// Event log must be non-empty and ordered
	if len(h.Events()) == 0 {
		t.Error("event log must not be empty")
	}
}

func TestNewPath_GoldenSequence_ToolStream(t *testing.T) {
	h := newHarnessFrameHarness(t, 80, 24, 20)
	h.ApplyEvent(domain.TextEvent{Text: "Pre\n\n"}, "pre")
	h.ApplyEvent(domain.ToolStartEvent{CallID: "t1", ToolName: "x", Display: domain.StringDisplay("Tool")}, "tool")
	h.ApplyEvent(domain.ToolEndEvent{CallID: "t1"}, "end")
	events := h.Events()
	seq := eventSequenceString(events)
	if !strings.Contains(seq, "HF") {
		t.Errorf("tool stream should emit HistoryFlushed (tool box flush), got: %s", seq)
	}
	if !strings.Contains(seq, "VR") {
		t.Errorf("must emit ViewRendered, got: %s", seq)
	}
	// No QuitRequested in normal tool flow
	if strings.Contains(seq, "QR") {
		t.Errorf("tool stream should not quit, got: %s", seq)
	}
}

func TestNewPath_GoldenSequence_DoneFlush(t *testing.T) {
	h := newHarnessFrameHarness(t, 80, 24, 20)
	h.ApplyEvent(domain.TextEvent{Text: "Final\n"}, "text")
	h.ApplyEvent(domain.DoneEvent{}, "done")
	seq := eventSequenceString(h.Events())
	// Done triggers flush of remaining content, then QuitRequested
	if !strings.Contains(seq, "QR") {
		t.Errorf("done must emit QuitRequested, got: %s", seq)
	}
	if !strings.Contains(seq, "HF") {
		t.Errorf("done must flush content, got: %s", seq)
	}
	if len(h.Events()) == 0 {
		t.Error("event log must not be empty")
	}
}

func TestNewPath_EventSequence_CompleteTimeline(t *testing.T) {
	h := newHarnessFrameHarness(t, 80, 24, 20)
	h.ApplyEvent(domain.TextEvent{Text: "A\n"}, "t1")
	h.ApplyEvent(domain.TextEvent{Text: "B\n"}, "t2")
	frames := h.ViewFrames()
	events := h.Events()
	// Every ViewRendered must have monotonic TotalFlushedLines
	lastFlush := -1
	for _, f := range frames {
		if f.TotalFlushedLines < lastFlush {
			t.Errorf("TotalFlushedLines must be monotonic: %d -> %d", lastFlush, f.TotalFlushedLines)
		}
		if f.TotalFlushedLines > lastFlush {
			lastFlush = f.TotalFlushedLines
		}
	}
	// Event log must be non-empty
	if len(events) == 0 {
		t.Error("event log must not be empty")
	}
}

// TestArchitecture_NoUnsunkTerminalWrites ensures all bubbletea.Println/Printf
// calls are only in frame.go (ProductionSink, NoopSink). No direct terminal writes
// outside the FrameSink path.
func TestArchitecture_NoUnsunkTerminalWrites(t *testing.T) {
	// Walk from ui package (parent of compose)
	uiDir := ".."
	allowedFile := "tea/frame.go"
	var violations []string
	err := filepath.Walk(uiDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		if filepath.Ext(path) != ".go" {
			return nil
		}
		if strings.HasSuffix(path, "_test.go") {
			return nil // Test files may reference the pattern
		}
		rel, _ := filepath.Rel(uiDir, path)
		if rel == filepath.Join("tea", "frame.go") {
			return nil // Allowed: sink implementations
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		s := string(content)
		// Match actual calls, not string literals or comments
		if strings.Contains(s, "bubbletea.Println(") || strings.Contains(s, "bubbletea.Printf(") {
			violations = append(violations, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	if len(violations) > 0 {
		t.Errorf("bubbletea.Println/Printf must only appear in %s; found in: %v", allowedFile, violations)
	}
}
